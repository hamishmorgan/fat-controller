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
//
// Precedence (highest to lowest):
//  1. --color=<mode> CLI flag
//  2. FAT_CONTROLLER_COLOR env var
//  3. NO_COLOR env var (any non-empty value disables color; see https://no-color.org)
//  4. Auto-detect terminal capabilities
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

	// Fall back to env vars if no CLI flag.
	if mode == "" {
		mode = os.Getenv("FAT_CONTROLLER_COLOR")
	}

	switch mode {
	case "never":
		lipgloss.SetColorProfile(termenv.Ascii)
	case "always":
		lipgloss.SetColorProfile(termenv.ANSI)
	default: // "auto" or unset
		// Respect NO_COLOR convention (https://no-color.org).
		if _, ok := os.LookupEnv("NO_COLOR"); ok {
			lipgloss.SetColorProfile(termenv.Ascii)
		}
		// Otherwise let lipgloss/termenv auto-detect.
	}
}
