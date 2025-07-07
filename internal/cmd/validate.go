package cmd

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/dendrascience/dendra-archive-fuse/util"
	"github.com/spf13/cobra"
)

// NewValidateCmd creates and returns the validate subcommand for the djafs CLI.
// It provides archive validation and consistency checking functionality.
func NewValidateCmd() *cobra.Command {
	var (
		storagePath string
		verbose     bool
		repair      bool
	)

	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate djafs archives for corruption and consistency",
		Long: `Validate djafs archives for corruption and consistency issues.

This command checks the structure of .djfz archives, verifies that all files
referenced in lookup tables exist, and validates metadata consistency.
Optionally can attempt repairs on corrupted archives.`,
		Run: func(cmd *cobra.Command, args []string) {
			runValidate(storagePath, verbose, repair)
		},
	}

	cmd.Flags().StringVarP(&storagePath, "path", "p", "", "Path to djafs storage directory to validate (required)")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
	cmd.Flags().BoolVarP(&repair, "repair", "r", false, "Attempt to repair corrupted archives")

	cmd.MarkFlagRequired("path")

	return cmd
}

func runValidate(storagePath string, verbose, repair bool) {
	// Validate storage directory exists
	if _, err := os.Stat(storagePath); os.IsNotExist(err) {
		log.Fatalf("Storage directory does not exist: %s", storagePath)
	}

	if verbose {
		fmt.Printf("Validating djafs storage at %s\n", storagePath)
	}

	var totalErrors int
	var totalArchives int

	// Find all .djfz archives
	err := filepath.Walk(storagePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !strings.HasSuffix(path, ".djfz") {
			return nil
		}

		totalArchives++
		if verbose {
			fmt.Printf("Validating archive: %s\n", path)
		}

		errors := validateArchive(path)
		if len(errors) > 0 {
			fmt.Printf("Archive %s has %d errors:\n", path, len(errors))
			for _, err := range errors {
				fmt.Printf("  - %s\n", err)
			}
			totalErrors += len(errors)

			if repair {
				fmt.Printf("Attempting to repair %s...\n", path)
				if repairArchive(path) {
					fmt.Printf("Successfully repaired %s\n", path)
				} else {
					fmt.Printf("Failed to repair %s\n", path)
				}
			}
		} else if verbose {
			fmt.Printf("Archive %s is valid\n", path)
		}

		return nil
	})
	if err != nil {
		log.Fatalf("Error walking storage directory: %v", err)
	}

	fmt.Printf("\nValidation complete:\n")
	fmt.Printf("  Archives checked: %d\n", totalArchives)
	fmt.Printf("  Total errors: %d\n", totalErrors)

	if totalErrors > 0 {
		os.Exit(1)
	}
}

func validateArchive(archivePath string) []string {
	var errors []string

	// Open the archive
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return []string{fmt.Sprintf("Failed to open archive: %v", err)}
	}
	defer r.Close()

	var lookupTable util.LookupTable
	var metadata util.Metadata
	var hasLookup, hasMetadata bool

	// Check for required files
	for _, f := range r.File {
		switch f.Name {
		case "lookups.djfl":
			hasLookup = true
			// Validate lookup table
			rc, err := f.Open()
			if err != nil {
				errors = append(errors, fmt.Sprintf("Failed to open lookup table: %v", err))
				continue
			}
			defer rc.Close()

			err = json.NewDecoder(rc).Decode(&lookupTable)
			if err != nil {
				errors = append(errors, fmt.Sprintf("Failed to parse lookup table: %v", err))
			}

		case "metadata.djfm":
			hasMetadata = true
			// Validate metadata
			rc, err := f.Open()
			if err != nil {
				errors = append(errors, fmt.Sprintf("Failed to open metadata: %v", err))
				continue
			}
			defer rc.Close()

			err = json.NewDecoder(rc).Decode(&metadata)
			if err != nil {
				errors = append(errors, fmt.Sprintf("Failed to parse metadata: %v", err))
			}
		}
	}

	if !hasLookup {
		errors = append(errors, "Missing lookup table (lookups.djfl)")
	}

	if !hasMetadata {
		errors = append(errors, "Missing metadata (metadata.djfm)")
	}

	// Validate that all files referenced in lookup table exist in archive
	if hasLookup {
		archiveFiles := make(map[string]bool)
		for _, f := range r.File {
			archiveFiles[f.Name] = true
		}

		for entry := range lookupTable.Iterate {
			if entry.Target == "" {
				continue // Deleted file
			}
			if !archiveFiles[entry.Target] {
				errors = append(errors, fmt.Sprintf("Lookup table references missing file: %s", entry.Target))
			}
		}
	}

	// Validate metadata consistency
	if hasLookup && hasMetadata {
		actualFileCount := lookupTable.GetTotalFileCount()
		if actualFileCount != metadata.TotalFileCount {
			errors = append(errors, fmt.Sprintf("Metadata file count mismatch: expected %d, got %d",
				metadata.TotalFileCount, actualFileCount))
		}

		actualTargetCount := lookupTable.GetTargetFileCount()
		if actualTargetCount != metadata.TargetFileCount {
			errors = append(errors, fmt.Sprintf("Metadata target count mismatch: expected %d, got %d",
				metadata.TargetFileCount, actualTargetCount))
		}
	}

	return errors
}

func repairArchive(archivePath string) bool {
	// For now, just return false - repair functionality would be complex
	// and require careful implementation to avoid data loss
	_ = archivePath
	return false
}
