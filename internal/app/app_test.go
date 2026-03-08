package app_test

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/app"
	"github.com/hamishmorgan/fat-controller/internal/config"
)

type fakeFetcher struct {
	resolveWorkspace   string
	resolveProject     string
	resolveEnvironment string

	fetchProjectID     string
	fetchEnvironmentID string
	fetchServices      []string

	projectID     string
	environmentID string
	live          *config.LiveConfig
}

func (f *fakeFetcher) Resolve(_ context.Context, workspace, project, environment string) (string, string, error) {
	f.resolveWorkspace = workspace
	f.resolveProject = project
	f.resolveEnvironment = environment
	return f.projectID, f.environmentID, nil
}

func (f *fakeFetcher) Fetch(_ context.Context, projectID, environmentID string, services []string) (*config.LiveConfig, error) {
	f.fetchProjectID = projectID
	f.fetchEnvironmentID = environmentID
	f.fetchServices = services
	return f.live, nil
}

func TestLoadAndFetch_UsesConfigFallbackAndInterpolates(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "fat-controller.toml")
	envPath := filepath.Join(dir, "test.env")

	writeFile(t, configPath, `name = "production"

[workspace]
name = "acme"

[project]
name = "proj"

[tool]
env_file = "test.env"

[variables]
TOKEN = "${TOKEN}"

[[service]]
name = "api"
[service.variables]
PORT = "8080"
`)
	writeFile(t, envPath, "TOKEN=secret")

	fetcher := &fakeFetcher{
		projectID:     "proj-1",
		environmentID: "env-1",
		live: &config.LiveConfig{
			ProjectID:     "proj-1",
			EnvironmentID: "env-1",
			Services: map[string]*config.ServiceConfig{
				"api": {Name: "api", ID: "svc-1"},
			},
		},
	}

	got, err := app.LoadAndFetch(context.Background(), "", "", "", dir, configPath, "", fetcher)
	if err != nil {
		t.Fatalf("LoadAndFetch() error: %v", err)
	}
	if fetcher.resolveWorkspace != "acme" {
		t.Fatalf("Resolve workspace = %q, want %q", fetcher.resolveWorkspace, "acme")
	}
	if fetcher.resolveProject != "proj" {
		t.Fatalf("Resolve project = %q, want %q", fetcher.resolveProject, "proj")
	}
	if fetcher.resolveEnvironment != "production" {
		t.Fatalf("Resolve environment = %q, want %q", fetcher.resolveEnvironment, "production")
	}
	if got.ProjectID != "proj-1" || got.EnvironmentID != "env-1" {
		t.Fatalf("ProjectID/EnvironmentID = %q/%q, want proj-1/env-1", got.ProjectID, got.EnvironmentID)
	}
	if got.Desired.Variables["TOKEN"] != "secret" {
		t.Fatalf("TOKEN = %q, want %q", got.Desired.Variables["TOKEN"], "secret")
	}
}

func TestLoadAndFetch_ServiceFilter(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "fat-controller.toml")

	writeFile(t, configPath, `name = "production"

[project]
name = "proj"

[[service]]
name = "api"

[[service]]
name = "worker"
`)

	fetcher := &fakeFetcher{
		projectID:     "proj-1",
		environmentID: "env-1",
		live: &config.LiveConfig{
			ProjectID:     "proj-1",
			EnvironmentID: "env-1",
			Services: map[string]*config.ServiceConfig{
				"api":    {Name: "api", ID: "svc-1"},
				"worker": {Name: "worker", ID: "svc-2"},
			},
		},
	}

	got, err := app.LoadAndFetch(context.Background(), "", "", "", dir, configPath, "worker", fetcher)
	if err != nil {
		t.Fatalf("LoadAndFetch() error: %v", err)
	}
	if !reflect.DeepEqual(fetcher.fetchServices, []string{"worker"}) {
		t.Fatalf("Fetch services = %v, want [worker]", fetcher.fetchServices)
	}
	if len(got.Desired.Services) != 1 {
		t.Fatalf("Desired services len = %d, want 1", len(got.Desired.Services))
	}
	if got.Desired.Services[0].Name != "worker" {
		t.Fatalf("Desired service name = %q, want %q", got.Desired.Services[0].Name, "worker")
	}
}

func TestScopeDesiredByPath(t *testing.T) {
	cfg := &config.DesiredConfig{
		Variables: config.Variables{"A": "1"},
		Services: []*config.DesiredService{
			{Name: "api"},
			{Name: "worker"},
		},
	}

	variables := app.ScopeDesiredByPath(cfg, "variables")
	if len(variables.Services) != 0 || variables.Variables["A"] != "1" {
		t.Fatalf("variables scope mismatch: %#v", variables)
	}

	api := app.ScopeDesiredByPath(cfg, "api")
	if len(api.Services) != 1 || api.Services[0].Name != "api" {
		t.Fatalf("api scope mismatch: %#v", api.Services)
	}

	apiVars := app.ScopeDesiredByPath(cfg, "api.variables")
	if len(apiVars.Services) != 1 || apiVars.Services[0].Name != "api" {
		t.Fatalf("api.variables scope mismatch: %#v", apiVars.Services)
	}

	unknown := app.ScopeDesiredByPath(cfg, "missing")
	if unknown != cfg {
		t.Fatalf("missing path should return original config")
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
