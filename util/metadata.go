package util

import "time"

type Metadata struct {
	DJFSVersion      string
	NewestFileTS     time.Time
	OldestFileTS     time.Time
	CompressedSize   int
	UncompressedSize int
	FileCount        int
	UniqueCount      int
}

func GetVersion() string {
	return "development"
}

// TODO compressed size can't be done until the compression can take place
func GenerateMetadata(path string) (Metadata, error) {
	var m Metadata
	m.DJFSVersion = GetVersion()
	return m, nil
}
