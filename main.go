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

	globals := &cmd.Globals{
		Token:       cli.Token,
		Project:     cli.Project,
		Environment: cli.Environment,
		Output:      cli.Output,
		Color:       cli.Color,
		Timeout:     cli.Timeout,
		Confirm:     cli.Confirm,
		DryRun:      cli.DryRun,
		ShowSecrets: cli.ShowSecrets,
		SkipDeploys: cli.SkipDeploys,
		FailFast:    cli.FailFast,
		Config:      cli.ConfigFiles,
		Service:     cli.Service,
		Full:        cli.Full,
		Verbose:     cli.Verbose,
		Quiet:       cli.Quiet,
	}

	if err := ctx.Run(globals); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
