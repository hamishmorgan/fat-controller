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
	ctx, cancel := globals.TimeoutContext(globals.BaseCtx)
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

	pair, err := loadAndFetch(ctx, globals, wd, c.ConfigFiles, c.Service, fetcher)
	if err != nil {
		return err
	}

	// Emit validation warnings to stderr.
	emitWarnings(pair, globals.Quiet, wd)

	applier := &apply.RailwayApplier{
		Client:        client,
		ProjectID:     pair.ProjectID,
		EnvironmentID: pair.EnvironmentID,
	}

	return runConfigApplyWithPair(ctx, globals, pair, c.DryRun, c.Yes, c.ShowSecrets, c.SkipDeploys, c.FailFast, applier, os.Stdout)
}

// ApplyOpts holds command-specific options for RunConfigApply / runConfigApplyWithPair.
type ApplyOpts struct {
	DryRun      bool
	Yes         bool
	ShowSecrets bool
	SkipDeploys bool
	FailFast    bool
}

// RunConfigApply is the testable core of `config apply`.
func RunConfigApply(ctx context.Context, globals *Globals, configDir string, extraFiles []string, service string, opts ApplyOpts, fetcher configFetcher, applier apply.Applier, out io.Writer) error {
	pair, err := loadAndFetch(ctx, globals, configDir, extraFiles, service, fetcher)
	if err != nil {
		return err
	}
	// Emit validation warnings to stderr.
	emitWarnings(pair, globals.Quiet, configDir)
	return runConfigApplyWithPair(ctx, globals, pair, opts.DryRun, opts.Yes, opts.ShowSecrets, opts.SkipDeploys, opts.FailFast, applier, out)
}

// runConfigApplyWithPair contains the apply logic once configs are loaded and fetched.
func runConfigApplyWithPair(ctx context.Context, globals *Globals, pair *configPair, dryRun, yes, showSecrets, skipDeploys, failFast bool, applier apply.Applier, out io.Writer) error {
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
	if dryRun {
		// Output diff for dry-run. If structured output is requested, fall back to text diff for now.
		formatted := diff.Format(changes, showSecrets)
		_, err := fmt.Fprintf(out, "dry run: would apply the following changes\n\n%s\n", formatted)
		return err
	}

	if !yes {
		formatted := diff.Format(changes, showSecrets)
		if !prompt.StdinIsInteractive() {
			_, err := fmt.Fprintf(out, "dry run: would apply the following changes (use --yes to execute)\n\n%s\n", formatted)
			return err
		}

		_, err := fmt.Fprintf(out, "The following changes will be applied:\n\n%s\n\n", formatted)
		if err != nil {
			return err
		}

		confirmed, err := prompt.Confirm("Are you sure you want to apply these changes?", false)
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
	slog.Debug("executing apply", "fail_fast", failFast, "skip_deploys", skipDeploys)
	applyResult, applyErr := apply.Apply(ctx, desired, live, applier, apply.Options{
		FailFast:    failFast,
		SkipDeploys: skipDeploys,
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
