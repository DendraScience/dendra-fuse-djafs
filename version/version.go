package version

import (
	"fmt"
	"runtime/debug"
)

var (
	// These will be set by build flags or default to development values
	Version = "dev"
	Commit  = "unknown"
	Date    = "unknown"
)

// Info contains version information
type Info struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Date    string `json:"date"`
	Package string `json:"package"`
}

// GetVersion returns the version string, preferring compile-time version if available
func GetVersion() string {
	// Prefer compile-time version if set
	if Version != "dev" && Version != "" {
		return Version
	}

	// Fall back to build info
	if info, ok := debug.ReadBuildInfo(); ok {
		if info.Main.Version != "" && info.Main.Version != "(devel)" {
			return info.Main.Version
		}
	}

	return "development"
}

// GetCommit returns the git commit hash, preferring compile-time commit if available
func GetCommit() string {
	// Prefer compile-time commit if set
	if Commit != "unknown" && Commit != "" {
		return Commit
	}

	// Fall back to build info
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" {
				return setting.Value
			}
		}
	}

	return "unknown"
}

// GetBuildDate returns the build date, preferring compile-time date if available
func GetBuildDate() string {
	// Prefer compile-time date if set
	if Date != "unknown" && Date != "" {
		return Date
	}

	// Fall back to build info
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.time" {
				return setting.Value
			}
		}
	}

	return "unknown"
}

// GetInfo returns complete version information
func GetInfo() Info {
	return Info{
		Version: GetVersion(),
		Commit:  GetCommit(),
		Date:    GetBuildDate(),
		Package: "dendra-archive-fuse",
	}
}

// GetFullVersion returns a formatted version string with commit and date
func GetFullVersion() string {
	info := GetInfo()
	if info.Commit != "unknown" && len(info.Commit) > 7 {
		shortCommit := info.Commit[:7]
		if info.Date != "unknown" {
			return fmt.Sprintf("%s (%s, built %s)", info.Version, shortCommit, info.Date)
		}
		return fmt.Sprintf("%s (%s)", info.Version, shortCommit)
	}
	return info.Version
}

// PrintVersion prints version information to stdout
func PrintVersion(appName string) {
	info := GetInfo()
	fmt.Printf("%s version %s\n", appName, GetFullVersion())
	fmt.Printf("Package: %s\n", info.Package)
	fmt.Printf("Commit: %s\n", info.Commit)
	fmt.Printf("Build Date: %s\n", info.Date)
}
