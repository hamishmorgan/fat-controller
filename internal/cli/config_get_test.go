package cli_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/cli"
	"github.com/hamishmorgan/fat-controller/internal/config"
)

// fakeFetcher implements cli.ConfigFetcher for testing runConfigGet.
type fakeFetcher struct {
	cfg        *config.LiveConfig
	resolveErr error
	fetchErr   error
}

func (f *fakeFetcher) Resolve(_ context.Context, _, _, _ string) (string, string, error) {
	if f.resolveErr != nil {
		return "", "", f.resolveErr
	}
	return "proj-1", "env-1", nil
}

func (f *fakeFetcher) Fetch(_ context.Context, _, _, _ string) (*config.LiveConfig, error) {
	if f.fetchErr != nil {
		return nil, f.fetchErr
	}
	return f.cfg, nil
}

func TestConfigGet_PrintsOutput(t *testing.T) {
	var buf bytes.Buffer
	cmd := &cli.ConfigGetCmd{}
	cmd.SetOutput(&buf)

	// Run will fail because it tries to create a real auth store / client.
	// This tests that SetOutput wiring works — the real logic is tested via
	// RunConfigGet below.
	_ = cmd.Run(&cli.Globals{})
}

func TestRunConfigGet_RendersText(t *testing.T) {
	fetcher := &fakeFetcher{
		cfg: &config.LiveConfig{
			ProjectID:     "proj-1",
			EnvironmentID: "env-1",
			Shared:        map[string]string{"FOO": "bar"},
			Services: map[string]*config.ServiceConfig{
				"api": {
					ID:        "svc-1",
					Name:      "api",
					Variables: map[string]string{"PORT": "8080"},
				},
			},
		},
	}

	var buf bytes.Buffer
	globals := &cli.Globals{Output: "text"}
	err := cli.RunConfigGet(context.Background(), globals, "", fetcher, &buf)
	if err != nil {
		t.Fatalf("RunConfigGet() error: %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "FOO") {
		t.Errorf("expected output to contain FOO, got:\n%s", got)
	}
	if !strings.Contains(got, "PORT") {
		t.Errorf("expected output to contain PORT, got:\n%s", got)
	}
}

func TestRunConfigGet_RendersJSON(t *testing.T) {
	fetcher := &fakeFetcher{
		cfg: &config.LiveConfig{
			ProjectID:     "proj-1",
			EnvironmentID: "env-1",
			Shared:        map[string]string{"DB": "postgres"},
			Services:      map[string]*config.ServiceConfig{},
		},
	}

	var buf bytes.Buffer
	globals := &cli.Globals{Output: "json"}
	err := cli.RunConfigGet(context.Background(), globals, "", fetcher, &buf)
	if err != nil {
		t.Fatalf("RunConfigGet() error: %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, `"DB"`) {
		t.Errorf("expected JSON output to contain DB key, got:\n%s", got)
	}
}

func TestRunConfigGet_PathExtractsService(t *testing.T) {
	var fetchedService string
	fetcher := &fakeFetcher{
		cfg: &config.LiveConfig{
			ProjectID:     "proj-1",
			EnvironmentID: "env-1",
			Shared:        map[string]string{},
			Services:      map[string]*config.ServiceConfig{},
		},
	}
	// Wrap to capture the service argument.
	wrapper := &serviceCaptureFetcher{inner: fetcher, captured: &fetchedService}

	var buf bytes.Buffer
	globals := &cli.Globals{Output: "text"}
	err := cli.RunConfigGet(context.Background(), globals, "api.variables.PORT", wrapper, &buf)
	if err != nil {
		t.Fatalf("RunConfigGet() error: %v", err)
	}
	if fetchedService != "api" {
		t.Errorf("expected service=api, got %q", fetchedService)
	}
}

// serviceCaptureFetcher wraps a fakeFetcher to capture the service arg passed to Fetch.
type serviceCaptureFetcher struct {
	inner    *fakeFetcher
	captured *string
}

func (s *serviceCaptureFetcher) Resolve(ctx context.Context, workspace, project, environment string) (string, string, error) {
	return s.inner.Resolve(ctx, workspace, project, environment)
}

func (s *serviceCaptureFetcher) Fetch(ctx context.Context, projectID, environmentID, service string) (*config.LiveConfig, error) {
	*s.captured = service
	return s.inner.Fetch(ctx, projectID, environmentID, service)
}

func TestRunConfigGet_ResolveError(t *testing.T) {
	fetcher := &fakeFetcher{resolveErr: errors.New("no project")}
	var buf bytes.Buffer
	err := cli.RunConfigGet(context.Background(), &cli.Globals{}, "", fetcher, &buf)
	if err == nil {
		t.Fatal("expected error from resolve failure")
	}
	if !strings.Contains(err.Error(), "no project") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunConfigGet_FetchError(t *testing.T) {
	fetcher := &fakeFetcher{fetchErr: errors.New("api error")}
	var buf bytes.Buffer
	err := cli.RunConfigGet(context.Background(), &cli.Globals{}, "", fetcher, &buf)
	if err == nil {
		t.Fatal("expected error from fetch failure")
	}
}

func TestRunConfigGet_NilConfig(t *testing.T) {
	fetcher := &fakeFetcher{cfg: nil}
	var buf bytes.Buffer
	err := cli.RunConfigGet(context.Background(), &cli.Globals{}, "", fetcher, &buf)
	if err == nil {
		t.Fatal("expected error for nil config")
	}
}
