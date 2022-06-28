package util

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var ErrExpectedFile error

type LookupTable struct {
	Entries EntrySet `json:"entries"`
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
	Name     string    `json:"name"`
	Target   string    `json:"target"`
	Modified time.Time `json:"modified"`
	FileSize int64     `json:"size"`
}

// GetOldest returns the first modification entry, taking for granted
// that the Entries slice is sorted.
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
	fullName := filepath.Dir(path)
	hash, err := GetFileHash(path)
	if err != nil {
		return l, err
	}
	fullName = filepath.Join(fullName, hash, GetFileDotExtension(path))
	l.Name = fullName
	l.Target = path
	l.Modified = info.ModTime()
	l.FileSize = info.Size()
	return l, nil
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
		return "", nil
	}
	defer file.Close()
	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func GetFileDotExtension(path string) string {
	split := strings.Split(path, ".")
	if len(split) == 0 {
		return ""
	}
	return "." + split[len(split)-1]
}

func HashFile(path string) error {
	return nil
}
