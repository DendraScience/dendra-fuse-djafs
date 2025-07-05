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

	if flag.NArg() != 2 {
		help()
		os.Exit(2)
	}
	storagePath := flag.Arg(0)
	mountpoint := flag.Arg(1)

	// Ensure storage directory exists
	if err := os.MkdirAll(storagePath, 0755); err != nil {
		log.Fatalf("Failed to create storage directory: %v", err)
	}

	// Create filesystem instance
	filesystem := djafs.NewFS(storagePath)

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
		log.Println("Received interrupt signal, shutting down...")
		
		// Stop filesystem gracefully
		filesystem.Stop()
		
		// Unmount filesystem
		fuse.Unmount(mountpoint)
		c.Close()
		
		log.Println("Shutdown complete")
		os.Exit(0)
	}()

	log.Printf("djafs mounted at %s (storage: %s)", mountpoint, storagePath)
	err = fs.Serve(c, filesystem)
	if err != nil {
		log.Fatal(err)
	}
}

func help() {
	fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "  %s STORAGE_PATH MOUNTPOINT\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "\nArguments:\n")
	fmt.Fprintf(os.Stderr, "  STORAGE_PATH   Path to djafs storage directory\n")
	fmt.Fprintf(os.Stderr, "  MOUNTPOINT     Directory to mount the filesystem\n")
	flag.PrintDefaults()
}
