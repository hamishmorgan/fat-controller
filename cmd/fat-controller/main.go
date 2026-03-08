package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/alecthomas/kong"
	"github.com/hamishmorgan/fat-controller/internal/cli"
	"github.com/hamishmorgan/fat-controller/internal/version"
	kongcompletion "github.com/jotaen/kong-completion"
)

func main() {
	// Apply --color / FAT_CONTROLLER_OUTPUT_COLOR before kong.Parse so that help
	// output (triggered via BeforeReset on --help) respects the setting.
	cli.ApplyColorMode(os.Args[1:])

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

	// Resolve --json/--toml/--raw shorthands to --output value.
	c.ResolveOutputFormat()

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
