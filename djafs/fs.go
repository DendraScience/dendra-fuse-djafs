package djafs

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	"github.com/dendrascience/dendra-archive-fuse/util"
)

// FS implements the djafs FUSE filesystem
type FS struct {
	StoragePath string              // Path to djafs storage directory
	Archives    map[string]*Archive // Cached archive handles
	HotCache    *HotCache           // Write buffer
	mu          sync.RWMutex        // Protects Archives map
}

// Archive represents a loaded .djfz archive with its lookup table
type Archive struct {
	Path        string
	LookupTable util.LookupTable
	LastAccess  time.Time
	mu          sync.RWMutex
}

// HotCache manages the write buffer for immediate write completion
type HotCache struct {
	IncomingDir string
	StagingDir  string
	fs          *FS
	gcTicker    *time.Ticker
	stopGC      chan bool
	mu          sync.RWMutex
}

// NewFS creates a new djafs filesystem instance
func NewFS(storagePath string) *FS {
	fs := &FS{
		StoragePath: storagePath,
		Archives:    make(map[string]*Archive),
	}

	fs.HotCache = NewHotCache(fs, storagePath)
	return fs
}

// Stop gracefully shuts down the filesystem
func (fs *FS) Stop() {
	if fs.HotCache != nil {
		fs.HotCache.Stop()
	}
}

// NewHotCache creates a new hot cache instance
func NewHotCache(fs *FS, storagePath string) *HotCache {
	hc := &HotCache{
		IncomingDir: filepath.Join(storagePath, "hot_cache", "incoming"),
		StagingDir:  filepath.Join(storagePath, "hot_cache", "staging"),
		fs:          fs,
		gcTicker:    time.NewTicker(30 * time.Second), // GC every 30 seconds
		stopGC:      make(chan bool),
	}

	// Create directories
	os.MkdirAll(hc.IncomingDir, 0755)
	os.MkdirAll(hc.StagingDir, 0755)

	// Start background garbage collection
	go hc.backgroundGC()

	return hc
}

// Root returns the root directory node
func (fs *FS) Root() (fs.Node, error) {
	return &Dir{
		fs:   fs,
		path: "/",
	}, nil
}

// Dir implements both Node and Handle for directories
type Dir struct {
	fs           *FS
	path         string
	isSnapshot   bool
	snapshotTime *time.Time
}

// Attr returns directory attributes
func (d *Dir) Attr(ctx context.Context, a *fuse.Attr) error {
	a.Inode = 1 // Root directory gets inode 1
	a.Mode = os.ModeDir | 0o755
	a.Mtime = time.Now()
	a.Ctime = time.Now()
	a.Atime = time.Now()
	return nil
}

// Lookup resolves file/directory names to nodes
func (d *Dir) Lookup(ctx context.Context, name string) (fs.Node, error) {
	switch d.path {
	case "/":
		// Root directory - only allow "live" and "snapshots"
		switch name {
		case "live":
			return &Dir{
				fs:   d.fs,
				path: "/live",
			}, nil
		case "snapshots":
			return &Dir{
				fs:         d.fs,
				path:       "/snapshots",
				isSnapshot: true,
			}, nil
		}
		return nil, syscall.ENOENT

	case "/live":
		// Live directory - resolve actual files and directories
		return d.resolveLivePath(name)

	case "/snapshots":
		// Snapshots directory - list available snapshots
		return d.resolveSnapshotPath(name)

	default:
		if strings.HasPrefix(d.path, "/live/") {
			// Within live directory structure
			return d.resolveLivePath(name)
		} else if strings.HasPrefix(d.path, "/snapshots/") {
			// Within snapshot directory structure
			return d.resolveSnapshotPath(name)
		}
	}

	return nil, syscall.ENOENT
}

// resolveLivePath resolves paths within the /live directory
func (d *Dir) resolveLivePath(name string) (fs.Node, error) {
	// Construct the full path
	fullPath := filepath.Join(strings.TrimPrefix(d.path, "/live"), name)
	if fullPath == "." {
		fullPath = name
	}

	// Try to find the file in storage
	entry, err := d.fs.findFileEntry(fullPath)
	if err == nil {
		// Found a file
		return &File{
			fs:    d.fs,
			entry: entry,
		}, nil
	}

	// Check if it's a directory by looking for files with this prefix
	if d.fs.hasFilesWithPrefix(fullPath + "/") {
		return &Dir{
			fs:   d.fs,
			path: "/live/" + fullPath,
		}, nil
	}

	return nil, syscall.ENOENT
}

