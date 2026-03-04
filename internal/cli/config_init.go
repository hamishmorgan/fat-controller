package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/hamishmorgan/fat-controller/internal/config"
)

func ensureGitignoreHasLine(dir, line string) (bool, error) {
	gitignorePath := filepath.Join(dir, ".gitignore")

	b, err := os.ReadFile(gitignorePath)
	if err != nil {
		if os.IsNotExist(err) {
			if err := os.WriteFile(gitignorePath, []byte(line+"\n"), 0o644); err != nil {
				return false, err
			}
			return true, nil
		}
		return false, err
	}

	lines := strings.Split(string(b), "\n")
	for _, existing := range lines {
		if strings.TrimSpace(existing) == line {
			return false, nil
		}
	}

	f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return false, err
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()

	if len(b) > 0 && b[len(b)-1] != '\n' {
		if _, err := f.WriteString("\n"); err != nil {
			return false, err
		}
	}
	if _, err := f.WriteString(line + "\n"); err != nil {
		return false, err
	}
	return true, nil
}

const localConfigStub = `# Local overrides (gitignored). Use for secrets and per-developer settings.
# Example:
#   [api.variables]
#   STRIPE_KEY = "${STRIPE_KEY}"
`

// Run implements `config init`.
func (c *ConfigInitCmd) Run(globals *Globals) error {
	ctx, cancel := globals.TimeoutContext(context.Background())
	defer cancel()
	client, err := newClient(globals)
	if err != nil {
		return err
	}
	fetcher := &defaultConfigFetcher{client: client}

	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	return RunConfigInit(ctx, wd, globals.Project, globals.Environment, fetcher, os.Stdout)
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
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("checking %s: %w", config.BaseConfigFile, err)
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
	if _, err := fmt.Fprintf(out, "wrote %s (%d services)\n", config.BaseConfigFile, len(live.Services)); err != nil {
		return err
	}

	// 5. Create .local.toml stub if it doesn't exist.
	localPath := filepath.Join(dir, config.LocalConfigFile)
	if _, err := os.Stat(localPath); os.IsNotExist(err) {
		if err := os.WriteFile(localPath, []byte(localConfigStub), 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", config.LocalConfigFile, err)
		}
		if _, err := fmt.Fprintf(out, "wrote %s (local overrides, gitignored)\n", config.LocalConfigFile); err != nil {
			return err
		}
	}

	added, err := ensureGitignoreHasLine(dir, config.LocalConfigFile)
	if err != nil {
		return fmt.Errorf("updating .gitignore: %w", err)
	}
	if added {
		if _, err := fmt.Fprintf(out, "updated %s (added %s)\n", ".gitignore", config.LocalConfigFile); err != nil {
			return err
		}
	}

	return nil
}
