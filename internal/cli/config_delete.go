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

// configDeleter allows injection for tests.
type configDeleter interface {
	DeleteVar(ctx context.Context, service, key string) error
}

// RunConfigDelete validates the path, checks confirm/dry-run, and calls the deleter.
// In dry-run mode (default when --yes is not set), it writes a preview
// message to out and returns nil. Pass out=nil to use os.Stdout.
func RunConfigDelete(ctx context.Context, path string, dryRun, yes bool, deleter configDeleter, out io.Writer) error {
	slog.Debug("starting config delete", "path", path)
	if out == nil {
		out = os.Stdout
	}
	parsed, err := config.ParsePath(path)
	if err != nil {
		return err
	}
	if parsed.Section != "variables" || parsed.Key == "" {
		return errors.New("config delete currently supports only variables (path: service.variables.KEY)")
	}
	if dryRun {
		_, err := fmt.Fprintf(out, "dry run: would delete %s\n", path)
		return err
	}
	if !yes {
		if !prompt.StdinIsInteractive() {
			_, err := fmt.Fprintf(out, "dry run: would delete %s (use --yes to apply)\n", path)
			return err
		}
		_, _ = fmt.Fprintf(out, "Will delete %s\n\n", path)
		confirmed, err := prompt.Confirm("Are you sure?", false)
		if err != nil {
			return fmt.Errorf("reading confirmation: %w", err)
		}
		if !confirmed {
			_, err := fmt.Fprintln(out, "Cancelled.")
			return err
		}
	}
	return deleter.DeleteVar(ctx, parsed.Service, parsed.Key)
}

// railwayDeleter wraps Railway mutations for delete operations.
type railwayDeleter struct {
	client        *railway.Client
	projectID     string
	environmentID string
}

func (r *railwayDeleter) DeleteVar(ctx context.Context, service, key string) error {
	serviceID, err := railway.ResolveServiceID(ctx, r.client, r.projectID, service)
	if err != nil {
		return err
	}
	return railway.DeleteVariable(ctx, r.client, r.projectID, r.environmentID, serviceID, key)
}

// Run implements `config delete`.
func (c *ConfigDeleteCmd) Run(globals *Globals) error {
	ctx, cancel := globals.TimeoutContext(globals.BaseCtx)
	defer cancel()
	client, err := newClient(globals)
	if err != nil {
		return err
	}
	fetcher := &defaultConfigFetcher{client: client}
	projID, envID, err := fetcher.Resolve(ctx, globals.Workspace, globals.Project, globals.Environment)
	if err != nil {
		return err
	}
	deleter := &railwayDeleter{
		client:        client,
		projectID:     projID,
		environmentID: envID,
	}
	return RunConfigDelete(ctx, c.Path, c.DryRun, c.Yes, deleter, os.Stdout)
}
