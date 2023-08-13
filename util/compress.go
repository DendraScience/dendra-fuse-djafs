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

var ErrNotDJFZExtension error

func init() {
	ErrNotDJFZExtension = errors.New("file path extension is not '.djfz'")
}

// assumes that the path is the name of a djfz file
func ExtractLookupFromDJFZ(path string) (LookupTable, error) {
	if filepath.Ext(path) != "djfz" {
		return LookupTable{}, ErrNotDJFZExtension
	}
	zrc, err := zip.OpenReader(path)
	if err != nil {
		return LookupTable{}, err
	}
	f, err := zrc.Open("lookup.djfl")
	if err != nil {
		return LookupTable{}, err
	}
	x := bufio.NewReader(f)
	jd := json.NewDecoder(x)
	lt := LookupTable{}
	err = jd.Decode(&lt)
	return lt, err
}

func CountFilesInDJFZ(path string) (int, error) {
	zrc, err := zip.OpenReader(path)
	if err != nil {
		return 0, err
	}
	defer zrc.Close()
	return len(zrc.File), nil
}

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

func CompressHashed(path string, dest string) error {
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
	fileSet, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	for _, v := range fileSet {
		if v.IsDir() {
			continue
		}
		//	fmt.Println("compressing: ", filepath.Join(path, v.Name()))
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

func ZipInside(path string, filesOnly bool) error {
	filename := "subdirs.djfz"
	if filesOnly {
		filename = "files.djfz"
	}
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
