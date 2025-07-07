package util

import (
	"archive/zip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

var (
	gcLock  sync.Mutex
	WorkDir = "work"
	DataDir = "data"
)

// CopyToWorkDir copies a file to the work directory with the specified hash as filename.
// It validates that the source is a file (not directory) and returns the destination path.
func CopyToWorkDir(path, workDirPath, hash string) (string, error) {
	stat, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if stat.IsDir() {
		return "", ErrExpectedFile
	}

	hashPath := HashPathFromHash(hash)
	workspacePath := filepath.Join(workDirPath, hashPath)

	// Create directory structure if it doesn't exist
	workspacePrefix, err := WorkspacePrefixFromHashPath(hashPath)
	if err != nil {
		return "", err
	}
	workspacePrefix = filepath.Join(workDirPath, workspacePrefix)

	gcLock.Lock()
	defer gcLock.Unlock()

	err = os.MkdirAll(workspacePrefix, 0o755)
	if err != nil {
		return "", err
	}

	// Check if file already exists (deduplication)
	if _, err := os.Stat(workspacePath); err == nil {
		return workspacePath, nil
	}

	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	// Copy file to work dir
	newFile, err := os.Create(workspacePath)
	if err != nil {
		return "", err
	}
	defer newFile.Close()

	_, err = io.Copy(newFile, file)
	return workspacePath, err
}

// ListWorkDirs returns a list of all work directories found in the work directory path.
// It scans for top-level directories that contain files to be processed.
func ListWorkDirs(workDirPath string) ([]string, error) {
	topLevel, err := filepath.Glob(filepath.Join(workDirPath, "*"))
	if err != nil {
		return nil, err
	}
	var workDirs []string
	for _, dir := range topLevel {
		children, err := filepath.Glob(filepath.Join(dir, "*"))
		if err != nil {
			return nil, err
		}
		workDirs = append(workDirs, children...)
	}
	return workDirs, nil
}

// WorkDirPathToZipPath converts a work directory path to the corresponding ZIP archive path.
// It removes the work directory prefix and formats the path for archive naming.
// The basePath is the root work directory that should be stripped from workDir.
func WorkDirPathToZipPath(workDir, basePath, dataDir string) string {
	workDir = filepath.Clean(workDir)
	basePath = filepath.Clean(basePath)
	wd := strings.TrimPrefix(workDir, basePath)
	wd = strings.TrimPrefix(wd, string(filepath.Separator))
	wd = strings.ReplaceAll(wd, string(filepath.Separator), "-")
	return filepath.Join(dataDir, wd+".djfz")
}

// workerResult holds the result of a worker's processing
type workerResult struct {
	err error
}

func gcWorker(jobChan <-chan string, resultChan chan<- workerResult, basePath, dataDir string, wg *sync.WaitGroup) {
	defer wg.Done()
	for workDir := range jobChan {
		err := PackWorkDir(workDir, basePath, dataDir)
		if err != nil {
			resultChan <- workerResult{err: err}
			continue
		}
		err = os.RemoveAll(workDir)
		if err != nil {
			resultChan <- workerResult{err: err}
			continue
		}
		resultChan <- workerResult{err: nil}
	}
}

// GCWorkDirs performs garbage collection on work directories by packing them into archives.
// It processes all work directories concurrently and uses a lock to prevent concurrent execution.
func GCWorkDirs(workDirPath string) error {
	gcLock.Lock()
	defer gcLock.Unlock()

	workDirs, err := ListWorkDirs(workDirPath)
	if err != nil {
		return err
	}
	if len(workDirs) == 0 {
		return nil
	}

	// Data directory is sibling to work directory
	dataDir := filepath.Join(filepath.Dir(workDirPath), DataDir)
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return err
	}

	numWorkers := min(runtime.NumCPU(), len(workDirs))

	jobChan := make(chan string, len(workDirs))
	resultChan := make(chan workerResult, len(workDirs))

	var wg sync.WaitGroup
	wg.Add(numWorkers)
	for range numWorkers {
		go gcWorker(jobChan, resultChan, workDirPath, dataDir, &wg)
	}

	// Send all jobs
	for _, workDir := range workDirs {
		jobChan <- workDir
	}
	close(jobChan)

	// Wait for workers to finish
	wg.Wait()
	close(resultChan)

	// Collect errors
	var errs []error
	for result := range resultChan {
		if result.err != nil {
			errs = append(errs, result.err)
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}

// PackWorkDir packs a work directory into a ZIP archive.
// It checks if an existing archive exists and merges the contents if necessary.
func PackWorkDir(workDir, basePath, dataDir string) error {
	zipPath := WorkDirPathToZipPath(workDir, basePath, dataDir)

	// Check if zip file exists and merge contents
	if _, err := os.Stat(zipPath); err == nil {
		if err := extractZipToDir(zipPath, workDir); err != nil {
			return err
		}
	}

	return CompressDirectoryToDest(workDir, zipPath)
}

// extractZipToDir extracts all files from a ZIP archive into a directory.
func extractZipToDir(zipPath, destDir string) error {
	rc, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer rc.Close()

	for _, f := range rc.File {
		fpath := filepath.Join(destDir, f.Name)

		// Skip if file already exists (we keep the newer version in workDir)
		if _, err := os.Stat(fpath); err == nil {
			continue
		}

		srcFile, err := f.Open()
		if err != nil {
			return err
		}

		destFile, err := os.Create(fpath)
		if err != nil {
			srcFile.Close()
			if errors.Is(err, os.ErrExist) {
				continue
			}
			return err
		}

		_, err = io.Copy(destFile, srcFile)
		srcFile.Close()
		destFile.Close()
		if err != nil {
			return err
		}
	}
	return nil
}
