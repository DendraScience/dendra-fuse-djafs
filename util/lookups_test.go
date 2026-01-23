package util

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLookupTable_Sort_PersistsState(t *testing.T) {
	// This test verifies that Sort() with pointer receiver persists the sorted flag
	lt := &LookupTable{}

	// Add entries in reverse chronological order
	lt.Add(LookupEntry{Name: "file3", Modified: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)})
	lt.Add(LookupEntry{Name: "file1", Modified: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)})
	lt.Add(LookupEntry{Name: "file2", Modified: time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)})

	// Should not be sorted initially
	if lt.sorted {
		t.Error("LookupTable should not be marked sorted after Add")
	}

	// Sort
	lt.Sort()

	// Should now be marked sorted
	if !lt.sorted {
		t.Error("LookupTable should be marked sorted after Sort()")
	}

	// Verify order is chronological
	if lt.Get(0).Name != "file1" {
		t.Errorf("First entry should be file1, got %s", lt.Get(0).Name)
	}
	if lt.Get(1).Name != "file2" {
		t.Errorf("Second entry should be file2, got %s", lt.Get(1).Name)
	}
	if lt.Get(2).Name != "file3" {
		t.Errorf("Third entry should be file3, got %s", lt.Get(2).Name)
	}
}

func TestLookupTable_Sort_ValueReceiverWouldFail(t *testing.T) {
	// This test demonstrates why we need pointer receivers
	// If Sort used value receiver, this would fail

	lt := LookupTable{}
	lt.Add(LookupEntry{Name: "b", Modified: time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)})
	lt.Add(LookupEntry{Name: "a", Modified: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)})

	// Sort via pointer
	(&lt).Sort()

	// Check sorted flag persisted
	if !lt.sorted {
		t.Error("Sort should persist sorted flag through pointer receiver")
	}
}

func TestLookupTable_GetActiveFileCount(t *testing.T) {
	tests := []struct {
		name    string
		entries []LookupEntry
		want    int
	}{
		{
			name:    "empty table",
			entries: nil,
			want:    0,
		},
		{
			name: "all active",
			entries: []LookupEntry{
				{Name: "file1", Target: "hash1"},
				{Name: "file2", Target: "hash2"},
				{Name: "file3", Target: "hash3"},
			},
			want: 3,
		},
		{
			name: "one deleted",
			entries: []LookupEntry{
				{Name: "file1", Target: "hash1"},
				{Name: "file2", Target: ""}, // deleted
				{Name: "file3", Target: "hash3"},
			},
			want: 2,
		},
		{
			name: "all deleted",
			entries: []LookupEntry{
				{Name: "file1", Target: ""},
				{Name: "file2", Target: ""},
			},
			want: 0,
		},
		{
			name: "file updated then deleted",
			entries: []LookupEntry{
				{Name: "file1", Target: "hash1", Modified: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
				{Name: "file1", Target: "hash2", Modified: time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)},
				{Name: "file1", Target: "", Modified: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)}, // deleted
			},
			want: 0, // Latest state is deleted
		},
		{
			name: "file deleted then recreated",
			entries: []LookupEntry{
				{Name: "file1", Target: "hash1", Modified: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
				{Name: "file1", Target: "", Modified: time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)},     // deleted
				{Name: "file1", Target: "hash3", Modified: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)}, // recreated
			},
			want: 1, // Latest state is active
		},
		{
			name: "multiple files with history",
			entries: []LookupEntry{
				{Name: "file1", Target: "hash1"},
				{Name: "file1", Target: "hash2"}, // update
				{Name: "file2", Target: "hash3"},
				{Name: "file2", Target: ""},      // delete
				{Name: "file3", Target: "hash4"},
			},
			want: 2, // file1 and file3 active, file2 deleted
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lt := &LookupTable{}
			for _, e := range tt.entries {
				lt.Add(e)
			}

			got := lt.GetActiveFileCount()
			if got != tt.want {
				t.Errorf("GetActiveFileCount() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestLookupTable_GetOldestFileTS(t *testing.T) {
	lt := &LookupTable{}

	oldest := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	middle := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	newest := time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC)

	// Add in random order
	lt.Add(LookupEntry{Name: "middle", Modified: middle})
	lt.Add(LookupEntry{Name: "newest", Modified: newest})
	lt.Add(LookupEntry{Name: "oldest", Modified: oldest})

	got := lt.GetOldestFileTS()
	if !got.Equal(oldest) {
		t.Errorf("GetOldestFileTS() = %v, want %v", got, oldest)
	}

	// Verify table is now sorted
	if !lt.sorted {
		t.Error("GetOldestFileTS should sort the table")
	}
}

func TestLookupTable_GetNewestFileTS(t *testing.T) {
	lt := &LookupTable{}

	oldest := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	newest := time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC)

	lt.Add(LookupEntry{Name: "oldest", Modified: oldest})
	lt.Add(LookupEntry{Name: "newest", Modified: newest})

	got := lt.GetNewestFileTS()
	if !got.Equal(newest) {
		t.Errorf("GetNewestFileTS() = %v, want %v", got, newest)
	}
}