// resolveSnapshotPath resolves paths within the /snapshots directory
func (d *Dir) resolveSnapshotPath(name string) (fs.Node, error) {
	if d.path == "/snapshots" {
		// Root snapshots directory - list available snapshots
		// For now, support a few predefined snapshot formats:
		// - "latest" for most recent state
		// - ISO timestamp format like "2024-01-01T12:00:00Z"
		// - Date format like "2024-01-01"

		switch name {
		case "latest":
			return &Dir{
				fs:           d.fs,
				path:         "/snapshots/latest",
				isSnapshot:   true,
				snapshotTime: nil, // nil means latest
			}, nil
		default:
			// Try to parse as timestamp
			if timestamp, err := time.Parse(time.RFC3339, name); err == nil {
				return &Dir{
					fs:           d.fs,
					path:         "/snapshots/" + name,
					isSnapshot:   true,
					snapshotTime: &timestamp,
				}, nil
			}

			// Try to parse as date (assume end of day)
			if date, err := time.Parse("2006-01-02", name); err == nil {
				// Set to end of day
				endOfDay := date.Add(23*time.Hour + 59*time.Minute + 59*time.Second)
				return &Dir{
					fs:           d.fs,
					path:         "/snapshots/" + name,
					isSnapshot:   true,
					snapshotTime: &endOfDay,
				}, nil
			}
		}

		return nil, syscall.ENOENT
	}

	// Within a snapshot directory - resolve files and directories at that point in time
	if strings.HasPrefix(d.path, "/snapshots/") {
		// Extract the snapshot time from the path
		pathParts := strings.Split(d.path, "/")
		if len(pathParts) < 3 {
			return nil, syscall.ENOENT
		}

		snapshotName := pathParts[2]
		var snapshotTime *time.Time

		if snapshotName != "latest" {
			if timestamp, err := time.Parse(time.RFC3339, snapshotName); err == nil {
				snapshotTime = &timestamp
			} else if date, err := time.Parse("2006-01-02", snapshotName); err == nil {
				endOfDay := date.Add(23*time.Hour + 59*time.Minute + 59*time.Second)
				snapshotTime = &endOfDay
			} else {
				return nil, syscall.ENOENT
			}
		}

		// Build the virtual path within the snapshot
		virtualPath := strings.Join(pathParts[3:], "/")
		if virtualPath != "" {
			virtualPath = "/" + virtualPath
		}
		virtualPath = virtualPath + "/" + name

		// Try to find the file at the snapshot time
		entry, err := d.fs.findFileEntryAtTime(virtualPath, snapshotTime)
		if err == nil {
			// Found a file
			return &File{
				fs:    d.fs,
				entry: entry,
			}, nil
		}

		// Check if it's a directory by looking for files with this prefix at snapshot time
		if d.fs.hasFilesWithPrefixAtTime(virtualPath+"/", snapshotTime) {
			return &Dir{
				fs:           d.fs,
				path:         d.path + "/" + name,
				isSnapshot:   true,
				snapshotTime: snapshotTime,
			}, nil
		}
	}

	return nil, syscall.ENOENT
}

// Create creates a new file
func (d *Dir) Create(ctx context.Context, req *fuse.CreateRequest, resp *fuse.CreateResponse) (fs.Node, fs.Handle, error) {
	// Only allow creation in /live directory
	if !strings.HasPrefix(d.path, "/live") {
		return nil, nil, syscall.EPERM
	}

	// Create new file node
	file := &File{
		fs:       d.fs,
		path:     filepath.Join(strings.TrimPrefix(d.path, "/live"), req.Name),
		isNew:    true,
		data:     []byte{},
		modified: time.Now(),
	}

	// Set response attributes
	resp.Attr.Inode = util.GetNewInode()
	resp.Attr.Mode = req.Mode
	resp.Attr.Size = 0
	resp.Attr.Mtime = file.modified
	resp.Attr.Ctime = file.modified
	resp.Attr.Atime = file.modified

	return file, file, nil
}

// Mkdir creates a new directory
func (d *Dir) Mkdir(ctx context.Context, req *fuse.MkdirRequest) (fs.Node, error) {
	// Only allow creation in /live directory
	if !strings.HasPrefix(d.path, "/live") {
		return nil, syscall.EPERM
	}

	newPath := filepath.Join(d.path, req.Name)

	return &Dir{
		fs:   d.fs,
		path: newPath,
	}, nil
}

