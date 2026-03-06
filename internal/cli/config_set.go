package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/hamishmorgan/fat-controller/internal/config"
	"github.com/hamishmorgan/fat-controller/internal/prompt"
	"github.com/hamishmorgan/fat-controller/internal/railway"
)

// configSetter allows injection for tests.
type configSetter interface {
	SetVar(ctx context.Context, service, key, value string) error
}

// RunConfigSet validates the path, checks confirm/dry-run, and calls the setter.
// In dry-run mode (default when --yes is not set), it writes a preview
// message to out and returns nil. Pass out=nil to use os.Stdout.
func RunConfigSet(ctx context.Context, path, value string, dryRun, yes bool, setter configSetter, out io.Writer) error {
	slog.Debug("starting config set", "path", path)
	if out == nil {
		out = os.Stdout
	}
	parsed, err := config.ParsePath(path)
	if err != nil {
		return err
	}
	if parsed.Section != "variables" || parsed.Key == "" {
		return errors.New("config set currently supports only variables (path: service.variables.KEY)")
	}
	if dryRun {
		_, err := fmt.Fprintf(out, "dry run: would set %s = %q\n", path, value)
		return err
	}
	if !yes {
		if !prompt.StdinIsInteractive() {
			_, err := fmt.Fprintf(out, "dry run: would set %s = %q (use --yes to apply)\n", path, value)
			return err
		}
		_, _ = fmt.Fprintf(out, "Will set %s = %q\n\n", path, value)
		confirmed, err := prompt.Confirm("Are you sure?", false)
		if err != nil {
			return fmt.Errorf("reading confirmation: %w", err)
		}
		if !confirmed {
			_, err := fmt.Fprintln(out, "Cancelled.")
			return err
		}
	}
	return setter.SetVar(ctx, parsed.Service, parsed.Key, value)
}

// railwaySetter wraps Railway mutations for set operations.
type railwaySetter struct {
	client        *railway.Client
	projectID     string
	environmentID string
	skipDeploys   bool
}

func (r *railwaySetter) SetVar(ctx context.Context, service, key, value string) error {
	serviceID, err := railway.ResolveServiceID(ctx, r.client, r.projectID, service)
	if err != nil {
		return err
	}
	return railway.UpsertVariable(ctx, r.client, r.projectID, r.environmentID, serviceID, key, value, r.skipDeploys)
}

// Run implements `config set`.
func (c *ConfigSetCmd) Run(globals *Globals) error {
	ctx, cancel := globals.TimeoutContext(globals.BaseCtx)
	defer cancel()
	client, err := newClient(globals)
	if err != nil {
		return err
	}
	fetcher := &defaultConfigFetcher{client: client}
	projID, envID, err := fetcher.Resolve(ctx, c.Workspace, c.Project, c.Environment)
	if err != nil {
		return err
	}
	setter := &railwaySetter{
		client:        client,
		projectID:     projID,
		environmentID: envID,
		skipDeploys:   c.SkipDeploys,
	}
	return RunConfigSet(ctx, c.Path, c.Value, c.DryRun, c.Yes, setter, os.Stdout)
}
