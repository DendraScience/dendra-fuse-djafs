package util

import (
	"sync"
)

var (
	highestInode uint64 = 0
	// could use atomic package for better performance, but this is simpler
	inodeLock = sync.Mutex{}
)

// GetNewInode returns a new unique inode number in a thread-safe manner.
// It increments the global inode counter and returns the new value.
func GetNewInode() uint64 {
	inodeLock.Lock()
	defer inodeLock.Unlock()
	highestInode++
	return highestInode
}

// Set updates the global inode counter if the provided inode is higher than the current maximum.
// This function runs asynchronously to avoid blocking the caller.
func Set(inode uint64) {
	go func(i uint64) {
		inodeLock.Lock()
		defer inodeLock.Unlock()
		if i > highestInode {
			highestInode = i
		}
	}(inode)
}

// FileNameFromInode returns the filename associated with the given inode number.
// Currently returns an empty string as this requires a global inode-to-filename mapping
// that is not yet implemented.
func FileNameFromInode(inode uint64) string {
	// This function would need a global inode-to-filename mapping
	// For now, return empty string as this requires a more complex implementation
	// involving a global registry of all files and their inodes
	return ""
}
