package util

import (
	"errors"
	"os"
	"path/filepath"
)

var ErrExpectedDirectory = errors.New("expected directory but got file")

func CountSubfile(path string, target int) (count int, overage bool, err error) {
	var info os.FileInfo
	info, err = os.Stat(path)
	if err != nil {
		return
	}
	if !info.IsDir() {
		err = ErrExpectedDirectory
		return
	}
	var files []os.DirEntry
	files, err = os.ReadDir(path)
	if err != nil {
		return
	}
	for _, f := range files {
		if f.Name() == "." || f.Name() == ".." {
			continue
		}
		if !f.IsDir() {
			count++
			if count > target {
				return count, true, nil
			}
		} else {
			c, o, e := CountSubfile(filepath.Join(path, f.Name()), target)
			count += c
			if o {
				return count, true, nil
			}
			err = e
			if err != nil {
				return
			}
		}
	}
	return
}

// given a path and a maximum number of files per zip
// this function tells you the path locations where you should
// recursively zip and where to zip all relative files to build
// out an initial directory
// It does not take into account existing djfz archive files.
func DetermineZipBoundaries(path string, target int) (subfolderRoots []string, subfileRoots []string, err error) {
	_, over, err := CountSubfile(path, target)
	if err != nil {
		return []string{}, []string{}, err
	}

	files, err := os.ReadDir(path)
	if err != nil {
		return []string{}, []string{}, err
	}

	hasFiles := false
	hasSubdirs := false

	// First pass: check if we have files or subdirectories
	for _, f := range files {
		if !f.IsDir() {
			hasFiles = true
		} else {
			hasSubdirs = true
		}
	}

	// If we're under target and have subdirs, this is a subfolder root
	if !over && hasSubdirs {
		return []string{path}, []string{}, nil
	}

	// If we're over target and have subdirs, process subdirs recursively
	if over && hasSubdirs {
		for _, f := range files {
			if f.IsDir() {
				dirs, files, err := DetermineZipBoundaries(filepath.Join(path, f.Name()), target)
				if err != nil {
					return []string{}, []string{}, err
				}
				subfolderRoots = append(subfolderRoots, dirs...)
				subfileRoots = append(subfileRoots, files...)
			}
		}
		return subfolderRoots, subfileRoots, nil
	}

	// If we have files at this level, this is a subfile root
	if hasFiles {
		return []string{}, []string{path}, nil
	}

	// Empty directory case
	return []string{}, []string{}, nil
}
