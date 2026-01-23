# DJAFS Storage Migration Guide

This guide covers migrating from a traditional filesystem to djafs-backed storage for queue-based data ingestion systems.

## Prerequisites

- Ubuntu 18.04+ LTS
- ext4 volume with sufficient storage (current data + 30% headroom)
- FUSE3 support
- Ability to pause/resume queue subscribers

## Available Commands

| Command | Usage | Description |
|---------|-------|-------------|
| `djafs mount` | `djafs mount STORAGE_PATH MOUNTPOINT` | Mount filesystem |
| `djafs convert` | `djafs convert -i INPUT -o OUTPUT [-v] [--dry-run] [--legacy]` | Convert existing data |
| `djafs validate` | `djafs validate -p PATH [-v] [-r]` | Validate archives, optionally repair |
| `djafs count` | `djafs count [PATH] [--progress]` | Count files in directory |

## Migration Phases

### Phase 1: Infrastructure Setup

| Step | Task | Verification |
|------|------|--------------|
| 1.1 | Provision Ubuntu 22.04+ LTS | SSH access works |
| 1.2 | Provision ext4 volume, mount at `/mnt/djafs-storage` | `df -Th` shows ext4 |
| 1.3 | `apt install fuse3 libfuse3-dev` | `fusermount3 --version` |
| 1.4 | Build/install djafs binary | `djafs --version` |
| 1.5 | `mkdir -p /mnt/djafs-mount` | Directory exists |
| 1.6 | `djafs mount /mnt/djafs-storage /mnt/djafs-mount` | `mount \| grep fuse` |

```bash
# Infrastructure setup
sudo apt update && sudo apt install -y fuse3 libfuse3-dev
sudo mkfs.ext4 /dev/sdX
sudo mount /dev/sdX /mnt/djafs-storage
mkdir -p /mnt/djafs-mount

# Mount djafs
djafs mount /mnt/djafs-storage /mnt/djafs-mount
```

### Phase 2: Initial Data Migration

| Step | Task | Verification |
|------|------|--------------|
| 2.1 | `rsync -avP --progress /source/ /mnt/djafs-mount/live/` | Completes (errors on new files OK) |
| 2.2 | Record baseline: `djafs count /mnt/djafs-mount/live --progress` | File count noted |

The first rsync will error on files being added during sync - this is expected. We'll do a final sync during the cutover window.

```bash
# Initial bulk sync (will have errors for new files - OK)
rsync -avP --progress /path/to/source/ /mnt/djafs-mount/live/

# Record baseline count
djafs count /mnt/djafs-mount/live --progress
```

### Phase 3: Queue Subscriber Setup

| Step | Task | Verification |
|------|------|--------------|
| 3.1 | Deploy new subscriber pointing to djafs mount | Service starts |
| 3.2 | Register consumer state in queue tracking | Consumer visible |
| 3.3 | Test write to djafs | Test message processed |

Configure your new subscriber to write to `/mnt/djafs-mount/live/` instead of the original storage location.

### Phase 4: Cutover

| Step | Task | Verification |
|------|------|--------------|
| 4.1 | Pause prod data sink queue | Messages accumulating |
| 4.2 | Pause test data sink queue | Messages accumulating |
| 4.3 | Wait for in-flight to drain | No active processing |
| 4.4 | `rsync -avP --delete /source/ /mnt/djafs-mount/live/` | "0 files transferred" |
| 4.5 | Unpause test queue | Test messages flowing |
| 4.6 | Monitor 15-30 min | No errors |
| 4.7 | Unpause prod queue | Prod messages flowing |

```bash
# Final sync (should show minimal or zero changes)
rsync -avP --delete /path/to/source/ /mnt/djafs-mount/live/

# Dry-run first to verify
rsync -avP --delete --dry-run /path/to/source/ /mnt/djafs-mount/live/
```

### Phase 5: Validation

| Step | Task | Verification |
|------|------|--------------|
| 5.1 | `djafs count /mnt/djafs-mount/live` vs source | Counts match |
| 5.2 | Sample 100 files, compare checksums | All match |
| 5.3 | Check `/mnt/djafs-mount/snapshots/` | Date hierarchy exists |
| 5.4 | `djafs validate -p /mnt/djafs-storage -v` | 0 errors |

```bash
# Validate archives
djafs validate -p /mnt/djafs-storage -v

# If issues found, attempt repair
djafs validate -p /mnt/djafs-storage -v -r

# Compare file counts
echo "Source: $(find /path/to/source -type f | wc -l)"
echo "DJAFS:  $(djafs count /mnt/djafs-mount/live)"

# Sample checksum comparison
for f in $(find /path/to/source -type f | shuf -n 100); do
  rel="${f#/path/to/source/}"
  src_hash=$(sha256sum "$f" | cut -d' ' -f1)
  dst_hash=$(sha256sum "/mnt/djafs-mount/live/$rel" | cut -d' ' -f1)
  if [ "$src_hash" != "$dst_hash" ]; then
    echo "MISMATCH: $rel"
  fi
done
```

## Rollback Plan

| Trigger | Action |
|---------|--------|
| djafs mount fails | Fall back to direct ext4 writes |
| Data corruption detected | Pause queues, restore from rsync source |
| Performance issues | Increase GC interval, tune hot cache |

To unmount and rollback:

```bash
# Graceful unmount
fusermount3 -u /mnt/djafs-mount

# Or force unmount if needed
fusermount3 -uz /mnt/djafs-mount
```

## Estimated Timeline

| Phase | Duration |
|-------|----------|
| Infrastructure setup | 1-2 hours |
| Initial rsync | Depends on data size |
| Subscriber setup | 30 min |
| Cutover window | 1-2 hours |
| Validation | 1 hour |

**Total: ~4-6 hours + initial rsync time**

## Post-Migration

### Monitoring

- Monitor queue lag to ensure backlog clears
- Watch djafs process for memory/CPU usage
- Check disk space on storage volume
- Verify snapshots are being created (check `/mnt/djafs-mount/snapshots/YYYY/MM/DD/`)

### Maintenance

```bash
# Periodic validation
djafs validate -p /mnt/djafs-storage -v

# Check storage health
df -h /mnt/djafs-storage
```

### Snapshot Access

Files are accessible at multiple points in time:

```
/mnt/djafs-mount/
├── live/                    # Current state
└── snapshots/
    ├── latest/              # Most recent state
    └── YYYY/
        └── MM/
            └── DD/          # State at end of that day
```
