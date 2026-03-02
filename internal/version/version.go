// Package version holds build-time version information.
//
// Version info comes from three sources, in priority order:
//  1. ldflags — set by goreleaser or the local mise build task
//  2. runtime/debug.BuildInfo — set automatically by go install/run @version
//  3. Compile-time defaults ("dev", "unknown")
package version

import (
	"fmt"
	"runtime/debug"
)

// Set by ldflags at build time. When not set, init() falls back to
// debug.BuildInfo (which Go populates for `go install foo@version` builds).
var (
	version = ""
	commit  = ""
	date    = ""
)

func init() {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		setDefaults()
		return
	}

	if version == "" {
		if v := info.Main.Version; v != "" && v != "(devel)" {
			version = v
		} else {
			version = "dev"
		}
	}

	if commit == "" || date == "" {
		for _, s := range info.Settings {
			switch s.Key {
			case "vcs.revision":
				if commit == "" && len(s.Value) >= 7 {
					commit = s.Value[:7]
				}
			case "vcs.time":
				if date == "" {
					date = s.Value
				}
			}
		}
	}

	setDefaults()
}

func setDefaults() {
	if version == "" {
		version = "dev"
	}
	if commit == "" {
		commit = "unknown"
	}
	if date == "" {
		date = "unknown"
	}
}

// String returns a human-readable version string.
func String() string {
	return fmt.Sprintf("%s (commit %s, built %s)", version, commit, date)
}

// Version returns the semantic version (e.g. "v1.0.0" or "dev").
func Version() string {
	return version
}
