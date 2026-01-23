package util

import (
	"os"
	"path/filepath"
)

// CountSubfile counts the number of files in a directory and checks if it exceeds the target.
// It returns the count, whether it's over the target, and any error encountered.
func CountSubfile(path string, target int) (count int, isOverTarget bool, err error) {
	var info os.FileInfo
	info, err = os.Stat(path)
	if err != nil {
		return 0, false, err
	}
	if !info.IsDir() {
		err = ErrExpectedDirectory
		return 0, false, err
	}
	var files []os.DirEntry
	files, err = os.ReadDir(path)
	if err != nil {
		return 0, false, err
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
				return 0, false, err
			}
		}
	}
	return count, count > target, nil
}

// Given a path and a maximum number of files per zip
// this function tells you the path locations where you should
// recursively zip and where to zip all relative files to build
// out an initial directory
// It does not take into account existing djfz archive files.
// Additionally, the maximum is a soft maximum, as it may be exceeded
// if the files are not stratified enough via subdirectories.
//
// Returns the subfolder roots and the subfile roots.
// These arrays are returned separately so that in a caller function, we know which directories
// should be recursively compressed, and which we should just zip the direct children of.

type ZipBoundary struct {
	Path           string
	IncludeSubdirs bool
}

func DetermineZipBoundaries(path string, target int) ([]ZipBoundary, error) {
	boundaries := []ZipBoundary{}
	_, isOver, err := CountSubfile(path, target)
	if err != nil {
		return boundaries, err
	}

	files, err := os.ReadDir(path)
	if err != nil {
		return boundaries, err
	}

	hasFiles := false
	hasSubdirs := false

	// First pass: check if we have files or subdirectories
	for _, f := range files {
		if f.Name() == "." || f.Name() == ".." {
			continue
		}
		if !f.IsDir() {
			hasFiles = true
		} else {
			hasSubdirs = true
		}
	}

	// If we're under target and have subdirs, this is a subfolder root (and possibly a subfile root)
	// but, either way, we're done processing this directory
	if !isOver {
		boundaries = append(boundaries, ZipBoundary{Path: path, IncludeSubdirs: true})
		return boundaries, nil
	}

	// Process subdirs recursively, since we're over target
	if hasSubdirs {
		for _, f := range files {
			if f.IsDir() {
				bounds, err := DetermineZipBoundaries(filepath.Join(path, f.Name()), target)
				if err != nil {
					return []ZipBoundary{}, err
				}
				boundaries = append(boundaries, bounds...)
			}
		}
	}

	// If we have files at this level and we're not a subfolder root, this is a subfile root
	if hasFiles {
		boundaries = append(boundaries, ZipBoundary{Path: path, IncludeSubdirs: false})
	}

	return boundaries, nil
}