// ReadDirAll lists directory contents
func (d *Dir) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	var dirents []fuse.Dirent

	switch d.path {
	case "/":
		// Root directory
		dirents = append(dirents, fuse.Dirent{
			Inode: 2,
			Name:  "live",
			Type:  fuse.DT_Dir,
		})
		dirents = append(dirents, fuse.Dirent{
			Inode: 3,
			Name:  "snapshots",
			Type:  fuse.DT_Dir,
		})

	case "/snapshots":
		// List available snapshots
		snapshots := d.fs.getAvailableSnapshots()
		for _, snapshot := range snapshots {
			dirents = append(dirents, fuse.Dirent{
				Inode: util.GetNewInode(),
				Name:  snapshot,
				Type:  fuse.DT_Dir,
			})
		}

	case "/live":
		// List all files and directories in live storage
		entries, err := d.fs.getAllEntries()
		if err != nil {
			return nil, err
		}

		// Build directory structure
		dirs := make(map[string]bool)
		for _, entry := range entries {
			if entry.Target == "" {
				continue // Skip deleted files
			}

			parts := strings.Split(entry.Name, "/")
			if len(parts) > 1 {
				dirs[parts[0]] = true
			} else {
				// File in root of live directory
				dirents = append(dirents, fuse.Dirent{
					Inode: entry.Inode,
					Name:  entry.Name,
					Type:  fuse.DT_File,
				})
			}
		}

		// Add directories
		for dir := range dirs {
			dirents = append(dirents, fuse.Dirent{
				Inode: util.GetNewInode(),
				Name:  dir,
				Type:  fuse.DT_Dir,
			})
		}

	default:
		if strings.HasPrefix(d.path, "/live/") {
			// List files in subdirectory
			prefix := strings.TrimPrefix(d.path, "/live/") + "/"
			entries, err := d.fs.getEntriesWithPrefix(prefix)
			if err != nil {
				return nil, err
			}

			dirs := make(map[string]bool)
			for _, entry := range entries {
				if entry.Target == "" {
					continue // Skip deleted files
				}

				relativePath := strings.TrimPrefix(entry.Name, prefix)
				parts := strings.Split(relativePath, "/")

				if len(parts) == 1 {
					// File directly in this directory
					dirents = append(dirents, fuse.Dirent{
						Inode: entry.Inode,
						Name:  parts[0],
						Type:  fuse.DT_File,
					})
				} else {
					// Subdirectory
					dirs[parts[0]] = true
				}
			}

			// Add directories
			for dir := range dirs {
				dirents = append(dirents, fuse.Dirent{
					Inode: util.GetNewInode(),
					Name:  dir,
					Type:  fuse.DT_Dir,
				})
			}
		} else if strings.HasPrefix(d.path, "/snapshots/") {
			// List files in snapshot directory
			pathParts := strings.Split(d.path, "/")
			if len(pathParts) >= 3 {
				snapshotName := pathParts[2]
				var snapshotTime *time.Time

				if snapshotName != "latest" {
					if timestamp, err := time.Parse(time.RFC3339, snapshotName); err == nil {
						snapshotTime = &timestamp
					} else if date, err := time.Parse("2006-01-02", snapshotName); err == nil {
						endOfDay := date.Add(23*time.Hour + 59*time.Minute + 59*time.Second)
						snapshotTime = &endOfDay
					}
				}

				// Build the virtual path within the snapshot
				virtualPath := ""
				if len(pathParts) > 3 {
					virtualPath = strings.Join(pathParts[3:], "/") + "/"
				}

				entries, err := d.fs.getEntriesWithPrefixAtTime(virtualPath, snapshotTime)
				if err != nil {
					return nil, err
				}

				dirs := make(map[string]bool)
				for _, entry := range entries {
					if entry.Target == "" {
						continue // Skip deleted files
					}

					relativePath := strings.TrimPrefix(entry.Name, virtualPath)
					parts := strings.Split(relativePath, "/")

					if len(parts) == 1 {
						// File directly in this directory
						dirents = append(dirents, fuse.Dirent{
							Inode: entry.Inode,
							Name:  parts[0],
							Type:  fuse.DT_File,
						})
					} else {
						// Subdirectory
						dirs[parts[0]] = true
					}
				}

				// Add directories
				for dir := range dirs {
					dirents = append(dirents, fuse.Dirent{
						Inode: util.GetNewInode(),
						Name:  dir,
						Type:  fuse.DT_Dir,
					})
				}
			}
		}
	}

	return dirents, nil
}

