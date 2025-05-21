package util

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/taigrr/colorhash"
)

// recommendation for ext3 is no more than 32000 files per directory
// so if you increase this, don't increase it by too much
const GlobalModulus = 5000

var (
	ErrExpectedFile      = fmt.Errorf("expected file, got directory")
	ErrUnexpectedSymlink = fmt.Errorf("expected file, got symlink")
	ErrInvalidHashPath   = fmt.Errorf("invalid hash path")
)

type (
	LookupEntry struct {
		FileSize int64     `json:"size"`     // size of the file in bytes
		Inode    uint64    `json:"inode"`    // inode number of the file
		Modified time.Time `json:"modified"` // modification time of the file
		Name     string    `json:"name"`     // name of the file as it appears in FUSE
		Target   string    `json:"target"`   // name of the hashed file (e.g., hash.ext)
	}
	LookupTable struct {
		entries []LookupEntry
		sorted  bool
	}
)

func (e LookupTable) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Entries []LookupEntry `json:"entries"`
		Sorted  bool          `json:"sorted"`
	}{
		Entries: e.entries,
		Sorted:  e.sorted,
	})
}

func (e LookupTable) Iterate(yield func(LookupEntry) bool) {
	for _, entry := range e.entries {
		if !yield(entry) {
			return
		}
	}
}

func (e LookupTable) Add(le LookupEntry) {
	e.sorted = false
	e.entries = append(e.entries, le)
}

func (e LookupTable) Remove(index int) error {
	if index < 0 || index >= len(e.entries) {
		return fmt.Errorf("index out of range")
	}
	e.entries = append(e.entries[:index], e.entries[index+1:]...)
	return nil
}

func (e LookupTable) Get(index int) LookupEntry {
	if index < 0 || index >= len(e.entries) {
		return LookupEntry{}
	}
	return e.entries[index]
}

func (e LookupTable) Sort() {
	sort.Sort(e)
	e.sorted = true
}

func (e LookupTable) Len() int {
	return len(e.entries)
}

func (e LookupTable) Swap(i, j int) {
	e.entries[i], e.entries[j] = e.entries[j], e.entries[i]
}

func (e LookupTable) Less(i, j int) bool {
	return e.entries[i].Modified.Before(e.entries[j].Modified)
}

// GetOldest returns the first modification entry, assuming
// the Entries slice is sorted.
// TODO ensure the entries are sorted during garbage collection
func (l LookupTable) GetOldestFileTS() time.Time {
	if l.Len() == 0 {
		return time.Time{}
	}
	if !l.sorted {
		l.Sort()
	}
	return l.Get(0).Modified
}

// CreateFileLookupEntry creates a LookupEntry for a given file.
// originalFileAbsPath: absolute path to the source file.
// boundaryStagingPath: path to the directory where the hashed file will be stored.
// rootPathForBoundary: the root path of the current processing boundary, used to make l.Name relative.
func CreateFileLookupEntry(originalFileAbsPath, boundaryStagingPath, rootPathForBoundary string) (LookupEntry, error) {
	var l LookupEntry
	info, err := os.Lstat(originalFileAbsPath)
	if os.IsNotExist(err) {
		return l, err
	}
	// if the file is a symlink, skip it
	if info.Mode()&os.ModeSymlink == os.ModeSymlink {
		fmt.Printf("skipping unsupported symlink %s\n", originalFileAbsPath)
		return l, errors.Join(ErrUnexpectedSymlink, fmt.Errorf("skipping unsupported symlink %s", originalFileAbsPath))
	}
	if err != nil {
		return l, err
	}
	if info.IsDir() {
		return l, ErrExpectedFile
	}

	hash, err := GetFileHash(originalFileAbsPath)
	if err != nil {
		return l, fmt.Errorf("failed to get hash for %s: %w", originalFileAbsPath, err)
	}

	targetFilename := hash + filepath.Ext(originalFileAbsPath)
	destPath := filepath.Join(boundaryStagingPath, targetFilename)

	sourceFile, err := os.Open(originalFileAbsPath)
	if err != nil {
		return l, fmt.Errorf("failed to open source file %s: %w", originalFileAbsPath, err)
	}
	defer sourceFile.Close()

	destinationFile, err := os.Create(destPath)
	if err != nil {
		return l, fmt.Errorf("failed to create destination file %s: %w", destPath, err)
	}
	defer destinationFile.Close()

	_, err = io.Copy(destinationFile, sourceFile)
	if err != nil {
		return l, fmt.Errorf("failed to copy file from %s to %s: %w", originalFileAbsPath, destPath, err)
	}

	l.Target = targetFilename // Store only the filename (hash + ext)

	relPath, err := filepath.Rel(rootPathForBoundary, originalFileAbsPath)
	if err != nil {
		log.Printf("Warning: could not make path relative for Name (%s relative to %s): %s. Using base name %s.", originalFileAbsPath, rootPathForBoundary, err, filepath.Base(originalFileAbsPath))
		l.Name = filepath.Base(originalFileAbsPath)
	} else {
		l.Name = relPath
	}

	l.Modified = info.ModTime()
	l.FileSize = info.Size()
	l.Inode = GetNewInode()
	return l, nil
}

