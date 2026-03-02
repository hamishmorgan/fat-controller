package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"

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
	var envs []EnvironmentInfo
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
	if globals.Project == "" {
		return fmt.Errorf("--project is required for environment list")
	}
	store := auth.NewTokenStore(auth.WithFallbackPath(platform.AuthFilePath()))
	resolved, err := auth.ResolveAuth(globals.Token, store)
	if err != nil {
		return err
	}
	client := railway.NewClient(railway.Endpoint, resolved, store, auth.NewOAuthClient())

	projID := globals.Project
	if !isUUID(projID) {
		projLister := &defaultProjectLister{client: client}
		projects, err := projLister.ListProjects(context.Background())
		if err != nil {
			return err
		}
		found := false
		for _, p := range projects {
			if p.Name == globals.Project {
				projID = p.ID
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("project not found: %s", globals.Project)
		}
	}

	lister := &defaultEnvironmentLister{client: client}
	return RunEnvironmentList(context.Background(), globals, projID, lister, os.Stdout)
}

var uuidRE = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func isUUID(s string) bool {
	return uuidRE.MatchString(s)
}
