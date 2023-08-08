package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/dendrascience/dendra-archive-fuse/util"
)

// This utility allows the user to convert a directory tree
// into a compliant djafs filesystem.
// The filesystem is then ready to be mounted and used.
// The binary takes in a directory path, the threshold size,
// the threshold tolerance, and the output path as arguments.
// The output path is where the filesystem will be created.
// The threshold size is the maximum number of files that can
// be stored in a zipped directory before it is split into subdirectories.
// The threshold tolerance is the additional number of files that
// can be stored in a zipped directory past the threshold before
// it is split into subdirectories.
// is allowed to be exceeded before the directory is split.
// The threshold tolerance is an integer
// The threshold size is an integer greater than 0.
// The directory path is a string.
// The output path is a string.
// The directory path must be a directory that exists.
// The directory path must be a valid directory path.
// The output path must be a valid directory path.

var (
	outputPath         = flag.String("o", "", "The output path for the filesystem.")
	thresholdSize      = flag.Int("s", 100, "The threshold size for the filesystem.")
	thresholdTolerance = flag.Int("t", 1, "The threshold tolerance for the filesystem.")
	directoryPath      = flag.String("d", "", "The directory path for the filesystem.")
)

func main() {
	flag.Parse()
	if *outputPath == "" || *directoryPath == "" {
		flag.CommandLine.Usage()
	}
	// Create the filesystem.
	// The filesystem is created at the output path.
	os.MkdirAll(*outputPath, 0o777)
	subfolders, subfiles, err := util.DetermineZipBoundaries(*directoryPath, *thresholdSize)
	if err != nil {
		panic(err)
	}
	fmt.Printf("subfolders: %v\nsubfiles: %v\n", subfolders, subfiles)
	_, _ = subfolders, subfiles

	for _, dir := range subfolders {
		err := util.CreateDJAFSArchive(dir, false)
		if err != nil {
			panic(err)
		}
	}
	for _, dir := range subfiles {
		err := util.CreateDJAFSArchive(dir, true)
		if err != nil {
			panic(err)
		}
	}

	// for each file under the subfiles path,
	// hash the file and create an entry in the metadata file.
	// then, zip all the files in the subfiles path into a .djfz (zip) file.

	// for each folder under the subfolders path,
	// hash all the files in the folder and create an entry in the metadata file.
	// for empty folders, create an entry in the metadata file pointing to that folder
	// then, zip all the files in the subfolders path into a .djfz (zip) file.

	// for all the .djfz files in the subfolders and subfiles path,
	// record the metrics into the djfl file. for api entries later
}
