package util

import (
	"sync"
	"sync/atomic"
)

var (
	highestInode  atomic.Uint64
	inodeRegistry sync.Map // map[uint64]string - inode -> filename
)

// GetNewInode returns a new unique inode number in a thread-safe manner.
// It increments the global inode counter and returns the new value.
func GetNewInode() uint64 {
	return highestInode.Add(1)
}

// GetNewInodeFor returns a new unique inode number and registers it with the given filename.
// This allows later lookup of filename by inode.
func GetNewInodeFor(filename string) uint64 {
	inode := highestInode.Add(1)
	inodeRegistry.Store(inode, filename)
	return inode
}

// SetInode updates the global inode counter if the provided inode is higher than the current maximum.
// This ensures that newly generated inodes won't conflict with existing ones.
func SetInode(inode uint64) {
	for {
		current := highestInode.Load()
		if inode <= current {
			return
		}
		if highestInode.CompareAndSwap(current, inode) {
			return
		}
	}
}

// RegisterInode registers an inode with its associated filename for later lookup.
func RegisterInode(inode uint64, filename string) {
	inodeRegistry.Store(inode, filename)
	SetInode(inode)
}

// UnregisterInode removes an inode from the registry.
func UnregisterInode(inode uint64) {
	inodeRegistry.Delete(inode)
}

// FileNameFromInode returns the filename associated with the given inode number.
// Returns the filename and nil error if found, or empty string and ErrInodeNotFound if not.
func FileNameFromInode(inode uint64) (string, error) {
	if filename, ok := inodeRegistry.Load(inode); ok {
		return filename.(string), nil
	}
	return "", ErrInodeNotFound
}

// ClearInodeRegistry clears all entries from the inode registry.
// This is primarily useful for testing.
func ClearInodeRegistry() {
	inodeRegistry.Range(func(key, _ any) bool {
		inodeRegistry.Delete(key)
		return true
	})
}

// GetInodeRegistrySize returns the number of entries in the inode registry.
func GetInodeRegistrySize() int {
	count := 0
	inodeRegistry.Range(func(_, _ any) bool {
		count++
		return true
	})
	return count
}
