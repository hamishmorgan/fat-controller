package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/alecthomas/kong"
	"github.com/charmbracelet/lipgloss"
	"github.com/hamishmorgan/fat-controller/internal/cli"
	"github.com/hamishmorgan/fat-controller/internal/version"
	kongcompletion "github.com/jotaen/kong-completion"
	"github.com/muesli/termenv"
)

func main() {
	// Apply --color / FAT_CONTROLLER_COLOR before kong.Parse so that help
	// output (triggered via BeforeReset on --help) respects the setting.
	applyColorMode()

	// Create a root context that gets cancelled on SIGINT (Ctrl+C) or SIGTERM.
	// This allows in-flight API calls and operations to be cancelled gracefully.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var c cli.CLI
	parser, err := kong.New(&c,
		kong.Name("fat-controller"),
		kong.Description("CLI for managing Railway projects. Pull live config, diff against desired state, apply the difference."),
		kong.Vars{"version": version.String()},
		kong.UsageOnError(),
		kong.Help(cli.ColorHelpPrinter),
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	kongcompletion.Register(parser)

	kongCtx, err := parser.Parse(os.Args[1:])
	parser.FatalIfErrorf(err)

	// Configure structured logging based on --verbose / --quiet.
	slog.SetDefault(c.Logger())

	// Thread the signal-aware context through to all commands.
	c.BaseCtx = ctx

	if err := kongCtx.Run(&c.Globals); err != nil {
		// Don't print error for context cancellation (user pressed Ctrl+C).
		if !errors.Is(err, context.Canceled) {
			fmt.Fprintln(os.Stderr, "error:", err)
		}
		os.Exit(1)
	}
}

// applyColorMode configures color output before kong.Parse runs.
func applyColorMode() {
	mode := ""

	// Check CLI args for --color=<value> or --color <value>.
	args := os.Args[1:]
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

	switch resolveColorMode(mode) {
	case "never":
		lipgloss.SetColorProfile(termenv.Ascii)
	case "always":
		lipgloss.SetColorProfile(termenv.ANSI)
	default:
		// auto (default): let lipgloss/termenv auto-detect.
	}
}

// resolveColorMode determines the effective color mode.
//
// Precedence (highest to lowest):
//  1. Explicit mode from CLI flag or FAT_CONTROLLER_COLOR (auto|always|never)
//  2. NO_COLOR (any value = never)
//  3. FORCE_COLOR (non-empty and not "0" = always)
//  4. CLICOLOR ("0" = never)
//  5. CLICOLOR_FORCE (non-empty and not "0" = always)
//  6. TERM=dumb (never)
//  7. auto
func resolveColorMode(explicit string) string {
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
