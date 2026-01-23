package util

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func TestNewDJFZ_ExtensionCheck(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{
			name:    "valid .djfz extension",
			path:    "archive.djfz",
			wantErr: false,
		},
		{
			name:    "valid .djfz with path",
			path:    "/some/path/to/archive.djfz",
			wantErr: false,
		},
		{
			name:    "missing dot in extension",
			path:    "archivedjfz",
			wantErr: true,
		},
		{
			name:    "wrong extension",
			path:    "archive.zip",
			wantErr: true,
		},
		{
			name:    "no extension",
			path:    "archive",
			wantErr: true,
		},
		{
			name:    "djfz without dot should fail",
			path:    "file_djfz",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewDJFZ(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewDJFZ(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
			}
			if tt.wantErr && err != ErrNotDJFZExtension {
				t.Errorf("NewDJFZ(%q) error = %v, want ErrNotDJFZExtension", tt.path, err)
			}
		})
	}
}

func TestZipInside_FilesOnly(t *testing.T) {
	// Create temp directory with test files
	dir := t.TempDir()

	// Create some test files
	testFiles := []string{"file1.txt", "file2.json", "file3.dat"}
	for _, name := range testFiles {
		f, err := os.Create(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}
		f.WriteString("test content for " + name)
		f.Close()
	}

	// Create a subdirectory with files (should be ignored in filesOnly mode)
	subdir := filepath.Join(dir, "subdir")
	os.Mkdir(subdir, 0755)
	os.WriteFile(filepath.Join(subdir, "subfile.txt"), []byte("subfile content"), 0644)

	// Run ZipInside with filesOnly=true
	err := ZipInside(dir, true)
	if err != nil {
		t.Fatalf("ZipInside failed: %v", err)
	}

	// Verify the archive was created
	archivePath := filepath.Join(dir, "files.djfz")
	if _, err := os.Stat(archivePath); os.IsNotExist(err) {
		t.Fatal("Archive file was not created")
	}

	// Open and verify contents
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		t.Fatalf("Failed to open archive: %v", err)
	}
	defer r.Close()

	// Check that all test files are in the archive with correct names
	foundFiles := make(map[string]bool)
	for _, f := range r.File {
		foundFiles[f.Name] = true
	}

	for _, name := range testFiles {
		if !foundFiles[name] {
			t.Errorf("Expected file %q not found in archive", name)
		}
	}

	// Verify subdirectory files are NOT included
	if foundFiles["subdir/subfile.txt"] || foundFiles["subfile.txt"] {
		t.Error("Subdirectory file should not be included in filesOnly mode")
	}
}

func TestZipInside_WithSubdirs(t *testing.T) {
	// Create temp directory structure
	dir := t.TempDir()

	// Create files at root level
	os.WriteFile(filepath.Join(dir, "root.txt"), []byte("root content"), 0644)

	// Create subdirectory with files
	subdir := filepath.Join(dir, "subdir")
	os.MkdirAll(subdir, 0755)
	os.WriteFile(filepath.Join(subdir, "sub.txt"), []byte("sub content"), 0644)

	// Create nested subdirectory
	nested := filepath.Join(subdir, "nested")
	os.MkdirAll(nested, 0755)
	os.WriteFile(filepath.Join(nested, "deep.txt"), []byte("deep content"), 0644)

	// Run ZipInside with filesOnly=false
	err := ZipInside(dir, false)
	if err != nil {
		t.Fatalf("ZipInside failed: %v", err)
	}

	// Open and verify contents
	archivePath := filepath.Join(dir, "files.djfz")
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		t.Fatalf("Failed to open archive: %v", err)
	}
	defer r.Close()

	// Check files are present with correct relative paths
	foundFiles := make(map[string]bool)
	for _, f := range r.File {
		foundFiles[f.Name] = true
	}

	expectedFiles := []string{"root.txt", "subdir/sub.txt", "subdir/nested/deep.txt"}
	for _, name := range expectedFiles {
		if !foundFiles[name] {
			t.Errorf("Expected file %q not found in archive. Found: %v", name, foundFiles)
		}
	}
}

