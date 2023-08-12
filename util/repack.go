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
	gcLock     sync.Mutex
	WorkDir    = ".work"
	DataDir    = ".data"
	MappingDir = ".mappings"
)

func CopyToWorkDir(path, workDirPath, hash string) (string, error) {
	stat, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if stat.IsDir() {
		return "", ErrExpectedFile
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	// create work dir
	gcLock.Lock()
	defer gcLock.Unlock()
	// create work dir
	newName := HashPathFromHashInitial(hash, workDirPath) + filepath.Ext(path)
	workspacePrefix, err := WorkspacePrefixFromHashPath(newName)
	workspacePrefix = filepath.Join(WorkDir, workspacePrefix)
	// fmt.Printf("workspacePrefix: %v\n", workspacePrefix)
	if err != nil {
		return "", err
	}
	err = os.MkdirAll(workspacePrefix, 0o755)
	if err != nil {
		return "", err
	}
	// copy file to work dir
	newFile, err := os.Create(filepath.Join(workspacePrefix, newName))
	if errors.Is(err, os.ErrExist) {
	} else if err != nil {
		return "", err
	}
	defer newFile.Close()
	_, err = io.Copy(newFile, file)
	return newName, err
}

func ListWorkDirs() ([]string, error) {
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

func worker(jobChan chan string, done chan struct{}) error {
	for workDir := range jobChan {
		err := PackWorkDir(workDir)
		if err != nil {
			return err
		}
		err = os.RemoveAll(workDir)
		if err != nil {
			return err
		}
	}
	done <- struct{}{}
	return nil
}

func GCWorkDirs() error {
	gcLock.Lock()
	defer gcLock.Unlock()
	workDirs, err := ListWorkDirs()
	if err != nil {
		return err
	}
	if _, statErr := os.Stat(DataDir); statErr != nil {
		os.MkdirAll(DataDir, 0o755)
	}

	doneChan := make(chan struct{}, 1)
	jobChan := make(chan string, 1)
	for _, workDir := range workDirs {
		jobChan <- workDir
	}
	for i := 0; i < runtime.NumCPU(); i++ {
		go worker(jobChan, doneChan)
	}
	close(jobChan)
	for i := 0; i < runtime.NumCPU(); i++ {
		<-doneChan
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
	return CompressHashed(workDir, zipPath)
}
