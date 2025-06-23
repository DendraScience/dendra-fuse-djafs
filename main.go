package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime/debug"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	_ "bazil.org/fuse/fs/fstestutil"
	"github.com/dendrascience/dendra-archive-fuse/djafs"
)

const (
	SemVer  string = "1.0.0"
	Package string = "dendra_archive_fuse"
)

var (
	hostname string
	version  = flag.Bool("version", false, "Get version string and exit")
)

func init() {
	flag.Parse()
	GitCommit := commit()
	if GitCommit == "" {
		log.Fatalf("Binary built improperly. Version variables not set!")
	}
	fmt.Printf("%s build commit: %s\n", Package, GitCommit)

	if *version {
		os.Exit(0)
	} else {
		fmt.Printf("Initialization success...\n")
	}
}

func commit() string {
	Commit := func() string {
		if info, ok := debug.ReadBuildInfo(); ok {
			for _, setting := range info.Settings {
				if setting.Key == "vcs.revision" {
					return setting.Value
				}
			}
		}
		return ""
	}()
	return Commit
}

func main() {
	flag.Usage = help
	flag.Parse()

	if flag.NArg() != 1 {
		help()
		os.Exit(2)
	}
	mountpoint := flag.Arg(0)

	c, err := fuse.Mount(
		mountpoint,
		fuse.FSName("djafs"),
		fuse.Subtype("djafs"),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)
	go func() {
		<-sigChan
		// here, close out the backing files and stop all garbage collection routines
		fuse.Unmount(mountpoint)
		c.Close()
		os.Exit(1)
	}()

	err = fs.Serve(c, djafs.NewFS(mountpoint))
	if err != nil {
		log.Fatal(err)
	}
}

func help() {
	fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "  %s MOUNTPOINT\n", os.Args[0])
	flag.PrintDefaults()
}
