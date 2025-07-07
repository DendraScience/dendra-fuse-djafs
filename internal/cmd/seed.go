package cmd

import (
	"crypto/rand"
	"fmt"
	"log"
	"math/big"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

// NewSeedCmd creates and returns the seed subcommand for the djafs CLI.
// It generates a large number of test files with randomized directory structure.
func NewSeedCmd() *cobra.Command {
	var (
		outputPath string
		fileCount  int
		verbose    bool
	)

	cmd := &cobra.Command{
		Use:   "seed",
		Short: "Generate test files with randomized directory structure",
		Long: `Generate a large number of test files for testing djafs functionality.

Creates files in a YYYY/MM/DD/HH/mm/SS directory structure with randomized
content. Files are distributed across the hierarchy with most files at the
deepest level (SS). Each file contains a single UUID line.`,
		Run: func(cmd *cobra.Command, args []string) {
			runSeed(outputPath, fileCount, verbose)
		},
	}

	cmd.Flags().StringVarP(&outputPath, "output", "o", "", "Path to output directory (required)")
	cmd.Flags().IntVarP(&fileCount, "count", "c", 10000, "Number of files to generate")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")

	cmd.MarkFlagRequired("output")

	return cmd
}

func runSeed(outputPath string, fileCount int, verbose bool) {
	if verbose {
		fmt.Printf("Generating %d test files in %s\n", fileCount, outputPath)
	}

	// Create output directory
	if err := os.MkdirAll(outputPath, 0755); err != nil {
		log.Fatalf("Failed to create output directory: %v", err)
	}

	// Generate pool of 50 UUIDs
	uuidPool := make([]string, 50)
	for i := 0; i < 50; i++ {
		uuidPool[i] = uuid.New().String()
	}

	// Generate files
	filesCreated := 0
	dirFileCounts := make(map[string]int)

	// Start from a base time and vary it
	baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	for filesCreated < fileCount {
		// Generate random time offset (within a year)
		maxDays := 365
		dayOffset, _ := rand.Int(rand.Reader, big.NewInt(int64(maxDays)))
		hourOffset, _ := rand.Int(rand.Reader, big.NewInt(24))
		minuteOffset, _ := rand.Int(rand.Reader, big.NewInt(60))
		secondOffset, _ := rand.Int(rand.Reader, big.NewInt(60))

		fileTime := baseTime.AddDate(0, 0, int(dayOffset.Int64())).
			Add(time.Duration(hourOffset.Int64()) * time.Hour).
			Add(time.Duration(minuteOffset.Int64()) * time.Minute).
			Add(time.Duration(secondOffset.Int64()) * time.Second)

		// Determine directory level (most files at deepest level)
		levelRand, _ := rand.Int(rand.Reader, big.NewInt(100))
		var dirPath string

		switch {
		case levelRand.Int64() < 5: // 5% at year level
			dirPath = filepath.Join(outputPath, fmt.Sprintf("%04d", fileTime.Year()))
		case levelRand.Int64() < 10: // 5% at month level
			dirPath = filepath.Join(outputPath, fmt.Sprintf("%04d", fileTime.Year()), fmt.Sprintf("%02d", fileTime.Month()))
		case levelRand.Int64() < 15: // 5% at day level
			dirPath = filepath.Join(outputPath, fmt.Sprintf("%04d", fileTime.Year()), fmt.Sprintf("%02d", fileTime.Month()), fmt.Sprintf("%02d", fileTime.Day()))
		case levelRand.Int64() < 25: // 10% at hour level
			dirPath = filepath.Join(outputPath, fmt.Sprintf("%04d", fileTime.Year()), fmt.Sprintf("%02d", fileTime.Month()), fmt.Sprintf("%02d", fileTime.Day()), fmt.Sprintf("%02d", fileTime.Hour()))
		case levelRand.Int64() < 40: // 15% at minute level
			dirPath = filepath.Join(outputPath, fmt.Sprintf("%04d", fileTime.Year()), fmt.Sprintf("%02d", fileTime.Month()), fmt.Sprintf("%02d", fileTime.Day()), fmt.Sprintf("%02d", fileTime.Hour()), fmt.Sprintf("%02d", fileTime.Minute()))
		default: // 60% at second level
			dirPath = filepath.Join(outputPath, fmt.Sprintf("%04d", fileTime.Year()), fmt.Sprintf("%02d", fileTime.Month()), fmt.Sprintf("%02d", fileTime.Day()), fmt.Sprintf("%02d", fileTime.Hour()), fmt.Sprintf("%02d", fileTime.Minute()), fmt.Sprintf("%02d", fileTime.Second()))
		}

		// Check if directory has too many files
		if dirFileCounts[dirPath] >= 1000 {
			continue // Try a different time/directory
		}

		// Create directory if it doesn't exist
		if err := os.MkdirAll(dirPath, 0755); err != nil {
			log.Printf("Warning: Failed to create directory %s: %v", dirPath, err)
			continue
		}

		// Generate random filename (lowercase hex)
		filenameNum, _ := rand.Int(rand.Reader, big.NewInt(0xFFFFFFFF))
		extRand, _ := rand.Int(rand.Reader, big.NewInt(2))
		ext := ".json"
		if extRand.Int64() == 1 {
			ext = ".txt"
		}
		filename := fmt.Sprintf("%08x%s", filenameNum.Int64(), ext)
		filePath := filepath.Join(dirPath, filename)

		// Skip if file already exists
		if _, err := os.Stat(filePath); err == nil {
			continue
		}

		// Select random UUID from pool
		uuidIndex, _ := rand.Int(rand.Reader, big.NewInt(50))
		content := uuidPool[uuidIndex.Int64()] + "\n"

		// Write file
		if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
			log.Printf("Warning: Failed to write file %s: %v", filePath, err)
			continue
		}

		dirFileCounts[dirPath]++
		filesCreated++

		if verbose && filesCreated%1000 == 0 {
			fmt.Printf("Created %d/%d files...\n", filesCreated, fileCount)
		}
	}

	if verbose {
		fmt.Printf("Successfully created %d files\n", filesCreated)
		fmt.Printf("Files distributed across %d directories\n", len(dirFileCounts))
		
		// Show some statistics
		maxFiles := 0
		minFiles := 1000
		for _, count := range dirFileCounts {
			if count > maxFiles {
				maxFiles = count
			}
			if count < minFiles {
				minFiles = count
			}
		}
		fmt.Printf("Directory file counts: min=%d, max=%d\n", minFiles, maxFiles)
	}
}