func TestLookupTable_JSONRoundTrip(t *testing.T) {
	original := &LookupTable{}
	original.Add(LookupEntry{
		Name:     "test.txt",
		Target:   "742-00000-abc123",
		FileSize: 100,
		Inode:    42,
		Modified: time.Date(2024, 6, 15, 10, 30, 0, 0, time.UTC),
	})
	original.Sort()

	// Marshal
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	// Unmarshal
	var restored LookupTable
	err = json.Unmarshal(data, &restored)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	// Verify
	if restored.Len() != original.Len() {
		t.Errorf("Length mismatch: got %d, want %d", restored.Len(), original.Len())
	}

	if restored.sorted != original.sorted {
		t.Errorf("Sorted flag mismatch: got %v, want %v", restored.sorted, original.sorted)
	}

	entry := restored.Get(0)
	if entry.Name != "test.txt" {
		t.Errorf("Name mismatch: got %q", entry.Name)
	}
	if entry.Target != "742-00000-abc123" {
		t.Errorf("Target mismatch: got %q", entry.Target)
	}
}

func TestCreateFileLookupEntry_TargetFormat(t *testing.T) {
	// Create a test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.json")
	os.WriteFile(testFile, []byte(`{"key": "value"}`), 0644)

	workDir := filepath.Join(tmpDir, ".work")

	entry, err := CreateFileLookupEntry(testFile, workDir, true)
	if err != nil {
		t.Fatalf("CreateFileLookupEntry failed: %v", err)
	}

	// Target should be in hash path format: "bucket-subbucket-hash"
	// e.g., "742-00000-abc123def456..."
	if len(entry.Target) == 0 {
		t.Error("Target should not be empty")
	}

	// Should contain two hyphens (format: X-XXXXX-hash)
	hyphenCount := 0
	for _, c := range entry.Target {
		if c == '-' {
			hyphenCount++
		}
	}
	if hyphenCount != 2 {
		t.Errorf("Target should have format 'bucket-subbucket-hash', got: %q", entry.Target)
	}

	// Name should be the original file path
	if entry.Name != testFile {
		t.Errorf("Name should be %q, got %q", testFile, entry.Name)
	}

	// Should have size
	if entry.FileSize != 16 { // len(`{"key": "value"}`)
		t.Errorf("FileSize should be 16, got %d", entry.FileSize)
	}
}

func TestCreateFileLookupEntry_RejectsSymlink(t *testing.T) {
	tmpDir := t.TempDir()
	realFile := filepath.Join(tmpDir, "real.txt")
	os.WriteFile(realFile, []byte("content"), 0644)

	linkFile := filepath.Join(tmpDir, "link.txt")
	err := os.Symlink(realFile, linkFile)
	if err != nil {
		t.Skip("Symlinks not supported on this system")
	}

	workDir := filepath.Join(tmpDir, ".work")

	_, err = CreateFileLookupEntry(linkFile, workDir, true)
	if err == nil {
		t.Error("CreateFileLookupEntry should reject symlinks")
	}
}

func TestCreateFileLookupEntry_RejectsDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	workDir := filepath.Join(tmpDir, ".work")

	_, err := CreateFileLookupEntry(tmpDir, workDir, true)
	if err != ErrExpectedFile {
		t.Errorf("Expected ErrExpectedFile, got: %v", err)
	}
}

func TestLookupTable_Collapse(t *testing.T) {
	lt := &LookupTable{}

	// Add multiple versions of same file
	lt.Add(LookupEntry{Name: "file1", Target: "v1", Modified: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)})
	lt.Add(LookupEntry{Name: "file1", Target: "v2", Modified: time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)})
	lt.Add(LookupEntry{Name: "file1", Target: "v3", Modified: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)})
	lt.Add(LookupEntry{Name: "file2", Target: "v1", Modified: time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)})

	if lt.Len() != 4 {
		t.Fatalf("Should have 4 entries before collapse, got %d", lt.Len())
	}

	lt.Collapse()

	// Should have 2 entries after collapse (latest version of each file)
	if lt.Len() != 2 {
		t.Errorf("Should have 2 entries after collapse, got %d", lt.Len())
	}

	// Find file1 entry
	var file1Entry LookupEntry
	for e := range lt.Iterate {
		if e.Name == "file1" {
			file1Entry = e
			break
		}
	}

	if file1Entry.Target != "v3" {
		t.Errorf("file1 should have latest target 'v3', got %q", file1Entry.Target)
	}
}

func TestLookupTable_Iterate(t *testing.T) {
	lt := &LookupTable{}
	lt.Add(LookupEntry{Name: "a"})
	lt.Add(LookupEntry{Name: "b"})
	lt.Add(LookupEntry{Name: "c"})

	var names []string
	for e := range lt.Iterate {
		names = append(names, e.Name)
	}

	if len(names) != 3 {
		t.Errorf("Should iterate over 3 entries, got %d", len(names))
	}
}

func TestLookupTable_Remove(t *testing.T) {
	lt := &LookupTable{}
	lt.Add(LookupEntry{Name: "a"})
	lt.Add(LookupEntry{Name: "b"})
	lt.Add(LookupEntry{Name: "c"})

	err := lt.Remove(1)
	if err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	if lt.Len() != 2 {
		t.Errorf("Should have 2 entries after remove, got %d", lt.Len())
	}

	// Check remaining entries
	if lt.Get(0).Name != "a" || lt.Get(1).Name != "c" {
		t.Error("Wrong entries remaining after remove")
	}
}

func TestLookupTable_Remove_OutOfRange(t *testing.T) {
	lt := &LookupTable{}
	lt.Add(LookupEntry{Name: "a"})

	err := lt.Remove(5)
	if err == nil {
		t.Error("Remove should fail for out of range index")
	}

	err = lt.Remove(-1)
	if err == nil {
		t.Error("Remove should fail for negative index")
	}
}
