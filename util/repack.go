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
	WorkDir = ".work"
	DataDir = ".data"
)

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

func ListWorkDirs(workDirPath string) ([]string, error) {
	topLevel, err := filepath.Glob(filepath.Join(WorkDir, "*"))
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

func WorkDirPathToZipPath(workDir string) string {
	workDir = filepath.Clean(workDir)
	wd := strings.TrimPrefix(workDir, WorkDir)
	wd = strings.TrimPrefix(wd, "/")
	wd = strings.ReplaceAll(wd, "/", "-")
	return filepath.Join(DataDir, wd+".djfz")
}

func worker(jobChan chan string, errChan chan error) error {
	for workDir := range jobChan {
		err := PackWorkDir(workDir)
		if err != nil {
			errChan <- err
			return err
		}
		err = os.RemoveAll(workDir)
		if err != nil {
			errChan <- err
			return err
		}
	}
	errChan <- nil
	return nil
}

func GCWorkDirs(WorkDirPath string) error {
	gcLock.Lock()
	defer gcLock.Unlock()
	workDirs, err := ListWorkDirs(WorkDirPath)
	if err != nil {
		return err
	}
	if _, statErr := os.Stat(DataDir); statErr != nil {
		os.MkdirAll(DataDir, 0o755)
	}

	jobChan := make(chan string, 1)
	errChan := make(chan error, runtime.NumCPU())
	for range runtime.NumCPU() {
		go worker(jobChan, errChan)
	}
	for _, workDir := range workDirs {
		jobChan <- workDir
	}

	close(jobChan)

	// Collect errors
	var errs []error
	for range runtime.NumCPU() {
		if err := <-errChan; err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		// Handle or return errors
		return errors.Join(errs...)
	}

	return nil
}

func PackWorkDir(workDir string) error {
	// fmt.Println("workDir: ", workDir)
	zipPath := WorkDirPathToZipPath(workDir)
	// fmt.Println("zipPath: ", zipPath)
	// before pack, check if zip file exists
	// and merge

	_, err := os.Stat(zipPath)
	if err == nil {
		rc, err := zip.OpenReader(zipPath)
		if err != nil {
			return err
		}
		defer rc.Close()
		// extract all files from the zip into
		// the work dir
		for _, f := range rc.File {
			fpath := filepath.Join(workDir, f.Name)
			newFile, err := os.Create(fpath)
			if err != nil {
				if errors.Is(err, os.ErrExist) {
					continue
				}
				return err
			}
			defer newFile.Close()
			cFile, err := f.Open()
			if err != nil {
				return err
			}
			defer cFile.Close()
			_, err = io.Copy(newFile, cFile)
			if err != nil {
				return err
			}
		}
	}
	// fmt.Println("compressing work dir: ", workDir, " to zip: ", zipPath)
	return CompressDirectoryToDest(workDir, zipPath)
}
