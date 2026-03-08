package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/BurntSushi/toml"
	"github.com/hamishmorgan/fat-controller/internal/railway"
)

// projectLister abstracts project listing for tests.
type projectLister interface {
	ListProjects(ctx context.Context, workspace string) ([]railway.ProjectInfo, error)
}

type defaultProjectLister struct {
	client *railway.Client
}

func (d *defaultProjectLister) ListProjects(ctx context.Context, workspace string) ([]railway.ProjectInfo, error) {
	workspaceID, err := railway.ResolveWorkspaceID(ctx, d.client, workspace)
	if err != nil {
		return nil, err
	}
	return railway.ListProjects(ctx, d.client, workspaceID)
}

// RunProjectList is the testable core of `project list`.
func RunProjectList(ctx context.Context, globals *Globals, workspace string, lister projectLister, out io.Writer) error {
	if out == nil {
		out = os.Stdout
	}
	slog.Debug("listing projects")
	projects, err := lister.ListProjects(ctx, workspace)
	if err != nil {
		return err
	}
	slog.Debug("listed projects", "count", len(projects))
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
			Projects []railway.ProjectInfo `toml:"projects"`
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
	ctx, cancel := c.TimeoutContext(globals.BaseCtx)
	defer cancel()
	client, err := newClient(&c.ApiFlags, globals.BaseCtx)
	if err != nil {
		return err
	}
	lister := &defaultProjectLister{client: client}
	return RunProjectList(ctx, globals, c.Workspace, lister, os.Stdout)
}
