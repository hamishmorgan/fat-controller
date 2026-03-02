package main

import (
	"fmt"
	"os"

	"github.com/alecthomas/kong"
	"github.com/charmbracelet/lipgloss"
	"github.com/hamishmorgan/fat-controller/internal/cli"
	"github.com/muesli/termenv"
)

func main() {
	// Apply --color / FAT_CONTROLLER_COLOR before kong.Parse so that help
	// output (triggered via BeforeReset on --help) respects the setting.
	applyColorMode()

	var c cli.CLI
	ctx := kong.Parse(&c,
		kong.Name("fat-controller"),
		kong.Description("CLI for managing Railway projects. Pull live config, diff against desired state, apply the difference."),
		kong.UsageOnError(),
		kong.Help(cli.ColorHelpPrinter),
	)

	if err := ctx.Run(&c.Globals); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// applyColorMode checks the --color flag and FAT_CONTROLLER_COLOR env var
// to configure color output before parsing begins.
func applyColorMode() {
	mode := os.Getenv("FAT_CONTROLLER_COLOR")

	// Check CLI args for --color=<value> or --color <value>.
	for i, arg := range os.Args[1:] {
		if arg == "--color" && i+1 < len(os.Args[1:])-1 {
			mode = os.Args[i+2]
		}
		if len(arg) > 8 && arg[:8] == "--color=" {
			mode = arg[8:]
		}
	}

	switch mode {
	case "never":
		lipgloss.SetColorProfile(termenv.Ascii)
	case "always":
		lipgloss.SetColorProfile(termenv.ANSI)
	default: // "auto" — let lipgloss/termenv detect capabilities
	}
}
