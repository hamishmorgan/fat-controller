package cli

import (
	"errors"
	"fmt"
	"os"
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
	_ = globals
	_ = c.Path
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}
	_ = wd
	return errors.New("adopt: not implemented")
}
