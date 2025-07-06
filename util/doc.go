// Package util provides core utilities and data structures for the djafs filesystem.
//
// This package contains the fundamental building blocks for djafs operations including
// file hashing, compression, lookup table management, metadata generation, and archive
// creation. It handles the low-level operations that enable the filesystem's
// content-addressable storage and compression capabilities.
//
// Key Components:
//
// File Hashing and Organization:
//   - SHA-256 based content addressing with configurable modulus (GlobalModulus = 5000)
//   - Concurrent file processing for performance
//   - Hash-based directory organization for efficient storage
//
// Compression and Archives:
//   - DJFZ compressed archive format for JSON files
//   - ZIP-based storage with gzip compression
//   - Archive validation and integrity checking
//
// Lookup Tables:
//   - LookupTable and LookupEntry types for file mapping
//   - Efficient file deduplication and metadata tracking
//   - Timestamp tracking for oldest/newest files
//
// Workspace Management:
//   - Hot cache (.work) for new file writes
//   - Background garbage collection and archive packing
//   - Concurrent processing for optimal performance
//
// Metadata and Versioning:
//   - Archive metadata generation with file counts and timestamps
//   - Version tracking for compatibility
//   - JSON-based persistence for all metadata
//
// Directory Analysis:
//   - Intelligent archive boundary determination
//   - File count-based optimization for archive sizes
//   - Recursive directory traversal with configurable thresholds
//
// The package is designed to be thread-safe and performant, with extensive use
// of goroutines for concurrent operations where appropriate.
package util