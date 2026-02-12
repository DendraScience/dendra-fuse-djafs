package djafs

import (
	"context"
	"testing"
	"time"

	"bazil.org/fuse"
)

// TestSetattrDeadlockFix verifies that Setattr doesn't deadlock
func TestSetattrDeadlockFix(t *testing.T) {
	// Create a file with some data
	f := &File{
		entry: &util.LookupEntry{
			Name:     "test.json",
			FileSize: 100,
			Inode:    12345,
			Modified: time.Now(),
		},
		data:     []byte("test data"),
		modified: time.Now(),
		isNew:    true,
	}

	// Test Setattr call - this should not deadlock
	ctx := context.Background()
	req := &fuse.SetattrRequest{
		Valid: fuse.SetattrMtime,
		Mtime: time.Now(),
	}
	resp := &fuse.SetattrResponse{}

	// This should complete without deadlock
	done := make(chan error, 1)
	go func() {
		done <- f.Setattr(ctx, req, resp)
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Setattr failed: %v", err)
		}
		// Success - no deadlock
	case <-time.After(2 * time.Second):
		t.Fatal("Setattr deadlocked - test timed out")
	}
}

// TestSetattrFunctionality verifies Setattr works correctly
func TestSetattrFunctionality(t *testing.T) {
	f := &File{
		entry: &util.LookupEntry{
			Name:     "test.json",
			FileSize: 10,
			Inode:    12345,
			Modified: time.Now(),
		},
		data:     []byte("test"),
		modified: time.Now(),
		isNew:    true,
	}

	ctx := context.Background()
	newTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	
	// Test mtime update
	req := &fuse.SetattrRequest{
		Valid: fuse.SetattrMtime,
		Mtime: newTime,
	}
	resp := &fuse.SetattrResponse{}

	err := f.Setattr(ctx, req, resp)
	if err != nil {
		t.Fatalf("Setattr failed: %v", err)
	}

	if f.modified != newTime {
		t.Errorf("Expected modified time %v, got %v", newTime, f.modified)
	}

	// Test size update (truncation)
	req = &fuse.SetattrRequest{
		Valid: fuse.SetattrSize,
		Size:  2,
	}

	err = f.Setattr(ctx, req, resp)
	if err != nil {
		t.Fatalf("Setattr size failed: %v", err)
	}

	if len(f.data) != 2 {
		t.Errorf("Expected data length 2, got %d", len(f.data))
	}

	if string(f.data) != "te" {
		t.Errorf("Expected data 'te', got '%s'", string(f.data))
	}
}