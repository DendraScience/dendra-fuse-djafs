// Package cmd provides the command-line interface implementation for djafs.
//
// This package contains all the subcommand implementations for the djafs CLI tool.
// It uses the Cobra library for command structure and Fang for beautiful styling.
//
// The package is organized into the following commands:
//   - root: Main command coordinator and entry point
//   - mount: FUSE filesystem mounting functionality
//   - convert: JSON directory tree conversion to djafs format
//   - validate: Archive validation and consistency checking
//   - count: File counting utilities
//
// Each command is implemented as a separate file with its own constructor function
// that returns a *cobra.Command. The root command coordinates all subcommands and
// provides backward compatibility for the original mount-only interface.
//
// The package leverages the util package for core djafs operations and the djafs
// package for filesystem implementation.
package cmd
