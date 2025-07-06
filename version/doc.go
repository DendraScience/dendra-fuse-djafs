// Package version provides version information and build metadata for djafs.
//
// This package handles version reporting for the djafs application, supporting both
// compile-time version injection via build flags and runtime version detection
// using Go's build info. It provides a flexible versioning system that works
// in development, CI/CD, and release scenarios.
//
// Version Information Sources:
//   - Compile-time variables (Version, Commit, Date) set via -ldflags
//   - Runtime build info from debug.ReadBuildInfo()
//   - Fallback defaults for development builds
//
// The package provides multiple version formats:
//   - GetVersion(): Simple version string
//   - GetFullVersion(): Formatted version with commit and build date
//   - GetInfo(): Complete version information as a struct
//   - PrintVersion(): Human-readable version output
//
// Build Integration:
// The Makefile sets version information at build time using:
//   -ldflags "-X package/version.Version=v1.0.0 -X package/version.Commit=abc123 -X package/version.Date=2023-01-01T00:00:00Z"
//
// This ensures consistent version reporting across all djafs binaries and subcommands.
package version