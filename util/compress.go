package util

import (
	"archive/zip"
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

var ErrNotDJFZExtension = errors.New("file path extension is not '.djfz'")

type DJFZ struct {
	Path string
}

// NewDJFZ creates a new DJFZ instance for the given file path.
// It validates that the path has a .djfz extension and returns an error if not.
func NewDJFZ(path string) (DJFZ, error) {
	if filepath.Ext(path) != "djfz" {
		return DJFZ{}, ErrNotDJFZExtension
	}
	return DJFZ{
		Path: path,
	}, nil
}

//func (d *DJFZ) LookupTable() (LookupTable, error) {
//	zrc, err := zip.OpenReader(d.Path)
//	if err != nil {
//		return LookupTable{}, err
//	}
//	f, err := zrc.Open("lookups.djfl")
//	if err != nil {
//		return LookupTable{}, err
//	}
//	x := bufio.NewReader(f)
//	jd := json.NewDecoder(x)
//	lt := LookupTable{}
//	err = jd.Decode(&lt)
//	return lt, err
//}

// LookupFromDJFZ extracts and returns the lookup table from a DJFZ archive file.
// It opens the .djfz file as a ZIP archive and reads the lookups.djfl file within it.
func LookupFromDJFZ(path string) (LookupTable, error) {
	if filepath.Ext(path) != "djfz" {
		return LookupTable{}, ErrNotDJFZExtension
	}
	zrc, err := zip.OpenReader(path)
	if err != nil {
		return LookupTable{}, err
	}
	f, err := zrc.Open("lookups.djfl")
	if err != nil {
		return LookupTable{}, err
	}
	x := bufio.NewReader(f)
	jd := json.NewDecoder(x)
	lt := LookupTable{}
	err = jd.Decode(&lt)
	return lt, err
}

// CountFilesInDJFZ returns the number of files contained in a DJFZ archive.
// It opens the archive and counts the entries in the ZIP file.
func CountFilesInDJFZ(path string) (int, error) {
	zrc, err := zip.OpenReader(path)
	if err != nil {
		return 0, err
	}
	defer zrc.Close()
	return len(zrc.File), nil
}

// CheckFileInDJFZ checks whether a specific filename exists within a DJFZ archive.
// It returns true if the file is found, false otherwise.
func CheckFileInDJFZ(path string, filename string) (bool, error) {
	zrc, err := zip.OpenReader(path)
	if err != nil {
		return false, err
	}
	defer zrc.Close()
	for _, v := range zrc.File {
		if v.Name == filename {
			return true, nil
		}
	}
	return false, nil
}

// CompressDirectoryToDest compresses an entire directory into a ZIP archive at the destination path.
// It validates that the source path is a directory and creates a new ZIP file containing all directory contents.
func CompressDirectoryToDest(path string, dest string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return ErrExpectedDirectory
	}
	os.Remove(dest)
	file, err := os.Create(dest)
	if err != nil {
		return err
	}
	w := zip.NewWriter(file)
	defer w.Close()

	// TODO:consider using a filewalker here instead of ReadDir
	dirents, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	for _, v := range dirents {
		if v.IsDir() {
			continue
		}
		f, openErr := os.Open(filepath.Join(path, v.Name()))
		if openErr != nil {
			return openErr
		}
		defer f.Close()
		writer, createErr := w.Create(v.Name())
		if createErr != nil {
			return createErr
		}
		_, copyErr := io.Copy(writer, f)
		if copyErr != nil {
			return copyErr
		}
	}

	return err
}

// ZipInside creates a ZIP archive named "files.djfz" inside the specified directory.
// If filesOnly is true, it only includes files (not subdirectories) in the archive.
func ZipInside(path string, filesOnly bool) error {
	filename := "files.djfz"

	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return ErrExpectedDirectory
	}
	outpath := filepath.Join(path, filename)
	file, err := os.Create(outpath)
	if err != nil {
		return err
	}
	w := zip.NewWriter(file)
	defer w.Close()
	if filesOnly {
		fileSet, err := os.ReadDir(path)
		if err != nil {
			return err
		}
		for _, v := range fileSet {
			suffix := filepath.Ext(v.Name())
			if suffix == "djfz" || suffix == "djfl" {
				continue
			}
			if v.Name() == outpath {
				continue
			}
			if v.IsDir() {
				continue
			}
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			defer f.Close()
			writer, err := w.Create(path)
			if err != nil {
				return err
			}
			io.Copy(writer, f)

		}
	} else {
		// TODO there's a bug somewhere here, not sure where.
		// I think we need to check to make sure we aren't including files at the
		// current level, and only get stuff in subdirs
		err = filepath.WalkDir(path, func(path string, d fs.DirEntry, _ error) error {
			suffix := filepath.Ext(d.Name())
			if suffix == ".djfz" || suffix == ".djfl" {
				return nil
			}
			if d.IsDir() {
				w.Create(filepath.Join(path, d.Name()) + "/")
				return nil
			}
			f, openErr := os.Open(path)
			if openErr != nil {
				return openErr
			}
			defer f.Close()
			writer, createErr := w.Create(path)
			if createErr != nil {
				return createErr
			}
			io.Copy(writer, f)
			return nil
		})
	}
	return err
}
