package version

import (
	"runtime/debug"
)

var (
	// Version is the current version of linko, set at build time via ldflags
	Version = "dev"
	// Commit is the git commit hash, set at build time via ldflags
	Commit = ""
	// Date is the build date, set at build time via ldflags
	Date = ""
)

func init() {
	info, _ := debug.ReadBuildInfo()
	if info.Main.Version != "" {
		Version = info.Main.Version
	}
}
