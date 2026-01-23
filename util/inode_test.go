package util

import (
	"sync"
	"testing"
)

func TestGetNewInode_Increments(t *testing.T) {
	// Get a baseline
	first := GetNewInode()
	second := GetNewInode()
	third := GetNewInode()

	if second != first+1 {
		t.Errorf("Second inode should be first+1: got %d, want %d", second, first+1)
	}
	if third != second+1 {
		t.Errorf("Third inode should be second+1: got %d, want %d", third, second+1)
	}
}

func TestGetNewInode_Concurrent(t *testing.T) {
	// Test that concurrent calls return unique inodes
	numGoroutines := 100
	inodesPerGoroutine := 100

	results := make(chan uint64, numGoroutines*inodesPerGoroutine)

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for range numGoroutines {
		go func() {
			defer wg.Done()
			for range inodesPerGoroutine {
				results <- GetNewInode()
			}
		}()
	}

	wg.Wait()
	close(results)

	// Collect all inodes and check for duplicates
	seen := make(map[uint64]bool)
	for inode := range results {
		if seen[inode] {
			t.Errorf("Duplicate inode found: %d", inode)
		}
		seen[inode] = true
	}

	expectedCount := numGoroutines * inodesPerGoroutine
	if len(seen) != expectedCount {
		t.Errorf("Expected %d unique inodes, got %d", expectedCount, len(seen))
	}
}

func TestSetInode_UpdatesMaximum(t *testing.T) {
	// Get current max
	current := GetNewInode()

	// Set to a higher value
	higher := current + 1000
	SetInode(higher)

	// Next inode should be higher+1
	next := GetNewInode()
	if next != higher+1 {
		t.Errorf("After SetInode(%d), GetNewInode should return %d, got %d", higher, higher+1, next)
	}
}

func TestSetInode_IgnoresLowerValue(t *testing.T) {
	// Get current value
	current := GetNewInode()

	// Try to set to a lower value
	SetInode(1)

	// Next inode should still be current+1, not 2
	next := GetNewInode()
	if next <= current {
		t.Errorf("SetInode should ignore lower values: after SetInode(1), got %d which is <= %d", next, current)
	}
}

func TestSetInode_Concurrent(t *testing.T) {
	// Test that concurrent SetInode calls work correctly

	// Get baseline
	baseline := GetNewInode()

	var wg sync.WaitGroup
	numGoroutines := 50

	// All try to set to different high values
	for i := range numGoroutines {
		wg.Add(1)
		go func(val uint64) {
			defer wg.Done()
			SetInode(val)
		}(baseline + uint64(i)*1000)
	}

	wg.Wait()

	// The highest value set was baseline + 49*1000
	expectedMin := baseline + uint64(numGoroutines-1)*1000

	next := GetNewInode()
	if next <= expectedMin {
		t.Errorf("After concurrent SetInode calls, next inode should be > %d, got %d", expectedMin, next)
	}
}

func TestSetInode_Synchronous(t *testing.T) {
	// Verify that SetInode is now synchronous (not async)
	// by checking that the value is immediately available

	current := GetNewInode()
	newMax := current + 5000

	SetInode(newMax)

	// Should be immediately effective - no race condition
	next := GetNewInode()
	if next != newMax+1 {
		t.Errorf("SetInode should be synchronous: expected %d, got %d", newMax+1, next)
	}
}

func TestFileNameFromInode_NotFound(t *testing.T) {
	ClearInodeRegistry()

	// Unregistered inode should return error
	_, err := FileNameFromInode(12345)
	if err != ErrInodeNotFound {
		t.Errorf("FileNameFromInode should return ErrInodeNotFound for unregistered inode, got %v", err)
	}
}

func TestGetNewInodeFor_RegistersFilename(t *testing.T) {
	ClearInodeRegistry()

	filename := "/path/to/testfile.txt"
	inode := GetNewInodeFor(filename)

	got, err := FileNameFromInode(inode)
	if err != nil {
		t.Fatalf("FileNameFromInode returned error: %v", err)
	}
	if got != filename {
		t.Errorf("FileNameFromInode = %q, want %q", got, filename)
	}
}

func TestRegisterInode(t *testing.T) {
	ClearInodeRegistry()

	inode := uint64(99999)
	filename := "/test/registered.txt"

	RegisterInode(inode, filename)

	got, err := FileNameFromInode(inode)
	if err != nil {
		t.Fatalf("FileNameFromInode returned error: %v", err)
	}
	if got != filename {
		t.Errorf("FileNameFromInode = %q, want %q", got, filename)
	}
}

func TestUnregisterInode(t *testing.T) {
	ClearInodeRegistry()

	inode := GetNewInodeFor("/test/file.txt")

	// Should exist
	_, err := FileNameFromInode(inode)
	if err != nil {
		t.Fatalf("Inode should be registered")
	}

	// Unregister
	UnregisterInode(inode)

	// Should no longer exist
	_, err = FileNameFromInode(inode)
	if err != ErrInodeNotFound {
		t.Errorf("Inode should be unregistered, got err=%v", err)
	}
}

func TestGetInodeRegistrySize(t *testing.T) {
	ClearInodeRegistry()

	if GetInodeRegistrySize() != 0 {
		t.Error("Empty registry should have size 0")
	}

	GetNewInodeFor("file1.txt")
	GetNewInodeFor("file2.txt")
	GetNewInodeFor("file3.txt")

	if GetInodeRegistrySize() != 3 {
		t.Errorf("Registry size = %d, want 3", GetInodeRegistrySize())
	}
}

func TestInodeRegistry_Concurrent(t *testing.T) {
	ClearInodeRegistry()

	var wg sync.WaitGroup
	numGoroutines := 100

	wg.Add(numGoroutines)
	for i := range numGoroutines {
		go func(idx int) {
			defer wg.Done()
			GetNewInodeFor("file" + string(rune('A'+idx%26)) + ".txt")
		}(i)
	}

	wg.Wait()

	if GetInodeRegistrySize() != numGoroutines {
		t.Errorf("Registry size = %d, want %d", GetInodeRegistrySize(), numGoroutines)
	}
}
