package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	_ "bazil.org/fuse/fs/fstestutil"
	"github.com/charmbracelet/fang"
	"github.com/dendrascience/dendra-archive-fuse/djafs"
	"github.com/dendrascience/dendra-archive-fuse/version"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "djafs STORAGE_PATH MOUNTPOINT",
	Short: "Mount a djafs filesystem",
	Long: `djafs is a FUSE-based filesystem for compressed, content-addressable JSON storage.

STORAGE_PATH is the path to the djafs storage directory.
MOUNTPOINT is the directory where the filesystem will be mounted.`,
	Args: func(cmd *cobra.Command, args []string) error {
		versionFlag, _ := cmd.Flags().GetBool("version")
		if versionFlag {
			return nil // Skip argument validation for version flag
		}
		return cobra.ExactArgs(2)(cmd, args)
	},
	Run: runMount,
}

func init() {
	rootCmd.Flags().BoolP("version", "v", false, "Show version information and exit")
}

func runMount(cmd *cobra.Command, args []string) {
	versionFlag, _ := cmd.Flags().GetBool("version")
	if versionFlag {
		version.PrintVersion("djafs")
		os.Exit(0)
	}

	// Print version info on startup
	fmt.Printf("djafs %s starting...\n", version.GetFullVersion())

	storagePath := args[0]
	mountpoint := args[1]

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

func main() {
	if err := fang.Execute(context.Background(), rootCmd); err != nil {
		os.Exit(1)
	}
}
