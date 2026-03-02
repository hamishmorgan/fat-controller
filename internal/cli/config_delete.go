package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/hamishmorgan/fat-controller/internal/auth"
	"github.com/hamishmorgan/fat-controller/internal/config"
	"github.com/hamishmorgan/fat-controller/internal/platform"
	"github.com/hamishmorgan/fat-controller/internal/railway"
)

// configDeleter allows injection for tests.
type configDeleter interface {
	DeleteVar(ctx context.Context, service, key string) error
}

// RunConfigDelete validates the path, checks confirm/dry-run, and calls the deleter.
func RunConfigDelete(ctx context.Context, globals *Globals, path string, deleter configDeleter) error {
	parsed, err := config.ParsePath(path)
	if err != nil {
		return err
	}
	if parsed.Section != "variables" || parsed.Key == "" {
		return errors.New("config delete currently supports only variables (path: service.variables.KEY)")
	}
	if !globals.Confirm || globals.DryRun {
		return fmt.Errorf("dry run: would delete %s (use --confirm to apply)", path)
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
	return railway.DeleteVariable(ctx, r.client, r.projectID, r.environmentID, service, key)
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
	projID, envID, err := fetcher.Resolve(context.Background(), globals.Project, globals.Environment)
	if err != nil {
		return err
	}
	deleter := &railwayDeleter{
		client:        client,
		projectID:     projID,
		environmentID: envID,
	}
	return RunConfigDelete(context.Background(), globals, c.Path, deleter)
}
