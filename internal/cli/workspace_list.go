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

// workspaceLister abstracts workspace listing for tests.
type workspaceLister interface {
	ListWorkspaces(ctx context.Context) ([]railway.WorkspaceInfo, error)
}

type defaultWorkspaceLister struct {
	client *railway.Client
}

func (d *defaultWorkspaceLister) ListWorkspaces(ctx context.Context) ([]railway.WorkspaceInfo, error) {
	return railway.ListWorkspaces(ctx, d.client)
}

// RunWorkspaceList is the testable core of `workspace list`.
func RunWorkspaceList(ctx context.Context, globals *Globals, lister workspaceLister, out io.Writer) error {
	if out == nil {
		out = os.Stdout
	}
	slog.Debug("listing workspaces")
	workspaces, err := lister.ListWorkspaces(ctx)
	if err != nil {
		return err
	}
	slog.Debug("listed workspaces", "count", len(workspaces))
	if len(workspaces) == 0 {
		_, err := fmt.Fprintln(out, "No workspaces found.")
		return err
	}
	switch globals.Output {
	case "json":
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(workspaces)
	case "toml":
		wrapper := struct {
			Workspaces []railway.WorkspaceInfo `toml:"workspaces"`
		}{Workspaces: workspaces}
		return toml.NewEncoder(out).Encode(wrapper)
	default:
		for _, ws := range workspaces {
			if _, err := fmt.Fprintf(out, "%s  %s\n", ws.Name, ws.ID); err != nil {
				return err
			}
		}
		return nil
	}
}

// Run implements `workspace list`.
func (c *WorkspaceListCmd) Run(globals *Globals) error {
	ctx, cancel := c.TimeoutContext(globals.BaseCtx)
	defer cancel()
	client, err := newClient(&c.ApiFlags, globals.BaseCtx)
	if err != nil {
		return err
	}
	lister := &defaultWorkspaceLister{client: client}
	return RunWorkspaceList(ctx, globals, lister, os.Stdout)
}
