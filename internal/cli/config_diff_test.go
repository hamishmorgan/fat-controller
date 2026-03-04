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

// writeTOML is defined in helpers_test.go.

func TestRunConfigDiff_ShowsChanges(t *testing.T) {
	dir := t.TempDir()
	writeTOML(t, dir, "fat-controller.toml", `
[api.variables]
PORT = "9090"
NEW_VAR = "hello"
`)
	fetcher := &fakeFetcher{
		cfg: &config.LiveConfig{
			ProjectID:     "proj-1",
			EnvironmentID: "env-1",
			Services: map[string]*config.ServiceConfig{
				"api": {
					Name:      "api",
					Variables: map[string]string{"PORT": "8080"},
				},
			},
		},
	}
	var buf bytes.Buffer
	globals := &cli.Globals{Output: "text"}
	err := cli.RunConfigDiff(context.Background(), globals, dir, nil, fetcher, &buf)
	if err != nil {
		t.Fatalf("RunConfigDiff() error: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "PORT") {
		t.Errorf("expected PORT in diff output:\n%s", got)
	}
	if !strings.Contains(got, "NEW_VAR") {
		t.Errorf("expected NEW_VAR in diff output:\n%s", got)
	}
}

func TestRunConfigDiff_NoChanges(t *testing.T) {
	dir := t.TempDir()
	writeTOML(t, dir, "fat-controller.toml", `
[api.variables]
PORT = "8080"
`)
	fetcher := &fakeFetcher{
		cfg: &config.LiveConfig{
			ProjectID:     "proj-1",
			EnvironmentID: "env-1",
			Services: map[string]*config.ServiceConfig{
				"api": {
					Name:      "api",
					Variables: map[string]string{"PORT": "8080"},
				},
			},
		},
	}
	var buf bytes.Buffer
	globals := &cli.Globals{Output: "text"}
	err := cli.RunConfigDiff(context.Background(), globals, dir, nil, fetcher, &buf)
	if err != nil {
		t.Fatalf("RunConfigDiff() error: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "No changes") {
		t.Errorf("expected 'No changes' in output:\n%s", got)
	}
}

func TestRunConfigDiff_WithInterpolation(t *testing.T) {
	t.Setenv("MY_PORT_FC_TEST", "9090")
	dir := t.TempDir()
	writeTOML(t, dir, "fat-controller.toml", `
[api.variables]
PORT = "${MY_PORT_FC_TEST}"
`)
	fetcher := &fakeFetcher{
		cfg: &config.LiveConfig{
			ProjectID:     "proj-1",
			EnvironmentID: "env-1",
			Services: map[string]*config.ServiceConfig{
				"api": {
					Name:      "api",
					Variables: map[string]string{"PORT": "8080"},
				},
			},
		},
	}
	var buf bytes.Buffer
	globals := &cli.Globals{Output: "text"}
	err := cli.RunConfigDiff(context.Background(), globals, dir, nil, fetcher, &buf)
	if err != nil {
		t.Fatalf("RunConfigDiff() error: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "9090") {
		t.Errorf("expected interpolated value 9090 in output:\n%s", got)
	}
}

func TestRunConfigDiff_MissingEnvVarErrors(t *testing.T) {
	dir := t.TempDir()
	writeTOML(t, dir, "fat-controller.toml", `
[api.variables]
SECRET = "${TOTALLY_MISSING_FC_VAR}"
`)
	fetcher := &fakeFetcher{
		cfg: &config.LiveConfig{
			ProjectID: "proj-1", EnvironmentID: "env-1",
			Services: map[string]*config.ServiceConfig{
				"api": {Name: "api", Variables: map[string]string{}},
			},
		},
	}
	var buf bytes.Buffer
	err := cli.RunConfigDiff(context.Background(), &cli.Globals{}, dir, nil, fetcher, &buf)
	if err == nil {
		t.Fatal("expected error for missing env var")
	}
	if !strings.Contains(err.Error(), "TOTALLY_MISSING_FC_VAR") {
		t.Errorf("error should mention missing var: %v", err)
	}
}

func TestRunConfigDiff_ResolveError(t *testing.T) {
	dir := t.TempDir()
	writeTOML(t, dir, "fat-controller.toml", `
[api.variables]
PORT = "8080"
`)
	fetcher := &fakeFetcher{resolveErr: errors.New("no project")}
	var buf bytes.Buffer
	err := cli.RunConfigDiff(context.Background(), &cli.Globals{}, dir, nil, fetcher, &buf)
	if err == nil {
		t.Fatal("expected error from resolve failure")
	}
}

func TestRunConfigDiff_ServiceFilter(t *testing.T) {
	dir := t.TempDir()
	writeTOML(t, dir, "fat-controller.toml", `
[api.variables]
PORT = "9090"

[worker.variables]
QUEUE = "high"
`)
	fetcher := &fakeFetcher{
		cfg: &config.LiveConfig{
			ProjectID: "proj-1", EnvironmentID: "env-1",
			Services: map[string]*config.ServiceConfig{
				"api":    {Name: "api", Variables: map[string]string{"PORT": "8080"}},
				"worker": {Name: "worker", Variables: map[string]string{"QUEUE": "default"}},
			},
		},
	}
	var buf bytes.Buffer
	globals := &cli.Globals{Service: "api"}
	err := cli.RunConfigDiff(context.Background(), globals, dir, nil, fetcher, &buf)
	if err != nil {
		t.Fatalf("RunConfigDiff() error: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "PORT") {
		t.Errorf("expected api PORT change:\n%s", got)
	}
	if strings.Contains(got, "QUEUE") {
		t.Errorf("worker should be filtered out:\n%s", got)
	}
}

// capturingFetcher is defined in helpers_test.go.

func TestRunConfigDiff_UsesConfigFileProject(t *testing.T) {
	dir := t.TempDir()
	writeTOML(t, dir, "fat-controller.toml", `
project = "my-app"
environment = "production"

[api.variables]
PORT = "9090"
`)
	// fakeFetcher doesn't validate project/environment args, but we
	// verify via a capturing fetcher that the config-file values are used.
	captureFetcher := &capturingFetcher{
		cfg: &config.LiveConfig{
			ProjectID: "proj-1", EnvironmentID: "env-1",
			Services: map[string]*config.ServiceConfig{
				"api": {Name: "api", Variables: map[string]string{"PORT": "8080"}},
			},
		},
	}
	var buf bytes.Buffer
	// Globals with empty Project/Environment — should fall back to config file.
	globals := &cli.Globals{Output: "text"}
	err := cli.RunConfigDiff(context.Background(), globals, dir, nil, captureFetcher, &buf)
	if err != nil {
		t.Fatalf("RunConfigDiff() error: %v", err)
	}
	if captureFetcher.project != "my-app" {
		t.Errorf("project passed to Resolve = %q, want %q", captureFetcher.project, "my-app")
	}
	if captureFetcher.environment != "production" {
		t.Errorf("environment passed to Resolve = %q, want %q", captureFetcher.environment, "production")
	}
}

func TestRunConfigDiff_FlagOverridesConfigFile(t *testing.T) {
	dir := t.TempDir()
	writeTOML(t, dir, "fat-controller.toml", `
project = "my-app"
environment = "production"

[api.variables]
PORT = "9090"
`)
	captureFetcher := &capturingFetcher{
		cfg: &config.LiveConfig{
			ProjectID: "proj-1", EnvironmentID: "env-1",
			Services: map[string]*config.ServiceConfig{
				"api": {Name: "api", Variables: map[string]string{"PORT": "8080"}},
			},
		},
	}
	var buf bytes.Buffer
	// Flag values should override config file.
	globals := &cli.Globals{Project: "other-project", Environment: "staging", Output: "text"}
	err := cli.RunConfigDiff(context.Background(), globals, dir, nil, captureFetcher, &buf)
	if err != nil {
		t.Fatalf("RunConfigDiff() error: %v", err)
	}
	if captureFetcher.project != "other-project" {
		t.Errorf("project = %q, want %q (flag should override)", captureFetcher.project, "other-project")
	}
	if captureFetcher.environment != "staging" {
		t.Errorf("environment = %q, want %q (flag should override)", captureFetcher.environment, "staging")
	}
}
