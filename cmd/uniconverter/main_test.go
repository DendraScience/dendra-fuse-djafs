package main

import (
	"archive/zip"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	// Qualify the import if types clash or for clarity, e.g. dendrautil "github.com/dendrascience/dendra-archive-fuse/util"
	"github.com/dendrascience/dendra-archive-fuse/util"
)

// Helper function to create a file with content
func createFile(t *testing.T, path string, content string) {
	t.Helper()
	err := os.MkdirAll(filepath.Dir(path), 0755)
	if err != nil {
		t.Fatalf("Failed to create directory for file %s: %v", path, err)
	}
	err = os.WriteFile(path, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to write file %s: %v", path, err)
	}
}

func TestUniconverter(t *testing.T) {
	// 5.a. Create a temporary directory for test setup
	testInputDir, err := os.MkdirTemp("", "test_input_dir_*")
	if err != nil {
		t.Fatalf("Failed to create temp input dir: %v", err)
	}
	// 5.c. Defer os.RemoveAll for input directory
	defer os.RemoveAll(testInputDir)

	// 5.b. Create a temporary directory for uniconverter output
	testOutputDir, err := os.MkdirTemp("", "test_output_dir_*")
	if err != nil {
		t.Fatalf("Failed to create temp output dir: %v", err)
	}
	// 5.c. Defer os.RemoveAll for output directory
	defer os.RemoveAll(testOutputDir)

	// 5.d. Populate test_input_dir
	originalFiles := map[string]string{
		"file1.txt":                        "hello file1",
		"file2.json":                       `{"data": "file2"}`,
		"subdir1/file3.txt":                "hello file3",
		"subdir1/subsubdir1/file4.json":    `{"data": "file4"}`,
		"anotherfile.md":                   "# Markdown Content",
		"subdir2/emptyfile.txt":            "",
		"subdir1/anotherempty.txt":         "",
		"subdir1/subsubdir1/bigfile.bin":   strings.Repeat("abcdefgh", 1024), // 8KB file
		"subdir1/subsubdir1/mediumfile.py": strings.Repeat("print('hello')\n", 200), // ~2KB file
	}

	for name, content := range originalFiles {
		fullPath := filepath.Join(testInputDir, name)
		createFile(t, fullPath, content)
	}

	// 6.a. Build the uniconverter binary
	// The test will be in cmd/uniconverter/, so main.go is in the current directory "."
	binaryName := "uniconverter_test_binary"
	if //goland:noinspection GoBoolExpressions
	testing.Verbose() {
		t.Logf("Building %s from .", binaryName)
	}
	buildCmd := exec.Command("go", "build", "-o", binaryName, ".")
	buildOutput, err := buildCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to build uniconverter binary: %v\nOutput:\n%s", err, string(buildOutput))
	}
	defer os.Remove(binaryName) // Defer removal of the compiled binary

	// 6.b. Run uniconverter
	// Using a threshold that should result in multiple boundaries
	// Total files = 9. Threshold 2 could mean many boundaries.
	// Let's try threshold 3 for a slightly less fragmented output initially.
	thresholdSize := "3"
	if testing.Verbose() {
		t.Logf("Running uniconverter with input: %s, output: %s, threshold: %s", testInputDir, testOutputDir, thresholdSize)
	}
	runCmd := exec.Command("./"+binaryName, "-d", testInputDir, "-o", testOutputDir, "-s", thresholdSize)
	runOutput, err := runCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("uniconverter execution failed: %v\nOutput:\n%s", err, string(runOutput))
	}
	if testing.Verbose() {
		t.Logf("uniconverter output:\n%s", string(runOutput))
	}

	// 7. Verification
	// 7.a. Check that the .staging directory is removed
	stagingDir := filepath.Join(testOutputDir, ".staging")
	if _, err := os.Stat(stagingDir); !os.IsNotExist(err) {
		if err == nil {
			t.Errorf(".staging directory (%s) was not removed after execution.", stagingDir)
			// Optionally, list contents of stagingDir for debugging
			filepath.WalkDir(stagingDir, func(path string, d os.DirEntry, errWalk error) error {
				if errWalk != nil {
					t.Logf("Error walking stagingDir %s: %v", path, errWalk)
					return errWalk
				}
				t.Logf("Found in stagingDir: %s (IsDir: %t)", path, d.IsDir())
				return nil
			})
		} else {
			t.Errorf("Error checking for .staging directory (%s): %v", stagingDir, err)
		}
	}

	dataDir := filepath.Join(testOutputDir, util.DataDir) // .data
	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		t.Fatalf(".data directory not found in output: %s", dataDir)
	}

	t.Logf("Test setup and uniconverter execution completed. Verification pending.")

	// Based on DetermineZipBoundaries logic and threshold=3:
	// Root directory (testInputDir) has 3 direct files. count (3) <= threshold (3).
	// So, one boundary is created for testInputDir with IncludeSubdirs = true.
	// This means all 9 files will be processed into a single archive set located at testOutputDir/.data/
	
	// Prepare the map for verifyArchiveSet. Keys are relative to the boundary's root.
	// Since the boundary is testInputDir, the keys are the same as in originalFiles.
	expectedBoundaryPath := filepath.Join(testOutputDir, util.DataDir)
	
	if testing.Verbose() {
		t.Logf("Verifying archive set at: %s", expectedBoundaryPath)
		// List structure of expectedBoundaryPath for debugging
		filepath.WalkDir(expectedBoundaryPath, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				t.Logf("Error walking expectedBoundaryPath %s: %v", path, err)
				return err
			}
			t.Logf("Found in output boundary: %s (IsDir: %t)", path, d.IsDir())
			return nil
		})
	}

	verifyArchiveSet(t, expectedBoundaryPath, originalFiles, testInputDir)
}