func RenameHashedFile(path string) (string, error) {
	hash, err := GetFileHash(path)
	if err != nil {
		return "", err
	}

	fullName := filepath.Dir(path)
	fullName = filepath.Join(fullName, hash+filepath.Ext(path))
	return fullName, os.Rename(path, fullName)
}

type lookupWorkerData struct {
	originalFileAbsPath string
	boundaryStagingPath string
	rootPathForBoundary string
}

func initialLookupWorker(lwd <-chan lookupWorkerData, c chan<- LookupEntry, errChan chan<- error, wg *sync.WaitGroup) {
	defer wg.Done()

	for x := range lwd {
		le, err := CreateFileLookupEntry(x.originalFileAbsPath, x.boundaryStagingPath, x.rootPathForBoundary)
		if err != nil {
			errChan <- fmt.Errorf("worker error processing %s: %w", x.originalFileAbsPath, err)
			continue
		}
		c <- le
	}
}

// CreateInitialDJAFSManifest scans the directory 'rootPathForBoundary', copies files to 'boundaryStagingPath',
// and generates a LookupTable.
// rootPathForBoundary: The root directory of the current boundary to scan.
// boundaryStagingPath: The staging directory where hashed files for this boundary will be copied.
// filesOnly: If true, only processes files directly under rootPathForBoundary, skipping subdirectories.
func CreateInitialDJAFSManifest(rootPathForBoundary, boundaryStagingPath string, filesOnly bool) (LookupTable, error) {
	// boundaryStagingPath is now used directly, no WorkDir joining.

	lt := LookupTable{sorted: false, entries: []LookupEntry{}}
	lookupEntryChan := make(chan LookupEntry, runtime.NumCPU())
	errChan := make(chan error, runtime.NumCPU())
	lwdChan := make(chan lookupWorkerData, runtime.NumCPU())
	var wg sync.WaitGroup

	// Start workers
	wg.Add(runtime.NumCPU())
	for i := 0; i < runtime.NumCPU(); i++ {
		go initialLookupWorker(lwdChan, lookupEntryChan, errChan, &wg)
	}

	// Start walker
	go func() {
		defer close(lwdChan)
		// rootPathForBoundary is the 'path' parameter of this function.
		err := filepath.WalkDir(rootPathForBoundary, func(currentFileAbsPath string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				// Send the error to errChan so it can be handled by the main goroutine.
				// This is better than returning it directly, which might halt WalkDir prematurely for recoverable errors.
				errChan <- fmt.Errorf("error walking at %s: %w", currentFileAbsPath, walkErr)
				return nil // Continue walking if possible, or handle specific errors to skip entries.
			}

			// Skip the manifest files themselves and staging directories
			ext := filepath.Ext(d.Name())
			if ext == ".djfl" || ext == ".djfm" || ext == ".djfz" {
				return nil
			}
			if d.IsDir() && d.Name() == ".staging" && filepath.Dir(currentFileAbsPath) == rootPathForBoundary {
				log.Printf("Skipping .staging directory: %s", currentFileAbsPath)
				return filepath.SkipDir
			}


			if filesOnly {
				if d.IsDir() && currentFileAbsPath != rootPathForBoundary {
					return filepath.SkipDir // Skip subdirectories if filesOnly is true
				}
				// If it's a directory and it is the rootPathForBoundary, allow WalkDir to list its children.
				// Do not send the directory itself to the worker.
				if d.IsDir() && currentFileAbsPath == rootPathForBoundary {
					return nil
				}
			}

			// If it's a directory (and not skipped above), allow WalkDir to recurse.
			// Do not send directories to the processing channel.
			if d.IsDir() {
				return nil
			}

			// At this point, 'd' is a file.
			lwdChan <- lookupWorkerData{
				originalFileAbsPath: currentFileAbsPath,
				boundaryStagingPath: boundaryStagingPath,
				rootPathForBoundary: rootPathForBoundary,
			}
			return nil
		})
		if err != nil {
			// This error is from WalkDir itself, e.g., if the rootPathForBoundary is invalid.
			// It's important to send this to errChan as well.
			errChan <- fmt.Errorf("filepath.WalkDir failed for %s: %w", rootPathForBoundary, err)
		}
	}()

	// Process results
	go func() {
		wg.Wait()
		close(lookupEntryChan)
		close(errChan)
	}()

	var chansClosed bool
	for !chansClosed {
		select {
		case le, ok := <-lookupEntryChan:
			if !ok {
				chansClosed = true
				continue
			}
			lt.Add(le)
		case err, ok := <-errChan:
			if !ok {
				chansClosed = true
				continue
			}
			switch {
			case err == nil:
				continue
			case errors.Is(err, os.ErrNotExist):
				// File might have been deleted between WalkDir and processing.
				log.Printf("Warning: File not found during processing (may have been deleted): %v", err)
				continue
			case errors.Is(err, ErrExpectedFile):
				// This should ideally not happen if WalkDir logic is correct (only sending files).
				log.Printf("Warning: ErrExpectedFile encountered for a path. Error: %v", err)
				continue
			case errors.Is(err, ErrUnexpectedSymlink):
				// Symlinks are skipped by CreateFileLookupEntry, this is informational.
				// log.Printf("Info: Skipped symlink: %v", err) // Already printed in CreateFileLookupEntry
				continue
			default:
				// Log the error and potentially return it if it's critical.
				// For now, logging and continuing to process other files.
				log.Printf("Error processing file entry: %s", err)
				// Depending on policy, you might want to return LookupTable{}, err here
				// For robustness, we'll try to continue and aggregate results.
				// Consider adding a counter for critical errors and failing if it exceeds a threshold.
			}
		}
	}
	sort.Sort(lt)
	return lt, nil
}

