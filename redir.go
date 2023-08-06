package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
)

var (
	mountPoint string
	dataDir    string
	hotCache   string
	workSpace  string
	workDir    string
	Blocksize  uint32
)

type FS struct{}

func (FS) Root() (fs.Node, error) {
	return Dir{Path: workDir, Inode: 1, Mode: os.ModeDir | 0o555}, nil
}

type Dir struct {
	Path   string
	Blocks int64
	Name   string
	Inode  uint64
	Mode   os.FileMode
	Size   uint64
	UID    uint32
	GID    uint32
	Expiry time.Time
}

// Attr sets the attributes for a given directory.
// and lists the contents of the directory

func (d Dir) Attr(ctx context.Context, a *fuse.Attr) error {
	a.Inode = d.Inode
	a.Mode = d.Mode
	a.Size = d.Size
	// get the uid and gid of the current user
	a.Uid = d.UID
	a.Gid = d.GID
	a.BlockSize = Blocksize
	a.Blocks = uint64(d.Blocks)
	a.Valid = time.Until(d.Expiry)
	return nil
}

func init() {
	var stat syscall.Statfs_t
	syscall.Statfs(workDir, &stat)
	Blocksize = uint32(stat.Bsize)
	if Blocksize == 0 {
		Blocksize = 4096
	}
}

func (d Dir) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	dPath := filepath.Clean(d.Path)
	dirRead, err := os.ReadDir(dPath)
	if err != nil {
		return nil, err
	}
	self := fuse.Dirent{
		Inode: d.Inode,
		Name:  ".",
		Type:  fuse.DT_Dir,
	}
	parentFolder := filepath.Dir(dPath)
	stat, err := os.Stat(parentFolder)
	if err != nil {
		return nil, err
	}
	parentInode := stat.Sys().(*syscall.Stat_t).Ino
	parent := fuse.Dirent{
		Inode: parentInode,
		Name:  "..",
		Type:  fuse.DT_Dir,
	}
	dirents := []fuse.Dirent{self, parent}
	for _, f := range dirRead {
		log.Println(f)
		info, err := f.Info()
		if err != nil {
			return nil, err
		}
		typeFlag := fuse.DT_File
		if info.IsDir() {
			typeFlag = fuse.DT_Dir
		}

		inode := info.Sys().(*syscall.Stat_t).Ino
		dirents = append(dirents, fuse.Dirent{
			Inode: inode,
			Name:  f.Name(),
			Type:  typeFlag,
		})
		log.Println(dirents)
	}
	return dirents, nil
}

func (d Dir) Create(ctx context.Context, req *fuse.CreateRequest, resp *fuse.CreateResponse) (fs.Node, fs.Handle, error) {
	fmt.Println(req.Name)

	return nil, nil, nil
}

func (d Dir) Read(ctx context.Context, req *fuse.ReadRequest, resp *fuse.ReadResponse) error {
	return nil
}

func (d Dir) Mkdir(ctx context.Context, req *fuse.MkdirRequest) (fs.Node, error) {
	path := filepath.Join(d.Path, req.Name)
	err := os.Mkdir(path, os.ModeDir|0o755)
	if err != nil {
		return nil, err
	}
	f := Dir{
		Path:  filepath.Join(d.Path, req.Name),
		Name:  req.Name,
		Inode: 42, // replaced with real inode below
		Mode:  os.ModeDir | 0o555,
	}
	stat, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	f.Inode = stat.Sys().(*syscall.Stat_t).Ino
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
	Name      string
	Expiry    time.Time
	Blocks    int64
	Blocksize uint32
	Inode     uint64
	Mode      os.FileMode
	Size      uint64
	UID       uint32
	GID       uint32
}

func (f File) Attr(ctx context.Context, a *fuse.Attr) error {
	a.Inode = f.Inode
	a.Mode = f.Mode
	a.Size = f.Size
	a.Uid = f.UID
	a.Gid = f.GID
	a.BlockSize = f.Blocksize
	a.Blocks = uint64(f.Blocks)
	a.Valid = time.Until(f.Expiry)
	return nil
}

func (f File) ReadAll(ctx context.Context) ([]byte, error) {
	return []byte("hello world"), nil
}

func (d Dir) Lookup(ctx context.Context, name string) (fs.Node, error) {
	log.Println("lookup", name)
	if name == "hello" {
		return File{}, nil
	}
	osPath := filepath.Join(d.Path, name)
	stat, err := os.Stat(osPath)
	if os.IsNotExist(err) {
		return nil, fuse.ENOENT
	}
	if err != nil {
		return nil, err
	}
	var node fs.Node
	if stat.IsDir() {
		node = Dir{
			Path:   osPath,
			Name:   name,
			Inode:  stat.Sys().(*syscall.Stat_t).Ino,
			Blocks: stat.Sys().(*syscall.Stat_t).Blocks,
			Mode:   stat.Mode(),
		}
	} else {
		node = File{
			Name:      name,
			Inode:     stat.Sys().(*syscall.Stat_t).Ino,
			Mode:      stat.Mode(),
			Expiry:    time.Now().Add(1 * time.Minute),
			Blocks:    stat.Sys().(*syscall.Stat_t).Blocks,
			Blocksize: Blocksize,
			Size:      uint64(stat.Size()),
		}
	}
	return node, nil
}

func main() {
	flag.Parse()
	mountPoint = flag.Arg(0)
	workDir = flag.Arg(1)

	mountPoint = filepath.Clean(mountPoint)
	workDir = filepath.Clean(workDir)

	os.MkdirAll(workDir, 0o755)

	_, err := os.Stat(mountPoint)
	if err != nil {
		log.Fatal(err)
	}

	workSpace = filepath.Join(workDir, ".workspace")
	dataDir = filepath.Join(workDir, ".data")
	hotCache = filepath.Join(workDir, ".cache")

	c, err := fuse.Mount(
		mountPoint,
		fuse.FSName("helloworld"),
		fuse.Subtype("hellofs"),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()
	_, err = os.Stat(workDir)
	if err != nil {
		log.Fatal(err)
	}
	err = fs.Serve(c, FS{})
	if err != nil {
		log.Fatal(err)
	}
}
