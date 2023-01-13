package main

import (
	"log"

	"github.com/dendrascience/dendra-archive-fuse/util"
)

func main() {
	dirs, files, err := util.DetermineZipBoundaries("test", 50)
	if err != nil {
		log.Fatal(err)
	}
	_ = files
	for _, f := range files {
		err := util.ZipInside(f, "", false)
		if err != nil {
			panic(err)
		}
	}
	for _, f := range dirs {
		err = util.ZipInside(f, "", true)
		if err != nil {
			panic(err)
		}
	}
}
