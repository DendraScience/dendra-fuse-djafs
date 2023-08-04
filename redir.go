package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
)

var (
	mountPoint string
	dataDir    string
	hotCache   string
	workSpace  string
)

type FS struct{}

func (FS) Root() (fs.Node, error) {
	return Dir{Path: mountPoint}, nil
}

type Dir struct {
	Path  string
	Name  string
	Inode uint64
	Mode  os.FileMode
}

// Attr sets the attributes for a given directory.
// and lists the contents of the directory

func (d Dir) Attr(ctx context.Context, a *fuse.Attr) error {
	a.Inode = 1
	a.Mode = os.ModeDir | 0o555
	a.Size = 4096
	// get the uid and gid of the current user
	a.Uid = 0
	a.Gid = 0
	return nil
}

func (d Dir) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	return []fuse.Dirent{
		{Inode: 2, Name: "hello", Type: fuse.DT_File},
	}, nil
}

func (d Dir) Create(ctx context.Context, req *fuse.CreateRequest, resp *fuse.CreateResponse) (fs.Node, fs.Handle, error) {
	fmt.Println(req.Name)

	return nil, nil, nil
}

func (d Dir) Read(ctx context.Context, req *fuse.ReadRequest, resp *fuse.ReadResponse) error {
	return nil
}

func (d Dir) Mkdir(ctx context.Context, req *fuse.MkdirRequest) (fs.Node, error) {
	err := os.Mkdir(filepath.Join(d.Path, req.Name), os.ModeDir|0o755)
	if err != nil {
		return nil, err
	}
	f := Dir{
		Path:  filepath.Join(d.Path, req.Name),
		Name:  req.Name,
		Inode: 7,
		Mode:  os.ModeDir | 0o555,
	}
	return f, nil
}

func (d Dir) Remove(ctx context.Context, req *fuse.RemoveRequest) error {
	return nil
}

func (d Dir) Rename(ctx context.Context, req *fuse.RenameRequest, newDir fs.Node) error {
	return nil
}

func (d Dir) Write(ctx context.Context, req *fuse.WriteRequest, resp *fuse.WriteResponse) error {
	return nil
}

type File struct {
	Name  string
	Inode uint64
	Mode  os.FileMode
	Size  uint64
}

func (f File) Attr(ctx context.Context, a *fuse.Attr) error {
	a.Inode = f.Inode
	a.Mode = f.Mode
	a.Size = f.Size
	return nil
}

func (f File) ReadAll(ctx context.Context) ([]byte, error) {
	return []byte("hello world"), nil
}

func (d Dir) Lookup(ctx context.Context, name string) (fs.Node, error) {
	if name == "hello" {
		return File{}, nil
	}
	return nil, fuse.ENOENT
}

func main() {
	flag.Parse()
	mountPoint = flag.Arg(0)

	c, err := fuse.Mount(
		mountPoint,
		fuse.FSName("helloworld"),
		fuse.Subtype("hellofs"),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()

	err = fs.Serve(c, FS{})
	if err != nil {
		log.Fatal(err)
	}
}
