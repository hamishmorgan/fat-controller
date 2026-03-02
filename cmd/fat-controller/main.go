package main

import (
	"fmt"
	"os"

	"github.com/alecthomas/kong"
	"github.com/hamishmorgan/fat-controller/internal/cli"
)

func main() {
	var c cli.CLI
	ctx := kong.Parse(&c,
		kong.Name("fat-controller"),
		kong.Description("CLI for managing Railway projects. Pull live config, diff against desired state, apply the difference."),
		kong.UsageOnError(),
	)

	if err := ctx.Run(&c.Globals); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
