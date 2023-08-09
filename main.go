package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"

	"bazil.org/fuse"
	_ "bazil.org/fuse/fs/fstestutil"
)

const (
	SemVer  string = "1.0.0"
	Package string = "dendra_archive_fuse"
)

var (
	Authors   string
	BuildNo   string
	BuildTime string
	GitCommit string
	Tag       string

	hostname string
	version  = flag.Bool("version", false, "Get detailed version string and exit")
)

func init() {
	flag.Parse()

	Authors = strings.ReplaceAll(Authors, "SpAcE", " ")
	Tag = strings.ReplaceAll(Tag, ";", "; ")

	if GitCommit == "" || BuildTime == "" {
		log.Fatalf("Binary built improperly. Version variables not set!")
	}
	fmt.Printf("%s Version information:\n|| Authors: %s\n|| Commit: %s\n|| Tag: %s\n|| Build No: %s\n|| Build Date: %s\n", Package, Authors, GitCommit, Tag, BuildNo, BuildTime)

	if *version {
		os.Exit(0)
	} else {
		fmt.Printf("Initialization success...\n")
	}
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

	// err = fs.Serve(c, archivefs.NewFS())
	//
	//	if err != nil {
	//		log.Fatal(err)
	//	}
}

func help() {
	fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "  %s MOUNTPOINT\n", os.Args[0])
	flag.PrintDefaults()
}
