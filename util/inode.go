package util

import (
	"sync"
)

var (
	highestInode uint64 = 0
	// could use atomic package for better performance, but this is simpler
	inodeLock = sync.Mutex{}
)

func GetNewInode() uint64 {
	inodeLock.Lock()
	defer inodeLock.Unlock()
	highestInode++
	return highestInode
}

func Set(inode uint64) {
	go func(i uint64) {
		inodeLock.Lock()
		defer inodeLock.Unlock()
		if i > highestInode {
			highestInode = i
		}
	}(inode)
}

func FileNameFromInode(inode uint64) string {
	return ""
}
