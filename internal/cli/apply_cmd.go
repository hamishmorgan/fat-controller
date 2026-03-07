package cli

import (
	"fmt"
	"os"

	"github.com/hamishmorgan/fat-controller/internal/apply"
)

// ApplyCmd implements the top-level `apply` command.
type ApplyCmd struct {
	ServiceFlags    `kong:"embed"`
	MergeFlags      `kong:"embed"`
	PromptFlags     `kong:"embed"`
	ConfigFileFlags `kong:"embed"`
	DryRun          bool   `help:"Force preview of mutations." name:"dry-run" env:"FAT_CONTROLLER_DRY_RUN"`
	ShowSecrets     bool   `help:"Show secret values instead of masking." name:"show-secrets" env:"FAT_CONTROLLER_SHOW_SECRETS"`
	SkipDeploys     bool   `help:"Don't trigger redeployments." name:"skip-deploys" env:"FAT_CONTROLLER_SKIP_DEPLOYS"`
	FailFast        bool   `help:"Stop on first error during apply." name:"fail-fast" env:"FAT_CONTROLLER_FAIL_FAST"`
	Path            string `arg:"" optional:"" help:"Dot-path to scope apply (e.g. api, variables)."`
}

// Run implements `apply`.
func (c *ApplyCmd) Run(globals *Globals) error {
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

	pair, err := loadAndFetch(ctx, c.Workspace, c.Project, c.Environment, wd, c.ConfigFiles, c.Service, fetcher)
	if err != nil {
		return err
	}

	// Emit validation warnings to stderr.
	emitWarnings(pair, globals.Quiet, wd)

	applier := &apply.RailwayApplier{
		Client:        client,
		ProjectID:     pair.ProjectID,
		EnvironmentID: pair.EnvironmentID,
	}

	// TODO: Wire MergeFlags and Path into apply computation.
	return runConfigApplyWithPair(ctx, globals, pair, c.DryRun, c.Yes, c.ShowSecrets, c.SkipDeploys, c.FailFast, applier, os.Stdout)
}
