package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/BurntSushi/toml"

	"github.com/hamishmorgan/fat-controller/internal/app"
	"github.com/hamishmorgan/fat-controller/internal/apply"
	"github.com/hamishmorgan/fat-controller/internal/diff"
	"github.com/hamishmorgan/fat-controller/internal/prompt"
)

// ApplyOpts holds command-specific options for RunConfigApply.
type ApplyOpts struct {
	DryRun      bool
	Yes         bool
	ShowSecrets bool
	SkipDeploys bool
	FailFast    bool
	Create      bool // include creates (default true when all three are zero)
	Update      bool // include updates (default true when all three are zero)
	Delete      bool // include deletes (default false)
}

// RunConfigApply is the testable core of `config apply`.
func RunConfigApply(ctx context.Context, globals *Globals, workspace, project, environment, configDir string, configFile string, service string, opts ApplyOpts, fetcher app.ConfigFetcher, applier apply.Applier, out io.Writer) error {
	pair, err := app.LoadAndFetch(ctx, workspace, project, environment, configDir, configFile, service, fetcher)
	if err != nil {
		return err
	}
	// Emit validation warnings to stderr.
	emitWarnings(pair, globals.Quiet, configDir)
	diffOpts := diff.Options{Create: opts.Create, Update: opts.Update, Delete: opts.Delete}
	// Default to create+update when none are explicitly set.
	if !diffOpts.Create && !diffOpts.Update && !diffOpts.Delete {
		diffOpts.Create = true
		diffOpts.Update = true
	}
	return runConfigApplyWithPairAndOpts(ctx, globals, pair, opts.DryRun, opts.Yes, opts.ShowSecrets, opts.SkipDeploys, opts.FailFast, diffOpts, "", applier, out)
}

func runConfigApplyWithPairAndOpts(ctx context.Context, globals *Globals, pair *app.ConfigPair, dryRun, yes, showSecrets, skipDeploys, failFast bool, diffOpts diff.Options, path string, applier apply.Applier, out io.Writer) error {
	if out == nil {
		out = os.Stdout
	}

	desired := pair.Desired
	if path != "" {
		desired = app.ScopeDesiredByPath(desired, path)
	}
	live := pair.Live

	// Compute diff.
	changes := diff.ComputeWithOptions(desired, live, diffOpts)
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
		AllowCreate: diffOpts.Create,
		AllowUpdate: diffOpts.Update,
		AllowDelete: diffOpts.Delete,
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
