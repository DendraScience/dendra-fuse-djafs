package util

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestDetermineZipBoundaries(t *testing.T) {
	t.Run("Empty directory", func(t *testing.T) {
		dir := t.TempDir()
		boundaries, err := DetermineZipBoundaries(dir, 5)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		// Empty dir is under target, should return single boundary with IncludeSubdirs=true
		if len(boundaries) != 1 {
			t.Errorf("Expected 1 boundary for empty dir, got %d", len(boundaries))
		}
		if len(boundaries) > 0 && !boundaries[0].IncludeSubdirs {
			t.Error("Empty directory boundary should have IncludeSubdirs=true")
		}
	})

	t.Run("Single file no subdirs under target", func(t *testing.T) {
		dir := t.TempDir()
		os.Create(filepath.Join(dir, "file.txt"))

		boundaries, err := DetermineZipBoundaries(dir, 5)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if len(boundaries) != 1 {
			t.Errorf("Expected 1 boundary, got %d", len(boundaries))
		}
		if len(boundaries) > 0 && !boundaries[0].IncludeSubdirs {
			t.Error("Under-target directory should have IncludeSubdirs=true")
		}
	})

	t.Run("Only files exceeding target", func(t *testing.T) {
		dir := t.TempDir()
		for i := range 10 {
			os.Create(filepath.Join(dir, fmt.Sprintf("file%d.txt", i)))
		}

		boundaries, err := DetermineZipBoundaries(dir, 5)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		// Over target with files but no subdirs - should create filesOnly boundary
		if len(boundaries) != 1 {
			t.Errorf("Expected 1 boundary, got %d", len(boundaries))
		}
		if len(boundaries) > 0 && boundaries[0].IncludeSubdirs {
			t.Error("Files-only over-target should have IncludeSubdirs=false")
		}
	})

	t.Run("Only empty subdirs", func(t *testing.T) {
		dir := t.TempDir()
		os.Mkdir(filepath.Join(dir, "sub1"), 0o755)
		os.Mkdir(filepath.Join(dir, "sub2"), 0o755)

		boundaries, err := DetermineZipBoundaries(dir, 5)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		// No files anywhere, under target
		if len(boundaries) != 1 {
			t.Errorf("Expected 1 boundary for empty subdirs, got %d", len(boundaries))
		}
	})

	t.Run("Nonexistent path", func(t *testing.T) {
		_, err := DetermineZipBoundaries("/nonexistent/path/12345", 5)
		if err == nil {
			t.Error("Expected error for nonexistent path")
		}
	})

	t.Run("File instead of directory", func(t *testing.T) {
		dir := t.TempDir()
		file := filepath.Join(dir, "file.txt")
		os.Create(file)

		_, err := DetermineZipBoundaries(file, 5)
		if err != ErrExpectedDirectory {
			t.Errorf("Expected ErrExpectedDirectory, got %v", err)
		}
	})

	t.Run("Target zero", func(t *testing.T) {
		dir := t.TempDir()
		os.Create(filepath.Join(dir, "file.txt"))

		boundaries, err := DetermineZipBoundaries(dir, 0)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		// 1 file > 0 target, so should be over
		if len(boundaries) != 1 {
			t.Errorf("Expected 1 boundary, got %d", len(boundaries))
		}
	})

	t.Run("Deep nesting under target", func(t *testing.T) {
		dir := t.TempDir()
		// Create deep structure: dir/a/b/c/d with 1 file each
		path := dir
		for _, name := range []string{"a", "b", "c", "d"} {
			path = filepath.Join(path, name)
			os.Mkdir(path, 0o755)
			os.Create(filepath.Join(path, "file.txt"))
		}

		boundaries, err := DetermineZipBoundaries(dir, 10)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		// 4 files total, under target of 10
		if len(boundaries) != 1 {
			t.Errorf("Expected 1 boundary for under-target deep structure, got %d", len(boundaries))
		}
		if len(boundaries) > 0 && !boundaries[0].IncludeSubdirs {
			t.Error("Under-target should have IncludeSubdirs=true")
		}
	})

	t.Run("Mixed subdirs some over some under", func(t *testing.T) {
		dir := t.TempDir()

		// Create subdir1 with 3 files (under target of 5)
		sub1 := filepath.Join(dir, "sub1")
		os.Mkdir(sub1, 0o755)
		for i := range 3 {
			os.Create(filepath.Join(sub1, fmt.Sprintf("file%d.txt", i)))
		}

		// Create subdir2 with 10 files (over target of 5)
		sub2 := filepath.Join(dir, "sub2")
		os.Mkdir(sub2, 0o755)
		for i := range 10 {
			os.Create(filepath.Join(sub2, fmt.Sprintf("file%d.txt", i)))
		}

		boundaries, err := DetermineZipBoundaries(dir, 5)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		// Total is 13, over target, so should split
		// sub1 (3 files) gets IncludeSubdirs=true
		// sub2 (10 files) gets IncludeSubdirs=false (files only, no subdirs to recurse into)
		if len(boundaries) != 2 {
			t.Errorf("Expected 2 boundaries, got %d", len(boundaries))
		}
	})

	t.Run("Triple nested subfolders without higher files", func(t *testing.T) {
		dir := t.TempDir()

		patha := filepath.Join(dir, "a")
		os.Mkdir(patha, 0o755)

		pathb1 := filepath.Join(patha, "b1")
		os.Mkdir(pathb1, 0o755)

		pathb2 := filepath.Join(patha, "b2")
		os.Mkdir(pathb2, 0o755)

		for i := range 5 {
			os.Create(filepath.Join(pathb1, fmt.Sprintf("%d", i)))
			os.Create(filepath.Join(pathb2, fmt.Sprintf("%d", i)))
		}
		boundaries, err := DetermineZipBoundaries(dir, 5)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if len(boundaries) != 2 {
			t.Errorf("Expected 2 subfolder roots, got %d", len(boundaries))
		}
	})

	t.Run("Quadruple nested subfolders without higher files", func(t *testing.T) {
		dir := t.TempDir()
		patha := filepath.Join(dir, "a")
		os.Mkdir(patha, 0o755)

		pathb1 := filepath.Join(patha, "b1")
		os.Mkdir(pathb1, 0o755)

		pathb2 := filepath.Join(patha, "b2")
		pathc1 := filepath.Join(pathb2, "c1")
		pathc2 := filepath.Join(pathb2, "c2")
		os.MkdirAll(pathc2, 0o755)
		os.MkdirAll(pathc1, 0o755)

		for i := range 5 {
			os.Create(filepath.Join(pathb1, fmt.Sprintf("%d", i)))
			os.Create(filepath.Join(pathc2, fmt.Sprintf("%d", i)))
			os.Create(filepath.Join(pathc1, fmt.Sprintf("%d", i)))
		}

		boundaries, err := DetermineZipBoundaries(dir, 11)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if len(boundaries) != 2 {
			t.Errorf("Expected 2 subfolder roots, got %d", len(boundaries))
		}
	})

	t.Run("Triple nested subfolders with a higher file", func(t *testing.T) {
		dir := t.TempDir()
		patha := filepath.Join(dir, "a")
		os.Mkdir(patha, 0o755)
		pathb1 := filepath.Join(patha, "b1")
		os.Mkdir(pathb1, 0o755)
		pathb2 := filepath.Join(patha, "b2")
		os.Mkdir(pathb2, 0o755)

		for i := range 5 {
			os.Create(filepath.Join(pathb1, fmt.Sprintf("%d", i)))
			os.Create(filepath.Join(pathb2, fmt.Sprintf("%d", i)))
		}
		os.Create(filepath.Join(patha, fmt.Sprintf("%d", 0)))

		boundaries, err := DetermineZipBoundaries(dir, 5)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if len(boundaries) != 3 {
			t.Errorf("Expected 3 subfolder roots, got %d", len(boundaries))
		}

		// Now make sure only the toplevel subfolder is not recursive
		numRecursive := 0
		for _, boundary := range boundaries {
			if boundary.IncludeSubdirs {
				numRecursive++
			}
		}
		if numRecursive != 2 {
			t.Errorf("Expected 2 recursive subfolder roots, got %d", numRecursive)
		}
	})
}

