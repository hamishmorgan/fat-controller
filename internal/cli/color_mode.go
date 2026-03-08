package cli

import (
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// ApplyColorMode configures color output before kong.Parse runs.
//
// It checks the provided CLI args for --color=<value> or --color <value>.
// If not present, it falls back to FAT_CONTROLLER_OUTPUT_COLOR.
func ApplyColorMode(args []string) {
	mode := ""

	for i, arg := range args {
		if v, ok := strings.CutPrefix(arg, "--color="); ok {
			mode = v
		} else if arg == "--color" && i+1 < len(args) {
			mode = args[i+1]
		}
	}

	if mode == "" {
		mode = os.Getenv("FAT_CONTROLLER_OUTPUT_COLOR")
	}

	switch ResolveColorMode(mode) {
	case "never":
		lipgloss.SetColorProfile(termenv.Ascii)
	case "always":
		lipgloss.SetColorProfile(termenv.ANSI)
	default:
		// auto (default): let lipgloss/termenv auto-detect.
	}
}

// ResolveColorMode determines the effective color mode.
//
// Precedence (highest to lowest):
//  1. Explicit mode from CLI flag or FAT_CONTROLLER_OUTPUT_COLOR (auto|always|never)
//  2. NO_COLOR (any value = never)
//  3. FORCE_COLOR (non-empty and not "0" = always)
//  4. CLICOLOR ("0" = never)
//  5. CLICOLOR_FORCE (non-empty and not "0" = always)
//  6. TERM=dumb (never)
//  7. auto
func ResolveColorMode(explicit string) string {
	switch explicit {
	case "auto", "always", "never":
		return explicit
	case "":
		// fall through to env-based resolution
	default:
		// Unknown values behave like auto.
	}

	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		return "never"
	}
	if v := os.Getenv("FORCE_COLOR"); v != "" && v != "0" {
		return "always"
	}
	if os.Getenv("CLICOLOR") == "0" {
		return "never"
	}
	if v := os.Getenv("CLICOLOR_FORCE"); v != "" && v != "0" {
		return "always"
	}
	if os.Getenv("TERM") == "dumb" {
		return "never"
	}
	return "auto"
}
