package util

import (
	"errors"
	"io/fs"
	"io/ioutil"
	"os"
	"path/filepath"
)

var ErrExpectedDirectory error

func init() {
	ErrExpectedDirectory = errors.New("Expected directory but got file")
}

func CountSubfile(path string, target int) (count int, overage bool, err error) {
	var info os.FileInfo
	info, err = os.Stat(path)
	if err != nil {
		return
	}
	if !info.IsDir() {
		// TODO extract error into global variable
		err = ErrExpectedDirectory
		return
	}
	files := []fs.FileInfo{}
	files, err = ioutil.ReadDir(path)
	for _, f := range files {
		if count > target {
			return count, true, nil
		}
		if f.Name() == "." || f.Name() == ".." {
			continue
		}
		if !f.IsDir() {
			count++
		} else {
			c, _, e := CountSubfile(filepath.Join(path, f.Name()), target-count)
			count += c
			overage = count > target
			err = e
			if err != nil {
				return
			}
		}
	}
	return
}
