package util

import (
	"os"
	"time"
)

type Metadata struct {
	CompressedSize   int
	DJFSVersion      string
	NewestFileTS     time.Time
	OldestFileTS     time.Time
	TargetFileCount  int
	TotalFileCount   int
	UncompressedSize int
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
	stat, err := os.Stat(path)
	if err != nil {
		return m, err
	}
	m.CompressedSize = int(stat.Size())
	m.DJFSVersion = GetVersion()
	m.NewestFileTS = l.GetNewestFileTS()
	m.OldestFileTS = l.GetOldestFileTS()
	m.TargetFileCount = l.GetTargetFileCount()
	m.TotalFileCount = l.GetTotalFileCount()
	m.UncompressedSize = l.GetUncompressedSize()
	return m, nil
}
