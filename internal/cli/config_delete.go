package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/hamishmorgan/fat-controller/internal/auth"
	"github.com/hamishmorgan/fat-controller/internal/config"
	"github.com/hamishmorgan/fat-controller/internal/platform"
	"github.com/hamishmorgan/fat-controller/internal/prompt"
	"github.com/hamishmorgan/fat-controller/internal/railway"
)

// configDeleter allows injection for tests.
type configDeleter interface {
	DeleteVar(ctx context.Context, service, key string) error
}

// RunConfigDelete validates the path, checks confirm/dry-run, and calls the deleter.
// In dry-run mode (default when --confirm is not set), it writes a preview
// message to out and returns nil. Pass out=nil to use os.Stdout.
func RunConfigDelete(ctx context.Context, globals *Globals, path string, deleter configDeleter, out io.Writer) error {
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
	if globals.DryRun {
		_, err := fmt.Fprintf(out, "dry run: would delete %s\n", path)
		return err
	}
	if !globals.Confirm {
		if !prompt.StdinIsInteractive() {
			_, err := fmt.Fprintf(out, "dry run: would delete %s (use --confirm to apply)\n", path)
			return err
		}
		fmt.Fprintf(out, "Will delete %s\n\n", path)
		confirmed, err := prompt.ConfirmRW(os.Stdin, out, "Are you sure?", false)
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
	store := auth.NewTokenStore(auth.WithFallbackPath(platform.AuthFilePath()))
	resolved, err := auth.ResolveAuth(globals.Token, store)
	if err != nil {
		return err
	}
	client := railway.NewClient(railway.Endpoint, resolved, store, auth.NewOAuthClient())
	fetcher := &defaultConfigFetcher{client: client}
	projID, envID, err := fetcher.Resolve(context.Background(), globals.Workspace, globals.Project, globals.Environment)
	if err != nil {
		return err
	}
	deleter := &railwayDeleter{
		client:        client,
		projectID:     projID,
		environmentID: envID,
	}
	return RunConfigDelete(context.Background(), globals, c.Path, deleter, os.Stdout)
}
