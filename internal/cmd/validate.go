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
	"syscall"

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
	// ErrInsufficientDiskSpace indicates not enough disk space for repair operation.
	ErrInsufficientDiskSpace = errors.New("insufficient disk space for repair")
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

// RepairStats tracks what repairs were performed on an archive.
type RepairStats struct {
	MetadataRegenerated  bool
	OrphanedFilesRemoved int
	MissingEntriesFixed  int
}

func (r RepairStats) String() string {
	var parts []string
	if r.MetadataRegenerated {
		parts = append(parts, "metadata regenerated")
	}
	if r.OrphanedFilesRemoved > 0 {
		parts = append(parts, fmt.Sprintf("%d orphaned files removed", r.OrphanedFilesRemoved))
	}
	if r.MissingEntriesFixed > 0 {
		parts = append(parts, fmt.Sprintf("%d missing entries fixed", r.MissingEntriesFixed))
	}
	if len(parts) == 0 {
		return "no repairs needed"
	}
	return strings.Join(parts, ", ")
}

// ValidateOptions contains options for validation and repair operations.
type ValidateOptions struct {
	Verbose      bool
	Repair       bool
	DryRun       bool
	RemoveBackup bool
}

// NewValidateCmd creates and returns the validate subcommand for the djafs CLI.
// It provides archive validation and consistency checking functionality.
func NewValidateCmd() *cobra.Command {
	var (
		storagePath  string
		verbose      bool
		repair       bool
		dryRun       bool
		removeBackup bool
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
  - Remove orphaned files from archive
  - Remove lookup entries referencing missing files
  - Create missing metadata files

Flags:
  --dry-run shows what repairs would be made without modifying files
  --remove-backup deletes .bak files after successful repair`,
		Run: func(cmd *cobra.Command, args []string) {
			opts := ValidateOptions{
				Verbose:      verbose,
				Repair:       repair,
				DryRun:       dryRun,
				RemoveBackup: removeBackup,
			}
			runValidate(storagePath, opts)
		},
	}

	cmd.Flags().StringVarP(&storagePath, "path", "p", "", "Path to djafs storage directory to validate (required)")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
	cmd.Flags().BoolVarP(&repair, "repair", "r", false, "Attempt to repair corrupted archives")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview repairs without modifying files (requires --repair)")
	cmd.Flags().BoolVar(&removeBackup, "remove-backup", false, "Remove .bak files after successful repair")

	cmd.MarkFlagRequired("path")

	return cmd
}

func runValidate(storagePath string, opts ValidateOptions) {
	if _, err := os.Stat(storagePath); os.IsNotExist(err) {
		log.Fatalf("Storage directory does not exist: %s", storagePath)
	}

	if opts.DryRun && !opts.Repair {
		log.Fatalf("--dry-run requires --repair flag")
	}

	if opts.Verbose {
		fmt.Printf("Validating djafs storage at %s\n", storagePath)
		if opts.DryRun {
			fmt.Println("(dry-run mode - no files will be modified)")
		}
	}

	var totalErrors int
	var totalArchives int
	var totalRepaired int
	var archivesWithErrors int
	var allRepairStats []struct {
		path  string
		stats RepairStats
	}

	err := filepath.Walk(storagePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !strings.HasSuffix(path, ".djfz") {
			return nil
		}

		totalArchives++
		if opts.Verbose {
			fmt.Printf("Validating archive: %s\n", path)
		}

		validationErrors := validateArchive(path)
		if len(validationErrors) > 0 {
			archivesWithErrors++
			fmt.Printf("Archive %s has %d errors:\n", path, len(validationErrors))
			for _, verr := range validationErrors {
				fmt.Printf("  - %s\n", verr)
			}
			totalErrors += len(validationErrors)

			if opts.Repair {
				if opts.DryRun {
					fmt.Printf("Would repair %s (dry-run)...\n", path)
					stats := previewRepair(path, validationErrors, opts.Verbose)
					fmt.Printf("  Preview: %s\n", stats)
				} else {
					fmt.Printf("Attempting to repair %s...\n", path)
					stats, repairErr := repairArchive(path, validationErrors, opts.Verbose)
					if repairErr != nil {
						fmt.Printf("Repair failed: %v\n", repairErr)
					} else if stats.MetadataRegenerated || stats.OrphanedFilesRemoved > 0 || stats.MissingEntriesFixed > 0 {
						// Verify repair was successful
						postRepairErrors := validateArchive(path)
						if len(postRepairErrors) > 0 {
							fmt.Printf("Warning: Archive still has %d errors after repair:\n", len(postRepairErrors))
							for _, verr := range postRepairErrors {
								fmt.Printf("  - %s\n", verr)
							}
						} else {
							fmt.Printf("Successfully repaired %s: %s\n", path, stats)
							totalRepaired++
							allRepairStats = append(allRepairStats, struct {
								path  string
								stats RepairStats
							}{path, stats})

							// Remove backup if requested
							if opts.RemoveBackup {
								backupPath := path + ".bak"
								if err := os.Remove(backupPath); err != nil {
									if !os.IsNotExist(err) {
										fmt.Printf("Warning: failed to remove backup %s: %v\n", backupPath, err)
									}
								} else if opts.Verbose {
									fmt.Printf("  Removed backup: %s\n", backupPath)
								}
							}
						}
					} else {
						fmt.Printf("No repairs were possible for %s\n", path)
					}
				}
			}
		} else if opts.Verbose {
			fmt.Printf("Archive %s is valid\n", path)
		}

		return nil
	})
	if err != nil {
		log.Fatalf("Error walking storage directory: %v", err)
	}

	// Print summary
	fmt.Printf("\nValidation complete:\n")
	fmt.Printf("  Archives checked: %d\n", totalArchives)
	fmt.Printf("  Archives with errors: %d\n", archivesWithErrors)
	fmt.Printf("  Total errors: %d\n", totalErrors)
	if opts.Repair {
		if opts.DryRun {
			fmt.Printf("  (dry-run mode - no repairs were made)\n")
		} else {
			fmt.Printf("  Archives repaired: %d\n", totalRepaired)
			if len(allRepairStats) > 0 && opts.Verbose {
				fmt.Println("\nRepair details:")
				for _, rs := range allRepairStats {
					fmt.Printf("  %s: %s\n", rs.path, rs.stats)
				}
			}
		}
	}

	// Exit with error if there are unfixed errors
	unfixedArchives := archivesWithErrors - totalRepaired
	if unfixedArchives > 0 && !opts.DryRun {
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
	var lookupParseError, metadataParseError bool
	archiveFiles := make(map[string]bool)

	for _, f := range r.File {
		archiveFiles[f.Name] = true

		switch f.Name {
		case "lookups.djfl":
			hasLookup = true
			if err := decodeZipFile(f, &lookupTable); err != nil {
				lookupParseError = true
				errs = append(errs, ValidationError{
					Err:     ErrArchiveCorrupted,
					Context: fmt.Sprintf("failed to parse lookup table: %v", err),
				})
			}

		case "metadata.djfm":
			hasMetadata = true
			if err := decodeZipFile(f, &metadata); err != nil {
				metadataParseError = true
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

	// Validate lookup table references (only if lookup was parsed successfully)
	if hasLookup && !lookupParseError {
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

	// Validate metadata consistency (only if both were parsed successfully)
	if hasLookup && hasMetadata && !lookupParseError && !metadataParseError {
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

// previewRepair analyzes what repairs would be made without modifying files.
func previewRepair(archivePath string, validationErrors []ValidationError, verbose bool) RepairStats {
	var stats RepairStats

	for _, verr := range validationErrors {
		switch {
		case errors.Is(verr.Err, ErrArchiveCorrupted):
			// Cannot repair
		case errors.Is(verr.Err, ErrMissingLookup):
			// Cannot repair
		case errors.Is(verr.Err, ErrMissingMetadata):
			stats.MetadataRegenerated = true
		case errors.Is(verr.Err, ErrMetadataMismatch):
			stats.MetadataRegenerated = true
		case errors.Is(verr.Err, ErrOrphanedFile):
			stats.OrphanedFilesRemoved++
			if verbose {
				fmt.Printf("  Would remove orphaned file: %s\n", verr.Context)
			}
		case errors.Is(verr.Err, ErrMissingTarget):
			stats.MissingEntriesFixed++
			if verbose {
				fmt.Printf("  Would remove lookup entry for missing file: %s\n", verr.Context)
			}
		}
	}

	return stats
}

// repairArchive attempts to repair common archive issues.
// Returns repair statistics and any error encountered.
func repairArchive(archivePath string, validationErrors []ValidationError, verbose bool) (RepairStats, error) {
	var stats RepairStats

	// Analyze what repairs are needed
	var needsMetadataRegeneration bool
	var needsLookupCleanup bool
	var hasUnrecoverableErrors bool

	for _, verr := range validationErrors {
		switch {
		case errors.Is(verr.Err, ErrArchiveCorrupted):
			hasUnrecoverableErrors = true
		case errors.Is(verr.Err, ErrMissingLookup):
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
		return stats, nil
	}

	if !needsMetadataRegeneration && !needsLookupCleanup {
		return stats, nil
	}

	// Check disk space before repair
	archiveInfo, err := os.Stat(archivePath)
	if err != nil {
		return stats, fmt.Errorf("failed to stat archive: %w", err)
	}

	archiveDir := filepath.Dir(archivePath)
	availableSpace, err := getAvailableDiskSpace(archiveDir)
	if err != nil {
		if verbose {
			fmt.Printf("  Warning: could not check disk space: %v\n", err)
		}
	} else {
		// Need at least 2x the archive size (temp file + original during swap)
		requiredSpace := uint64(archiveInfo.Size()) * 2
		if availableSpace < requiredSpace {
			return stats, fmt.Errorf("%w: need %d bytes, have %d bytes",
				ErrInsufficientDiskSpace, requiredSpace, availableSpace)
		}
	}

	// Acquire file lock to prevent concurrent access
	lockPath := archivePath + ".lock"
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		if os.IsExist(err) {
			return stats, fmt.Errorf("archive is locked by another process: %s", lockPath)
		}
		return stats, fmt.Errorf("failed to acquire lock: %w", err)
	}
	defer func() {
		lockFile.Close()
		os.Remove(lockPath)
	}()

	// Create a temporary file for the repaired archive
	tmpFile, err := os.CreateTemp(archiveDir, "repair-*.djfz")
	if err != nil {
		return stats, fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	tmpClosed := false
	repairSucceeded := false

	defer func() {
		if !tmpClosed {
			tmpFile.Close()
		}
		// Only remove temp file if repair failed
		if !repairSucceeded {
			os.Remove(tmpPath)
		}
	}()

	// Open the original archive
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return stats, fmt.Errorf("failed to open archive: %w", err)
	}
	defer r.Close()

	// Create new archive writer
	w := zip.NewWriter(tmpFile)

	// Load the lookup table
	var lookupTable util.LookupTable
	for _, f := range r.File {
		if f.Name == "lookups.djfl" {
			if err := decodeZipFile(f, &lookupTable); err != nil {
				w.Close()
				return stats, fmt.Errorf("failed to load lookup table: %w", err)
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
			stats.OrphanedFilesRemoved++
			if verbose {
				fmt.Printf("  Removing orphaned file: %s\n", f.Name)
			}
			continue
		}

		filesInArchive[f.Name] = true

		// Copy file to new archive
		if err := copyZipFile(w, f); err != nil {
			w.Close()
			return stats, fmt.Errorf("failed to copy file %s: %w", f.Name, err)
		}
	}

	// Clean up lookup table entries that reference missing files
	if needsLookupCleanup {
		var removed int
		lookupTable, removed = cleanLookupTable(&lookupTable, filesInArchive, verbose)
		stats.MissingEntriesFixed = removed
	}

	// Write cleaned lookup table
	lookupWriter, err := w.Create("lookups.djfl")
	if err != nil {
		w.Close()
		return stats, fmt.Errorf("failed to create lookup file: %w", err)
	}
	if err := json.NewEncoder(lookupWriter).Encode(lookupTable); err != nil {
		w.Close()
		return stats, fmt.Errorf("failed to write lookup table: %w", err)
	}

	// Generate and write new metadata
	metadata, err := lookupTable.GenerateMetadata("")
	if err != nil {
		w.Close()
		return stats, fmt.Errorf("failed to generate metadata: %w", err)
	}

	metadataWriter, err := w.Create("metadata.djfm")
	if err != nil {
		w.Close()
		return stats, fmt.Errorf("failed to create metadata file: %w", err)
	}
	if err := json.NewEncoder(metadataWriter).Encode(metadata); err != nil {
		w.Close()
		return stats, fmt.Errorf("failed to write metadata: %w", err)
	}

	if needsMetadataRegeneration {
		stats.MetadataRegenerated = true
	}

	// Close writers before replacing
	if err := w.Close(); err != nil {
		return stats, fmt.Errorf("failed to finalize archive: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return stats, fmt.Errorf("failed to close temp file: %w", err)
	}
	tmpClosed = true

	// Backup original and replace with repaired
	backupPath := archivePath + ".bak"
	if err := os.Rename(archivePath, backupPath); err != nil {
		return stats, fmt.Errorf("failed to backup original: %w", err)
	}

	if err := os.Rename(tmpPath, archivePath); err != nil {
		// Try to restore backup
		os.Rename(backupPath, archivePath)
		return stats, fmt.Errorf("failed to replace with repaired archive: %w", err)
	}

	repairSucceeded = true

	if verbose {
		fmt.Printf("  Original backed up to: %s\n", backupPath)
	}

	return stats, nil
}

// getAvailableDiskSpace returns the available disk space in bytes for the given path.
func getAvailableDiskSpace(path string) (uint64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, err
	}
	return stat.Bavail * uint64(stat.Bsize), nil
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
// Returns the cleaned lookup table and the count of removed entries.
func cleanLookupTable(lt *util.LookupTable, filesInArchive map[string]bool, verbose bool) (util.LookupTable, int) {
	var cleaned util.LookupTable
	var removed int

	for entry := range lt.Iterate {
		// Keep deletion markers (empty target)
		if entry.Target == "" {
			cleaned.Add(entry)
			continue
		}

		// Keep entries with valid targets
		if filesInArchive[entry.Target] {
			cleaned.Add(entry)
		} else {
			removed++
			if verbose {
				fmt.Printf("  Removing lookup entry for missing file: %s -> %s\n", entry.Name, entry.Target)
			}
		}
	}

	return cleaned, removed
}
