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
	"github.com/hamishmorgan/fat-controller/internal/railway"
)

// configFetcher allows injection for tests.
type configFetcher interface {
	Resolve(ctx context.Context, project, environment string) (string, string, error)
	Fetch(ctx context.Context, projectID, environmentID, service string) (*config.LiveConfig, error)
}

type defaultConfigFetcher struct {
	client *railway.Client
}

func (d *defaultConfigFetcher) Resolve(ctx context.Context, project, environment string) (string, string, error) {
	return railway.ResolveProjectEnvironment(ctx, d.client, project, environment)
}

func (d *defaultConfigFetcher) Fetch(ctx context.Context, projectID, environmentID, service string) (*config.LiveConfig, error) {
	return railway.FetchLiveConfig(ctx, d.client, projectID, environmentID, service)
}

// SetOutput overrides the output writer (for testing).
func (c *ConfigGetCmd) SetOutput(w io.Writer) {
	c.output = w
}

// Run implements `config get`.
func (c *ConfigGetCmd) Run(globals *Globals) error {
	store := auth.NewTokenStore(auth.WithFallbackPath(platform.AuthFilePath()))
	resolved, err := auth.ResolveAuth(globals.Token, store)
	if err != nil {
		return err
	}
	client := railway.NewClient(railway.Endpoint, resolved, store, auth.NewOAuthClient())
	fetcher := &defaultConfigFetcher{client: client}
	return RunConfigGet(context.Background(), globals, c.Path, fetcher, c.output)
}

// RunConfigGet is the testable core of `config get`.
func RunConfigGet(ctx context.Context, globals *Globals, path string, fetcher configFetcher, out io.Writer) error {
	if out == nil {
		out = os.Stdout
	}
	projID, envID, err := fetcher.Resolve(ctx, globals.Project, globals.Environment)
	if err != nil {
		return err
	}
	service := globals.Service
	if path != "" {
		parsed, err := config.ParsePath(path)
		if err != nil {
			return err
		}
		if parsed.Service != "" {
			service = parsed.Service
		}
	}
	cfg, err := fetcher.Fetch(ctx, projID, envID, service)
	if err != nil {
		return err
	}
	if cfg == nil {
		return errors.New("no config returned")
	}
	output, err := config.Render(*cfg, globals.Output, globals.Full)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, output)
	return err
}
