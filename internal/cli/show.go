package cli

import (
	"errors"
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
	_ = globals
	return errors.New("show: not implemented")
}
