package cli_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/cli"
)

type fakeProjectLister struct {
	projects []cli.ProjectInfo
	err      error
}

func (f *fakeProjectLister) ListProjects(ctx context.Context, workspace string) ([]cli.ProjectInfo, error) {
	return f.projects, f.err
}

func TestRunProjectList_Text(t *testing.T) {
	lister := &fakeProjectLister{
		projects: []cli.ProjectInfo{
			{ID: "proj-1", Name: "my-app"},
			{ID: "proj-2", Name: "my-api"},
		},
	}
	var buf bytes.Buffer
	err := cli.RunProjectList(context.Background(), &cli.Globals{Output: "text"}, lister, &buf)
	if err != nil {
		t.Fatalf("RunProjectList() error: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "my-app") {
		t.Errorf("expected my-app in output, got:\n%s", got)
	}
	if !strings.Contains(got, "my-api") {
		t.Errorf("expected my-api in output, got:\n%s", got)
	}
}

func TestRunProjectList_JSON(t *testing.T) {
	lister := &fakeProjectLister{
		projects: []cli.ProjectInfo{
			{ID: "proj-1", Name: "my-app"},
		},
	}
	var buf bytes.Buffer
	err := cli.RunProjectList(context.Background(), &cli.Globals{Output: "json"}, lister, &buf)
	if err != nil {
		t.Fatalf("RunProjectList() error: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, `"id"`) {
		t.Errorf("expected JSON with id field, got:\n%s", got)
	}
}

func TestRunProjectList_Empty(t *testing.T) {
	lister := &fakeProjectLister{}
	var buf bytes.Buffer
	err := cli.RunProjectList(context.Background(), &cli.Globals{Output: "text"}, lister, &buf)
	if err != nil {
		t.Fatalf("RunProjectList() error: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "No projects") {
		t.Errorf("expected 'No projects' message, got:\n%s", got)
	}
}

func TestRunProjectList_PropagatesError(t *testing.T) {
	lister := &fakeProjectLister{err: errors.New("api error")}
	var buf bytes.Buffer
	err := cli.RunProjectList(context.Background(), &cli.Globals{Output: "text"}, lister, &buf)
	if err == nil {
		t.Fatal("expected error from lister")
	}
	if !strings.Contains(err.Error(), "api error") {
		t.Errorf("unexpected error: %v", err)
	}
}
