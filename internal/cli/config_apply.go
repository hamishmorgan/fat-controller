package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/BurntSushi/toml"

	"github.com/hamishmorgan/fat-controller/internal/apply"
	"github.com/hamishmorgan/fat-controller/internal/auth"
	"github.com/hamishmorgan/fat-controller/internal/config"
	"github.com/hamishmorgan/fat-controller/internal/diff"
	"github.com/hamishmorgan/fat-controller/internal/platform"
	"github.com/hamishmorgan/fat-controller/internal/prompt"
	"github.com/hamishmorgan/fat-controller/internal/railway"
)

// Run implements `config apply`.
func (c *ConfigApplyCmd) Run(globals *Globals) error {
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

	projID, envID, err := fetcher.Resolve(context.Background(), globals.Workspace, globals.Project, globals.Environment)
	if err != nil {
		return err
	}

	applier := &apply.RailwayApplier{
		Client:        client,
		ProjectID:     projID,
		EnvironmentID: envID,
	}

	return RunConfigApply(context.Background(), globals, wd, globals.ConfigFiles, fetcher, applier, os.Stdout)
}

// RunConfigApply is the testable core of `config apply`.
func RunConfigApply(ctx context.Context, globals *Globals, configDir string, extraFiles []string, fetcher configFetcher, applier apply.Applier, out io.Writer) error {
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
	changes := diff.Compute(desired, live)

	// 6. If no changes, report and return.
	if changes.IsEmpty() {
		switch globals.Output {
		case "json":
			b, _ := json.MarshalIndent(&apply.Result{}, "", "  ")
			if _, err := fmt.Fprintln(out, string(b)); err != nil {
				return err
			}
		case "toml":
			b, _ := toml.Marshal(&apply.Result{})
			if _, err := fmt.Fprint(out, string(b)); err != nil {
				return err
			}
		default:
			if _, err := fmt.Fprintln(out, "No changes."); err != nil {
				return err
			}
		}
		return nil
	}

	// 7. Handle dry-run and confirmation.
	if globals.DryRun {
		// Output diff for dry-run. If structured output is requested, fall back to text diff for now.
		formatted := diff.Format(changes, globals.ShowSecrets)
		_, err := fmt.Fprintf(out, "dry run: would apply the following changes\n\n%s\n", formatted)
		return err
	}

	if !globals.Confirm {
		formatted := diff.Format(changes, globals.ShowSecrets)
		if !prompt.StdinIsInteractive() {
			_, err := fmt.Fprintf(out, "dry run: would apply the following changes (use --confirm to execute)\n\n%s\n", formatted)
			return err
		}

		_, err := fmt.Fprintf(out, "The following changes will be applied:\n\n%s\n\n", formatted)
		if err != nil {
			return err
		}

		confirmed, err := prompt.ConfirmRW(os.Stdin, out, "Are you sure you want to apply these changes?", false)
		if err != nil {
			return fmt.Errorf("reading confirmation: %w", err)
		}
		if !confirmed {
			if _, err := fmt.Fprintln(out, "Apply cancelled."); err != nil {
				return err
			}
			return nil
		}
	}

	// 8. Apply changes.
	applyResult, applyErr := apply.Apply(ctx, desired, live, applier, apply.Options{
		FailFast:    globals.FailFast,
		SkipDeploys: globals.SkipDeploys,
	})

	switch globals.Output {
	case "json":
		b, _ := json.MarshalIndent(applyResult, "", "  ")
		if _, err := fmt.Fprintln(out, string(b)); err != nil {
			return err
		}
	case "toml":
		b, _ := toml.Marshal(applyResult)
		if _, err := fmt.Fprint(out, string(b)); err != nil {
			return err
		}
	default:
		if _, err := fmt.Fprintln(out, applyResult.Summary()); err != nil {
			return err
		}
	}

	if applyErr != nil {
		return applyErr
	}

	if applyResult.HasFailures() {
		return fmt.Errorf("apply completed with %d failure(s)", applyResult.Failed)
	}
	return nil
}
