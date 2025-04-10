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
	FileSize int64     `json:"size"`     // size of the file in bytes
	Inode    uint64    `json:"inode"`    // inode number of the file
	Modified time.Time `json:"modified"` // modification time of the file
	Name     string    `json:"name"`     // name of the file as it appears in FUSE
	Target   string    `json:"target"`   // filepath of the hashed filename
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

	_, err = CopyToWorkDir(path, workDirPath, hash)
	l.Target = HashPathFromHashInitial(hash, workDirPath) + filepath.Ext(path)
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

func initialLookupWorker(lwd chan lookupWorkerData, c chan LookupEntry, errChan chan error, doneChan chan struct{}) {
	for x := range lwd {
		le, err := CreateFileLookupEntry(x.subpath, x.output, x.initial)
		if err != nil {
			errChan <- err
			continue
		}
		c <- le
	}
	doneChan <- struct{}{}
}

func CreateInitialDJAFSManifest(path, output string, filesOnly bool) (LookupTable, error) {
	if output == "" {
		output = WorkDir
	} else {
		output = filepath.Join(output, WorkDir)
	}
	lt := LookupTable{sorted: false, Entries: EntrySet{}}
	lookupEntryChan := make(chan LookupEntry, 1)
	errChan := make(chan error, 1)
	lwdChan := make(chan lookupWorkerData, 1)
	doneChan := make(chan struct{}, 1)
	threads := runtime.NumCPU()
	for i := 0; i < threads; i++ {
		go initialLookupWorker(lwdChan, lookupEntryChan, errChan, doneChan)
	}
	go func() {
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
		close(lwdChan)
	}()
workLoop:
	for {
		select {
		case <-doneChan:
			threads--
			if threads == 0 {
				break workLoop
			}
		case le := <-lookupEntryChan:
			lt.Entries = append(lt.Entries, le)
		case errCErr := <-errChan:
			switch {
			case errCErr == nil:
			case os.IsNotExist(errCErr):
			case errors.Is(errCErr, ErrExpectedFile):
			case errors.Is(errCErr, ErrUnexpectedSymlink):
			default:
				log.Printf("error walking path %s: %s", path, errCErr)
				return LookupTable{}, errCErr
			}
		}
	}

	close(doneChan)
	close(lookupEntryChan)
	close(errChan)
	sort.Sort(lt.Entries)
	return lt, nil
}

func CreateDJAFSArchive(path, output string, includeSubdirs bool) error {
	filesOnly := !includeSubdirs
	lt := LookupTable{sorted: false, Entries: EntrySet{}}

	err := filepath.WalkDir(path, func(subpath string, info os.DirEntry, err error) error {
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

		lt.Entries = append(lt.Entries, le)
		return nil
	})
	if err != nil {
		return err
	}
	sort.Sort(lt.Entries)
	manifest := filepath.Join(path, "lookup.djfl")
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
	second := 0
	// TODO check if directory is getting too big and split

	third := hash
	return fmt.Sprintf("%d-%05d-%s", first, second, third)
}

func HashPathFromHashInitial(hash, workDir string) string {
	hInt := colorhash.HashString(hash)
	hInt = hInt % GlobalModulus
	first := hInt
	second := 0
	third := hash

	// first, format directory prefix
	dir := filepath.Join(workDir, fmt.Sprintf("%05d", first))
	// check to see how many iterables are in that directory
	des, err := os.ReadDir(dir)
	// if that directory doesn't exist at all, just return the hash
	// as there's no need to iterate on a non-existent directory
	// TODO check for other errors
	if os.IsNotExist(err) || err != nil {
		return fmt.Sprintf("%05d-%05d-%s", first, second, third)
	}

	// if there are no iterables in that directory, just return the hash
	if len(des) == 0 {
		return fmt.Sprintf("%05d-%05d-%s", first, second, third)
	}

	// for each of the iterable directories inside of the parent
	for _, de := range des {
		// first make sure it's a directory before any other checks
		if de.IsDir() {
			// get the path to the iterable directory
			iDir := filepath.Join(dir, de.Name())
			// get the contents of the iterable directory
			iDEs, err := os.ReadDir(iDir)
			// if there's an error, just return the hash
			if err != nil {
				return fmt.Sprintf("%05d-%05d-%s", first, second, third)
			}
			// if there are less than GlobalModulus files in the iterable directory
			if len(iDEs) <= GlobalModulus {
				return fmt.Sprintf("%05d-%05d-%s", first, second, third)
			}
			// special case: if we've already seen this file, just return the hash
			maybeFile := filepath.Join(iDir, fmt.Sprintf("%05d-%05d-%s", first, second, third))
			_, err = os.Stat(maybeFile)
			if err != nil {
				return fmt.Sprintf("%05d-%05d-%s", first, second, third)
			}
			// otherwise, increment the second counter and try again
			second++
		}
	}
	return fmt.Sprintf("%05d-%05d-%s", first, second, third)
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