func CreateDJAFSArchive(path, output string, includeSubdirs bool) error {
	filesOnly := !includeSubdirs // if true, only files directly under 'path'; if false, recurse.
	lt := LookupTable{sorted: false, entries: []LookupEntry{}}

	// The 'output' directory will contain the final archive assets (manifest, zip).
	// 'boundaryStagingPath' is a temporary directory for collecting files before zipping.
	// It's made somewhat unique to avoid conflicts if this func is called multiple times for different 'path's but same 'output'.
	boundaryStagingPath := filepath.Join(output, ".archive-staging-"+filepath.Base(path))
	err := os.MkdirAll(boundaryStagingPath, 0o755)
	if err != nil {
		return fmt.Errorf("failed to create staging directory %s for DJAFS archive: %w", boundaryStagingPath, err)
	}
	// Ensure cleanup of the staging directory for this archive operation
	defer func() {
		if rErr := os.RemoveAll(boundaryStagingPath); rErr != nil {
			log.Printf("Warning: failed to remove staging directory %s: %v", boundaryStagingPath, rErr)
		}
	}()

	err = filepath.WalkDir(path, func(currentFileAbsPath string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			log.Printf("Error during WalkDir in CreateDJAFSArchive at %s: %v", currentFileAbsPath, walkErr)
			// Depending on desired behavior, might return walkErr to stop, or nil to try to continue.
			return walkErr
		}

		// If filesOnly, skip any subdirectories.
		// The check currentFileAbsPath != path ensures that if path itself is a directory, its direct children are processed.
		if filesOnly && d.IsDir() && currentFileAbsPath != path {
			return filepath.SkipDir
		}

		// Do not process directories as lookup entries. WalkDir handles recursion.
		if d.IsDir() {
			return nil
		}

		// At this point, 'd' is a file.
		// 'path' is the root of this archive operation, so it's the rootPathForBoundary.
		// 'currentFileAbsPath' is the absolute path to the current file.
		// 'boundaryStagingPath' is where the hashed file will be copied.
		le, err := CreateFileLookupEntry(currentFileAbsPath, boundaryStagingPath, path)
		if os.IsNotExist(err) { // File might have been deleted since WalkDir listed it.
			log.Printf("File %s not found during CreateFileLookupEntry in CreateDJAFSArchive, skipping.", currentFileAbsPath)
			return nil
		}
		if errors.Is(err, ErrExpectedFile) {
			return nil
		}
		if errors.Is(err, ErrUnexpectedSymlink) {
			os.Remove(subpath)
			return nil
		}
		if err != nil {
			return err
		}

		lt.Add(le)
		return nil
	})
	if err != nil {
		return err
	}
	sort.Sort(lt)
	manifest := filepath.Join(path, "lookup.djfl")
	err = WriteJSONFile(manifest, lt)
	if err != nil {
		return err
	}
	err = ZipInside(path, filesOnly)
	if err != nil {
		return err
	}
	for e := range lt.Iterate {
		err = os.Remove(e.Name)
		if err != nil {
			log.Printf("Failed to remove %s: %s", e.Name, err)
		}
	}
	return nil
	// TODO
}

