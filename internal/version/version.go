// Package version holds build-time version information set via ldflags.
//
// These variables are populated by goreleaser or the local build task:
//
//	go build -ldflags "-X github.com/hamishmorgan/fat-controller/internal/version.version=v1.0.0 ..."
package version

import "fmt"

// Set by ldflags at build time.
var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

// String returns a human-readable version string.
func String() string {
	return fmt.Sprintf("%s (commit %s, built %s)", version, commit, date)
}

// Version returns the semantic version (e.g. "v1.0.0" or "dev").
func Version() string {
	return version
}
