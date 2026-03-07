package cli

import (
	"os"
)

// ShowCmd implements the `show` command.
type ShowCmd struct {
	ServiceFlags `kong:"embed"`
	ShowSecrets  bool   `help:"Show secret values instead of masking." name:"show-secrets" env:"FAT_CONTROLLER_SHOW_SECRETS"`
	Full         bool   `help:"Include IDs and read-only fields."`
	Raw          bool   `help:"Output raw value (no formatting)."`
	Path         string `arg:"" optional:"" help:"Dot-path to show (e.g. api, api.variables.PORT, workspace, project)."`
}

// Run implements `show`.
func (c *ShowCmd) Run(globals *Globals) error {
	ctx, cancel := c.TimeoutContext(globals.BaseCtx)
	defer cancel()
	client, err := newClient(&c.ApiFlags, globals.BaseCtx)
	if err != nil {
		return err
	}
	fetcher := &defaultConfigFetcher{client: client}
	// Raw flag overrides output format for single values.
	_ = c.Raw // TODO: wire Raw into output formatting
	return RunConfigGet(ctx, globals, c.Workspace, c.Project, c.Environment, c.Path, c.Full, c.Service, c.ShowSecrets, fetcher, os.Stdout)
}
