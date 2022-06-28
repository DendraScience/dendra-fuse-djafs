package util

import "time"

type Metadata struct {
	DJFSVersion      string
	NewestFileTS     time.Time
	OldestFileTS     time.Time
	CompressedSize   int
	UncompressedSize int
	TotalFileCount   int
	TargetFileCount  int
}

func GetVersion() string {
	return "development"
}

// TODO compressed size can't be done until the compression can take place
// TODO there are some optimizations to be made when adding new files here, such
// as incrementing counters and size provided a file is new, etc. instead of calling
// LT member funcs

func (l LookupTable) GenerateMetadata(path string) (Metadata, error) {
	var m Metadata
	m.DJFSVersion = GetVersion()
	m.OldestFileTS = l.GetOldestFileTS()
	m.NewestFileTS = l.GetNewestFileTS()
	m.UncompressedSize = l.GetUncompressedSize()
	m.TotalFileCount = l.GetTotalFileCount()
	m.TargetFileCount = l.GetTargetFileCount()
	return m, nil
}
