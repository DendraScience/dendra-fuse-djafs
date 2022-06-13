
## Background info
Original documentation on fuse:
[fuse at nmsu](https://www.cs.nmsu.edu/~pfeiffer/fuse-tutorial/html/index.html)

### Bazel example
[link to hellofs docs](https://pkg.go.dev/bazil.org/fuse@v0.0.0-20200524192727-fb710f7dfd05/examples/hellofs)
[link to gh repo](https://github.com/bazil/fuse/blob/fb710f7dfd05/examples/hellofs/hello.go)

### Blog Example
[link to blog](https://blog.trieoflogs.com/2021-05-25-fuse-filesystem-go/)

Note, the blog example is the one that will be followed when building the fuse driver.
However, at early inspection, there's a notable bug in it: the inode counter doesn't use an atomic counter, but it should.

## Problem
The archive is currently a simple, file-backed system.
Nested year/month/day folders separate json files.
These json files are taking up far too much space, and are highly compressible.
It would be great to store the files compressed, and decompress them as needed on the fly when needed.

## Constraints
Must haves:
- As this is a filesystem, it needs to be performant
  - We need to be able to access files quickly
  - We need to be able to store files quickly
- We shouldn't store files in an opaque format in case the driver breaks
- We need an easily-backed-up system
- Good documentation. See: README.md

Nice to haves:
- We would like to be able to see a snapshot of the archive as it was for any given day
- We would like the driver to be extensible such that file creation can be blocked
  - block creation of empty file on top of non-empty file


## Architecture / Design

### Similar work
As with anything, let's use the same principles used elsewhere in software architecture.
This architecture is similar to the following concepts:
 - influxdb write-through caching / garbage collection
 - inode / dirent scaffolding
 - garbage collection
 - content-addressable hashing (ipfs)
 - library of babel
 - heap / stack memory model
 - probably a lot more





