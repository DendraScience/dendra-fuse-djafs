// Package util provides utility functions for the djafs filesystem.
package util

import "errors"

// Sentinel errors for package util.
// These errors can be checked with errors.Is() for specific error handling.
var (
	// File and directory errors
	ErrExpectedFile      = errors.New("expected file, got directory")
	ErrExpectedDirectory = errors.New("expected directory but got file")
	ErrUnexpectedSymlink = errors.New("expected file, got symlink")

	// Hash path errors
	ErrInvalidHashPath = errors.New("invalid hash path format")

	// Archive errors
	ErrNotDJFZExtension = errors.New("file path extension is not '.djfz'")

	// Inode errors
	ErrInodeNotFound = errors.New("inode not found in registry")

	// Lookup table errors
	ErrIndexOutOfRange = errors.New("index out of range")
)
