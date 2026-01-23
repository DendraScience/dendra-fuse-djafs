package cmd

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/dendrascience/dendra-archive-fuse/util"
	"github.com/spf13/cobra"
)

var (
	// ErrArchiveCorrupted indicates the archive cannot be opened or is structurally invalid.
	ErrArchiveCorrupted = errors.New("archive is corrupted")
	// ErrMissingLookup indicates the archive is missing the lookup table file.
	ErrMissingLookup = errors.New("missing lookup table (lookups.djfl)")
	// ErrMissingMetadata indicates the archive is missing the metadata file.
	ErrMissingMetadata = errors.New("missing metadata (metadata.djfm)")
	// ErrOrphanedFile indicates a file in the archive is not referenced by the lookup table.
	ErrOrphanedFile = errors.New("orphaned file not referenced in lookup table")
	// ErrMissingTarget indicates the lookup table references a file not in the archive.
	ErrMissingTarget = errors.New("lookup table references missing file")
	// ErrMetadataMismatch indicates metadata counts don't match actual lookup table values.
	ErrMetadataMismatch = errors.New("metadata count mismatch")
)

// ValidationError represents a specific validation error with context.
type ValidationError struct {
	Err     error
	Context string
}

func (e ValidationError) Error() string {
	if e.Context != "" {
		return fmt.Sprintf("%s: %s", e.Err, e.Context)
	}
	return e.Err.Error()
}

func (e ValidationError) Unwrap() error {
	return e.Err
}

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
Optionally can attempt repairs on corrupted archives.