// File implements both Node and Handle for files
type File struct {
	fs       *FS
	entry    *util.LookupEntry
	path     string    // Path for new files
	data     []byte    // Cached file content
	isNew    bool      // True for newly created files
	modified time.Time // Modification time for new files
	mu       sync.RWMutex
}

// Attr returns file attributes
func (f *File) Attr(ctx context.Context, a *fuse.Attr) error {
	f.mu.RLock()
	defer f.mu.RUnlock()

	if f.isNew {
		a.Inode = util.GetNewInode()
		a.Mode = 0o644
		a.Size = uint64(len(f.data))
		a.Mtime = f.modified
		a.Ctime = f.modified
		a.Atime = time.Now()
	} else {
		a.Inode = f.entry.Inode
		a.Mode = 0o644
		a.Size = uint64(f.entry.FileSize)
		a.Mtime = f.entry.Modified
		a.Ctime = f.entry.Modified
		a.Atime = time.Now()
	}
	return nil
}

// ReadAll reads the entire file content
func (f *File) ReadAll(ctx context.Context) ([]byte, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	if f.isNew {
		return f.data, nil
	}

	if f.data != nil {
		return f.data, nil
	}

	// Load file content from archive
	data, err := f.fs.loadFileContent(f.entry)
	if err != nil {
		return nil, err
	}

	f.data = data
	return data, nil
}

// Write writes data to the file
func (f *File) Write(ctx context.Context, req *fuse.WriteRequest, resp *fuse.WriteResponse) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Extend data slice if necessary
	newLen := int(req.Offset) + len(req.Data)
	if newLen > len(f.data) {
		newData := make([]byte, newLen)
		copy(newData, f.data)
		f.data = newData
	}

	// Write the data
	copy(f.data[req.Offset:], req.Data)
	resp.Size = len(req.Data)

	f.modified = time.Now()
	f.isNew = true // Mark as modified

	return nil
}

// Flush ensures data is written to storage
func (f *File) Flush(ctx context.Context, req *fuse.FlushRequest) error {
	f.mu.RLock()
	defer f.mu.RUnlock()

	if !f.isNew || len(f.data) == 0 {
		return nil
	}

	// Write to hot cache
	return f.fs.HotCache.WriteFile(f.path, f.data)
}

// Fsync forces synchronization
func (f *File) Fsync(ctx context.Context, req *fuse.FsyncRequest) error {
	return f.Flush(ctx, &fuse.FlushRequest{})
}

// Setattr sets file attributes
func (f *File) Setattr(ctx context.Context, req *fuse.SetattrRequest, resp *fuse.SetattrResponse) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if req.Valid.Size() {
		// Truncate or extend file
		if req.Size < uint64(len(f.data)) {
			f.data = f.data[:req.Size]
		} else if req.Size > uint64(len(f.data)) {
			newData := make([]byte, req.Size)
			copy(newData, f.data)
			f.data = newData
		}
		f.modified = time.Now()
		f.isNew = true
	}

	if req.Valid.Mtime() {
		f.modified = req.Mtime
	}

	// Return current attributes
	return f.Attr(ctx, &resp.Attr)
}

// Hot Cache Methods

// Stop stops the hot cache garbage collection
func (hc *HotCache) Stop() {
	hc.gcTicker.Stop()
	hc.stopGC <- true
}

// WriteFile writes a file to the hot cache
func (hc *HotCache) WriteFile(path string, data []byte) error {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	// Create directory structure in hot cache
	fullPath := filepath.Join(hc.IncomingDir, path)
	dir := filepath.Dir(fullPath)

	err := os.MkdirAll(dir, 0755)
	if err != nil {
		return fmt.Errorf("failed to create directory structure: %w", err)
	}

	// Write file to hot cache
	err = os.WriteFile(fullPath, data, 0644)
	if err != nil {
		return fmt.Errorf("failed to write file to hot cache: %w", err)
	}

	return nil
}

// backgroundGC runs the garbage collection process
func (hc *HotCache) backgroundGC() {
	for {
		select {
		case <-hc.gcTicker.C:
			hc.processFiles()
		case <-hc.stopGC:
			return
		}
	}
}

