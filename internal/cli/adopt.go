package cli

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/hamishmorgan/fat-controller/internal/app"
	"github.com/hamishmorgan/fat-controller/internal/config"
	"github.com/hamishmorgan/fat-controller/internal/prompt"
)

// AdoptCmd implements the `adopt` command.
type AdoptCmd struct {
	ServiceFlags    `kong:"embed"`
	MergeFlags      `kong:"embed"`
	PromptFlags     `kong:"embed"`
	ConfigFileFlags `kong:"embed"`
	DryRun          bool   `help:"Force preview of mutations." name:"dry-run" env:"FAT_CONTROLLER_DRY_RUN"`
	ShowSecrets     bool   `help:"Show secret values instead of masking." name:"show-secrets" env:"FAT_CONTROLLER_SHOW_SECRETS"`
	Path            string `arg:"" optional:"" help:"Dot-path to scope adoption (e.g. api)."`
}

// Run implements `adopt`.
func (c *AdoptCmd) Run(globals *Globals) error {
	ctx, cancel := c.TimeoutContext(globals.BaseCtx)
	defer cancel()
	client, err := newClient(&c.ApiFlags, globals.BaseCtx)
	if err != nil {
		return err
	}

	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	interactive := prompt.StdinIsInteractive()
	if c.PromptMode() == "all" {
		interactive = true
	} else if c.PromptMode() == "none" {
		interactive = false
	}

	// Try to load an existing config. If none exists, fall back to the
	// wizard-style init flow for first-time bootstrap.
	result, loadErr := config.LoadCascade(config.LoadOptions{WorkDir: wd, ConfigFile: c.ConfigFile})
	if loadErr != nil || result == nil || result.Config == nil {
		slog.Debug("no existing config found, using init wizard")
		resolver := &railwayInitResolver{client: client}
		return RunConfigInit(ctx, wd, c.ConfigFile, c.Workspace, c.Project, c.Environment, resolver, interactive, c.DryRun, c.Yes, os.Stdout)
	}

	// Existing config found — run the merge-based adopt flow.
	desired := result.Config
	out := os.Stdout

	// Interpolate ${VAR} references so we can resolve names.
	if err := config.Interpolate(desired, result.EnvVars); err != nil {
		return err
	}

	// Resolve workspace/project/environment from flags → config fallback.
	project := c.Project
	if project == "" && desired.Project != nil {
		project = desired.Project.Name
	}
	environment := c.Environment
	if environment == "" {
		environment = desired.Name
	}
	workspace := c.Workspace
	if workspace == "" && desired.Workspace != nil {
		workspace = desired.Workspace.Name
	}

	fetcher := &defaultConfigFetcher{client: client}
	resolved, err := fetcher.Resolve(ctx, workspace, project, environment)
	if err != nil {
		return err
	}
	projID, envID := resolved.ProjectID, resolved.EnvironmentID

	// Fetch live state.
	slog.Debug("fetching live state", "project_id", projID, "environment_id", envID)
	var adoptFilter []string
	if c.Service != "" {
		adoptFilter = []string{c.Service}
	}
	live, err := fetcher.Fetch(ctx, projID, envID, adoptFilter)
	if err != nil {
		return err
	}

	// Scope by path if provided.
	scopedLive := live
	if c.Path != "" {
		scopedLive = app.ScopeLiveByPath(live, c.Path)
	}

	// Build the adopted config by merging live into existing desired,
	// respecting MergeFlags (create/update/delete).
	adopted := app.AdoptMerge(desired, scopedLive, c.Create, c.Update, c.Delete, c.Path)

	// Use the resolved names (populated by API lookup) with config/flag fallback.
	wsName := resolved.WorkspaceName
	if wsName == "" {
		wsName = workspace
	}
	projName := resolved.ProjectName
	if projName == "" {
		projName = project
	}
	envName := resolved.EnvironmentName
	if envName == "" {
		envName = environment
	}

	// Summarize what changed.
	_, _ = fmt.Fprintf(out, "  Workspace: %s\n", wsName)
	_, _ = fmt.Fprintf(out, "  Project: %s\n", projName)
	_, _ = fmt.Fprintf(out, "  Environment: %s\n", envName)
	_, _ = fmt.Fprintf(out, "  Services: %s (%d)\n", app.JoinServiceNames(adopted), len(adopted.Services))
	_, _ = fmt.Fprintln(out)

	// Choose output paths.
	configPath := result.PrimaryFile
	if configPath == "" {
		configPath = filepath.Join(wd, config.BaseConfigFile)
	}
	if interactive && !c.Yes {
		p, err := prompt.Input("Save adopted config to:", configPath)
		if err != nil {
			return err
		}
		if p != "" {
			configPath = p
		}
	}

	envPath := defaultSecretsPath(configPath)
	if interactive && !c.Yes {
		p, err := prompt.Input("Save secrets to:", envPath)
		if err != nil {
			return err
		}
		if p != "" {
			envPath = p
		}
	}
	configDir := filepath.Dir(configPath)
	envFileSetting := envPath
	if rel, err := filepath.Rel(configDir, envPath); err == nil {
		envFileSetting = rel
	}

	// Render the adopted config.
	content := config.RenderInitTOMLWithEnvFile(wsName, projName, envName, *adopted, envFileSetting)

	envFileName := filepath.Base(envPath)
	envContent := app.RenderEnvFile(adopted)

	if c.DryRun {
		_, _ = fmt.Fprintf(out, "dry run: would write %s (%d services)\n\n%s\n",
			configPath, len(adopted.Services), content)
		if envContent != "" {
			_, _ = fmt.Fprintf(out, "\ndry run: would write %s\n\n%s\n",
				envPath, envContent)
		}
		return nil
	}

	if !c.Yes {
		if !interactive {
			_, _ = fmt.Fprintf(out, "would write %s (%d services)\n\n%s\n",
				configPath, len(adopted.Services), content)
			if envContent != "" {
				_, _ = fmt.Fprintf(out, "\nwould write %s\n\n%s\n", envPath, envContent)
			}
			_, _ = fmt.Fprintln(out, "use --yes to write files")
			return nil
		}

		_, _ = fmt.Fprintf(out, "Will write %s (%d services):\n\n%s\n\n",
			configPath, len(adopted.Services), content)
		confirmed, err := prompt.Confirm("Write changes?", true)
		if err != nil {
			return fmt.Errorf("reading confirmation: %w", err)
		}
		if !confirmed {
			_, _ = fmt.Fprintln(out, "Adopt cancelled.")
			return nil
		}
	}

	// Write the config file.
	writeConfig, err := confirmWrite(configPath, filepath.Base(configPath), c.Yes, interactive)
	if err != nil {
		return err
	}
	if writeConfig {
		if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
			return fmt.Errorf("creating config dir: %w", err)
		}
		if err := os.WriteFile(configPath, []byte(content+"\n"), 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", filepath.Base(configPath), err)
		}
		_, _ = fmt.Fprintf(out, "wrote %s (%d services)\n", configPath, len(adopted.Services))
	} else {
		_, _ = fmt.Fprintf(out, "skipped %s (already exists)\n", configPath)
	}

	// Write env file if there are secrets.
	if envContent != "" {
		writeEnv, err := confirmWrite(envPath, envFileName, c.Yes, interactive)
		if err != nil {
			return err
		}
		if writeEnv {
			if err := os.MkdirAll(filepath.Dir(envPath), 0o755); err != nil {
				return fmt.Errorf("creating secrets dir: %w", err)
			}
			if err := os.WriteFile(envPath, []byte(envContent), 0o600); err != nil {
				return fmt.Errorf("writing %s: %w", envFileName, err)
			}
			_, _ = fmt.Fprintf(out, "wrote %s (secret values — do not commit)\n", envFileName)

			added, err := app.EnsureGitignoreHasLine(filepath.Dir(envPath), envFileName)
			if err != nil {
				return fmt.Errorf("updating .gitignore: %w", err)
			}
			if added {
				_, _ = fmt.Fprintf(out, "updated .gitignore (added %s)\n", envFileName)
			}
		}
	}

	return nil
}

func defaultSecretsPath(configPath string) string {
	configPath = strings.TrimSpace(configPath)
	if configPath == "" {
		return config.DefaultEnvFile
	}
	ext := filepath.Ext(configPath)
	if ext == "" {
		return configPath + ".secrets"
	}
	return strings.TrimSuffix(configPath, ext) + ".secrets"
}
