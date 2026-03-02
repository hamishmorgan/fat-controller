package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/hamishmorgan/fat-controller/internal/auth"
	"github.com/hamishmorgan/fat-controller/internal/platform"
	"github.com/hamishmorgan/fat-controller/internal/railway"
)

// EnvironmentInfo is a simplified environment record for display.
type EnvironmentInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// environmentLister abstracts environment listing for tests.
type environmentLister interface {
	ListEnvironments(ctx context.Context, projectID string) ([]EnvironmentInfo, error)
}

type defaultEnvironmentLister struct {
	client *railway.Client
}

func (d *defaultEnvironmentLister) ListEnvironments(ctx context.Context, projectID string) ([]EnvironmentInfo, error) {
	resp, err := railway.Environments(ctx, d.client.GQL(), projectID)
	if err != nil {
		return nil, err
	}
	envs := make([]EnvironmentInfo, 0, len(resp.Environments.Edges))
	for _, edge := range resp.Environments.Edges {
		envs = append(envs, EnvironmentInfo{
			ID:   edge.Node.Id,
			Name: edge.Node.Name,
		})
	}
	return envs, nil
}

// RunEnvironmentList is the testable core of `environment list`.
func RunEnvironmentList(ctx context.Context, globals *Globals, projectID string, lister environmentLister, out io.Writer) error {
	if out == nil {
		out = os.Stdout
	}
	envs, err := lister.ListEnvironments(ctx, projectID)
	if err != nil {
		return err
	}
	if len(envs) == 0 {
		_, err := fmt.Fprintln(out, "No environments found.")
		return err
	}
	switch globals.Output {
	case "json":
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(envs)
	default:
		for _, e := range envs {
			if _, err := fmt.Fprintf(out, "%s  %s\n", e.Name, e.ID); err != nil {
				return err
			}
		}
		return nil
	}
}

// Run implements `environment list`.
// Requires --project flag (or env var) to know which project to list environments for.
func (c *EnvironmentListCmd) Run(globals *Globals) error {
	store := auth.NewTokenStore(auth.WithFallbackPath(platform.AuthFilePath()))
	resolved, err := auth.ResolveAuth(globals.Token, store)
	if err != nil {
		return err
	}
	client := railway.NewClient(railway.Endpoint, resolved, store, auth.NewOAuthClient())

	projID, err := railway.ResolveProjectID(context.Background(), client, globals.Workspace, globals.Project)
	if err != nil {
		return err
	}

	lister := &defaultEnvironmentLister{client: client}
	return RunEnvironmentList(context.Background(), globals, projID, lister, os.Stdout)
}
