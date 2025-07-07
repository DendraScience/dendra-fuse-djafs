package util

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListWorkDirs_UsesParameter(t *testing.T) {
	// Create a temp directory structure that mimics work dir layout
	baseDir := t.TempDir()
	workDir := filepath.Join(baseDir, "custom_work")

	// Create bucket structure: workDir/bucket1/subbucket1/
	bucket1 := filepath.Join(workDir, "123")
	subbucket1 := filepath.Join(bucket1, "00000")
	os.MkdirAll(subbucket1, 0755)

	bucket2 := filepath.Join(workDir, "456")
	subbucket2 := filepath.Join(bucket2, "00001")
	os.MkdirAll(subbucket2, 0755)

	// Create some files in subbuckets
	os.WriteFile(filepath.Join(subbucket1, "file1.txt"), []byte("test"), 0644)
	os.WriteFile(filepath.Join(subbucket2, "file2.txt"), []byte("test"), 0644)

	// Test that ListWorkDirs uses the provided path, not the global WorkDir
	dirs, err := ListWorkDirs(workDir)
	if err != nil {
		t.Fatalf("ListWorkDirs failed: %v", err)
	}

	// Should find the subbuckets
	if len(dirs) != 2 {
		t.Errorf("Expected 2 work dirs, got %d: %v", len(dirs), dirs)
	}

	// Verify the paths are correct
	for _, dir := range dirs {
		if !hasPathPrefix(dir, workDir) {
			t.Errorf("Work dir %q should be under %q", dir, workDir)
		}
	}
}

// hasPathPrefix checks if path starts with prefix (respecting path boundaries)
func hasPathPrefix(path, prefix string) bool {
	path = filepath.Clean(path)
	prefix = filepath.Clean(prefix)
	return path == prefix || len(path) > len(prefix) && path[:len(prefix)] == prefix && path[len(prefix)] == filepath.Separator
}

func TestListWorkDirs_EmptyDir(t *testing.T) {
	workDir := t.TempDir()

	dirs, err := ListWorkDirs(workDir)
	if err != nil {
		t.Fatalf("ListWorkDirs failed: %v", err)
	}

	if len(dirs) != 0 {
		t.Errorf("Expected 0 work dirs for empty directory, got %d", len(dirs))
	}
}

func TestWorkDirPathToZipPath(t *testing.T) {
	tests := []struct {
		name     string
		workDir  string
		basePath string
		dataDir  string
		want     string
	}{
		{
			name:     "simple path",
			workDir:  "/storage/work/123/00000",
			basePath: "/storage/work",
			dataDir:  "/storage/data",
			want:     "/storage/data/123-00000.djfz",
		},
		{
			name:     "relative paths",
			workDir:  "work/456/00001",
			basePath: "work",
			dataDir:  "data",
			want:     "data/456-00001.djfz",
		},
		{
			name:     "deep nesting",
			workDir:  "/a/b/c/work/789/00002",
			basePath: "/a/b/c/work",
			dataDir:  "/a/b/c/data",
			want:     "/a/b/c/data/789-00002.djfz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := WorkDirPathToZipPath(tt.workDir, tt.basePath, tt.dataDir)
			if got != tt.want {
				t.Errorf("WorkDirPathToZipPath(%q, %q, %q) = %q, want %q",
					tt.workDir, tt.basePath, tt.dataDir, got, tt.want)
			}
		})
	}
}

func TestGCWorkDirs_CreatesDataDir(t *testing.T) {
	baseDir := t.TempDir()
	workDir := filepath.Join(baseDir, "work")
	dataDir := filepath.Join(baseDir, "data")

	// Create work dir structure with a file
	bucket := filepath.Join(workDir, "123", "00000")
	os.MkdirAll(bucket, 0755)
	os.WriteFile(filepath.Join(bucket, "testfile"), []byte("content"), 0644)

	// Data dir should not exist yet
	if _, err := os.Stat(dataDir); !os.IsNotExist(err) {
		t.Fatal("Data dir should not exist before GC")
	}

	// Run GC
	err := GCWorkDirs(workDir)
	if err != nil {
		t.Fatalf("GCWorkDirs failed: %v", err)
	}

	// Data dir should now exist
	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		t.Error("Data dir should be created by GC")
	}
}

func TestGCWorkDirs_EmptyWorkDir(t *testing.T) {
	workDir := t.TempDir()

	// Should not error on empty work dir
	err := GCWorkDirs(workDir)
	if err != nil {
		t.Errorf("GCWorkDirs should not error on empty dir: %v", err)
	}
}

func TestGCWorkDirs_PacksAndRemoves(t *testing.T) {
	baseDir := t.TempDir()
	workDir := filepath.Join(baseDir, "work")
	dataDir := filepath.Join(baseDir, "data")

	// Create work dir structure with files
	bucket := filepath.Join(workDir, "123", "00000")
	os.MkdirAll(bucket, 0755)
	os.WriteFile(filepath.Join(bucket, "file1"), []byte("content1"), 0644)
	os.WriteFile(filepath.Join(bucket, "file2"), []byte("content2"), 0644)

	// Run GC
	err := GCWorkDirs(workDir)
	if err != nil {
		t.Fatalf("GCWorkDirs failed: %v", err)
	}

	// Work bucket should be removed
	if _, err := os.Stat(bucket); !os.IsNotExist(err) {
		t.Error("Work bucket should be removed after GC")
	}

	// Archive should be created
	archivePath := filepath.Join(dataDir, "123-00000.djfz")
	if _, err := os.Stat(archivePath); os.IsNotExist(err) {
		t.Error("Archive should be created by GC")
	}
}

