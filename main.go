package main

import (
	"fmt"
	"os"

	"github.com/alecthomas/kong"
	"github.com/hamishmorgan/fat-controller/cmd"
)

func main() {
	var cli cmd.CLI
	ctx := kong.Parse(&cli,
		kong.Name("fat-controller"),
		kong.Description("CLI for managing Railway projects. Pull live config, diff against desired state, apply the difference."),
		kong.UsageOnError(),
	)

	if err := ctx.Run(&cli.Globals); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
