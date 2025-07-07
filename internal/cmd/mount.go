package cmd

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	_ "bazil.org/fuse/fs/fstestutil"
	"github.com/dendrascience/dendra-archive-fuse/djafs"
	"github.com/dendrascience/dendra-archive-fuse/version"
	"github.com/spf13/cobra"
)

// NewMountCmd creates and returns the mount subcommand for the djafs CLI.
// It handles mounting djafs filesystems at specified mountpoints.
func NewMountCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mount STORAGE_PATH MOUNTPOINT",
		Short: "Mount a djafs filesystem",
		Long: `Mount a djafs filesystem at the specified mountpoint.

STORAGE_PATH is the path to the djafs storage directory.
MOUNTPOINT is the directory where the filesystem will be mounted.`,
		Args: cobra.ExactArgs(2),
		Run:  runMount,
	}
}

// pathsOverlap checks if two paths overlap (one contains the other).
// It returns true if either path is a parent/child of the other.
func pathsOverlap(path1, path2 string) bool {
	// Convert to absolute paths and clean them
	abs1, err1 := filepath.Abs(path1)
	abs2, err2 := filepath.Abs(path2)
	
	// If we can't resolve absolute paths, compare as-is
	if err1 != nil {
		abs1 = filepath.Clean(path1)
	}
	if err2 != nil {
		abs2 = filepath.Clean(path2)
	}
	
	// Ensure paths end with separator for accurate prefix checking
	if !strings.HasSuffix(abs1, string(filepath.Separator)) {
		abs1 += string(filepath.Separator)
	}
	if !strings.HasSuffix(abs2, string(filepath.Separator)) {
		abs2 += string(filepath.Separator)
	}
	
	// Check if either path is a prefix of the other
	return strings.HasPrefix(abs1, abs2) || strings.HasPrefix(abs2, abs1)
}

func runMount(cmd *cobra.Command, args []string) {
	// Print version info on startup
	fmt.Printf("djafs %s starting...\n", version.GetFullVersion())

	storagePath := args[0]
	mountpoint := args[1]

	// Validate that storage path and mountpoint don't overlap
	if pathsOverlap(storagePath, mountpoint) {
		log.Fatalf("Storage path and mountpoint cannot overlap: storage=%s, mount=%s", storagePath, mountpoint)
	}

	// Ensure storage directory exists
	if err := os.MkdirAll(storagePath, 0755); err != nil {
		log.Fatalf("Failed to create storage directory: %v", err)
	}

	// Create filesystem instance
	filesystem := djafs.NewFS(storagePath)

	c, err := fuse.Mount(
		mountpoint,
		fuse.FSName("djafs"),
		fuse.Subtype("djafs"),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)
	go func() {
		<-sigChan
		log.Println("Received interrupt signal, shutting down...")

		// Stop filesystem gracefully
		filesystem.Stop()

		// Unmount filesystem
		fuse.Unmount(mountpoint)
		c.Close()

		log.Println("Shutdown complete")
		os.Exit(0)
	}()

	log.Printf("djafs %s mounted at %s (storage: %s)", version.GetVersion(), mountpoint, storagePath)
	err = fs.Serve(c, filesystem)
	if err != nil {
		log.Fatal(err)
	}
}
