package cli_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/cli"
	"github.com/hamishmorgan/fat-controller/internal/config"
)

// countingApplier counts calls for CLI integration tests.
type countingApplier struct {
	upserts   int
	deletes   int
	settings  int
	resources int
}

func (c *countingApplier) UpsertVariable(_ context.Context, _, _, _ string, _ bool) error {
	c.upserts++
	return nil
}
func (c *countingApplier) DeleteVariable(_ context.Context, _, _ string) error {
	c.deletes++
	return nil
}
func (c *countingApplier) UpdateServiceSettings(_ context.Context, _ string, _ *config.DesiredDeploy) error {
	c.settings++
	return nil
}
func (c *countingApplier) UpdateServiceResources(_ context.Context, _ string, _ *config.DesiredResources) error {
	c.resources++
	return nil
}

func writeApplyTOML(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func TestRunConfigApply_DryRunByDefault(t *testing.T) {
	dir := t.TempDir()
	writeApplyTOML(t, dir, "fat-controller.toml", `
[api.variables]
PORT = "9090"
`)
	fetcher := &fakeFetcher{
		cfg: &config.LiveConfig{
			ProjectID: "proj-1", EnvironmentID: "env-1",
			Services: map[string]*config.ServiceConfig{
				"api": {Name: "api", Variables: map[string]string{"PORT": "8080"}},
			},
		},
	}
	applier := &countingApplier{}
	var buf bytes.Buffer
	globals := &cli.Globals{Output: "text"}

	err := cli.RunConfigApply(context.Background(), globals, dir, nil, fetcher, applier, &buf)
	if err != nil {
		t.Fatalf("RunConfigApply() error: %v", err)
	}
	if applier.upserts != 0 {
		t.Error("applier should not be called in dry-run mode")
	}
	got := buf.String()
	if !strings.Contains(got, "dry run") {
		t.Errorf("expected dry-run output, got:\n%s", got)
	}
}

func TestRunConfigApply_ConfirmExecutes(t *testing.T) {
	dir := t.TempDir()
	writeApplyTOML(t, dir, "fat-controller.toml", `
[api.variables]
PORT = "9090"
NEW = "hello"
`)
	fetcher := &fakeFetcher{
		cfg: &config.LiveConfig{
			ProjectID: "proj-1", EnvironmentID: "env-1",
			Services: map[string]*config.ServiceConfig{
				"api": {Name: "api", Variables: map[string]string{"PORT": "8080"}},
			},
		},
	}
	applier := &countingApplier{}
	var buf bytes.Buffer
	globals := &cli.Globals{Confirm: true}

	err := cli.RunConfigApply(context.Background(), globals, dir, nil, fetcher, applier, &buf)
	if err != nil {
		t.Fatalf("RunConfigApply() error: %v", err)
	}
	if applier.upserts != 2 {
		t.Errorf("expected 2 upserts (update PORT + create NEW), got %d", applier.upserts)
	}
	got := buf.String()
	if !strings.Contains(got, "applied=2") {
		t.Errorf("expected summary with applied=2, got:\n%s", got)
	}
}

func TestRunConfigApply_DryRunOverridesConfirm(t *testing.T) {
	dir := t.TempDir()
	writeApplyTOML(t, dir, "fat-controller.toml", `
[api.variables]
PORT = "9090"
`)
	fetcher := &fakeFetcher{
		cfg: &config.LiveConfig{
			ProjectID: "proj-1", EnvironmentID: "env-1",
			Services: map[string]*config.ServiceConfig{
				"api": {Name: "api", Variables: map[string]string{"PORT": "8080"}},
			},
		},
	}
	applier := &countingApplier{}
	var buf bytes.Buffer
	globals := &cli.Globals{Confirm: true, DryRun: true}

	err := cli.RunConfigApply(context.Background(), globals, dir, nil, fetcher, applier, &buf)
	if err != nil {
		t.Fatalf("RunConfigApply() error: %v", err)
	}
	if applier.upserts != 0 {
		t.Error("--dry-run should override --confirm")
	}
}

func TestRunConfigApply_NoChanges(t *testing.T) {
	dir := t.TempDir()
	writeApplyTOML(t, dir, "fat-controller.toml", `
[api.variables]
PORT = "8080"
`)
	fetcher := &fakeFetcher{
		cfg: &config.LiveConfig{
			ProjectID: "proj-1", EnvironmentID: "env-1",
			Services: map[string]*config.ServiceConfig{
				"api": {Name: "api", Variables: map[string]string{"PORT": "8080"}},
			},
		},
	}
	applier := &countingApplier{}
	var buf bytes.Buffer
	globals := &cli.Globals{Confirm: true}

	err := cli.RunConfigApply(context.Background(), globals, dir, nil, fetcher, applier, &buf)
	if err != nil {
		t.Fatalf("RunConfigApply() error: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "No changes") {
		t.Errorf("expected 'No changes' for no-op apply, got:\n%s", got)
	}
}

func TestRunConfigApply_ServiceFilter(t *testing.T) {
	dir := t.TempDir()
	writeApplyTOML(t, dir, "fat-controller.toml", `
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
	applier := &countingApplier{}
	var buf bytes.Buffer
	globals := &cli.Globals{Service: "api", Confirm: true}

	err := cli.RunConfigApply(context.Background(), globals, dir, nil, fetcher, applier, &buf)
	if err != nil {
		t.Fatalf("RunConfigApply() error: %v", err)
	}
	// Only api should be applied (1 upsert for PORT), not worker.
	if applier.upserts != 1 {
		t.Errorf("expected 1 upsert (api only), got %d", applier.upserts)
	}
}

func TestRunConfigApply_UsesConfigFileProject(t *testing.T) {
	dir := t.TempDir()
	writeApplyTOML(t, dir, "fat-controller.toml", `
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
	applier := &countingApplier{}
	var buf bytes.Buffer
	// Globals with empty Project/Environment — should fall back to config file.
	globals := &cli.Globals{Confirm: true}

	err := cli.RunConfigApply(context.Background(), globals, dir, nil, captureFetcher, applier, &buf)
	if err != nil {
		t.Fatalf("RunConfigApply() error: %v", err)
	}
	if captureFetcher.project != "my-app" {
		t.Errorf("project passed to Resolve = %q, want %q", captureFetcher.project, "my-app")
	}
	if captureFetcher.environment != "production" {
		t.Errorf("environment passed to Resolve = %q, want %q", captureFetcher.environment, "production")
	}
}

func TestRunConfigApply_FlagOverridesConfigFile(t *testing.T) {
	dir := t.TempDir()
	writeApplyTOML(t, dir, "fat-controller.toml", `
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
	applier := &countingApplier{}
	var buf bytes.Buffer
	// Flag values should override config file.
	globals := &cli.Globals{Project: "other-project", Environment: "staging", Confirm: true}

	err := cli.RunConfigApply(context.Background(), globals, dir, nil, captureFetcher, applier, &buf)
	if err != nil {
		t.Fatalf("RunConfigApply() error: %v", err)
	}
	if captureFetcher.project != "other-project" {
		t.Errorf("project = %q, want %q (flag should override)", captureFetcher.project, "other-project")
	}
	if captureFetcher.environment != "staging" {
		t.Errorf("environment = %q, want %q (flag should override)", captureFetcher.environment, "staging")
	}
}
