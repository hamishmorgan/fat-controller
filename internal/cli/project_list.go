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

// ProjectInfo is a simplified project record for display.
type ProjectInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// projectLister abstracts project listing for tests.
type projectLister interface {
	ListProjects(ctx context.Context) ([]ProjectInfo, error)
}

type defaultProjectLister struct {
	client *railway.Client
}

func (d *defaultProjectLister) ListProjects(ctx context.Context) ([]ProjectInfo, error) {
	resp, err := railway.Projects(ctx, d.client.GQL())
	if err != nil {
		return nil, err
	}
	var projects []ProjectInfo
	for _, edge := range resp.Projects.Edges {
		projects = append(projects, ProjectInfo{
			ID:   edge.Node.Id,
			Name: edge.Node.Name,
		})
	}
	return projects, nil
}

// RunProjectList is the testable core of `project list`.
func RunProjectList(ctx context.Context, globals *Globals, lister projectLister, out io.Writer) error {
	if out == nil {
		out = os.Stdout
	}
	projects, err := lister.ListProjects(ctx)
	if err != nil {
		return err
	}
	if len(projects) == 0 {
		fmt.Fprintln(out, "No projects found.")
		return nil
	}
	switch globals.Output {
	case "json":
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(projects)
	default:
		for _, p := range projects {
			fmt.Fprintf(out, "%s  %s\n", p.Name, p.ID)
		}
		return nil
	}
}

// Run implements `project list`.
func (c *ProjectListCmd) Run(globals *Globals) error {
	store := auth.NewTokenStore(auth.WithFallbackPath(platform.AuthFilePath()))
	resolved, err := auth.ResolveAuth(globals.Token, store)
	if err != nil {
		return err
	}
	client := railway.NewClient(railway.Endpoint, resolved, store, auth.NewOAuthClient())
	lister := &defaultProjectLister{client: client}
	return RunProjectList(context.Background(), globals, lister, os.Stdout)
}
