package util

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"time"
)

type (
	LookupEntry struct {
		FileSize int64     `json:"size"`     // size of the file in bytes
		Inode    uint64    `json:"inode"`    // inode number of the file
		Modified time.Time `json:"modified"` // modification time of the file
		Name     string    `json:"name"`     // name of the file as it appears in FUSE
		Target   string    `json:"target"`   // filepath of the hashed filename
	}
	LookupTable struct {
		entries []LookupEntry
		sorted  bool
	}
)

func (e *LookupTable) UnmarshalJSON(data []byte) error {
	var aux struct {
		Entries []LookupEntry `json:"entries"`
		Sorted  bool          `json:"sorted"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	e.entries = aux.Entries
	e.sorted = aux.Sorted
	return nil
}

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

func (e *LookupTable) Add(le LookupEntry) {
	e.sorted = false
	e.entries = append(e.entries, le)
}

func (e *LookupTable) Remove(index int) error {
	if index < 0 || index >= len(e.entries) {
		return fmt.Errorf("index out of range")
	}
	e.entries = slices.Delete(e.entries, index, index+1)
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

// Returns the first modification entry, assuming the Entries slice is sorted.
func (l LookupTable) GetOldestFileTS() time.Time {
	if l.Len() == 0 {
		return time.Time{}
	}
	if !l.sorted {
		l.Sort()
	}
	return l.Get(0).Modified
}

// CreateFileLookupEntry creates a lookup table entry for a file at the specified path.
// It calculates the file hash, copies it to the work directory if initial is true,
// and returns a LookupEntry with all necessary metadata.
func CreateFileLookupEntry(path, workDirPath string, initial bool) (LookupEntry, error) {
	var l LookupEntry
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return l, err
	}
	// if the file is a symlink, skip it
	if info.Mode()&os.ModeSymlink == os.ModeSymlink {
		fmt.Printf("skipping unsupported symlink %s\n", path)
		return l, errors.Join(ErrUnexpectedSymlink, fmt.Errorf("skipping unsupported symlink %s", path))
	}
	if err != nil {
		return l, err
	}
	if info.IsDir() {
		return l, ErrExpectedFile
	}
	hash, err := GetFileHash(path)
	if err != nil {
		return l, err
	}

	_, err = CopyToWorkDir(path, workDirPath, hash)
	l.Target = filepath.Join(hash, filepath.Ext(path))
	l.Name = path
	l.Modified = info.ModTime()
	l.FileSize = info.Size()
	l.Inode = GetNewInode()
	return l, err
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

// GetTotalFileCount returns the total number of content-unique files in the lookup table
func (l LookupTable) GetTotalFileCount() int {
	files := make(map[string]bool)
	for e := range l.Iterate {
		files[e.Name] = true
	}
	return len(files)
}

// GetTargetFileCount returns the total number of name-unique files in the lookup table
func (l LookupTable) GetTargetFileCount() int {
	files := make(map[string]bool)
	for e := range l.Iterate {
		files[e.Target] = true
	}
	return len(files)
}

// GetActiveFileCount returns the total number of content-unique files in the lookup table that are still active (after deletions)
func (l LookupTable) GetActiveFileCount() int {
	files := make(map[string]bool)
	for e := range l.Iterate {
		files[e.Name] = true
		if e.Target == "" {
			files[e.Name] = false
		}
	}
	return len(files)
}

func (l LookupTable) GetUncompressedSize() int {
	total := 0
	for e := range l.Iterate {
		total += int(e.FileSize)
	}
	return total
}

// TODO add LookupTableCollapse function for combining consecutive entries
// of the same name to target

// LookupTableCollapse combines consecutive entries of the same name,
// keeping only the most recent entry for each file
func (l *LookupTable) Collapse() {
	if len(l.entries) <= 1 {
		return
	}

	// Ensure table is sorted by modification time
	if !l.sorted {
		l.Sort()
	}

	// Map to track the latest entry for each file name
	latestEntries := make(map[string]LookupEntry)

	// Process entries in chronological order
	for _, entry := range l.entries {
		latestEntries[entry.Name] = entry
	}

	// Rebuild entries slice with only the latest version of each file
	l.entries = make([]LookupEntry, 0, len(latestEntries))
	for _, entry := range latestEntries {
		l.entries = append(l.entries, entry)
	}

	// Re-sort after collapse
	l.sorted = false
	l.Sort()
}

