package main

import (
	"fmt"
	"log"

	"github.com/dendrascience/dendra-archive-fuse/util"
)

func main() {
	dirs, files, err := util.DetermineZipBoundaries(".", 50)
	if err != nil {
		log.Fatal(err)
	}
	_ = files
	err = util.ZipInside(dirs[0], "", false)
	if err != nil {
		fmt.Printf("%v\n", err)
	}
	err = util.ZipInside(dirs[0], "", true)
	if err != nil {
		fmt.Printf("%v\n", err)
	}
}
