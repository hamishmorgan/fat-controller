package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/hamishmorgan/fat-controller/internal/auth"
	"github.com/hamishmorgan/fat-controller/internal/config"
	"github.com/hamishmorgan/fat-controller/internal/platform"
	"github.com/hamishmorgan/fat-controller/internal/railway"
)

const localConfigStub = `# local overrides (gitignored). Use for secrets and per-developer settings.
# Example:
#   [api.variables]
#   STRIPE_KEY = "${STRIPE_KEY}"
`

// Run implements `config init`.
func (c *ConfigInitCmd) Run(globals *Globals) error {
	store := auth.NewTokenStore(auth.WithFallbackPath(platform.AuthFilePath()))
	resolved, err := auth.ResolveAuth(globals.Token, store)
	if err != nil {
		return err
	}
	client := railway.NewClient(railway.Endpoint, resolved, store, auth.NewOAuthClient())
	fetcher := &defaultConfigFetcher{client: client}

	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	return RunConfigInit(context.Background(), wd, globals.Project, globals.Environment, fetcher, os.Stdout)
}

// RunConfigInit is the testable core of `config init`.
func RunConfigInit(ctx context.Context, dir, project, environment string, fetcher configFetcher, out io.Writer) error {
	if out == nil {
		out = os.Stdout
	}

	// 1. Refuse to overwrite existing config.
	configPath := filepath.Join(dir, config.BaseConfigFile)
	if _, err := os.Stat(configPath); err == nil {
		return fmt.Errorf("%s already exists — refusing to overwrite", config.BaseConfigFile)
	}

	// 2. Resolve project/environment (may prompt interactively).
	projID, envID, err := fetcher.Resolve(ctx, "", project, environment)
	if err != nil {
		return err
	}

	// 3. Fetch live state.
	live, err := fetcher.Fetch(ctx, projID, envID, "")
	if err != nil {
		return err
	}

	// 4. Render and write the config file.
	// project/environment args are names (not IDs) — used as the header values.
	content := config.RenderInitTOML(project, environment, *live)
	if err := os.WriteFile(configPath, []byte(content+"\n"), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", config.BaseConfigFile, err)
	}
	fmt.Fprintf(out, "wrote %s (%d services)\n", config.BaseConfigFile, len(live.Services))

	// 6. Create .local.toml stub if it doesn't exist.
	localPath := filepath.Join(dir, config.LocalConfigFile)
	if _, err := os.Stat(localPath); os.IsNotExist(err) {
		if err := os.WriteFile(localPath, []byte(localConfigStub), 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", config.LocalConfigFile, err)
		}
		fmt.Fprintf(out, "wrote %s (local overrides, gitignored)\n", config.LocalConfigFile)
	}

	return nil
}
