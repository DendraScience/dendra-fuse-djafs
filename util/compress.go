package util

import (
	"archive/zip"
	"bufio"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

type DJFZ struct {
	Path string
}

// NewDJFZ creates a new DJFZ instance for the given file path.
// It validates that the path has a .djfz extension and returns an error if not.
func NewDJFZ(path string) (DJFZ, error) {
	if filepath.Ext(path) != ".djfz" {
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
	if filepath.Ext(path) != ".djfz" {
		return LookupTable{}, ErrNotDJFZExtension
	}
	zrc, err := zip.OpenReader(path)
	if err != nil {
		return LookupTable{}, err
	}
	defer zrc.Close()
	f, err := zrc.Open("lookups.djfl")
	if err != nil {
		return LookupTable{}, err
	}
	defer f.Close()
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
	defer file.Close()

	w := zip.NewWriter(file)
	defer w.Close()

	dirents, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	for _, v := range dirents {
		if v.IsDir() {
			continue
		}
		if err := addFileToZip(w, filepath.Join(path, v.Name()), v.Name()); err != nil {
			return err
		}
	}

	return nil
}

// addFileToZip adds a single file to a zip archive with proper resource cleanup.
func addFileToZip(w *zip.Writer, srcPath, nameInArchive string) error {
	f, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer f.Close()

	writer, err := w.Create(nameInArchive)
	if err != nil {
		return err
	}
	_, err = io.Copy(writer, f)
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
	defer file.Close()

	w := zip.NewWriter(file)
	defer w.Close()

	if filesOnly {
		fileSet, err := os.ReadDir(path)
		if err != nil {
			return err
		}
		for _, v := range fileSet {
			suffix := filepath.Ext(v.Name())
			if suffix == ".djfz" || suffix == ".djfl" {
				continue
			}
			if v.IsDir() {
				continue
			}
			if err := addFileToZip(w, filepath.Join(path, v.Name()), v.Name()); err != nil {
				return err
			}
		}
	} else {
		// Walk directory and add all files from subdirectories
		basePath := path
		err = filepath.WalkDir(path, func(walkPath string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			suffix := filepath.Ext(d.Name())
			if suffix == ".djfz" || suffix == ".djfl" {
				return nil
			}
			if d.IsDir() {
				return nil
			}

			// Calculate relative path for archive entry name
			relPath, err := filepath.Rel(basePath, walkPath)
			if err != nil {
				return err
			}
			// Use forward slashes in zip archive names for cross-platform compatibility
			archiveName := filepath.ToSlash(relPath)

			return addFileToZip(w, walkPath, archiveName)
		})
	}
	return err
}

// ZipToOutput creates a ZIP archive from the source directory and saves it to the output directory.
// If filesOnly is true, it only includes files (not subdirectories) in the archive.
func ZipToOutput(sourcePath, outputPath string, filesOnly bool) error {
	filename := "files.djfz"

	info, err := os.Stat(sourcePath)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return ErrExpectedDirectory
	}
	
	outpath := filepath.Join(outputPath, filename)
	file, err := os.Create(outpath)
	if err != nil {
		return err
	}
	defer file.Close()
	
	w := zip.NewWriter(file)
	defer w.Close()
	
	if filesOnly {
		fileSet, err := os.ReadDir(sourcePath)
		if err != nil {
			return err
		}
		for _, v := range fileSet {
			if v.IsDir() {
				continue
			}
			
			// Skip djafs files
			suffix := filepath.Ext(v.Name())
			if suffix == ".djfz" || suffix == ".djfl" || suffix == ".djfm" {
				continue
			}
			
			sourcefile := filepath.Join(sourcePath, v.Name())
			f, err := os.Open(sourcefile)
			if err != nil {
				return err
			}
			defer f.Close()
			
			writer, err := w.Create(v.Name())
			if err != nil {
				return err
			}
			_, err = io.Copy(writer, f)
			if err != nil {
				return err
			}
		}
	} else {
		// Walk the directory tree and add all files
		err = filepath.WalkDir(sourcePath, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			
			if d.IsDir() {
				return nil
			}
			
			// Skip djafs files
			suffix := filepath.Ext(d.Name())
			if suffix == ".djfz" || suffix == ".djfl" || suffix == ".djfm" {
				return nil
			}
			
			// Get relative path for the zip entry
			relPath, err := filepath.Rel(sourcePath, path)
			if err != nil {
				return err
			}
			
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			defer f.Close()
			
			writer, err := w.Create(relPath)
			if err != nil {
				return err
			}
			_, err = io.Copy(writer, f)
			return err
		})
	}
	return err
}
