package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/BurntSushi/toml"

	"github.com/hamishmorgan/fat-controller/internal/apply"
	"github.com/hamishmorgan/fat-controller/internal/diff"
	"github.com/hamishmorgan/fat-controller/internal/prompt"
)

// Run implements `config apply`.
func (c *ConfigApplyCmd) Run(globals *Globals) error {
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

	pair, err := loadAndFetch(ctx, globals, wd, globals.ConfigFiles, fetcher)
	if err != nil {
		return err
	}

	// Emit validation warnings to stderr.
	emitWarnings(pair, globals, wd)

	applier := &apply.RailwayApplier{
		Client:        client,
		ProjectID:     pair.ProjectID,
		EnvironmentID: pair.EnvironmentID,
	}

	return runConfigApplyWithPair(ctx, globals, pair, applier, os.Stdout)
}

// RunConfigApply is the testable core of `config apply`.
func RunConfigApply(ctx context.Context, globals *Globals, configDir string, extraFiles []string, fetcher configFetcher, applier apply.Applier, out io.Writer) error {
	pair, err := loadAndFetch(ctx, globals, configDir, extraFiles, fetcher)
	if err != nil {
		return err
	}
	// Emit validation warnings to stderr.
	emitWarnings(pair, globals, configDir)
	return runConfigApplyWithPair(ctx, globals, pair, applier, out)
}

// runConfigApplyWithPair contains the apply logic once configs are loaded and fetched.
func runConfigApplyWithPair(ctx context.Context, globals *Globals, pair *configPair, applier apply.Applier, out io.Writer) error {
	if out == nil {
		out = os.Stdout
	}

	desired := pair.Desired
	live := pair.Live

	// Compute diff.
	changes := diff.Compute(desired, live)
	slog.Debug("diff computed", "is_empty", changes.IsEmpty())

	// If no changes, report and return.
	if changes.IsEmpty() {
		switch globals.Output {
		case "json":
			b, err := json.MarshalIndent(&apply.Result{}, "", "  ")
			if err != nil {
				return fmt.Errorf("marshalling result: %w", err)
			}
			if _, err := fmt.Fprintln(out, string(b)); err != nil {
				return err
			}
		case "toml":
			b, err := toml.Marshal(&apply.Result{})
			if err != nil {
				return fmt.Errorf("marshalling result: %w", err)
			}
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

	// Handle dry-run and confirmation.
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

	// Apply changes.
	slog.Debug("executing apply", "fail_fast", globals.FailFast, "skip_deploys", globals.SkipDeploys)
	applyResult, applyErr := apply.Apply(ctx, desired, live, applier, apply.Options{
		FailFast:    globals.FailFast,
		SkipDeploys: globals.SkipDeploys,
	})

	switch globals.Output {
	case "json":
		b, err := json.MarshalIndent(applyResult, "", "  ")
		if err != nil {
			return fmt.Errorf("marshalling result: %w", err)
		}
		if _, err := fmt.Fprintln(out, string(b)); err != nil {
			return err
		}
	case "toml":
		b, err := toml.Marshal(applyResult)
		if err != nil {
			return fmt.Errorf("marshalling result: %w", err)
		}
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
