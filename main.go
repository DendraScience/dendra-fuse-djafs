package main

import (
	"context"
	"os"

	"github.com/charmbracelet/fang"
	"github.com/dendrascience/dendra-archive-fuse/internal/cmd"
)

func main() {
	rootCmd := cmd.NewRootCmd()
	if err := fang.Execute(context.Background(), rootCmd); err != nil {
		os.Exit(1)
	}
}
