// Package main provides the djafs command-line interface.
//
// djafs is a FUSE-based filesystem for compressed, content-addressable JSON storage.
// It provides transparent JSON compression, content-addressable storage, and time-travel
// capabilities through snapshots. The filesystem supports high-performance read/write
// operations with background garbage collection.
//
// The main binary supports multiple subcommands:
//   - mount: Mount a djafs filesystem at a specified mountpoint
//   - convert: Convert existing JSON directory trees to djafs format
//   - validate: Validate djafs archives for corruption and consistency
//   - count: Count files in directory trees
package main
