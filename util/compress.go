package util

import (
	"archive/zip"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

func ZipInside(path string, dest string, filesOnly bool) error {
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
			if v.Name() == outpath {
				continue
			}
			if v.IsDir() {
				continue
			}
			f, err := os.Open(filepath.Join(path, v.Name()))
			if err != nil {
				return err
			}
			defer f.Close()
			writer, err := w.Create(filepath.Join(path, v.Name()))
			io.Copy(writer, f)

		}
	} else {
		// TODO there's a bug somewhere here, not sure where.
		// I think we need to check to make sure we aren't including files at the
		// current level, and only get stuff in subdirs
		filepath.WalkDir(path, func(path string, d fs.DirEntry, err error) error {
			if d.Name() == outpath {
				return nil
			}
			if d.IsDir() {
				w.Create(filepath.Join(path, d.Name()) + "/")
				return nil
			}
			f, err := os.Open(filepath.Join(path, d.Name()))
			if err != nil {
				return err
			}
			defer f.Close()
			writer, err := w.Create(filepath.Join(path, d.Name()))
			if err != nil {
				return err
			}
			io.Copy(writer, f)
			return nil
		})
	}
	return nil
}
