package cli

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/hamishmorgan/fat-controller/internal/config"
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

	// 1. Load and merge config files.
	desired, err := config.LoadConfigs(configDir, extraFiles)
	if err != nil {
		return err
	}

	// 2. Interpolate local env vars.
	if err := config.Interpolate(desired); err != nil {
		return err
	}

	// 2b. Use config-file project/environment as fallback for resolution.
	project := globals.Project
	if project == "" {
		project = desired.Project
	}
	environment := globals.Environment
	if environment == "" {
		environment = desired.Environment
	}

	// 3. Fetch live state.
	projID, envID, err := fetcher.Resolve(ctx, globals.Workspace, project, environment)
	if err != nil {
		return err
	}
	live, err := fetcher.Fetch(ctx, projID, envID, globals.Service)
	if err != nil {
		return err
	}

	// 4. Filter desired config by --service if set.
	if globals.Service != "" {
		filtered := &config.DesiredConfig{
			Shared:   desired.Shared,
			Services: make(map[string]*config.DesiredService),
		}
		if svc, ok := desired.Services[globals.Service]; ok {
			filtered.Services[globals.Service] = svc
		}
		desired = filtered
	}

	// 5. Compute diff.
	result := diff.Compute(desired, live)

	// 6. Format and display (live values are masked unless --show-secrets is set).
	formatted := diff.Format(result, globals.ShowSecrets)
	_, err = fmt.Fprintln(out, formatted)
	return err
}
