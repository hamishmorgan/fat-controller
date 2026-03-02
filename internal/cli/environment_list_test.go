package cli_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/cli"
)

type fakeEnvironmentLister struct {
	environments []cli.EnvironmentInfo
}

func (f *fakeEnvironmentLister) ListEnvironments(ctx context.Context, projectID string) ([]cli.EnvironmentInfo, error) {
	return f.environments, nil
}

func TestRunEnvironmentList_Text(t *testing.T) {
	lister := &fakeEnvironmentLister{
		environments: []cli.EnvironmentInfo{
			{ID: "env-1", Name: "production"},
			{ID: "env-2", Name: "staging"},
		},
	}
	var buf bytes.Buffer
	err := cli.RunEnvironmentList(context.Background(), &cli.Globals{Output: "text"}, "proj-1", lister, &buf)
	if err != nil {
		t.Fatalf("RunEnvironmentList() error: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "production") {
		t.Errorf("expected production in output, got:\n%s", got)
	}
}

func TestRunEnvironmentList_JSON(t *testing.T) {
	lister := &fakeEnvironmentLister{
		environments: []cli.EnvironmentInfo{
			{ID: "env-1", Name: "production"},
		},
	}
	var buf bytes.Buffer
	err := cli.RunEnvironmentList(context.Background(), &cli.Globals{Output: "json"}, "proj-1", lister, &buf)
	if err != nil {
		t.Fatalf("RunEnvironmentList() error: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, `"id"`) {
		t.Errorf("expected JSON with id field, got:\n%s", got)
	}
}

func TestRunEnvironmentList_Empty(t *testing.T) {
	lister := &fakeEnvironmentLister{}
	var buf bytes.Buffer
	err := cli.RunEnvironmentList(context.Background(), &cli.Globals{Output: "text"}, "proj-1", lister, &buf)
	if err != nil {
		t.Fatalf("RunEnvironmentList() error: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "No environments") {
		t.Errorf("expected 'No environments' message, got:\n%s", got)
	}
}
