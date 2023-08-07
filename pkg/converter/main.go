package main

import (
	"flag"
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
}