func TestCountSubfile(t *testing.T) {
	t.Run("Empty directory", func(t *testing.T) {
		dir := t.TempDir()
		count, isOver, err := CountSubfile(dir, 1)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if count != 0 {
			t.Errorf("Expected 0 files, got %d", count)
		}
		if isOver {
			t.Error("Empty dir should not be over target")
		}
	})

	t.Run("Target zero with files", func(t *testing.T) {
		dir := t.TempDir()
		os.Create(filepath.Join(dir, "file.txt"))

		count, isOver, err := CountSubfile(dir, 0)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if count != 1 {
			t.Errorf("Expected 1 file, got %d", count)
		}
		if !isOver {
			t.Error("1 file should be over target of 0")
		}
	})

	t.Run("Single file under target", func(t *testing.T) {
		dir := t.TempDir()
		os.Create(filepath.Join(dir, "file.txt"))

		count, isOver, err := CountSubfile(dir, 5)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if count != 1 {
			t.Errorf("Expected 1 file, got %d", count)
		}
		if isOver {
			t.Error("1 file should not be over target of 5")
		}
	})

	t.Run("Empty subdirectories", func(t *testing.T) {
		dir := t.TempDir()
		os.Mkdir(filepath.Join(dir, "empty1"), 0o755)
		os.Mkdir(filepath.Join(dir, "empty2"), 0o755)
		os.Mkdir(filepath.Join(dir, "empty3"), 0o755)

		count, isOver, err := CountSubfile(dir, 5)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if count != 0 {
			t.Errorf("Expected 0 files in empty subdirs, got %d", count)
		}
		if isOver {
			t.Error("Empty subdirs should not be over target")
		}
	})

	t.Run("Mixed files and empty subdirs", func(t *testing.T) {
		dir := t.TempDir()
		os.Create(filepath.Join(dir, "file1.txt"))
		os.Create(filepath.Join(dir, "file2.txt"))
		os.Mkdir(filepath.Join(dir, "empty"), 0o755)

		count, isOver, err := CountSubfile(dir, 5)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if count != 2 {
			t.Errorf("Expected 2 files, got %d", count)
		}
		if isOver {
			t.Error("2 files should not be over target of 5")
		}
	})

	t.Run("Exact target count", func(t *testing.T) {
		dir := t.TempDir()
		for i := range 5 {
			os.Create(filepath.Join(dir, fmt.Sprintf("file%d.txt", i)))
		}

		count, isOver, err := CountSubfile(dir, 5)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if count != 5 {
			t.Errorf("Expected 5 files, got %d", count)
		}
		if isOver {
			t.Error("Exact target count should not be over")
		}
	})

	t.Run("One over target count", func(t *testing.T) {
		dir := t.TempDir()
		for i := range 6 {
			os.Create(filepath.Join(dir, fmt.Sprintf("file%d.txt", i)))
		}

		count, isOver, err := CountSubfile(dir, 5)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		// Early exit when over, so count is 6 (first over)
		if count != 6 {
			t.Errorf("Expected 6 files (early exit), got %d", count)
		}
		if !isOver {
			t.Error("6 files should be over target of 5")
		}
	})

	testCases := []struct {
		Name          string
		FilesToCreate int
		Target        int
		Count         int
		Overage       bool
		Error         error
	}{
		{Name: "test no files", FilesToCreate: 0, Target: 1, Count: 0, Overage: false},
		{Name: "test with subdirs but no target 1", FilesToCreate: 15, Target: 1, Count: 2, Overage: true},
		{Name: "test target higher than count with one subdir", FilesToCreate: 15, Target: 16, Count: 15, Overage: false},
		{Name: "test target higher than count with many subdir", FilesToCreate: 1000, Target: 1001, Count: 1000, Overage: false},
		{Name: "test over limit", FilesToCreate: 5, Target: 1, Count: 2, Overage: true},
	}
	for _, c := range testCases {
		t.Run(c.Name, func(t *testing.T) {
			dir := t.TempDir()
			path := dir
			for i := 0; i < c.FilesToCreate/10; i++ {
				path = filepath.Join(path, fmt.Sprintf("%d", i))
				os.Mkdir(path, 0o755)
				for w := range 10 {
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
				t.Errorf("Expected Overage to be %v but got %v", c.Overage, overage)
			}
			if err != c.Error {
				t.Errorf("Expected Error to be %v but got %v", c.Error, err)
			}
		})
	}

	t.Run("Nonexistent path", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "nonexistent")
		_, _, err := CountSubfile(path, 100)
		if !os.IsNotExist(err) {
			t.Errorf("Expected error of type IsNotExist but got %v", err)
		}
	})

	t.Run("File instead of directory", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "file")
		os.Create(path)
		_, _, err := CountSubfile(path, 100)
		if err != ErrExpectedDirectory {
			t.Errorf("Expected error of type %v but got %v", ErrExpectedDirectory, err)
		}
	})

	t.Run("Triple nested subfolders without higher files", func(t *testing.T) {
		dir := t.TempDir()
		patha := filepath.Join(dir, "a")
		totalCreated := 0
		err := os.Mkdir(patha, 0o755)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		pathb1 := filepath.Join(patha, "b1")
		err = os.Mkdir(pathb1, 0o755)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		pathb2 := filepath.Join(patha, "b2")
		err = os.Mkdir(pathb2, 0o755)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		for i := range 5 {
			file, err := os.Create(filepath.Join(pathb1, fmt.Sprintf("%d", i)))
			totalCreated++
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			file.Close()
			file, err = os.Create(filepath.Join(pathb2, fmt.Sprintf("%d", i)))
			totalCreated++
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			file.Close()
		}
		totalFound, isOver, err := CountSubfile(dir, 5)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if !isOver {
			t.Errorf("Expected overage but got not overage")
		}
		if totalFound != totalCreated {
			t.Errorf("Expected %d files, got %d", totalCreated, totalFound)
		}
	})
}
