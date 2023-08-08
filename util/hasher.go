package util

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"
)

var ErrExpectedFile error

type LookupTable struct {
	Entries EntrySet `json:"entries"`
	sorted  bool
}

type EntrySet []LookupEntry

func (e EntrySet) Len() int {
	return len(e)
}

func (e EntrySet) Swap(i, j int) {
	e[i], e[j] = e[j], e[i]
}

func (e EntrySet) Less(i, j int) bool {
	return e[i].Modified.Before(e[j].Modified)
}

type LookupEntry struct {
	FileSize int64     `json:"size"`
	Inode    uint64    `json:"inode"`
	Modified time.Time `json:"modified"`
	Name     string    `json:"name"`
	Target   string    `json:"target"`
}

// GetOldest returns the first modification entry, assuming
// the Entries slice is sorted.
// TODO ensure the entries are sorted during garbage collection
func (l LookupTable) GetOldestFileTS() time.Time {
	if len(l.Entries) == 0 {
		return time.Time{}
	}
	return l.Entries[0].Modified
}

func CreateLookupEntry(path string) (LookupEntry, error) {
	var l LookupEntry
	info, err := os.Stat(path)
	if err != nil {
		return l, err
	}
	if info.IsDir() {
		return l, ErrExpectedFile
	}
	newPath, err := RenameHashedFile(path)
	l.Name = newPath
	l.Target = path
	l.Modified = info.ModTime()
	l.FileSize = info.Size()
	return l, err
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

func CreateDJAFSArchive(path string) error {
	lt := LookupTable{sorted: false, Entries: EntrySet{}}
	err := filepath.WalkDir(path, func(path string, info os.DirEntry, err error) error {
		le, err := CreateLookupEntry(path)
		if err != nil {
			return err
		}
		lt.Entries = append(lt.Entries, le)
		return nil
	})
	if err != nil {
		return err
	}
	sort.Sort(lt.Entries)
	manifest := filepath.Join(path, "manifest.djfl")
	err = WriteJSONFile(manifest, lt)
	if err != nil {
		return err
	}
	err = ZipInside(path, false)
	if err != nil {
		return err
	}
	for _, e := range lt.Entries {
		os.Remove(e.Name)
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
	if len(l.Entries) == 0 {
		return time.Time{}
	}
	return l.Entries[len(l.Entries)-1].Modified
}

func (l LookupTable) GetTotalFileCount() int {
	files := make(map[string]bool)
	for _, e := range l.Entries {
		files[e.Name] = true
	}
	return len(files)
}

func (l LookupTable) GetTargetFileCount() int {
	files := make(map[string]bool)
	for _, e := range l.Entries {
		files[e.Target] = true
	}
	return len(files)
}

// TODO Optimization: consider using a taint variable instead of sorting on every addition
func (l LookupTable) AddFileEntry(e LookupEntry) LookupTable {
	l.Entries = append(l.Entries, e)
	sort.Sort(l.Entries)
	return l
}

func (l LookupTable) GetUncompressedSize() int {
	total := 0
	for _, e := range l.Entries {
		total += int(e.FileSize)
	}
	return total
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
