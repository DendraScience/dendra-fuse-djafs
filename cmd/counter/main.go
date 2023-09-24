package main

import (
	"fmt"
	"io/fs"
	"path/filepath"
)

func main() {
	count := 0
	filepath.WalkDir("./", func(path string, d fs.DirEntry, err error) error {
		if !d.IsDir() {
			count++
		}
		if count%10000 == 0 {
			fmt.Println(count)
		}
		return nil
	})
	fmt.Println(count)
}
