package cli

import (
	"errors"
)

// ValidateCmd implements the top-level `validate` command.
type ValidateCmd struct {
	ConfigFileFlags `kong:"embed"`
	Path            string `arg:"" optional:"" help:"Dot-path to scope validation (e.g. api)."`
}

// Run implements `validate`.
func (c *ValidateCmd) Run(globals *Globals) error {
	_ = globals
	return errors.New("validate: not implemented")
}