func TestExtractZipToDir(t *testing.T) {
	// Create a zip file first
	srcDir := t.TempDir()
	os.WriteFile(filepath.Join(srcDir, "file1.txt"), []byte("content1"), 0644)
	os.WriteFile(filepath.Join(srcDir, "file2.txt"), []byte("content2"), 0644)

	zipPath := filepath.Join(t.TempDir(), "test.zip")
	err := CompressDirectoryToDest(srcDir, zipPath)
	if err != nil {
		t.Fatalf("Failed to create test zip: %v", err)
	}

	// Extract to new directory
	destDir := t.TempDir()
	err = extractZipToDir(zipPath, destDir)
	if err != nil {
		t.Fatalf("extractZipToDir failed: %v", err)
	}

	// Verify files were extracted
	content1, err := os.ReadFile(filepath.Join(destDir, "file1.txt"))
	if err != nil {
		t.Errorf("Failed to read extracted file1: %v", err)
	}
	if string(content1) != "content1" {
		t.Errorf("Extracted content mismatch: got %q, want %q", content1, "content1")
	}

	content2, err := os.ReadFile(filepath.Join(destDir, "file2.txt"))
	if err != nil {
		t.Errorf("Failed to read extracted file2: %v", err)
	}
	if string(content2) != "content2" {
		t.Errorf("Extracted content mismatch: got %q, want %q", content2, "content2")
	}
}

func TestExtractZipToDir_SkipsExisting(t *testing.T) {
	// Create a zip file
	srcDir := t.TempDir()
	os.WriteFile(filepath.Join(srcDir, "file.txt"), []byte("old content"), 0644)

	zipPath := filepath.Join(t.TempDir(), "test.zip")
	err := CompressDirectoryToDest(srcDir, zipPath)
	if err != nil {
		t.Fatalf("Failed to create test zip: %v", err)
	}

	// Create dest with existing file (different content)
	destDir := t.TempDir()
	os.WriteFile(filepath.Join(destDir, "file.txt"), []byte("new content"), 0644)

	// Extract should skip existing file
	err = extractZipToDir(zipPath, destDir)
	if err != nil {
		t.Fatalf("extractZipToDir failed: %v", err)
	}

	// Existing file should not be overwritten
	content, _ := os.ReadFile(filepath.Join(destDir, "file.txt"))
	if string(content) != "new content" {
		t.Errorf("Existing file should not be overwritten: got %q", content)
	}
}

func TestCopyToWorkDir(t *testing.T) {
	// Create source file
	srcDir := t.TempDir()
	srcFile := filepath.Join(srcDir, "source.txt")
	os.WriteFile(srcFile, []byte("test content"), 0644)

	// Create work dir
	workDir := t.TempDir()

	hash := "abc123def456"
	destPath, err := CopyToWorkDir(srcFile, workDir, hash)
	if err != nil {
		t.Fatalf("CopyToWorkDir failed: %v", err)
	}

	// Verify destination path format
	if !hasPathPrefix(destPath, workDir) {
		t.Errorf("Dest path %q should be under work dir %q", destPath, workDir)
	}

	// Verify file was copied
	content, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("Failed to read copied file: %v", err)
	}
	if string(content) != "test content" {
		t.Errorf("Content mismatch: got %q", content)
	}
}

func TestCopyToWorkDir_Deduplication(t *testing.T) {
	// Create source file
	srcDir := t.TempDir()
	srcFile := filepath.Join(srcDir, "source.txt")
	os.WriteFile(srcFile, []byte("original"), 0644)

	workDir := t.TempDir()
	hash := "samehash123"

	// First copy
	destPath1, err := CopyToWorkDir(srcFile, workDir, hash)
	if err != nil {
		t.Fatalf("First CopyToWorkDir failed: %v", err)
	}

	// Modify source (shouldn't matter due to dedup)
	os.WriteFile(srcFile, []byte("modified"), 0644)

	// Second copy with same hash
	destPath2, err := CopyToWorkDir(srcFile, workDir, hash)
	if err != nil {
		t.Fatalf("Second CopyToWorkDir failed: %v", err)
	}

	// Should return same path
	if destPath1 != destPath2 {
		t.Errorf("Dedup should return same path: %q vs %q", destPath1, destPath2)
	}

	// Content should be original (first copy)
	content, _ := os.ReadFile(destPath1)
	if string(content) != "original" {
		t.Errorf("Dedup should preserve original content: got %q", content)
	}
}

func TestCopyToWorkDir_RejectsDirectory(t *testing.T) {
	srcDir := t.TempDir()
	workDir := t.TempDir()

	_, err := CopyToWorkDir(srcDir, workDir, "somehash")
	if err != ErrExpectedFile {
		t.Errorf("Expected ErrExpectedFile for directory, got: %v", err)
	}
}
