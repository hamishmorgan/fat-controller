package cli

import (
	"fmt"
	"os"
)

// ValidateCmd implements the top-level `validate` command.
type ValidateCmd struct {
	ConfigFileFlags `kong:"embed"`
	Path            string `arg:"" optional:"" help:"Dot-path to scope validation (e.g. api)."`
}

// Run implements `validate`.
func (c *ValidateCmd) Run(globals *Globals) error {
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}
	return RunConfigValidateScoped(globals, wd, c.ConfigFile, c.Path, os.Stdout)
}
