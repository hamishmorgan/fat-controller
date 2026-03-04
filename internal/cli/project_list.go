package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/BurntSushi/toml"
	"github.com/hamishmorgan/fat-controller/internal/railway"
)

// ProjectInfo is a simplified project record for display.
type ProjectInfo struct {
	ID   string `json:"id" toml:"id"`
	Name string `json:"name" toml:"name"`
}

// projectLister abstracts project listing for tests.
type projectLister interface {
	ListProjects(ctx context.Context, workspace string) ([]ProjectInfo, error)
}

type defaultProjectLister struct {
	client *railway.Client
}

func (d *defaultProjectLister) ListProjects(ctx context.Context, workspace string) ([]ProjectInfo, error) {
	workspaceID, err := railway.ResolveWorkspaceID(ctx, d.client, workspace)
	if err != nil {
		return nil, err
	}
	resp, err := railway.Projects(ctx, d.client.GQL(), &workspaceID)
	if err != nil {
		return nil, err
	}
	projects := make([]ProjectInfo, 0, len(resp.Projects.Edges))
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
	projects, err := lister.ListProjects(ctx, globals.Workspace)
	if err != nil {
		return err
	}
	if len(projects) == 0 {
		_, err := fmt.Fprintln(out, "No projects found.")
		return err
	}
	switch globals.Output {
	case "json":
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(projects)
	case "toml":
		wrapper := struct {
			Projects []ProjectInfo `toml:"projects"`
		}{Projects: projects}
		return toml.NewEncoder(out).Encode(wrapper)
	default:
		for _, p := range projects {
			if _, err := fmt.Fprintf(out, "%s  %s\n", p.Name, p.ID); err != nil {
				return err
			}
		}
		return nil
	}
}

// Run implements `project list`.
func (c *ProjectListCmd) Run(globals *Globals) error {
	ctx, cancel := globals.TimeoutContext(context.Background())
	defer cancel()
	client, err := newClient(globals)
	if err != nil {
		return err
	}
	lister := &defaultProjectLister{client: client}
	return RunProjectList(ctx, globals, lister, os.Stdout)
}
