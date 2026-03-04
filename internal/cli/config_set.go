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

// configSetter allows injection for tests.
type configSetter interface {
	SetVar(ctx context.Context, service, key, value string) error
}

// RunConfigSet validates the path, checks confirm/dry-run, and calls the setter.
// In dry-run mode (default when --confirm is not set), it writes a preview
// message to out and returns nil. Pass out=nil to use os.Stdout.
func RunConfigSet(ctx context.Context, globals *Globals, path, value string, setter configSetter, out io.Writer) error {
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
	if globals.DryRun {
		_, err := fmt.Fprintf(out, "dry run: would set %s = %q\n", path, value)
		return err
	}
	if !globals.Confirm {
		if !prompt.StdinIsInteractive() {
			_, err := fmt.Fprintf(out, "dry run: would set %s = %q (use --confirm to apply)\n", path, value)
			return err
		}
		fmt.Fprintf(out, "Will set %s = %q\n\n", path, value)
		confirmed, err := prompt.ConfirmRW(os.Stdin, out, "Are you sure?", false)
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
	setter := &railwaySetter{
		client:        client,
		projectID:     projID,
		environmentID: envID,
		skipDeploys:   globals.SkipDeploys,
	}
	return RunConfigSet(context.Background(), globals, c.Path, c.Value, setter, os.Stdout)
}