// processFiles processes files from incoming to staging to final storage
func (hc *HotCache) processFiles() {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	// Move files from incoming to staging
	err := filepath.Walk(hc.IncomingDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Continue on errors
		}

		if info.IsDir() {
			return nil
		}

		// Calculate relative path
		relPath, err := filepath.Rel(hc.IncomingDir, path)
		if err != nil {
			return nil
		}

		// Move to staging
		stagingPath := filepath.Join(hc.StagingDir, relPath)
		stagingDir := filepath.Dir(stagingPath)

		os.MkdirAll(stagingDir, 0755)
		err = os.Rename(path, stagingPath)
		if err != nil {
			return nil // Continue on errors
		}

		// Process the file
		go hc.processFile(stagingPath, relPath)

		return nil
	})
	if err != nil {
		// Log error but continue
		fmt.Printf("Error during GC walk: %v\n", err)
	}
}

// processFile processes a single file through the pipeline
func (hc *HotCache) processFile(stagingPath, relPath string) {
	// Calculate hash
	hash, err := util.GetFileHash(stagingPath)
	if err != nil {
		fmt.Printf("Error hashing file %s: %v\n", stagingPath, err)
		return
	}

	// Copy to work directory
	workDir := filepath.Join(hc.fs.StoragePath, util.WorkDir)
	workPath, err := util.CopyToWorkDir(stagingPath, workDir, hash)
	if err != nil {
		fmt.Printf("Error copying file to work dir: %v\n", err)
		return
	}

	// Create lookup entry
	info, err := os.Stat(stagingPath)
	if err != nil {
		fmt.Printf("Error getting file info: %v\n", err)
		return
	}

	entry := util.LookupEntry{
		FileSize: info.Size(),
		Inode:    util.GetNewInode(),
		Modified: info.ModTime(),
		Name:     relPath,
		Target:   filepath.Base(workPath),
	}

	// Update lookup table (simplified - would need proper boundary detection)
	err = hc.updateLookupTable(entry)
	if err != nil {
		fmt.Printf("Error updating lookup table: %v\n", err)
		return
	}

	// Remove from staging
	os.Remove(stagingPath)

	// Clean up empty directories
	hc.cleanupEmptyDirs(filepath.Dir(stagingPath))
}

// updateLookupTable updates the appropriate lookup table with a new entry
func (hc *HotCache) updateLookupTable(entry util.LookupEntry) error {
	// For simplicity, create a lookup table in the root of storage
	// In a real implementation, this would use proper boundary detection
	lookupPath := filepath.Join(hc.fs.StoragePath, "lookups.djfl")

	var lookupTable util.LookupTable

	// Load existing lookup table if it exists
	if _, err := os.Stat(lookupPath); err == nil {
		file, err := os.Open(lookupPath)
		if err != nil {
			return err
		}
		defer file.Close()

		err = json.NewDecoder(file).Decode(&lookupTable)
		if err != nil {
			return err
		}
	}

	// Add new entry
	lookupTable.Add(entry)

	// Save updated lookup table
	return util.WriteJSONFile(lookupPath, lookupTable)
}

// cleanupEmptyDirs removes empty directories
func (hc *HotCache) cleanupEmptyDirs(dir string) {
	if dir == hc.StagingDir || dir == hc.IncomingDir {
		return
	}

	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) > 0 {
		return
	}

	os.Remove(dir)
	hc.cleanupEmptyDirs(filepath.Dir(dir))
}

// FS Helper Methods

