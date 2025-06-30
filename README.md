# djafs - the Dendra JSON Archive File System

**djafs** (DeeJay-fs) is a high-performance FUSE-based filesystem that provides compressed, content-addressable storage for JSON files with time-travel capabilities.

## Table of Contents

- [Overview](#overview)
- [The Problem](#the-problem)
- [Solution Architecture](#solution-architecture)
- [FUSE Technology](#fuse-technology)
- [System Design](#system-design)
- [File Formats](#file-formats)
- [Usage Examples](#usage-examples)
- [Implementation Status](#implementation-status)
- [Development Roadmap](#development-roadmap)
- [Technical References](#technical-references)

## Overview

djafs solves the problem of efficiently storing and accessing large volumes of compressible JSON data while maintaining filesystem semantics and providing advanced features like point-in-time snapshots.

### Key Features

- **Transparent Compression**: JSON files are automatically compressed without changing application interfaces
- **Content-Addressable Storage**: Eliminates data duplication using SHA-256 hashing
- **Time-Travel Snapshots**: View filesystem state at any point in time
- **High Performance**: Optimized for both read and write operations
- **Backup-Friendly**: Non-opaque storage format allows manual recovery
- **FUSE-Based**: Standard filesystem interface compatible with all applications

### Use Cases

- **Time-Series Data**: IoT sensor readings, metrics, logs
- **Event Sourcing**: Application events and state changes
- **Archive Storage**: Long-term retention of structured data
- **Data Lakes**: Structured data storage with efficient compression

## The Problem

Traditional approaches to storing JSON time-series data face several challenges:

```ascii
Current Structure:
archive/
├── 2024/
│   ├── 01/
│   │   ├── 01/
│   │   │   ├── sensor_001_1704067200.json  (12KB)
│   │   │   ├── sensor_001_1704067260.json  (12KB)
│   │   │   └── sensor_001_1704067320.json  (12KB)
│   │   └── 02/
│   └── 02/
└── 2023/
```

**Problems:**

- **Storage Inefficiency**: JSON files are highly compressible but stored uncompressed
- **Inode Exhaustion**: Millions of small files can exhaust filesystem inodes
- **Backup Overhead**: Many small files slow down backup operations
- **No Deduplication**: Identical or similar content is stored multiple times
- **Limited Snapshots**: No easy way to view historical filesystem states

## Solution Architecture

djafs transforms the storage model while maintaining the same access patterns:

```ascii
FUSE Interface (what applications see):
/mnt/djafs/
├── live/                    <- Current active data
│   ├── 2024/01/01/
│   │   ├── sensor_001_1704067200.json
│   │   ├── sensor_001_1704067260.json
│   │   └── sensor_001_1704067320.json
│   └── 2024/01/02/
└── snapshots/               <- Time-travel interface
    ├── 2024/01/01/
    ├── 2024/01/02/
    └── 2024/02/02/

Backend Storage (actual disk layout):
/data/djafs/
├── hot_cache/               <- Write buffer
├── archive_2024_01.djfz     <- Compressed archives
├── archive_2024_02.djfz
└── workdir/                 <- Content-addressable storage
    ├── a1/
    │   └── a1b2c3...def.json    <- Hashed files
    └── b2/
        └── b2c3d4...abc.json
```

## FUSE Technology

### What is FUSE?

FUSE (Filesystem in Userspace) is a software interface that allows non-privileged users to create their own file systems without editing kernel code. It works by:

1. **Kernel Module**: A thin kernel module that receives filesystem calls
2. **User Space Daemon**: Your custom filesystem implementation
3. **Protocol Bridge**: Communication between kernel and userspace via `/dev/fuse`

### How djafs Uses FUSE

When a user runs `cat /mnt/djafs/live/2024/01/01/sensor_001.json`:

1. Kernel receives read() syscall
2. FUSE kernel module forwards to djafs daemon
3. djafs daemon:
   a. Looks up file in lookup table
   b. Finds hash: a1b2c3...def
   c. Decompresses archive containing the file
   d. Returns content to kernel
4. Kernel returns data to application

### FUSE Implementation (bazil.org/fuse)

djafs uses the [bazil.org/fuse](https://github.com/bazil/fuse) library, a pure Go implementation of the FUSE protocol that doesn't rely on the C FUSE library.

**Key Components:**

- **fs.FS**: Root filesystem interface
- **fs.Node**: Represents files and directories
- **fs.Handle**: Represents opened files
- **Lookup/Read/Write**: Core filesystem operations

## System Design

### Four Data Pools

djafs architecture consists of four interconnected data storage systems:

#### 1. FUSE Interface Layer

The user-facing filesystem that maintains familiar directory structures:

- **`/live/`**: Current active data with standard hierarchy
- **`/snapshots/`**: Time-based views generated on-demand
- **Virtual Directories**: Dynamically created based on lookup tables
- **Standard Operations**: Full support for read, write, stat, readdir

#### 2. Content-Addressable Storage (CAS)

Files are stored by their SHA-256 hash to eliminate duplication:

```ascii
workdir/
├── a1/
│   ├── a1b2c3d4e5f6789abcdef012345.json    <- Original: sensor_001_1704067200.json
│   └── a1f7e8d9c2b3a4f5e6d7c8b9a0f.json    <- Original: sensor_002_1704067200.json
└── b2/
    └── b2c3d4e5f6789abcdef012345a1b.json    <- Original: sensor_001_1704067260.json
```

**Benefits:**

- **Automatic Deduplication**: Identical files stored only once
- **Integrity Checking**: Hash verification prevents corruption
- **Efficient Storage**: Only unique content consumes space

#### 3. Compressed Archives

Related files are grouped into compressed archives for optimal storage efficiency:

```ascii
archive_2024_01_week_1.djfz    <- ZIP archive containing:
├── lookups.djfl               <- JSON lookup table
├── metadata.djfm              <- Archive metadata
├── a1b2c3d4e5f6789abcdef012345.json
├── a1f7e8d9c2b3a4f5e6d7c8b9a0f.json
└── b2c3d4e5f6789abcdef012345a1b.json
```

**Compression Strategy:**

- **Time-Based Grouping**: Files from similar time periods compress better
- **Configurable Periods**: Weekly, monthly, or custom grouping
- **Standard ZIP Format**: No proprietary formats for maximum recoverability

#### 4. Hot Cache System

A write-through cache that optimizes write performance:

```ascii
hot_cache/
├── incoming/                  <- New files land here first
│   ├── sensor_001_1704067380.json
│   └── sensor_002_1704067380.json
└── staging/                   <- Files being processed by GC
```

**Write Flow:**

1. New file written to `hot_cache/incoming/`
2. Write completes immediately (fast response)
3. Background garbage collector:
   - Computes SHA-256 hash
   - Moves to content-addressable storage
   - Updates lookup tables
   - Adds to compressed archive
   - Removes from hot cache

### Lookup Tables

Lookup tables map human-readable filenames to content-addressable hashes:

```json
{
  "entries": [
    {
      "name": "2024/01/01/sensor_001_1704067200.json",
      "target": "a1b2c3d4e5f6789abcdef012345.json",
      "size": 12484,
      "modified": "2024-01-01T12:00:00Z",
      "inode": 100001
    },
    {
      "name": "2024/01/01/sensor_001_1704067260.json",
      "target": "b2c3d4e5f6789abcdef012345a1b.json",
      "size": 12490,
      "modified": "2024-01-01T12:01:00Z",
      "inode": 100002
    }
  ],
  "sorted": true
}
```

**Snapshot Functionality:**

- Lookup tables are append-only logs
- To view snapshots, read entries up to specific timestamp
- Deleted files have empty `target` field
- Modified files create new entries without deleting old content

### File Resolution Algorithm

One of the most elegant aspects of djafs is how it resolves which zip archive contains a specific file **without requiring a master index**. The backing filesystem directory structure itself serves as the index.

#### The "Dead End" Detection Method

When looking for a file like `/sensors/location1/device5/reading.json`:

1. **Walk down the backing filesystem**: `.data/sensors/location1/device5/`
2. **Hit a "dead end"**: The directory doesn't exist (because it was a zip boundary)
3. **Back up one level**: `.data/sensors/location1/` exists
4. **Check the sibling lookup table**: `.data/sensors/location1/lookups.djfl`
5. **Find the file entry**: The lookup table contains `device5/reading.json`

#### Example Walkthrough

**Original files:**

```
/sensors/location1/device5/reading.json
/sensors/location1/device5/config.json
/sensors/location1/device6/reading.json
/sensors/location1/summary.json
```

**After zip boundary determination:**

```
.data/
└── sensors/location1/
    ├── lookups.djfl     <- Contains: device5/reading.json, device5/config.json,
    │                                 device6/reading.json, summary.json
    └── files.djfz       <- Compressed archive
```

**File lookup for `/sensors/location1/device5/reading.json`:**

1. Try to access `.data/sensors/location1/device5/` → **Dead end!**
2. Back up to `.data/sensors/location1/` → **Exists!**
3. Open `.data/sensors/location1/lookups.djfl`
4. Search for entry with `name: "device5/reading.json"`
5. Extract from `files.djfz` using the target hash

#### Why This Works

- **Self-Indexing**: The filesystem structure eliminates the need for separate index files
- **O(path-depth) Lookup**: Maximum directory traversals equal to path depth
- **No Master Index**: Each boundary is self-contained with its own lookup table
- **Intuitive**: The "dead end" naturally points to the exact lookup table containing your file

This approach scales efficiently even with thousands of zip boundaries across a deep directory tree.

### Metadata Files

Each archive includes metadata for performance optimization:

```json
{
  "djafs_version": "1.0.0",
  "compressed_size": 2457600,
  "uncompressed_size": 8392704,
  "total_file_count": 1440,
  "target_file_count": 1200,
  "oldest_file_ts": "2024-01-01T00:00:00Z",
  "newest_file_ts": "2024-01-07T23:59:59Z"
}
```

## File Formats

### Extension Conventions

- **`.djfz`**: Compressed archive files (ZIP format)
- **`.djfl`**: JSON lookup table files
- **`.djfm`**: JSON metadata files

### Archive Structure

Each `.djfz` file contains:

```ascii
archive_2024_01_week_1.djfz
├── lookups.djfl              <- Lookup table for this archive
├── metadata.djfm             <- Archive metadata
├── <hash1>.json              <- Content-addressable files
├── <hash2>.json
└── <hashN>.json
```

## Quick Start

### Prerequisites

- Go 1.24.4 or later
- FUSE support on your system:
  - **Linux**: `sudo apt-get install fuse` or `sudo yum install fuse`
  - **macOS**: Install [FUSE for macOS](https://osxfuse.github.io/)
  - **FreeBSD**: FUSE is included in base system

### Building and Running

```bash
# Clone the repository
git clone https://github.com/your-org/dendra-fuse-djafs
cd dendra-fuse-djafs

# Build the filesystem
go build -o djafs .

# Create a mount point
mkdir /tmp/djafs-mount

# Mount the filesystem
./djafs /tmp/djafs-mount

# In another terminal, use the filesystem
echo '{"temperature": 23.5, "timestamp": "2024-01-01T12:00:00Z"}' > /tmp/djafs-mount/live/2024/01/01/sensor.json
cat /tmp/djafs-mount/live/2024/01/01/sensor.json

# Unmount when done
fusermount -u /tmp/djafs-mount  # Linux
umount /tmp/djafs-mount         # macOS/FreeBSD
```

## Usage Examples

### Basic Operations

```bash
# Mount the filesystem
./djafs /mnt/djafs

# Write a file (goes to hot cache)
echo '{"sensor_id": "001", "value": 23.5}' > /mnt/djafs/live/2024/01/01/reading.json

# Read the file (transparent decompression)
cat /mnt/djafs/live/2024/01/01/reading.json

# List current files
ls -la /mnt/djafs/live/2024/01/01/

# View snapshots
ls /mnt/djafs/snapshots/
ls /mnt/djafs/snapshots/2024-01-01T12:00:00Z/2024/01/01/
```

### Time-Travel Snapshots

```bash
# View filesystem as it was at noon on Jan 1st
cd /mnt/djafs/snapshots/2024-01-01T12:00:00Z/
ls 2024/01/01/                 # Only files that existed at that time

# Compare different points in time
diff /mnt/djafs/snapshots/2024-01-01T12:00:00Z/2024/01/01/data.json \
     /mnt/djafs/snapshots/2024-01-01T18:00:00Z/2024/01/01/data.json
```

### Backup Operations

```bash
# Pause garbage collection for consistent backup
killall -USR1 djafs

# Backup the actual storage (much smaller than original)
rsync -av /data/djafs/ backup_location/

# Resume garbage collection
killall -USR2 djafs
```

## Implementation Status

### ✅ Completed Components

- **Utility Functions** (`util/` package):

  - SHA-256 hashing with content-addressable storage
  - ZIP compression/decompression
  - Lookup table management
  - Metadata generation
  - File counting and validation

- **Core Data Structures**:
  - `LookupEntry` and `LookupTable` types
  - `Metadata` structure with JSON serialization
  - DJFZ archive handling

### 🔄 In Progress

- **FUSE Filesystem Interface** (`main.go`):
  - Basic FUSE mounting infrastructure
  - Command-line interface
  - Signal handling for graceful shutdown

### ❌ Pending Implementation

- **Complete FUSE Operations**:

  - Directory listing (`ReadDir`)
  - File lookup (`Lookup`)
  - File reading (`Read`, `Open`)
  - File writing (`Write`, `Create`)
  - File metadata (`Attr`, `Getattr`)

- **Snapshot System**:

  - Virtual snapshot directory generation
  - Time-based file filtering
  - Snapshot browsing interface

- **Hot Cache Management**:

  - Background garbage collection
  - Write-through caching
  - Archive generation and compression

- **Advanced Features**:
  - Backup pause/resume functionality
  - Performance monitoring
  - Error recovery mechanisms

## Development Roadmap

### Phase 1: Core Filesystem ✅

- [x] Utility functions and data structures
- [x] SHA-256 hashing and content addressing
- [x] ZIP compression/decompression
- [x] Lookup table management
- [x] Basic FUSE mounting

### Phase 2: Basic Operations 🔄

- [ ] Implement FUSE `Lookup` operation
- [ ] Implement FUSE `Read` and `Open` operations
- [ ] Implement FUSE `ReadDir` for directory listing
- [ ] Implement FUSE `Attr` for file metadata
- [ ] Basic file reading from archives

### Phase 3: Write Operations

- [ ] Implement hot cache system
- [ ] Implement FUSE `Write` and `Create` operations
- [ ] Background garbage collection process
- [ ] Archive generation and compression
- [ ] Lookup table updates

### Phase 4: Snapshot System

- [ ] Virtual snapshot directory generation
- [ ] Time-based file filtering
- [ ] Historical lookup table parsing
- [ ] Snapshot browsing interface

### Phase 5: Production Features

- [ ] Backup pause/resume signals
- [ ] Performance monitoring and metrics
- [ ] Error recovery and fault tolerance
- [ ] Configuration management
- [ ] Comprehensive testing suite

### Phase 6: Optimizations

- [ ] Read caching and LRU eviction
- [ ] Compression ratio optimization
- [ ] Memory usage optimization
- [ ] Concurrent operation support

## Technical References

### FUSE Documentation

- [FUSE Tutorial by Joseph Pfeiffer](https://www.cs.nmsu.edu/~pfeiffer/fuse-tutorial/html/index.html) - Comprehensive FUSE development guide
- [bazil.org/fuse Documentation](https://pkg.go.dev/bazil.org/fuse) - Go FUSE library documentation
- [bazil.org/fuse Examples](https://github.com/bazil/fuse/tree/master/examples) - Example FUSE implementations

### Reference Implementations

- [hellofs](https://github.com/bazil/fuse/blob/master/examples/hellofs/hello.go) - Simple FUSE filesystem example
- [zipfs](https://github.com/bazil/zipfs) - FUSE filesystem serving ZIP archives
- [Writing Filesystems in Go with FUSE](https://blog.gopheracademy.com/advent-2014/fuse-zipfs/) - Detailed tutorial

### Architecture Inspiration

- **InfluxDB**: Write-through caching and garbage collection patterns
- **IPFS**: Content-addressable storage design
- **Git**: Object storage and content hashing
- **ZFS**: Snapshot and deduplication concepts

### Development Tools

- [FUSE Debug Mode](https://github.com/bazil/fuse#debugging): Enable with `-o debug` for operation tracing
- [Go Race Detector](https://golang.org/doc/articles/race_detector.html): Essential for concurrent FUSE operations
- [Bazil Project](https://bazil.org/): Distributed filesystem using similar technologies

## Performance Considerations

### Write Performance

- **Hot Cache**: New writes complete immediately to local cache
- **Batched Compression**: Files are compressed in groups for better ratios
- **Background Processing**: Garbage collection runs asynchronously

### Read Performance

- **Decompression Caching**: Recently accessed archives stay decompressed in memory
- **Lookup Table Optimization**: Sorted lookup tables enable binary search
- **Content Addressing**: Duplicate content is stored only once

### Storage Efficiency

- **Compression Ratios**: JSON typically compresses 5-10x with gzip
- **Deduplication**: Identical files consume zero additional storage
- **Time-Based Grouping**: Similar files compress better when archived together

### Scalability Limits

- **Memory Usage**: Proportional to number of open archives and cache size
- **File Count**: Lookup tables support millions of entries efficiently
- **Archive Size**: Individual archives should stay under 1GB for optimal performance

---

_djafs_ - Efficient, compressed, time-travel enabled storage for JSON archives.
