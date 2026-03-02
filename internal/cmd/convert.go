package cmd

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/dendrascience/dendra-archive-fuse/util"
	"github.com/dendrascience/dendra-archive-fuse/version"
	"github.com/spf13/cobra"
)

// NewConvertCmd creates and returns the convert subcommand for the djafs CLI.
// It handles conversion of JSON directory trees to djafs format with various options.
func NewConvertCmd() *cobra.Command {
	var (
		inputPath  string
		outputPath string
		verbose    bool
		dryRun     bool
	)

	cmd := &cobra.Command{
		Use:   "convert",
		Short: "Convert JSON directory trees to djafs format",
		Long: `Convert existing JSON directory structures into djafs filesystem format.

This tool processes directory trees and creates the necessary data structures 
for efficient content-addressable storage. It creates the necessary data structures
for efficient content-addressable storage.`,
		Run: func(cmd *cobra.Command, args []string) {
			runConvert(inputPath, outputPath, verbose, dryRun)
		},
	}

	cmd.Flags().StringVarP(&inputPath, "input", "i", "", "Path to input directory containing JSON files (required)")
	cmd.Flags().StringVarP(&outputPath, "output", "o", "", "Path to output djafs storage directory (required)")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be done without making changes")

	cmd.MarkFlagRequired("input")
	cmd.MarkFlagRequired("output")

	return cmd
}

func runConvert(inputPath, outputPath string, verbose, dryRun bool) {
	// Validate input directory exists
	if _, err := os.Stat(inputPath); os.IsNotExist(err) {
		log.Fatalf("Input directory does not exist: %s", inputPath)
	}

	// Create output directory if it doesn't exist
	if !dryRun {
		if err := os.MkdirAll(outputPath, 0755); err != nil {
			log.Fatalf("Failed to create output directory: %v", err)
		}
	}

	if verbose {
		fmt.Printf("Converting %s to djafs format in %s\n", inputPath, outputPath)
		if dryRun {
			fmt.Println("DRY RUN - no changes will be made")
		}
	}

	// Create initial manifest for all files
	if verbose {
		fmt.Println("Scanning input directory and creating manifest...")
	}

	manifest, err := util.CreateInitialDJAFSManifest(inputPath, outputPath, false)
	if err != nil {
		log.Fatalf("Failed to create manifest: %v", err)
	}

	if verbose {
		fmt.Printf("Found %d files to process\n", manifest.Len())
	}

	if dryRun {
		fmt.Println("Files that would be processed:")
		for entry := range manifest.Iterate {
			fmt.Printf("  %s -> %s\n", entry.Name, entry.Target)
		}
		return
	}

	// Determine zip boundaries based on file count
	if verbose {
		fmt.Println("Determining archive boundaries...")
	}

	boundaries, err := util.DetermineZipBoundaries(inputPath, util.GlobalModulus)
	if err != nil {
		log.Fatalf("Failed to determine zip boundaries: %v", err)
	}

	if verbose {
		fmt.Printf("Creating %d archives\n", len(boundaries))
	}

	// Create archives for each boundary
	for i, boundary := range boundaries {
		if verbose {
			fmt.Printf("Processing archive %d/%d: %s\n", i+1, len(boundaries), boundary.Path)
		}

		// Calculate relative path from input to boundary
		relativePath, err := filepath.Rel(inputPath, boundary.Path)
		if err != nil {
			log.Printf("Warning: Failed to get relative path for %s: %v", boundary.Path, err)
			relativePath = ""
		}

		err = util.CreateDJAFSArchiveWithPath(boundary.Path, outputPath, relativePath, !boundary.IncludeSubdirs)
		if err != nil {
			log.Printf("Warning: Failed to create archive for %s: %v", boundary.Path, err)
			continue
		}
	}

	// Generate metadata for the entire conversion
	if verbose {
		fmt.Println("Generating metadata...")
	}

	metadata := util.Metadata{
		DJAFSVersion:     version.GetVersion(),
		TotalFileCount:   manifest.GetTotalFileCount(),
		TargetFileCount:  manifest.GetTargetFileCount(),
		UncompressedSize: manifest.GetUncompressedSize(),
		OldestFileTS:     manifest.GetOldestFileTS(),
		NewestFileTS:     manifest.GetNewestFileTS(),
	}

	metadataPath := filepath.Join(outputPath, "conversion_metadata.djfm")
	err = util.WriteJSONFile(metadataPath, metadata)
	if err != nil {
		log.Printf("Warning: Failed to write metadata: %v", err)
	}

	if verbose {
		fmt.Printf("Conversion complete!\n")
		fmt.Printf("  Total files: %d\n", metadata.TotalFileCount)
		fmt.Printf("  Unique files: %d\n", metadata.TargetFileCount)
		fmt.Printf("  Uncompressed size: %d bytes\n", metadata.UncompressedSize)
		fmt.Printf("  Storage directory: %s\n", outputPath)
	}
}
