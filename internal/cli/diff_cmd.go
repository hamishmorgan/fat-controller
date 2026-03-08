package cli

import (
	"fmt"
	"os"

	"github.com/hamishmorgan/fat-controller/internal/diff"
)

// DiffCmd implements the top-level `diff` command.
type DiffCmd struct {
	ServiceFlags    `kong:"embed"`
	MergeFlags      `kong:"embed"`
	ConfigFileFlags `kong:"embed"`
	ShowSecrets     bool   `help:"Show secret values instead of masking." name:"show-secrets" env:"FAT_CONTROLLER_SHOW_SECRETS"`
	Path            string `arg:"" optional:"" help:"Dot-path to scope diff (e.g. api, api.variables)."`
}

// Run implements `diff`.
func (c *DiffCmd) Run(globals *Globals) error {
	ctx, cancel := c.TimeoutContext(globals.BaseCtx)
	defer cancel()
	client, err := newClient(&c.ApiFlags, globals.BaseCtx)
	if err != nil {
		return err
	}
	fetcher := &defaultConfigFetcher{client: client}

	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	return RunConfigDiffWithOpts(ctx, globals, c.Workspace, c.Project, c.Environment, wd, c.ConfigFile, c.Service, DiffOpts{
		ShowSecrets: c.ShowSecrets,
		DiffOptions: diff.Options{
			Create: c.Create,
			Update: c.Update,
			Delete: c.Delete,
		},
		Path: c.Path,
	}, fetcher, os.Stdout)
}