// findFileEntry finds a file entry by path using the "dead end" detection algorithm
func (fs *FS) findFileEntry(path string) (*util.LookupEntry, error) {
	// Use the "dead end" detection algorithm from the README
	storagePath := filepath.Join(fs.StoragePath, path)

	// Walk down the storage path until we hit a "dead end"
	currentPath := storagePath
	for {
		if _, err := os.Stat(currentPath); os.IsNotExist(err) {
			// Hit a "dead end" - back up one level
			parentPath := filepath.Dir(currentPath)
			if parentPath == currentPath {
				// Reached root without finding manifest
				return nil, fmt.Errorf("no manifest found for path %s", path)
			}

			// Look for lookup table in parent directory
			manifestPath := filepath.Join(parentPath, "lookups.djfl")
			if _, err := os.Stat(manifestPath); err == nil {
				// Found lookup table, search for our file
				relativePath := strings.TrimPrefix(path, strings.TrimPrefix(parentPath, fs.StoragePath))
				relativePath = strings.TrimPrefix(relativePath, "/")

				return fs.searchLookupTable(manifestPath, relativePath)
			}

			currentPath = parentPath
			continue
		}

		// Directory exists, check for lookup table here
		manifestPath := filepath.Join(currentPath, "lookups.djfl")
		if _, err := os.Stat(manifestPath); err == nil {
			// Found lookup table, search for our file
			relativePath := strings.TrimPrefix(path, strings.TrimPrefix(currentPath, fs.StoragePath))
			relativePath = strings.TrimPrefix(relativePath, "/")

			return fs.searchLookupTable(manifestPath, relativePath)
		}

		// Move up one level
		parentPath := filepath.Dir(currentPath)
		if parentPath == currentPath {
			// Reached root
			break
		}
		currentPath = parentPath
	}

	return nil, fmt.Errorf("file not found: %s", path)
}

// searchLookupTable searches a lookup table for a specific file
func (fs *FS) searchLookupTable(manifestPath, relativePath string) (*util.LookupEntry, error) {
	// Load lookup table
	lookupTable, err := fs.loadLookupTable(manifestPath)
	if err != nil {
		return nil, err
	}

	// Search for the file
	for entry := range lookupTable.Iterate {
		if entry.Name == relativePath && entry.Target != "" {
			return &entry, nil
		}
	}

	return nil, fmt.Errorf("file not found in lookup table: %s", relativePath)
}

// loadLookupTable loads and caches a lookup table
func (fs *FS) loadLookupTable(manifestPath string) (*util.LookupTable, error) {
	fs.mu.RLock()
	if archive, exists := fs.Archives[manifestPath]; exists {
		archive.LastAccess = time.Now()
		fs.mu.RUnlock()
		return &archive.LookupTable, nil
	}
	fs.mu.RUnlock()

	// Load lookup table from file
	file, err := os.Open(manifestPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lookupTable util.LookupTable
	err = json.NewDecoder(file).Decode(&lookupTable)
	if err != nil {
		return nil, err
	}

	// Cache the lookup table
	fs.mu.Lock()
	fs.Archives[manifestPath] = &Archive{
		Path:        manifestPath,
		LookupTable: lookupTable,
		LastAccess:  time.Now(),
	}
	fs.mu.Unlock()

	return &lookupTable, nil
}

// hasFilesWithPrefix checks if any files exist with the given prefix
func (fs *FS) hasFilesWithPrefix(prefix string) bool {
	// Walk through all lookup tables to find files with this prefix
	err := filepath.Walk(fs.StoragePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Continue walking on errors
		}

		if !strings.HasSuffix(path, "lookups.djfl") {
			return nil
		}

		lookupTable, err := fs.loadLookupTable(path)
		if err != nil {
			return nil // Continue on errors
		}

		for entry := range lookupTable.Iterate {
			if entry.Target != "" && strings.HasPrefix(entry.Name, prefix) {
				return fmt.Errorf("found") // Use error to break out of walk
			}
		}

		return nil
	})

	return err != nil && err.Error() == "found"
}

// getAllEntries returns all file entries from all lookup tables
func (fs *FS) getAllEntries() ([]util.LookupEntry, error) {
	var allEntries []util.LookupEntry

	err := filepath.Walk(fs.StoragePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Continue walking on errors
		}

		if !strings.HasSuffix(path, "lookups.djfl") {
			return nil
		}

		lookupTable, err := fs.loadLookupTable(path)
		if err != nil {
			return nil // Continue on errors
		}

		for entry := range lookupTable.Iterate {
			allEntries = append(allEntries, entry)
		}

		return nil
	})

	return allEntries, err
}

// getEntriesWithPrefix returns entries that start with the given prefix
func (fs *FS) getEntriesWithPrefix(prefix string) ([]util.LookupEntry, error) {
	var matchingEntries []util.LookupEntry

	err := filepath.Walk(fs.StoragePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Continue walking on errors
		}

		if !strings.HasSuffix(path, "lookups.djfl") {
			return nil
		}

		lookupTable, err := fs.loadLookupTable(path)
		if err != nil {
			return nil // Continue on errors
		}

		for entry := range lookupTable.Iterate {
			if entry.Target != "" && strings.HasPrefix(entry.Name, prefix) {
				matchingEntries = append(matchingEntries, entry)
			}
		}

		return nil
	})

	return matchingEntries, err
}

