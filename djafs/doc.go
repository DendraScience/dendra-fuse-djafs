// Package djafs implements a FUSE-based filesystem for compressed, content-addressable JSON storage.
//
// This package provides the core filesystem implementation that enables transparent
// JSON compression, content-addressable storage, and time-travel capabilities through
// snapshots. The filesystem is designed for high-performance read/write operations
// with background garbage collection.
//
// Key Features:
//   - Transparent JSON compression using gzip
//   - Content-addressable storage with SHA-256 hashing
//   - Time-travel snapshots for historical data access
//   - Hot cache for efficient write operations
//   - Background garbage collection for storage optimization
//   - POSIX-compliant filesystem interface via FUSE
//
// The filesystem stores data in a structured format:
//   - .data/: Contains compressed JSON files indexed by hash
//   - .work/: Hot cache for new writes before garbage collection
//   - .mappings/: Lookup tables mapping file paths to content hashes
//
// The main entry point is NewFS() which creates a new filesystem instance
// that can be mounted using the bazil.org/fuse library.
package djafs
