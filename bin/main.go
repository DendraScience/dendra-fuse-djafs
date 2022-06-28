package main

import (
	"log"

	"github.com/dendrascience/dendra-archive-fuse/util"
)

func main() {
	roots, err := util.DetermineZipBoundaries(".", 50)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("roots: %v\n", roots)
}
