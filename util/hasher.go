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
	"slices"
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
	subpath string
	output  string
	initial bool
}

func initialLookupWorker(lwd <-chan lookupWorkerData, c chan<- LookupEntry, errChan chan<- error, wg *sync.WaitGroup) {
	defer wg.Done()

	for x := range lwd {
		le, err := CreateFileLookupEntry(x.subpath, x.output, x.initial)
		if err != nil {
			errChan <- err
			continue
		}
		c <- le
	}
}

func CreateInitialDJAFSManifest(path, output string, filesOnly bool) (LookupTable, error) {
	if output == "" {
		output = WorkDir
	} else {
		output = filepath.Join(output, WorkDir)
	}

	lt := LookupTable{sorted: false, entries: []LookupEntry{}}
	lookupEntryChan := make(chan LookupEntry, runtime.NumCPU())
	errChan := make(chan error, runtime.NumCPU())
	lwdChan := make(chan lookupWorkerData, runtime.NumCPU())
	var wg sync.WaitGroup

	// Start workers
	wg.Add(runtime.NumCPU())
	for range runtime.NumCPU() {
		go initialLookupWorker(lwdChan, lookupEntryChan, errChan, &wg)
	}

	// Start walker
	go func() {
		defer close(lwdChan)
		err := filepath.WalkDir(path, func(subpath string, info os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if filepath.Ext(info.Name()) == ".djfl" {
				return nil
			}
			if filesOnly && info.IsDir() && subpath != path {
				return filepath.SkipDir
			} else if filesOnly && info.IsDir() {
				return nil
			}

			lwdChan <- lookupWorkerData{subpath, output, true}
			return nil
		})
		if err != nil {
			errChan <- err
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
				continue
			case errors.Is(err, ErrExpectedFile):
				continue
			case errors.Is(err, ErrUnexpectedSymlink):
				continue
			default:
				log.Printf("error walking path %s: %s", path, err)
				return LookupTable{}, err
			}
		}
	}
	sort.Sort(lt)
	return lt, nil
}

func CreateDJAFSArchive(path, output string, includeSubdirs bool) error {
	filesOnly := !includeSubdirs
	lt := LookupTable{sorted: false, entries: []LookupEntry{}}

	err := filepath.WalkDir(path, func(subpath string, info os.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("error walking path %s: %w", subpath, err)
		}
		if filesOnly && info.IsDir() {
			return filepath.SkipDir
		}
		if subpath == path {
			return nil
		}
		le, err := CreateFileLookupEntry(subpath, filepath.Join(output, WorkDir), false)
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

		lt.Add(le)
		return nil
	})
	if err != nil {
		return fmt.Errorf("error walking path %s: %w", path, err)
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

func WriteJSONFile(path string, v any) error {
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

// Hashes a file and returns the hash as a hex string suitable for use in a filepath
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
	return GetHash(file)
}

func GetHash(r io.Reader) (string, error) {
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// TODO add LookupTableCollapse function for combining consecutive entries
// of the same name to target
