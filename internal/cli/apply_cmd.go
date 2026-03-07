package cli

import (
	"errors"
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
	_ = globals
	return errors.New("apply: not implemented")
}