func WriteJSONFile(path string, v interface{}) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(v)
}

// Does the opposite of GetOldestFileTS
func (l LookupTable) GetNewestFileTS() time.Time {
	if l.Len() == 0 {
		return time.Time{}
	}
	if !l.sorted {
		l.Sort()
	}
	return l.Get(l.Len() - 1).Modified
}

func (l LookupTable) GetTotalFileCount() int {
	files := make(map[string]bool)
	for e := range l.Iterate {
		files[e.Name] = true
	}
	return len(files)
}

func (l LookupTable) GetTargetFileCount() int {
	files := make(map[string]bool)
	for e := range l.Iterate {
		files[e.Target] = true
	}
	return len(files)
}

// TODO Optimization: consider using a taint variable instead of sorting on every addition
func (l LookupTable) AddFileEntry(e LookupEntry) LookupTable {
	l.Add(e)
	return l
}

func (l LookupTable) GetUncompressedSize() int {
	total := 0
	for e := range l.Iterate {
		total += int(e.FileSize)
	}
	return total
}

func ManifestLocationForPath(path string) (string, error) {
	return "", nil
}

func HashFromHashPath(path string) (string, error) {
	parts := strings.Split(path, "-")
	if len(parts) != 3 {
		return "", ErrInvalidHashPath
	}
	return parts[2], nil
}

func HashPathFromHash(hash string) string {
	hInt := colorhash.HashString(hash)
	hInt = hInt % 1000
	first := hInt
	second := 0
	// TODO check if directory is getting too big and split

	third := hash
	return fmt.Sprintf("%d-%05d-%s", first, second, third)
}

func WorkspacePrefixFromHashPath(path string) (string, error) {
	parts := strings.Split(path, "-")
	if len(parts) < 3 {
		return "", ErrInvalidHashPath
	}
	return filepath.Join(parts[0], parts[1]), nil
}

func ZipPrefixFromHashPath(path string) (string, error) {
	parts := strings.Split(path, "-")
	if len(parts) < 3 {
		return "", ErrInvalidHashPath
	}
	return parts[0] + "-" + parts[1], nil
}

func GetFileHash(path string) (hash string, err error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", ErrExpectedFile
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// TODO add LookupTableCollapse function for combining consecutive entries
// of the same name to target
