package util

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
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

func GetVersion() string {
	return "development"
}

// TODO compressed size can't be done until the compression can take place
// TODO there are some optimizations to be made when adding new files here, such
// as incrementing counters and size provided a file is new, etc. instead of calling
// LT member funcs

// Using a Lookup Struct and passing the path to a zip file, we can create the
// metadata struct file to accompany the zip
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