func TestZipInside_SkipsDJFZAndDJFL(t *testing.T) {
	dir := t.TempDir()

	// Create test files including ones that should be skipped
	os.WriteFile(filepath.Join(dir, "data.txt"), []byte("data"), 0644)
	os.WriteFile(filepath.Join(dir, "existing.djfz"), []byte("zip"), 0644)
	os.WriteFile(filepath.Join(dir, "lookup.djfl"), []byte("lookup"), 0644)

	err := ZipInside(dir, true)
	if err != nil {
		t.Fatalf("ZipInside failed: %v", err)
	}

	archivePath := filepath.Join(dir, "files.djfz")
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		t.Fatalf("Failed to open archive: %v", err)
	}
	defer r.Close()

	for _, f := range r.File {
		ext := filepath.Ext(f.Name)
		if ext == ".djfz" || ext == ".djfl" {
			t.Errorf("Archive should not contain %s files, found: %s", ext, f.Name)
		}
	}
}

func TestCompressDirectoryToDest(t *testing.T) {
	// Create source directory with files
	srcDir := t.TempDir()
	os.WriteFile(filepath.Join(srcDir, "file1.txt"), []byte("content1"), 0644)
	os.WriteFile(filepath.Join(srcDir, "file2.txt"), []byte("content2"), 0644)

	// Create destination path
	destDir := t.TempDir()
	destPath := filepath.Join(destDir, "output.zip")

	err := CompressDirectoryToDest(srcDir, destPath)
	if err != nil {
		t.Fatalf("CompressDirectoryToDest failed: %v", err)
	}

	// Verify archive exists and contains correct files
	r, err := zip.OpenReader(destPath)
	if err != nil {
		t.Fatalf("Failed to open archive: %v", err)
	}
	defer r.Close()

	if len(r.File) != 2 {
		t.Errorf("Expected 2 files in archive, got %d", len(r.File))
	}

	foundFiles := make(map[string]bool)
	for _, f := range r.File {
		foundFiles[f.Name] = true
	}

	if !foundFiles["file1.txt"] || !foundFiles["file2.txt"] {
		t.Errorf("Missing expected files in archive: %v", foundFiles)
	}
}

func TestCompressDirectoryToDest_FileNotDir(t *testing.T) {
	// Create a file instead of directory
	tmpFile := filepath.Join(t.TempDir(), "notadir.txt")
	os.WriteFile(tmpFile, []byte("content"), 0644)

	destPath := filepath.Join(t.TempDir(), "output.zip")

	err := CompressDirectoryToDest(tmpFile, destPath)
	if err != ErrExpectedDirectory {
		t.Errorf("Expected ErrExpectedDirectory, got: %v", err)
	}
}

func TestAddFileToZip_ProperCleanup(t *testing.T) {
	// This test verifies that file handles are properly closed
	// by creating many files and ensuring we don't run out of handles

	srcDir := t.TempDir()
	destDir := t.TempDir()

	// Create many files
	numFiles := 100
	for i := range numFiles {
		path := filepath.Join(srcDir, filepath.Base(srcDir), string(rune('a'+i%26))+".txt")
		os.MkdirAll(filepath.Dir(path), 0755)
		os.WriteFile(path, []byte("content"), 0644)
	}

	destPath := filepath.Join(destDir, "many_files.zip")
	err := CompressDirectoryToDest(srcDir, destPath)
	if err != nil {
		t.Fatalf("CompressDirectoryToDest failed with many files: %v", err)
	}
}

func TestLookupFromDJFZ_ExtensionCheck(t *testing.T) {
	// Test that LookupFromDJFZ properly validates extension
	_, err := LookupFromDJFZ("notadjfz.zip")
	if err != ErrNotDJFZExtension {
		t.Errorf("Expected ErrNotDJFZExtension for wrong extension, got: %v", err)
	}

	_, err = LookupFromDJFZ("missing_dot_djfz")
	if err != ErrNotDJFZExtension {
		t.Errorf("Expected ErrNotDJFZExtension for missing dot, got: %v", err)
	}
}
