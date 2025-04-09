# djafs - the Dendra JSON Archive File System

(DeeJay-fs)

## Background info

Original documentation on fuse:
[fuse at nmsu](https://www.cs.nmsu.edu/~pfeiffer/fuse-tutorial/html/index.html)

### Bazel example

[link to hellofs docs](https://pkg.go.dev/bazil.org/fuse@v0.0.0-20200524192727-fb710f7dfd05/examples/hellofs)
[link to gh repo](https://github.com/bazil/fuse/blob/fb710f7dfd05/examples/hellofs/hello.go)
[Bazel ZipFS](https://github.com/bazil/zipfs)

### Blog Example

[link to blog](https://blog.trieoflogs.com/2021-05-25-fuse-filesystem-go/)

Note, the blog example is the one that will be followed when building
the fuse driver. However, at early inspection, there's a notable bug
in it: the inode counter doesn't use an atomic counter, but it should.

## Problem

The archive is currently a simple, file-backed system.
Nested year/month/day folders separate json files.
These json files are taking up far too much space, and are highly compressible.
It would be great to store the files compressed, and decompress them
on the fly when needed.

## Constraints

Must haves:

- As this is a filesystem, it needs to be performant
  - We need to be able to access files quickly
  - We need to be able to store files quickly
- We shouldn't store files in an opaque format in case the driver breaks
- We need an easily-backed-up system
- Good documentation. See: README.md

Nice to haves:

- We would like to be able to see a snapshot of the archive
  as it was for any given day
- We would like the driver to be extensible such that file
  creation can be blocked
  - block creation of empty file on top of non-empty file

## Architecture / Design

### Similar work

As with anything, let's use the same principles used elsewhere
in software architecture.
This architecture is similar to the following concepts:

- influxdb write-through caching / garbage collection
- inode / dirent scaffolding
- garbage collection
- content-addressable hashing (ipfs)
- library of babel
- heap / stack memory model
- probably a lot more

### Assumptions

- To make things simpler, when viewing snapshots, we assume no time-series
  data could have been generated for a date more than 24 hours in
  the future from the snapshot date.

### Design

There are four notable pools of data which are relevant to djafs.

1. FUSE Interface.
   The interface will follow the exact same directory structure as what's
   currently used, with nested directories of json.
   One notable exception is that there will be one additional top-level
   directory, "live" above the nested directories.
   The mountpoint for the JSON Archive API will be the "live" directory.
   At the same level as the "live" folder, will be a directory
   called "snapshots".
   Inside of snapshots, a library of babel-style directory tree
   "exists" (is generated on-the-fly).
   To see a snapshot of data for a given day, `cd` into the directory with
   that date as the path (i.e. `cd snapshots/2022/07/13/data`)
   As the actual data is not stored in the interface, the interface itself
   can be on a host drive (only a mountpoint is required.)
1. Data Backend.
   The data backend follows the same directory structure as the current data,
   but is gzipped at the weekly (or monthly) level.
   The exact splitting level will require analysis to determine the optimal
   tradeoff between memory, speed, and storage.
   Packing more days of data into the same files will give better compression
   and fewer real inodes, but will require more memory and CPU to
   pack and unpack.
   As the data itself is all zip files, no external software besides the
   coreutils should be required to manually intervene if necessary.
1. Lookup data.
   When data is added to the archive, if the timestamp is used as the filename,
   it may overwrite previous content, which makes snapshots impossible.
   Instead, added files are hashed using SHA-256, and stored as `<HASH>.<ext>`.
   Since the filename was the timestamp and had special meaning, the
   correlation between the hashed file's name and timestamp
   needs to be recorded.
   Because there are potentially millions of files, this data should be
   stored compressed as well.
   A binary data format would work well for this, but breaks the rule of
   "no external tools or data formats", as a corruption of the binary file
   would result in data loss.
   Instead, a tradeoff will be made, and the inode / dirent model is followed:
   there is no global metadata file, instead, each gzip file contains its
   own lookup table file.
   The lookup table file will be JSON, and get compressed alongside the data,
   into a file named `inode.djfl` (since it roughly corresponds to some of the
   responsibilities an inode would have).
   The lookup records will contain a list of pairings from the filenames
   that should exist for a directory of data to the backing hashed files.
   The lookup records will be arranged as an array of objects, containing
   the filename, target, and modification date (as a unix timestamp)
   for the record.
   To parse the file properly, the array of data will be read in from
   beginning to end, with newer records overriding the previous ones.
   In the special case of a file deletion, the target will be the empty
   string instead of a hashed file's name.
   Notably, when a file is "modified", the original data is never overwritten,
   as it is a hashed file; instead the new file is stored next to it and referenced.
   To parse the file to a specific snapshot in time, the metadata file will be
   parsed up until the first record which has a date after the snapshot date.
   To reduce the number of entries in the metadata file, consecutive entries
   for a given filename which point to the same hashed file target can have
   the second entry eliminated since it's effectively a statement of "no change".
1. Hot Cache.
   Writing files to any filesystem can be slow if writes are synchronous,
   especially if the filesystem in question is a hashing compression filesystem.
   Additionally, the reading patterns of the filesystem are known to be
   heavily sequential, so to optimize for CPU and memory, it makes sense to
   cache the decompression artifacts.
   An extra folder for reads and writes will be created alongside
   the backing data for this purpose.
   When a file is added to the archive, the direct file operations will only
   add it to the hot cache folder, writing it through without
   hashing or compressing.
   Periodically, a "garbage collection" process will collect all newly added
   files from the hot cache and bake them into the data backend by hashing
   them, decompressing the relevant archive, adding an entry to the
   metadata file, and recompressing the archive.
   Notably, functionality for backups will be made available, which
   temporarily disables the garbage collection procedure.
   This is so that when an external process copies the backing
   files somewhere else, there's not a moving target.
   This functionality is preferred over building a backup solution into the
   fuse driver itself because the backup solution may change over time, and
   changing the backup logic would require taking down the driver
   and bringing it back up.
   Additionally, the chance of critical errors occurring increases exponentially
   when a filesystem driver now must communicate over the internet.`
   When a file is read from the archive, the zip file is decompressed and the
   manifest is parsed as described above.
   The destination location for the archive expansion is marked in an in-memory
   data structure and scheduled for deletion in an LRU-style queue.

### Filesystem Structure

Compressed files use the extension `.djfz` (zip files)
Lookup files will use the extension `.djfl` (json files)
Metadata files will use the extension `.djfm` (json files)

Here's how the file structure will look:

The following tree output:

```ascii
.
├── bin
│   └── main.go
├── cmd/
│   ├── x.go
│   ├── converter
│   │   └── main.go
│   ├── counter/
│   │   ├── counter
│   │   └── main.go
│   └── uniconverter/
│       └── main.go
├── go.mod
├── go.sum
├── main.go
├── Makefile
├── README.md
└── util/
    ├── compress.go
    ├── compress_test.go
    ├── hasher.go
    ├── inode.go
    ├── metadata.go
    ├── repack.go
    ├── repack_test.go
    ├── subfile.go
    └── subfile_test.go

```

Would be transformed as follows (presuming the max is between 5-12):

```ascii
.
├── lookups.djfl
├── metadata.djfm
├── files.djfz
├── bin/
│   ├── lookups.djfl
│   ├── metadata.djfm
│   └── files.djfz
├── cmd/
│   ├── lookups.djfl
│   ├── metadata.djfm
│   └── files.djfz
└── util/
    ├── lookups.djfl
    ├── metadata.djfm
    └── files.djfz
```

Notice:

- All root files are always collapsed
- All subdirectories are collapsed up until they hit the max filecap
- Notice that the bin folder only had one file before, and now it has 3. This is a result of the following rules:
  - Any time there are directories which are siblings to files.djfz, the files.djfz will only contain top-level files
  - As a corollary, any time a Directory A contains Directory B and C, and sizeof(directory B) + sizeof(directory C) + files in A > max,
    - Each subdirectory gets its own toplevel structure, regardless of true contents count
- Finally, notice how x.go got folded into the same backing structure as its sibling subdirs

Uncompressed metadata files will contain the following data for quick summary/metrics calculations:

- Version of djafs used to pack the archive (useful for migrations)
- Last update (useful for timestamps)
- Oldest file timestamp
- Compressed file Size
- Uncompressed file size
- Count of Files
- Count of Unique files

## Development Roadmap

1. Build out "utility functions" and tests
  a. Count all files under a subfolder until X
  a. Metadata generator
  a. sha256 renamer + lookup table creator
1. Create loader
1. Create unloader
1. Create repacker
