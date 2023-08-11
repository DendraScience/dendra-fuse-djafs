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
	"sort"
	"strings"
	"time"

	"github.com/taigrr/colorhash"
)

var (
	ErrExpectedFile      = fmt.Errorf("expected file, got directory")
	ErrUnexpectedSymlink = fmt.Errorf("expected file, got symlink")
	ErrInvalidHashPath   = fmt.Errorf("invalid hash path")
)

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
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return l, err
	}
	// if the file is a symlink, skip it
	if info.Mode()&os.ModeSymlink == os.ModeSymlink {
		fmt.Printf("skippping unsupported symlink %s\n", path)
		return l, errors.Join(ErrUnexpectedSymlink, fmt.Errorf("skippping unsupported symlink %s", path))
	}
	if err != nil {
		return l, err
	}
	if info.IsDir() {
		return l, ErrExpectedFile
	}
	hash, err := GetFileHash(path)
	l.Name = hash + filepath.Ext(path)
	l.Target = path
	l.Modified = info.ModTime()
	l.FileSize = info.Size()
	l.Inode = GetNewInode()
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

func CreateInitialDJAFSManifest(path string, filesOnly bool) (LookupTable, error) {
	lt := LookupTable{sorted: false, Entries: EntrySet{}}
	err := filepath.WalkDir(path, func(subpath string, info os.DirEntry, err error) error {
		if filepath.Ext(info.Name()) == ".djfl" {
			return nil
		}
		if filesOnly && info.IsDir() && subpath != path {
			return filepath.SkipDir
		} else if filesOnly && info.IsDir() {
			return nil
		}

		le, err := CreateLookupEntry(subpath)
		if os.IsNotExist(err) {
			return nil
		}
		if errors.Is(err, ErrExpectedFile) {
			return nil
		}
		if errors.Is(err, ErrUnexpectedSymlink) {
			return nil
		}
		if err != nil {
			return err
		}
		lt.Entries = append(lt.Entries, le)
		return nil
	})
	if err != nil {
		log.Printf("error walking path %s: %s", path, err)
		return LookupTable{}, err
	}
	sort.Sort(lt.Entries)
	return lt, nil
}

func CreateDJAFSArchive(path string, filesOnly bool) error {
	lt := LookupTable{sorted: false, Entries: EntrySet{}}
	err := filepath.WalkDir(path, func(subpath string, info os.DirEntry, err error) error {
		if filesOnly {
			if info.IsDir() {
				return filepath.SkipDir
			}
		}
		if subpath == path {
			return nil
		}
		le, err := CreateLookupEntry(subpath)
		if os.IsNotExist(err) {
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
	err = ZipInside(path, filesOnly)
	if err != nil {
		return err
	}
	for _, e := range lt.Entries {
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
	second := "00000"
	third := hash
	// TODO check if directory is getting too big and split
	return fmt.Sprintf("%d-%s-%s", first, second, third)
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
