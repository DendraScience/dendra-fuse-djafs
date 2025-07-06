package cmd

import (
	"github.com/dendrascience/dendra-archive-fuse/version"
	"github.com/spf13/cobra"
)

// NewRootCmd creates and returns the root cobra command for the djafs CLI.
// It sets up all subcommands, command groups, and basic configuration.
func NewRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "djafs",
		Short: "djafs - A FUSE-based filesystem for compressed, content-addressable JSON storage",
		Long: `djafs is a FUSE-based filesystem for compressed, content-addressable JSON storage.

It provides transparent JSON compression, content-addressable storage, and time-travel
capabilities through snapshots. The filesystem supports high-performance read/write
operations with background garbage collection.

Use subcommands to perform different operations:
  - mount: Mount a djafs filesystem at a specified mountpoint
  - convert: Convert existing JSON directory trees to djafs format
  - validate: Validate djafs archives for corruption and consistency
  - count: Count files in directory trees`,
		Version: version.GetFullVersion(),
	}

	// Add subcommands
	rootCmd.AddCommand(NewMountCmd())
	rootCmd.AddCommand(NewConvertCmd())
	rootCmd.AddCommand(NewValidateCmd())
	rootCmd.AddCommand(NewCountCmd())

	// Add command groups for better organization
	rootCmd.AddGroup(&cobra.Group{
		ID:    "filesystem",
		Title: "Filesystem Operations",
	})
	rootCmd.AddGroup(&cobra.Group{
		ID:    "utilities",
		Title: "Utility Commands",
	})

	// Assign commands to groups
	NewMountCmd().GroupID = "filesystem"
	NewConvertCmd().GroupID = "utilities"
	NewValidateCmd().GroupID = "utilities"
	NewCountCmd().GroupID = "utilities"

	return rootCmd
}