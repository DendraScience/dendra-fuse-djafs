package main

import (
	"flag"
	"os"
	"path/filepath"
	"strings"

	"github.com/dendrascience/dendra-archive-fuse/util"
)

// This utility allows the user to convert a directory tree
// into a compliant djafs filesystem.
// The filesystem is then ready to be mounted and used.
// The binary takes in a directory path, the threshold size,
// the threshold tolerance, and the output path as arguments.
// The output path is where the filesystem will be created.
// The directory path must be a directory that exists.
// The directory path must be a valid directory path.
// The output path must be a valid directory path.

// New files are dropped into a hot cache folder.
// These files are given a temporary name, which corresponds to an inode number.
// There is one, global hot cache manifest file, which contains the mapping
// between the inode number and the original file name (and path).
//
// When the garbage collector is run, it will read the hot cache manifest file
// and hash the files in the hot cache folder, copying them to the work folder.
// When files are hashed into the work folder, their manifest entry is updated
// in the .mappings folder, which is a stratified directory structure.
//
// All actual files are stored in the .data folder, which is flat.

var (
	outputPath         = flag.String("o", "", "The output path for the filesystem.")
	thresholdSize      = flag.Int("s", util.GlobalModulus, "The threshold size for the filesystem.")
	thresholdTolerance = flag.Int("t", 1, "The threshold tolerance for the filesystem.")
	directoryPath      = flag.String("d", "", "The directory path for the filesystem.")
)

func main() {
	flag.Parse()
	if *outputPath == "" || *directoryPath == "" {
		flag.CommandLine.Usage()
		os.Exit(1)
	}
	// Create the filesystem.
	// The filesystem is created at the output path.
	os.MkdirAll(*outputPath, 0o777)
	boundaries, err := util.DetermineZipBoundaries(*directoryPath, *thresholdSize)
	if err != nil {
		panic(err)
	}

	for _, boundary := range boundaries {
		lt, err := util.CreateInitialDJAFSManifest(boundary.Path, *outputPath, boundary.IncludeSubdirs)
		if err != nil {
			panic(err)
		}
		subpath := strings.TrimPrefix(boundary.Path, *directoryPath)
		newPath := filepath.Join(*outputPath, util.DataDir, subpath)
		err = os.MkdirAll(newPath, 0o777)
		if err != nil {
			panic(err)
		}
		err = util.WriteJSONFile(filepath.Join(newPath, "lookups.djfl"), lt)
		if err != nil {
			panic(err)
		}
		metadata, err := lt.GenerateMetadata("")
		if err != nil {
			panic(err)
		}
		err = util.WriteJSONFile(filepath.Join(newPath, "metadata.djfm"), metadata)
		if err != nil {
			panic(err)
		}
	}

	err = util.GCWorkDirs(filepath.Join(*outputPath, util.WorkDir))
	if err != nil {
		panic(err)
	}
}
