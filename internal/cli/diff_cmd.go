package cli

import (
	"errors"
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
	_ = globals
	return errors.New("diff: not implemented")
}