// verifyArchiveSet checks a single archive set (lookups, metadata, zip)
// boundaryDataPath: The path to the directory containing lookups.djfl, metadata.djfm, files.djfz for a boundary.
// originalFilesInThisBoundary: A map where keys are file paths relative to the original boundary root,
//                             and values are their original content.
// originalTestRootPath: The absolute path to the root of the original input test directory (e.g., testInputDir).
//                       This is used to reconstruct full original paths for hashing.
func verifyArchiveSet(t *testing.T, boundaryDataPath string, originalFilesInThisBoundary map[string]string, originalTestRootPath string) {
	t.Helper()

	lookupsPath := filepath.Join(boundaryDataPath, "lookups.djfl")
	metadataPath := filepath.Join(boundaryDataPath, "metadata.djfm")
	filesZipPath := filepath.Join(boundaryDataPath, "files.djfz")

	if _, err := os.Stat(lookupsPath); os.IsNotExist(err) {
		t.Errorf("lookups.djfl not found in %s", boundaryDataPath)
		return // Cannot proceed without lookups
	}
	if _, err := os.Stat(metadataPath); os.IsNotExist(err) {
		t.Errorf("metadata.djfm not found in %s", boundaryDataPath)
	}
	if _, err := os.Stat(filesZipPath); os.IsNotExist(err) {
		t.Errorf("files.djfz not found in %s", boundaryDataPath)
	}

	// Verify lookups.djfl
	lookupData, err := os.ReadFile(lookupsPath)
	if err != nil {
		t.Errorf("Failed to read lookups.djfl from %s: %v", boundaryDataPath, err)
		return
	}
	var lookupTable util.LookupTable
	if err := json.Unmarshal(lookupData, &lookupTable); err != nil {
		t.Errorf("Failed to unmarshal lookups.djfl from %s: %v", boundaryDataPath, err)
		return
	}

	if len(originalFilesInThisBoundary) != lookupTable.Len() {
		t.Errorf("Mismatch in expected file count in boundary %s. Expected %d, got %d in lookups.djfl",
			boundaryDataPath, len(originalFilesInThisBoundary), lookupTable.Len())
	}

	foundInLookups := make(map[string]bool)
	lookupTable.Iterate(func(entry util.LookupEntry) bool {
		entryRelativePath := entry.Name // Name is already relative to the boundary's root
		foundInLookups[entryRelativePath] = true
		originalContent, ok := originalFilesInThisBoundary[entryRelativePath]
		if !ok {
			t.Errorf("File %s (target: %s) found in lookups for %s, but not expected for this boundary. Check test boundary definitions.", entryRelativePath, entry.Target, boundaryDataPath)
			return true // continue iteration
		}

		// Verify FileSize
		if int64(len(originalContent)) != entry.FileSize {
			t.Errorf("FileSize mismatch for %s in %s. Expected %d, got %d",
				entry.Name, boundaryDataPath, len(originalContent), entry.FileSize)
		}

		// Verify Target (hashed name format) and that it exists in files.djfz (later)
		// For now, just check extension
		expectedExt := filepath.Ext(entry.Name)
		if filepath.Ext(entry.Target) != expectedExt {
			t.Errorf("Target %s for file %s in %s has incorrect extension. Expected %s",
				entry.Target, entry.Name, boundaryDataPath, expectedExt)
		}
		// TODO: Verify hash once content from zip is available

		return true
	})

	for expectedFileRelativePath := range originalFilesInBoundary {
		if !foundInLookups[expectedFileRelativePath] {
			t.Errorf("Expected file %s not found in lookups.djfl for boundary %s", expectedFileRelativePath, boundaryDataPath)
		}
	}


	// Verify metadata.djfm (Basic checks)
	metadataData, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Errorf("Failed to read metadata.djfm from %s: %v", boundaryDataPath, err)
		return // Cannot proceed
	}
	var metadata util.DJFSMetadata
	if err := json.Unmarshal(metadataData, &metadata); err != nil {
		t.Errorf("Failed to unmarshal metadata.djfm from %s: %v", boundaryDataPath, err)
		return
	}

	if metadata.TotalFileCount != lookupTable.Len() {
		t.Errorf("Metadata TotalFileCount mismatch in %s. Expected %d (from lookups), got %d",
			boundaryDataPath, lookupTable.Len(), metadata.TotalFileCount)
	}
	// TargetFileCount can be different if there are duplicates, so this check is more complex.
	// For now, ensure it's <= TotalFileCount
	if metadata.TargetFileCount > metadata.TotalFileCount {
		t.Errorf("Metadata TargetFileCount (%d) > TotalFileCount (%d) in %s",
			metadata.TargetFileCount, metadata.TotalFileCount, boundaryDataPath)
	}
	if lookupTable.Len() > 0 && metadata.CompressedSize == 0 && metadata.ZipPath != "" { // ZipPath check for non-empty archives
		// Allow CompressedSize == 0 if there are only empty files and they compress to 0.
		// This needs a more nuanced check later. For now, if files exist, expect some size.
		// t.Errorf("Metadata CompressedSize is 0 in %s when files are present.", boundaryDataPath)
	}
	expectedUncompressedSize := int64(0)
	lookupTable.Iterate(func(entry util.LookupEntry) bool {
		expectedUncompressedSize += entry.FileSize
		return true
	})
	if metadata.UncompressedSize != expectedUncompressedSize {
		t.Errorf("Metadata UncompressedSize mismatch in %s. Expected %d (sum from lookups), got %d",
			boundaryDataPath, expectedUncompressedSize, metadata.UncompressedSize)
	}


	// Verify files.djfz
	zipReader, err := zip.OpenReader(filesZipPath)
	if err != nil {
		t.Errorf("Failed to open files.djfz (%s): %v", filesZipPath, err)
		return
	}
	defer zipReader.Close()

	zippedFileCount := 0
	hashedTargetsInZip := make(map[string]*zip.File)
	for _, f := range zipReader.File {
		hashedTargetsInZip[f.Name] = f
		zippedFileCount++
	}

	// Check if all targets in lookups are in zip
	lookupTable.Iterate(func(entry util.LookupEntry) bool {
		if _, ok := hashedTargetsInZip[entry.Target]; !ok {
			t.Errorf("Target file %s (for original %s) from lookups.djfl not found in files.djfz at %s",
				entry.Target, entry.Name, boundaryDataPath)
		} else {
			// Verify content hash and size
			zf := hashedTargetsInZip[entry.Target]
			rc, err := zf.Open()
			if err != nil {
				t.Errorf("Failed to open zipped file %s (target %s) in %s: %v", entry.Name, entry.Target, boundaryDataPath, err)
				return true
			}
			defer rc.Close()
			
			zippedData, err := io.ReadAll(rc)
			if err != nil {
				t.Errorf("Failed to read zipped file %s (target %s) in %s: %v", entry.Name, entry.Target, boundaryDataPath, err)
				return true
			}

			if zf.UncompressedSize64 != uint64(len(zippedData)) {
				// This is an internal zip consistency check, usually ReadAll handles it.
				t.Errorf("Zipped file %s (target %s) uncompressed size in header %d differs from actual read size %d", entry.Name, entry.Target, zf.UncompressedSize64, len(zippedData))
			}
			
			// Compare content with original (if available)
			originalContent, ok := originalFilesInBoundary[entry.Name]
			if ok {
				if string(zippedData) != originalContent {
					// For large files, this is not ideal. Hashing is better.
					// For now, this works for small text files.
					// t.Errorf("Content mismatch for %s (target %s). Expected '%s', got '%s'", entry.Name, entry.Target, originalContent, string(zippedData))
					
					// Verify by hashing instead for robustness
					originalFileFullPath := filepath.Join(inputDir, strings.TrimPrefix(boundaryDataPath, filepath.Join(filepath.Dir(inputDir), util.DataDir)), entry.Name)
					
					// Need to get the hash of the original file as it was stored.
					// The target name IS hash + ext. So we just need to compare the hash part.
					hashFromTarget := strings.TrimSuffix(entry.Target, filepath.Ext(entry.Target))
					
					// Calculate hash of the zipped content
					h := util.Sha256Stream(bytes.NewReader(zippedData))
					// Verify content hash
					hashFromTarget := strings.TrimSuffix(entry.Target, filepath.Ext(entry.Target))

					// Calculate hash of the zipped (which is uncompressed here) content
					hZipped := sha256.New()
					hZipped.Write(zippedData)
					actualContentHash := hex.EncodeToString(hZipped.Sum(nil))

					if hashFromTarget != actualContentHash {
						// Error if the content in the zip does not match the hash specified in its own filename (Target)
						t.Errorf("Integrity check failed for %s (target %s in %s): content hash (%s) does not match hash in target name (%s). Zipped content for debugging: <<%s>>",
							entry.Name, entry.Target, boundaryDataPath, actualContentHash, hashFromTarget, string(zippedData))
					}

					// Additionally, verify this target hash against the hash of the original file on disk.
					// This confirms that the file was hashed correctly during the uniconverter process.
					// entry.Name is relative to the boundary's root.
					// originalTestRootPath is the absolute path to the original overall input (e.g., testInputDir).
					
					// To reconstruct the original file's full path:
					// The current test setup (threshold=3) results in a single boundary where the boundary's root IS originalTestRootPath.
					// So, the relative path of the file within the boundary (entry.Name) is the same as its relative path from originalTestRootPath.
					originalFileFullPath := filepath.Join(originalTestRootPath, entry.Name)
					// If testing multiple boundaries, originalFileFullPath construction would be:
					// boundaryRootInDataDir := filepath.Join(testOutputDir, util.DataDir)
					// pathRelativeToDataDir, _ := filepath.Rel(boundaryRootInDataDir, boundaryDataPath)
					// originalFileFullPath = filepath.Join(originalTestRootPath, pathRelativeToDataDir, entry.Name)


					expectedHashFromOriginalFile, errGHF := util.GetFileHash(originalFileFullPath)
					if errGHF != nil {
						t.Errorf("Failed to get hash for original file %s (entry %s in %s): %v",
							originalFileFullPath, entry.Name, boundaryDataPath, errGHF)
					} else {
						if hashFromTarget != expectedHashFromOriginalFile {
							t.Errorf("Source hash mismatch for %s (target %s in %s). Target implies hash %s, but original file %s (path %s) hashes to %s.",
								entry.Name, entry.Target, boundaryDataPath, hashFromTarget, entry.Name, originalFileFullPath, expectedHashFromOriginalFile)
						}
					}
				}
			}
		}
		return true
	})

	// Check if all files in zip are accounted for in lookups (TargetFileCount)
	// This also implies metadata.TargetFileCount should match len(hashedTargetsInZip)
	if metadata.TargetFileCount != len(hashedTargetsInZip) {
		t.Errorf("Metadata TargetFileCount %d does not match actual unique files in zip %d for boundary %s",
			metadata.TargetFileCount, len(hashedTargetsInZip), boundaryDataPath)
	}
}

// TODO:
// 1. Add more test cases:
//    a. Different threshold sizes to create multiple boundaries.
//       - This will require careful prediction of `DetermineZipBoundaries` output.
//       - And multiple calls to `verifyArchiveSet` with correctly scoped `originalFilesInThisBoundary` maps.
//    b. Empty input directory.
//    c. Directory with only empty files.
//    d. Directory with duplicate files (content-wise) to check TargetFileCount vs TotalFileCount.
// 2. Check .staging directory is removed from the output.
// 3. (Optional) Check Inode values in lookups are unique if that's a strict requirement.
// 4. Test edge cases for file names (spaces, special characters) if not covered by basic test.
```