Repair operations:
  - Regenerate metadata from lookup table
  - Remove orphaned entries from lookup table
  - Create missing metadata files`,
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
	if _, err := os.Stat(storagePath); os.IsNotExist(err) {
		log.Fatalf("Storage directory does not exist: %s", storagePath)
	}

	if verbose {
		fmt.Printf("Validating djafs storage at %s\n", storagePath)
	}

	var totalErrors int
	var totalArchives int
	var totalRepaired int

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

		validationErrors := validateArchive(path)
		if len(validationErrors) > 0 {
			fmt.Printf("Archive %s has %d errors:\n", path, len(validationErrors))
			for _, verr := range validationErrors {
				fmt.Printf("  - %s\n", verr)
			}
			totalErrors += len(validationErrors)

			if repair {
				fmt.Printf("Attempting to repair %s...\n", path)
				repaired, repairErr := repairArchive(path, validationErrors, verbose)
				if repairErr != nil {
					fmt.Printf("Repair failed: %v\n", repairErr)
				} else if repaired {
					fmt.Printf("Successfully repaired %s\n", path)
					totalRepaired++
				} else {
					fmt.Printf("No repairs were possible for %s\n", path)
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
	if repair {
		fmt.Printf("  Archives repaired: %d\n", totalRepaired)
	}

	if totalErrors > 0 && totalRepaired < totalArchives {
		os.Exit(1)
	}
}

func validateArchive(archivePath string) []ValidationError {
	var errs []ValidationError

	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return []ValidationError{{Err: ErrArchiveCorrupted, Context: err.Error()}}
	}
	defer r.Close()

	var lookupTable util.LookupTable
	var metadata util.Metadata
	var hasLookup, hasMetadata bool
	archiveFiles := make(map[string]bool)

	for _, f := range r.File {
		archiveFiles[f.Name] = true

		switch f.Name {
		case "lookups.djfl":
			hasLookup = true
			if err := decodeZipFile(f, &lookupTable); err != nil {
				errs = append(errs, ValidationError{
					Err:     ErrArchiveCorrupted,
					Context: fmt.Sprintf("failed to parse lookup table: %v", err),
				})
			}

		case "metadata.djfm":
			hasMetadata = true
			if err := decodeZipFile(f, &metadata); err != nil {
				errs = append(errs, ValidationError{
					Err:     ErrArchiveCorrupted,
					Context: fmt.Sprintf("failed to parse metadata: %v", err),
				})
			}
		}
	}

	if !hasLookup {
		errs = append(errs, ValidationError{Err: ErrMissingLookup})
	}

	if !hasMetadata {
		errs = append(errs, ValidationError{Err: ErrMissingMetadata})
	}

	// Validate lookup table references
	if hasLookup {
		referencedFiles := make(map[string]bool)
		for entry := range lookupTable.Iterate {
			if entry.Target == "" {
				continue // Deleted file
			}
			referencedFiles[entry.Target] = true
			if !archiveFiles[entry.Target] {
				errs = append(errs, ValidationError{
					Err:     ErrMissingTarget,
					Context: entry.Target,
				})
			}
		}

		// Check for orphaned files (files in archive not referenced by lookup)
		for name := range archiveFiles {
			if name == "lookups.djfl" || name == "metadata.djfm" {
				continue
			}
			if !referencedFiles[name] {
				errs = append(errs, ValidationError{
					Err:     ErrOrphanedFile,
					Context: name,
				})
			}
		}
	}

	// Validate metadata consistency
	if hasLookup && hasMetadata {
		actualFileCount := lookupTable.GetTotalFileCount()
		if actualFileCount != metadata.TotalFileCount {
			errs = append(errs, ValidationError{
				Err:     ErrMetadataMismatch,
				Context: fmt.Sprintf("TotalFileCount: expected %d, got %d", metadata.TotalFileCount, actualFileCount),
			})
		}

		actualTargetCount := lookupTable.GetTargetFileCount()
		if actualTargetCount != metadata.TargetFileCount {
			errs = append(errs, ValidationError{
				Err:     ErrMetadataMismatch,
				Context: fmt.Sprintf("TargetFileCount: expected %d, got %d", metadata.TargetFileCount, actualTargetCount),
			})
		}
	}

	return errs
}

// decodeZipFile opens a zip file entry and decodes its JSON content into v.
func decodeZipFile(f *zip.File, v any) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	return json.NewDecoder(rc).Decode(v)
}

// repairArchive attempts to repair common archive issues.
// Returns true if repairs were made, false if no repairs were possible.
func repairArchive(archivePath string, validationErrors []ValidationError, verbose bool) (bool, error) {
	// Analyze what repairs are needed
	var needsMetadataRegeneration bool
	var needsLookupCleanup bool
	var hasUnrecoverableErrors bool

	for _, verr := range validationErrors {
		switch {
		case errors.Is(verr.Err, ErrArchiveCorrupted):
			// Cannot repair corrupted archives
			hasUnrecoverableErrors = true
		case errors.Is(verr.Err, ErrMissingLookup):
			// Cannot repair without lookup table
			hasUnrecoverableErrors = true
		case errors.Is(verr.Err, ErrMissingMetadata):
			needsMetadataRegeneration = true
		case errors.Is(verr.Err, ErrMetadataMismatch):
			needsMetadataRegeneration = true
		case errors.Is(verr.Err, ErrOrphanedFile):
			needsLookupCleanup = true
		case errors.Is(verr.Err, ErrMissingTarget):
			needsLookupCleanup = true
		}
	}

	if hasUnrecoverableErrors {
		return false, nil
	}

	if !needsMetadataRegeneration && !needsLookupCleanup {
		return false, nil
	}

	// Create a temporary file for the repaired archive
	tmpFile, err := os.CreateTemp(filepath.Dir(archivePath), "repair-*.djfz")
	if err != nil {
		return false, fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer func() {
		tmpFile.Close()
		os.Remove(tmpPath) // Clean up on failure
	}()

	// Open the original archive
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return false, fmt.Errorf("failed to open archive: %w", err)
	}
	defer r.Close()

	// Create new archive writer
	w := zip.NewWriter(tmpFile)
	defer w.Close()

	// Load the lookup table
	var lookupTable util.LookupTable
	for _, f := range r.File {
		if f.Name == "lookups.djfl" {
			if err := decodeZipFile(f, &lookupTable); err != nil {
				return false, fmt.Errorf("failed to load lookup table: %w", err)
			}
			break
		}
	}

	// Build set of valid targets from lookup table
	validTargets := make(map[string]bool)
	for entry := range lookupTable.Iterate {
		if entry.Target != "" {
			validTargets[entry.Target] = true
		}
	}

	// Copy valid files to new archive
	filesInArchive := make(map[string]bool)
	for _, f := range r.File {
		if f.Name == "lookups.djfl" || f.Name == "metadata.djfm" {
			continue // We'll regenerate these
		}

		// Skip orphaned files if cleaning up
		if needsLookupCleanup && !validTargets[f.Name] {
			if verbose {
				fmt.Printf("  Removing orphaned file: %s\n", f.Name)
			}
			continue
		}

		filesInArchive[f.Name] = true

		// Copy file to new archive
		if err := copyZipFile(w, f); err != nil {
			return false, fmt.Errorf("failed to copy file %s: %w", f.Name, err)
		}
	}

	// Clean up lookup table entries that reference missing files
	if needsLookupCleanup {
		cleanedLookup := cleanLookupTable(&lookupTable, filesInArchive, verbose)
		lookupTable = cleanedLookup
	}

	// Write cleaned lookup table
	lookupWriter, err := w.Create("lookups.djfl")
	if err != nil {
		return false, fmt.Errorf("failed to create lookup file: %w", err)
	}
	if err := json.NewEncoder(lookupWriter).Encode(lookupTable); err != nil {
		return false, fmt.Errorf("failed to write lookup table: %w", err)
	}

	// Generate and write new metadata
	metadata, err := lookupTable.GenerateMetadata("")
	if err != nil {
		return false, fmt.Errorf("failed to generate metadata: %w", err)
	}

	metadataWriter, err := w.Create("metadata.djfm")
	if err != nil {
		return false, fmt.Errorf("failed to create metadata file: %w", err)
	}
	if err := json.NewEncoder(metadataWriter).Encode(metadata); err != nil {
		return false, fmt.Errorf("failed to write metadata: %w", err)
	}

	// Close writers before replacing
	if err := w.Close(); err != nil {
		return false, fmt.Errorf("failed to finalize archive: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return false, fmt.Errorf("failed to close temp file: %w", err)
	}

	// Backup original and replace with repaired
	backupPath := archivePath + ".bak"
	if err := os.Rename(archivePath, backupPath); err != nil {
		return false, fmt.Errorf("failed to backup original: %w", err)
	}

	if err := os.Rename(tmpPath, archivePath); err != nil {
		// Try to restore backup
		os.Rename(backupPath, archivePath)
		return false, fmt.Errorf("failed to replace with repaired archive: %w", err)
	}

	if verbose {
		fmt.Printf("  Original backed up to: %s\n", backupPath)
	}

	return true, nil
}

// copyZipFile copies a file from one zip archive to another.
func copyZipFile(w *zip.Writer, f *zip.File) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	header := f.FileHeader
	writer, err := w.CreateHeader(&header)
	if err != nil {
		return err
	}

	_, err = io.Copy(writer, rc)
	return err
}

// cleanLookupTable removes entries that reference files not in the archive.
func cleanLookupTable(lt *util.LookupTable, filesInArchive map[string]bool, verbose bool) util.LookupTable {
	var cleaned util.LookupTable

	for entry := range lt.Iterate {
		// Keep deletion markers (empty target)
		if entry.Target == "" {
			cleaned.Add(entry)
			continue
		}

		// Keep entries with valid targets
		if filesInArchive[entry.Target] {
			cleaned.Add(entry)
		} else if verbose {
			fmt.Printf("  Removing lookup entry for missing file: %s -> %s\n", entry.Name, entry.Target)
		}
	}

	return cleaned
}
