package util

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dendrascience/dendra-archive-fuse/version"
)

type Metadata struct {
	CompressedSize   int       `json:"compressed_size"`
	DJAFSVersion     string    `json:"djafs_version"`
	NewestFileTS     time.Time `json:"newest_file_ts"`
	OldestFileTS     time.Time `json:"oldest_file_ts"`
	TargetFileCount  int       `json:"target_file_count"`
	TotalFileCount   int       `json:"total_file_count"`
	UncompressedSize int       `json:"uncompressed_size"`
}

// GetVersion returns the current djafs version string.
// It delegates to the version package to get the version information.
func GetVersion() string {
	return version.GetVersion()
}

// GenerateMetadata creates a Metadata struct from the lookup table.
// If path is provided, it reads the compressed size from the file at that path.
//
// Note: CompressedSize will be 0 if path is empty since compression hasn't occurred yet.
// Future optimization: incrementally update counters when adding files instead of
// recalculating from the full lookup table each time.
func (l LookupTable) GenerateMetadata(path string) (Metadata, error) {
	var m Metadata
	if path != "" {
		stat, err := os.Stat(path)
		if err != nil {
			return m, err
		}
		m.CompressedSize = int(stat.Size())
	}
	m.DJAFSVersion = GetVersion()
	m.NewestFileTS = l.GetNewestFileTS()
	m.OldestFileTS = l.GetOldestFileTS()
	m.TargetFileCount = l.GetTargetFileCount()
	m.TotalFileCount = l.GetTotalFileCount()
	m.UncompressedSize = l.GetUncompressedSize()
	return m, nil
}

func (m Metadata) Save(path string) error {
	if !strings.HasSuffix(path, "djfm") {
		path = filepath.Join(path, "metadata.djfm")
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	je := json.NewEncoder(f)
	err = je.Encode(m)
	return err
}

func (l LookupTable) Save(path string) error {
	if !strings.HasSuffix(path, "djfl") {
		path = filepath.Join(path, "lookup.djfl")
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	je := json.NewEncoder(f)
	err = je.Encode(l)
	return err
}
