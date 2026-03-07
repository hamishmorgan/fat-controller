package cli

import (
	"fmt"
	"os"

	"github.com/hamishmorgan/fat-controller/internal/prompt"
)

// AdoptCmd implements the `adopt` command.
type AdoptCmd struct {
	ServiceFlags    `kong:"embed"`
	MergeFlags      `kong:"embed"`
	PromptFlags     `kong:"embed"`
	ConfigFileFlags `kong:"embed"`
	DryRun          bool   `help:"Force preview of mutations." name:"dry-run" env:"FAT_CONTROLLER_DRY_RUN"`
	Path            string `arg:"" optional:"" help:"Dot-path to scope adoption (e.g. api)."`
}

// Run implements `adopt`.
func (c *AdoptCmd) Run(globals *Globals) error {
	ctx, cancel := c.TimeoutContext(globals.BaseCtx)
	defer cancel()
	client, err := newClient(&c.ApiFlags, globals.BaseCtx)
	if err != nil {
		return err
	}
	resolver := &railwayInitResolver{client: client}

	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	interactive := prompt.StdinIsInteractive()
	if c.PromptMode() == "all" {
		interactive = true
	} else if c.PromptMode() == "none" {
		interactive = false
	}

	// TODO: Wire MergeFlags, Path scoping, and ID bookkeeping.
	return RunConfigInit(ctx, wd, c.Workspace, c.Project, c.Environment, resolver, interactive, c.DryRun, c.Yes, os.Stdout)
}
