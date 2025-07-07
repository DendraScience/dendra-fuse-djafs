package cmd

import (
	"testing"
)

func TestPathsOverlap(t *testing.T) {
	tests := []struct {
		name     string
		path1    string
		path2    string
		expected bool
	}{
		{
			name:     "identical paths",
			path1:    "/tmp/storage",
			path2:    "/tmp/storage",
			expected: true,
		},
		{
			name:     "path1 contains path2",
			path1:    "/tmp/storage/data",
			path2:    "/tmp/storage",
			expected: true,
		},
		{
			name:     "path2 contains path1",
			path1:    "/tmp/storage",
			path2:    "/tmp/storage/mount",
			expected: true,
		},
		{
			name:     "completely separate paths",
			path1:    "/tmp/storage",
			path2:    "/mnt/mount",
			expected: false,
		},
		{
			name:     "sibling directories",
			path1:    "/tmp/storage",
			path2:    "/tmp/mount",
			expected: false,
		},
		{
			name:     "relative paths - overlapping",
			path1:    "storage",
			path2:    "storage/mount",
			expected: true,
		},
		{
			name:     "relative paths - separate",
			path1:    "storage",
			path2:    "mount",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := pathsOverlap(tt.path1, tt.path2)
			if result != tt.expected {
				t.Errorf("pathsOverlap(%q, %q) = %v, expected %v", tt.path1, tt.path2, result, tt.expected)
			}
		})
	}
}
