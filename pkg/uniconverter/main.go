package main

import (
	"encoding/gob"
	"flag"
	"fmt"
	"log"
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

type boundary struct {
	Subfolders []string
	Subfiles   []string
}

func main() {
	flag.Parse()
	if *outputPath == "" || *directoryPath == "" {
		flag.CommandLine.Usage()
		os.Exit(1)
	}
	// Create the filesystem.
	// The filesystem is created at the output path.
	os.MkdirAll(*outputPath, 0o777)
	subfolders, subfiles := []string{}, []string{}
	saveState, err := os.Open(filepath.Join(*outputPath, "boundaries.gob"))
	if err != nil {
		subfolders, subfiles, err = util.DetermineZipBoundaries(*directoryPath, *thresholdSize)
		if err != nil {
			panic(err)
		}
		f, err := os.Create(filepath.Join(*outputPath, "boundaries.gob"))
		if err != nil {
			panic(err)
		}
		gob.NewEncoder(f).Encode(boundary{Subfolders: subfolders, Subfiles: subfiles})
		f.Close()
	} else {
		b := boundary{}
		gob.NewDecoder(saveState).Decode(&b)
		subfolders, subfiles = b.Subfolders, b.Subfiles
		saveState.Close()
	}

	fmt.Printf("subfolders: %v\nsubfiles: %v\n", subfolders, subfiles)
	_, _ = subfolders, subfiles
	for _, sf := range subfolders {
		lt, err := util.CreateInitialDJAFSManifest(sf, *outputPath, false)
		if err != nil {
			panic(err)
		}
		subpath := strings.TrimPrefix(sf, *directoryPath)
		newPath := filepath.Join(*outputPath, util.MappingDir, subpath)
		err = os.MkdirAll(newPath, 0o777)
		if err != nil {
			panic(err)
		}
		err = util.WriteJSONFile(filepath.Join(newPath, "subdirs.djfl"), lt)
		if err != nil {
			panic(err)
		}
	}

	for _, sf := range subfiles {
		lt, err := util.CreateInitialDJAFSManifest(sf, *outputPath, true)
		if err != nil {
			panic(err)
		}
		subpath := strings.TrimPrefix(sf, *directoryPath)
		newPath := filepath.Join(*outputPath, util.MappingDir, subpath)
		err = os.MkdirAll(newPath, 0o777)
		if err != nil {
			panic(err)
		}
		err = util.WriteJSONFile(filepath.Join(newPath, "subfiles.djfl"), lt)
		if err != nil {
			panic(err)
		}
	}
	log.Println("created initial manifest files")

	err = util.GCWorkDirs(filepath.Join(*outputPath, util.WorkDir))
	if err != nil {
		panic(err)
	}
	//	for _, dir := range subfolders {
	//		err := util.CreateDJAFSArchive(dir, false)
	//		if err != nil {
	//			panic(err)
	//		}
	//	}
	//	for _, dir := range subfiles {
	//		err := util.CreateDJAFSArchive(dir, true)
	//		if err != nil {
	//			panic(err)
	//		}
	//	}

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
