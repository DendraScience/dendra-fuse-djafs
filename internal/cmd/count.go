package cmd

import (
	"fmt"
	"io/fs"
	"path/filepath"

	"github.com/spf13/cobra"
)

// NewCountCmd creates and returns the count subcommand for the djafs CLI.
// It provides file counting functionality for directory trees.
func NewCountCmd() *cobra.Command {
	var (
		path         string
		showProgress bool
	)

	cmd := &cobra.Command{
		Use:   "count [PATH]",
		Short: "Count files in a directory tree",
		Long: `Count the total number of files in a directory tree.

This is a utility command that recursively walks through a directory
and counts all files (excluding directories). Useful for getting
quick statistics about directory contents.`,
		Args: cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) > 0 {
				path = args[0]
			}
			runCount(path, showProgress)
		},
	}

	cmd.Flags().StringVarP(&path, "path", "p", "./", "Path to count files in")
	cmd.Flags().BoolVar(&showProgress, "progress", false, "Show progress every 10,000 files")

	return cmd
}

func runCount(path string, showProgress bool) {
	count := 0
	err := filepath.WalkDir(path, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			count++
		}
		if showProgress && count%10000 == 0 && count > 0 {
			fmt.Printf("Progress: %d files counted\n", count)
		}
		return nil
	})

	if err != nil {
		fmt.Printf("Error counting files: %v\n", err)
		return
	}

	fmt.Printf("Total files: %d\n", count)
}
