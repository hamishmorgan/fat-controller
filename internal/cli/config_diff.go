package cli

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/hamishmorgan/fat-controller/internal/diff"
)

// Run implements `config diff`.
func (c *ConfigDiffCmd) Run(globals *Globals) error {
	client, err := newClient(globals)
	if err != nil {
		return err
	}
	fetcher := &defaultConfigFetcher{client: client}

	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	return RunConfigDiff(context.Background(), globals, wd, globals.ConfigFiles, fetcher, os.Stdout)
}

// RunConfigDiff is the testable core of `config diff`.
func RunConfigDiff(ctx context.Context, globals *Globals, configDir string, extraFiles []string, fetcher configFetcher, out io.Writer) error {
	if out == nil {
		out = os.Stdout
	}

	pair, err := loadAndFetch(ctx, globals, configDir, extraFiles, fetcher)
	if err != nil {
		return err
	}

	// Compute diff.
	result := diff.Compute(pair.Desired, pair.Live)

	// Format and display (live values are masked unless --show-secrets is set).
	formatted := diff.Format(result, globals.ShowSecrets)
	_, err = fmt.Fprintln(out, formatted)
	return err
}
