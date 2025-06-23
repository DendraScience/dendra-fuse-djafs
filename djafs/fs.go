package djafs

import (
	"context"
	"path/filepath"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
)

type FS struct {
	RootPath string
}
type Node struct{}

func (n Node) Attr(ctx context.Context, a *fuse.Attr) error {
	return nil
}

func (fs FS) Root() (fs.Node, error) {
	return Node{}, nil
}

func NewFS(path string) FS {
	if filepath.Ext(path) != ".djfz" {
		return FS{}
	}
	return FS{}
}
