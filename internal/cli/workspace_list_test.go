package cli_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/cli"
)

type fakeWorkspaceLister struct {
	workspaces []cli.WorkspaceInfo
}

func (f *fakeWorkspaceLister) ListWorkspaces(ctx context.Context) ([]cli.WorkspaceInfo, error) {
	return f.workspaces, nil
}

func TestRunWorkspaceList_Text(t *testing.T) {
	lister := &fakeWorkspaceLister{
		workspaces: []cli.WorkspaceInfo{
			{ID: "ws-1", Name: "alpha"},
			{ID: "ws-2", Name: "beta"},
		},
	}
	var buf bytes.Buffer
	err := cli.RunWorkspaceList(context.Background(), &cli.Globals{Output: "text"}, lister, &buf)
	if err != nil {
		t.Fatalf("RunWorkspaceList() error: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "alpha") {
		t.Errorf("expected alpha in output, got:\n%s", got)
	}
}

func TestRunWorkspaceList_JSON(t *testing.T) {
	lister := &fakeWorkspaceLister{
		workspaces: []cli.WorkspaceInfo{{ID: "ws-1", Name: "alpha"}},
	}
	var buf bytes.Buffer
	err := cli.RunWorkspaceList(context.Background(), &cli.Globals{Output: "json"}, lister, &buf)
	if err != nil {
		t.Fatalf("RunWorkspaceList() error: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, `"id"`) {
		t.Errorf("expected JSON with id field, got:\n%s", got)
	}
}

func TestRunWorkspaceList_TOML(t *testing.T) {
	lister := &fakeWorkspaceLister{
		workspaces: []cli.WorkspaceInfo{
			{ID: "ws-1", Name: "alpha"},
			{ID: "ws-2", Name: "beta"},
		},
	}
	var buf bytes.Buffer
	err := cli.RunWorkspaceList(context.Background(), &cli.Globals{Output: "toml"}, lister, &buf)
	if err != nil {
		t.Fatalf("RunWorkspaceList() error: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "[[workspaces]]") {
		t.Errorf("expected TOML array of tables header, got:\n%s", got)
	}
	if !strings.Contains(got, `id = "ws-1"`) {
		t.Errorf("expected ws-1 id in output, got:\n%s", got)
	}
	if !strings.Contains(got, `name = "alpha"`) {
		t.Errorf("expected alpha name in output, got:\n%s", got)
	}
	if !strings.Contains(got, `name = "beta"`) {
		t.Errorf("expected beta name in output, got:\n%s", got)
	}
}

func TestRunWorkspaceList_Empty(t *testing.T) {
	lister := &fakeWorkspaceLister{}
	var buf bytes.Buffer
	err := cli.RunWorkspaceList(context.Background(), &cli.Globals{Output: "text"}, lister, &buf)
	if err != nil {
		t.Fatalf("RunWorkspaceList() error: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "No workspaces") {
		t.Errorf("expected 'No workspaces' message, got:\n%s", got)
	}
}