// loadFileContent loads file content from the appropriate archive
func (fs *FS) loadFileContent(entry *util.LookupEntry) ([]byte, error) {
	// Find the archive containing this file
	archivePath, err := fs.findArchiveForTarget(entry.Target)
	if err != nil {
		return nil, err
	}

	// Open the archive
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open archive %s: %w", archivePath, err)
	}
	defer r.Close()

	// Find the target file in the archive
	for _, f := range r.File {
		if f.Name == entry.Target {
			rc, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("failed to open file %s in archive: %w", entry.Target, err)
			}
			defer rc.Close()

			// Read the entire file content
			content, err := io.ReadAll(rc)
			if err != nil {
				return nil, fmt.Errorf("failed to read file content: %w", err)
			}

			return content, nil
		}
	}

	return nil, fmt.Errorf("file %s not found in archive %s", entry.Target, archivePath)
}

// findArchiveForTarget finds the .djfz archive containing a specific target file
func (fs *FS) findArchiveForTarget(target string) (string, error) {
	var foundArchive string

	err := filepath.Walk(fs.StoragePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Continue walking on errors
		}

		if !strings.HasSuffix(path, ".djfz") {
			return nil
		}

		// Check if this archive contains our target file
		r, err := zip.OpenReader(path)
		if err != nil {
			return nil // Continue on errors
		}
		defer r.Close()

		for _, f := range r.File {
			if f.Name == target {
				foundArchive = path
				return fmt.Errorf("found") // Use error to break out of walk
			}
		}

		return nil
	})

	if err != nil && err.Error() == "found" {
		return foundArchive, nil
	}

	return "", fmt.Errorf("archive containing target %s not found", target)
}

// Snapshot-related methods

// getAvailableSnapshots returns a list of available snapshot timestamps
func (fs *FS) getAvailableSnapshots() []string {
	snapshots := []string{"latest"}

	// Collect unique timestamps from all lookup tables
	timestampSet := make(map[string]bool)

	err := filepath.Walk(fs.StoragePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Continue walking on errors
		}

		if !strings.HasSuffix(path, "lookups.djfl") {
			return nil
		}

		lookupTable, err := fs.loadLookupTable(path)
		if err != nil {
			return nil // Continue on errors
		}

		for entry := range lookupTable.Iterate {
			// Add date-based snapshots
			dateStr := entry.Modified.Format("2006-01-02")
			timestampSet[dateStr] = true

			// Add hourly snapshots for recent dates (last 7 days)
			if time.Since(entry.Modified) < 7*24*time.Hour {
				hourStr := entry.Modified.Format("2006-01-02T15:04:05Z")
				timestampSet[hourStr] = true
			}
		}

		return nil
	})

	if err == nil {
		// Convert set to sorted slice
		for timestamp := range timestampSet {
			snapshots = append(snapshots, timestamp)
		}
	}

	return snapshots
}

// findFileEntryAtTime finds a file entry at a specific point in time
func (fs *FS) findFileEntryAtTime(path string, snapshotTime *time.Time) (*util.LookupEntry, error) {
	// Use the same "dead end" detection algorithm but filter by time
	storagePath := filepath.Join(fs.StoragePath, path)

	// Walk down the storage path until we hit a "dead end"
	currentPath := storagePath
	for {
		if _, err := os.Stat(currentPath); os.IsNotExist(err) {
			// Hit a "dead end" - back up one level
			parentPath := filepath.Dir(currentPath)
			if parentPath == currentPath {
				// Reached root without finding manifest
				return nil, fmt.Errorf("no manifest found for path %s", path)
			}

			// Look for lookup table in parent directory
			manifestPath := filepath.Join(parentPath, "lookups.djfl")
			if _, err := os.Stat(manifestPath); err == nil {
				// Found lookup table, search for our file at the snapshot time
				relativePath := strings.TrimPrefix(path, strings.TrimPrefix(parentPath, fs.StoragePath))
				relativePath = strings.TrimPrefix(relativePath, "/")

				return fs.searchLookupTableAtTime(manifestPath, relativePath, snapshotTime)
			}

			currentPath = parentPath
			continue
		}

		// Directory exists, check for lookup table here
		manifestPath := filepath.Join(currentPath, "lookups.djfl")
		if _, err := os.Stat(manifestPath); err == nil {
			// Found lookup table, search for our file at the snapshot time
			relativePath := strings.TrimPrefix(path, strings.TrimPrefix(currentPath, fs.StoragePath))
			relativePath = strings.TrimPrefix(relativePath, "/")

			return fs.searchLookupTableAtTime(manifestPath, relativePath, snapshotTime)
		}

		// Move up one level
		parentPath := filepath.Dir(currentPath)
		if parentPath == currentPath {
			// Reached root
			break
		}
		currentPath = parentPath
	}

	return nil, fmt.Errorf("file not found: %s", path)
}

