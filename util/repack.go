package util

import (
	"io"
	"os"
	"path/filepath"
	"sync"
)

var (
	gcLock  sync.Mutex
	WorkDir = ".work"
)

func CopyToWorkDir(path string) error {
	stat, err := os.Stat(path)
	if err != nil {
		return err
	}
	if stat.IsDir() {
		return ErrExpectedFile
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	// create work dir
	gcLock.Lock()
	defer gcLock.Unlock()
	// create work dir
	hash, err := GetFileHash(path)
	newPath := HashPathFromHash(hash, "", "") + filepath.Ext(path)
	workspacePrefix, err := WorkspacePrefixFromHashPath(newPath)
	err = os.MkdirAll(workspacePrefix, 0o755)
	if err != nil {
		return err
	}
	// copy file to work dir
	newFile, err := os.Create(newPath)
	if err != nil {
		return err
	}
	defer newFile.Close()
	_, err = io.Copy(newFile, file)
	return err
}
