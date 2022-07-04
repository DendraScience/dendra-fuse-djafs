package util

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestDetermineZipBoundaries(t *testing.T) {
	t.Run("Triple nested subfolders without higher files", func(t *testing.T) {
		dir := t.TempDir()
		var patha = filepath.Join(dir, "a")
		os.Mkdir(patha, 0755)
		pathb1 := filepath.Join(patha, "b1")
		os.Mkdir(pathb1, 0755)
		pathb2 := filepath.Join(patha, "b2")
		os.Mkdir(pathb2, 0755)

		for i := 0; i < 5; i++ {
			os.Create(filepath.Join(pathb1, fmt.Sprintf("%d", i)))
			os.Create(filepath.Join(pathb2, fmt.Sprintf("%d", i)))
		}

		// dirs, files, err := DetermineZipBoundaries(dir, 5)
		//fmt.Printf("Dirss: %v\tFiles: %s\tError: %v \n", dirs, files, err)
	})
	t.Run("Quadruple nested subfolders without higher files", func(t *testing.T) {
		dir := t.TempDir()
		var patha = filepath.Join(dir, "a")
		os.Mkdir(patha, 0755)
		pathb1 := filepath.Join(patha, "b1")
		os.Mkdir(pathb1, 0755)
		pathb2 := filepath.Join(patha, "b2")
		os.Mkdir(pathb2, 0755)
		pathc1 := filepath.Join(pathb2, "c1")
		pathc2 := filepath.Join(pathb2, "c2")
		os.Mkdir(pathc2, 0755)
		os.Mkdir(pathc1, 0755)

		for i := 0; i < 5; i++ {
			os.Create(filepath.Join(pathb1, fmt.Sprintf("%d", i)))
			os.Create(filepath.Join(pathc2, fmt.Sprintf("%d", i)))
			os.Create(filepath.Join(pathc1, fmt.Sprintf("%d", i)))
		}

		//	dirs, files, err := DetermineZipBoundaries(dir, 11)
		//fmt.Printf("Dirss: %v\tFiles: %s\tError: %v \n", dirs, files, err)
	})
	t.Run("Triple nested subfolders with a higher file", func(t *testing.T) {
		dir := t.TempDir()
		var patha = filepath.Join(dir, "a")
		os.Mkdir(patha, 0755)
		pathb1 := filepath.Join(patha, "b1")
		os.Mkdir(pathb1, 0755)
		pathb2 := filepath.Join(patha, "b2")
		os.Mkdir(pathb2, 0755)

		for i := 0; i < 5; i++ {
			os.Create(filepath.Join(pathb1, fmt.Sprintf("%d", i)))
			os.Create(filepath.Join(pathb2, fmt.Sprintf("%d", i)))
		}
		os.Create(filepath.Join(patha, fmt.Sprintf("%d", 0)))

		// dirs, files, err := DetermineZipBoundaries(dir, 5)
		//fmt.Printf("Dirss: %v\tFiles: %s\tError: %v \n", dirs, files, err)
	})
}
func TestCountSubfile(t *testing.T) {
	testCases := []struct {
		Name          string
		FilesToCreate int
		Target        int
		Count         int
		Overage       bool
		Error         error
	}{{Name: "test no files", FilesToCreate: 0, Target: 1, Count: 0, Overage: false},
		{Name: "test with subdirs but no target 1", FilesToCreate: 15, Target: 1, Count: 2, Overage: true},
		{Name: "test target higher than count with one subdir", FilesToCreate: 15, Target: 16, Count: 15, Overage: false},
		{Name: "test target higher than count with many subdir", FilesToCreate: 1000, Target: 1001, Count: 1000, Overage: false},
		{Name: "test over limit", FilesToCreate: 5, Target: 1, Count: 2, Overage: true}}
	for _, c := range testCases {
		t.Run("count pwd", func(t *testing.T) {
			dir := t.TempDir()
			var path = dir
			for i := 0; i < c.FilesToCreate/10; i++ {
				path = filepath.Join(path, fmt.Sprintf("%d", i))
				os.Mkdir(path, 0755)
				for w := 0; w < 10; w++ {
					os.Create(filepath.Join(path, fmt.Sprintf("%d.file", w)))
				}
			}
			for i := 0; i < c.FilesToCreate%10; i++ {
				os.Create(filepath.Join(dir, fmt.Sprintf("%d.file", i)))
			}
			count, overage, err := CountSubfile(dir, c.Target)
			if count != c.Count {
				t.Errorf("Expected Count to be %d but got %d", c.Count, count)
			}
			if overage != c.Overage {
				t.Errorf("Expected Count to be %v but got %v", c.Overage, overage)
			}
			if err != c.Error {
				t.Errorf("Expected Error to be %v but got %v", c.Error, err)
			}
		})
	}
	t.Run("Test nonexistant path", func(t *testing.T) {
		dir := t.TempDir()
		var path = filepath.Join(dir, "nonexistant")
		//	var nonexistantErr := nil
		_, _, err := CountSubfile(path, 100)
		if !os.IsNotExist(err) {
			t.Errorf("Expected error of type IsNotExist but got %v", err)
		}
	})
	t.Run("Test file instead of directory", func(t *testing.T) {
		dir := t.TempDir()
		var path = filepath.Join(dir, "file")
		os.Create(path)
		//	var nonexistantErr := nil
		_, _, err := CountSubfile(path, 100)
		if err != ErrExpectedDirectory {
			t.Errorf("Expected error of type %v but got %v", ErrExpectedDirectory, err)
		}
	})
}