// searchLookupTableAtTime searches a lookup table for a file at a specific time
func (fs *FS) searchLookupTableAtTime(manifestPath, relativePath string, snapshotTime *time.Time) (*util.LookupEntry, error) {
	// Load lookup table
	lookupTable, err := fs.loadLookupTable(manifestPath)
	if err != nil {
		return nil, err
	}

	// Filter entries by time and find the most recent version before snapshot time
	var latestEntry *util.LookupEntry

	for entry := range lookupTable.Iterate {
		if entry.Name == relativePath {
			// Check if this entry is within the snapshot time
			if snapshotTime == nil || entry.Modified.Before(*snapshotTime) || entry.Modified.Equal(*snapshotTime) {
				if latestEntry == nil || entry.Modified.After(latestEntry.Modified) {
					entryCopy := entry // Create a copy to avoid pointer issues
					latestEntry = &entryCopy
				}
			}
		}
	}

	if latestEntry != nil && latestEntry.Target != "" {
		return latestEntry, nil
	}

	return nil, fmt.Errorf("file not found in lookup table at snapshot time: %s", relativePath)
}

// hasFilesWithPrefixAtTime checks if any files exist with the given prefix at a specific time
func (fs *FS) hasFilesWithPrefixAtTime(prefix string, snapshotTime *time.Time) bool {
	// Walk through all lookup tables to find files with this prefix at the snapshot time
	err := filepath.Walk(fs.StoragePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Continue walking on errors
		}

		if !strings.HasSuffix(path, "lookups.djfl") {
			return nil
		}

		lookupTable, err := fs.loadLookupTable(path)
		if err != nil {
			return nil // Continue on errors
		}

		// Track the latest version of each file
		latestEntries := make(map[string]*util.LookupEntry)

		for entry := range lookupTable.Iterate {
			if snapshotTime == nil || entry.Modified.Before(*snapshotTime) || entry.Modified.Equal(*snapshotTime) {
				if existing, exists := latestEntries[entry.Name]; !exists || entry.Modified.After(existing.Modified) {
					entryCopy := entry // Create a copy
					latestEntries[entry.Name] = &entryCopy
				}
			}
		}

		// Check if any of the latest entries match our prefix and are not deleted
		for _, entry := range latestEntries {
			if entry.Target != "" && strings.HasPrefix(entry.Name, prefix) {
				return fmt.Errorf("found") // Use error to break out of walk
			}
		}

		return nil
	})

	return err != nil && err.Error() == "found"
}

// getEntriesWithPrefixAtTime returns entries that start with the given prefix at a specific time
func (fs *FS) getEntriesWithPrefixAtTime(prefix string, snapshotTime *time.Time) ([]util.LookupEntry, error) {
	var matchingEntries []util.LookupEntry

	err := filepath.Walk(fs.StoragePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Continue walking on errors
		}

		if !strings.HasSuffix(path, "lookups.djfl") {
			return nil
		}

		lookupTable, err := fs.loadLookupTable(path)
		if err != nil {
			return nil // Continue on errors
		}

		// Track the latest version of each file
		latestEntries := make(map[string]*util.LookupEntry)

		for entry := range lookupTable.Iterate {
			if snapshotTime == nil || entry.Modified.Before(*snapshotTime) || entry.Modified.Equal(*snapshotTime) {
				if existing, exists := latestEntries[entry.Name]; !exists || entry.Modified.After(existing.Modified) {
					entryCopy := entry // Create a copy
					latestEntries[entry.Name] = &entryCopy
				}
			}
		}

		// Add matching entries that are not deleted
		for _, entry := range latestEntries {
			if entry.Target != "" && strings.HasPrefix(entry.Name, prefix) {
				matchingEntries = append(matchingEntries, *entry)
			}
		}

		return nil
	})

	return matchingEntries, err
}

