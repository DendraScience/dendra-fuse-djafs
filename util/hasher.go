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
	"strings"
	"sync"

	"github.com/taigrr/colorhash"
)

// GlobalModulus is the maximum number of files per zip archive.
// recommendation for ext3 is no more than 32000 files per directory
// so if you increase this, don't increase it by too much
const GlobalModulus = 5000

// RenameHashedFile renames a file to its content hash with the original extension.
// It calculates the SHA-256 hash of the file content and renames the file accordingly.
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

// CreateInitialDJAFSManifest creates a lookup table manifest for all files in the specified path.
// It processes files concurrently, calculates their hashes, and creates lookup entries.
// If filesOnly is true, it only processes files (not subdirectories).
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
	lt.Sort()
	return lt, nil
}

// CreateDJAFSArchive creates a complete DJFZ archive from a directory path.
// It generates a manifest, compresses the directory, and creates the final archive.
// If includeSubdirs is true, subdirectories are included in the archive.
func CreateDJAFSArchive(path, output string, includeSubdirs bool) error {
	filesOnly := !includeSubdirs
	lt := LookupTable{sorted: false, entries: []LookupEntry{}}

	err := filepath.WalkDir(path, func(subpath string, info os.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("error walking path %s: %w", subpath, err)
		}
		
		// Skip the root directory entry itself
		if subpath == path {
			return nil
		}
		
		// If filesOnly is true, skip subdirectories (but not the files in the root)
		if filesOnly && info.IsDir() {
			return filepath.SkipDir
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

		// Fix the Name field to be relative to the boundary path
		relativePath, err := filepath.Rel(path, subpath)
		if err != nil {
			return fmt.Errorf("failed to get relative path: %w", err)
		}
		le.Name = relativePath

		lt.Add(le)
		return nil
	})
	if err != nil {
		return fmt.Errorf("error walking path %s: %w", path, err)
	}
	lt.Sort()
	
	outputDir := filepath.Join(output, DataDir)
	err = os.MkdirAll(outputDir, 0755)
	if err != nil {
		return fmt.Errorf("failed to create output directory %s: %w", outputDir, err)
	}
	
	manifest := filepath.Join(outputDir, "lookups.djfl")
	err = WriteJSONFile(manifest, lt)
	if err != nil {
		return err
	}
	
	// Create zip file in output directory instead of input directory
	err = ZipToOutput(path, outputDir, filesOnly)
	if err != nil {
		return err
	}
	
	// Generate and write metadata
	metadata, err := lt.GenerateMetadata("")
	if err != nil {
		return fmt.Errorf("failed to generate metadata: %w", err)
	}
	
	metadataPath := filepath.Join(outputDir, "metadata.djfm")
	err = WriteJSONFile(metadataPath, metadata)
	if err != nil {
		return fmt.Errorf("failed to write metadata: %w", err)
	}
	
	return nil
}

// WriteJSONFile writes any value as JSON to the specified file path.
// It creates the file and encodes the value using the standard JSON encoder.
func WriteJSONFile(path string, v any) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(v)
}

// ManifestLocationForPath finds the appropriate lookup table manifest file for a given path.
// It implements a "dead end" detection algorithm by walking up the directory tree
// to find the closest manifest file that should contain the path's lookup information.
func ManifestLocationForPath(path string) (string, error) {
	// Walk up the directory tree to find the appropriate lookup table
	// This implements the "dead end" detection algorithm described in the README

	cleanPath := filepath.Clean(path)
	currentPath := cleanPath

	for {
		// Check if current directory exists in storage
		if _, err := os.Stat(currentPath); os.IsNotExist(err) {
			// Hit a "dead end" - back up one level
			parentPath := filepath.Dir(currentPath)
			if parentPath == currentPath {
				// Reached root without finding manifest
				return "", fmt.Errorf("no manifest found for path %s", path)
			}

			// Look for lookup table in parent directory
			manifestPath := filepath.Join(parentPath, "lookups.djfl")
			if _, err := os.Stat(manifestPath); err == nil {
				return manifestPath, nil
			}

			currentPath = parentPath
			continue
		}

		// Directory exists, check for lookup table here
		manifestPath := filepath.Join(currentPath, "lookups.djfl")
		if _, err := os.Stat(manifestPath); err == nil {
			return manifestPath, nil
		}

		// Move up one level
		parentPath := filepath.Dir(currentPath)
		if parentPath == currentPath {
			// Reached root
			break
		}
		currentPath = parentPath
	}

	return "", fmt.Errorf("no manifest found for path %s", path)
}

// HashFromHashPath extracts the original hash from a hash-based file path.
// It expects a path in the format "prefix-hash-suffix" and returns the hash portion.
func HashFromHashPath(path string) (string, error) {
	parts := strings.Split(path, "-")
	if len(parts) != 3 {
		return "", ErrInvalidHashPath
	}
	return parts[2], nil
}

// HashPathFromHash generates a content-addressed identifier from a hash.
// The result is in the format "bucket-subbucket-hash" (e.g., "742-00000-abc123...")
// and is used as both:
//   - The filename in the work directory (.work/<result>)
//   - The entry name inside archive files (.djfz)
//   - The Target field in LookupEntry for file resolution
//
// The bucket (first component) is derived from a color hash mod 1000, giving 1000 buckets.
// The subbucket (second component) uses a secondary hash for further distribution when needed.
// This allows up to 1000 * 100000 = 100 million buckets total.
func HashPathFromHash(hash string) string {
	return HashPathFromHashWithSubbucket(hash, 0)
}

// HashPathFromHashWithSubbucket generates a hash path with a specific subbucket.
// This is useful when the default bucket is full and files need to be distributed
// to additional subbuckets.
func HashPathFromHashWithSubbucket(hash string, subbucket int) string {
	hInt := colorhash.HashString(hash)
	bucket := hInt % 1000
	return fmt.Sprintf("%d-%05d-%s", bucket, subbucket, hash)
}

// GetSubbucketFromHash returns a secondary subbucket index based on the hash.
// This can be used to distribute files when a primary bucket gets too large.
// Returns a value from 0-99999.
func GetSubbucketFromHash(hash string) int {
	// Use the last 5 characters of the hash as the secondary bucket
	if len(hash) < 5 {
		return 0
	}
	var subbucket int
	for i := len(hash) - 5; i < len(hash); i++ {
		subbucket = subbucket*16 + hexCharToInt(hash[i])
	}
	return subbucket % 100000
}

// hexCharToInt converts a hex character to its integer value.
func hexCharToInt(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c - 'a' + 10)
	case c >= 'A' && c <= 'F':
		return int(c - 'A' + 10)
	default:
		return 0
	}
}

// WorkspacePrefixFromHashPath extracts the workspace directory prefix from a hash path.
// It returns the first two components of the hash path as a directory path.
func WorkspacePrefixFromHashPath(path string) (string, error) {
	parts := strings.Split(path, "-")
	if len(parts) < 3 {
		return "", ErrInvalidHashPath
	}
	return filepath.Join(parts[0], parts[1]), nil
}

// ZipPrefixFromHashPath extracts the ZIP archive prefix from a hash path.
// It returns the first two components joined with a hyphen for archive naming.
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

// GetHash calculates the SHA-256 hash of data from an io.Reader.
// It returns the hash as a hexadecimal string.
func GetHash(r io.Reader) (string, error) {
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}
