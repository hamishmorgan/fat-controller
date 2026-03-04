package cli_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/cli"
)

type fakeEnvironmentLister struct {
	environments []cli.EnvironmentInfo
	err          error
}

func (f *fakeEnvironmentLister) ListEnvironments(ctx context.Context, projectID string) ([]cli.EnvironmentInfo, error) {
	return f.environments, f.err
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

func TestRunEnvironmentList_TOML(t *testing.T) {
	lister := &fakeEnvironmentLister{
		environments: []cli.EnvironmentInfo{
			{ID: "env-1", Name: "production"},
			{ID: "env-2", Name: "staging"},
		},
	}
	var buf bytes.Buffer
	err := cli.RunEnvironmentList(context.Background(), &cli.Globals{Output: "toml"}, "proj-1", lister, &buf)
	if err != nil {
		t.Fatalf("RunEnvironmentList() error: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "[[environments]]") {
		t.Errorf("expected TOML array of tables header, got:\n%s", got)
	}
	if !strings.Contains(got, `id = "env-1"`) {
		t.Errorf("expected env-1 id in output, got:\n%s", got)
	}
	if !strings.Contains(got, `name = "production"`) {
		t.Errorf("expected production name in output, got:\n%s", got)
	}
	if !strings.Contains(got, `name = "staging"`) {
		t.Errorf("expected staging name in output, got:\n%s", got)
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

func TestRunEnvironmentList_PropagatesError(t *testing.T) {
	lister := &fakeEnvironmentLister{err: errors.New("api error")}
	var buf bytes.Buffer
	err := cli.RunEnvironmentList(context.Background(), &cli.Globals{Output: "text"}, "proj-1", lister, &buf)
	if err == nil {
		t.Fatal("expected error from lister")
	}
	if !strings.Contains(err.Error(), "api error") {
		t.Errorf("unexpected error: %v", err)
	}
}
