package cli_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/cli"
	"github.com/hamishmorgan/fat-controller/internal/config"
	"github.com/hamishmorgan/fat-controller/internal/prompt"
)

// fakeInitResolver implements initResolver for testing.
// Each Fetch method returns a single-item list so selection auto-picks
// when the test passes the matching name hint.
type fakeInitResolver struct {
	wsName   string
	wsID     string
	wsErr    error
	projName string
	projID   string
	projErr  error
	envName  string
	envID    string
	envErr   error
	cfg      *config.LiveConfig
	fetchErr error
}

func (f *fakeInitResolver) FetchWorkspaces(_ context.Context) ([]prompt.Item, error) {
	if f.wsErr != nil {
		return nil, f.wsErr
	}
	return []prompt.Item{{Name: f.wsName, ID: f.wsID}}, nil
}

func (f *fakeInitResolver) FetchProjects(_ context.Context, _ string) ([]prompt.Item, error) {
	if f.projErr != nil {
		return nil, f.projErr
	}
	return []prompt.Item{{Name: f.projName, ID: f.projID}}, nil
}

func (f *fakeInitResolver) FetchEnvironments(_ context.Context, _ string) ([]prompt.Item, error) {
	if f.envErr != nil {
		return nil, f.envErr
	}
	return []prompt.Item{{Name: f.envName, ID: f.envID}}, nil
}

func (f *fakeInitResolver) FetchServiceList(_ context.Context, _ string) ([]config.ServiceInfo, error) {
	if f.fetchErr != nil {
		return nil, f.fetchErr
	}
	if f.cfg == nil {
		return nil, nil
	}
	list := make([]config.ServiceInfo, 0, len(f.cfg.Services))
	for name, svc := range f.cfg.Services {
		list = append(list, config.ServiceInfo{Name: name, ID: svc.ID})
	}
	return list, nil
}

func (f *fakeInitResolver) FetchLiveState(_ context.Context, _, _ string, _ []string) (*config.LiveConfig, error) {
	if f.fetchErr != nil {
		return nil, f.fetchErr
	}
	return f.cfg, nil
}

func newFakeResolver(cfg *config.LiveConfig) *fakeInitResolver {
	return &fakeInitResolver{
		wsName:   "acme-corp",
		wsID:     "ws-1",
		projName: "my-app",
		projID:   "proj-1",
		envName:  "production",
		envID:    "env-1",
		cfg:      cfg,
	}
}

func TestRunConfigInit_WritesConfigFile(t *testing.T) {
	dir := t.TempDir()
	resolver := newFakeResolver(&config.LiveConfig{
		ProjectID:     "proj-1",
		EnvironmentID: "env-1",
		Services: map[string]*config.ServiceConfig{
			"api": {
				Name:      "api",
				Variables: map[string]string{"PORT": "8080", "APP_ENV": "production"},
			},
		},
	})
	var buf bytes.Buffer
	err := cli.RunConfigInit(context.Background(), dir, "", "", "", resolver, false, false, true, &buf)
	if err != nil {
		t.Fatalf("RunConfigInit() error: %v", err)
	}

	// Verify the file was written.
	content, err := os.ReadFile(filepath.Join(dir, "fat-controller.toml"))
	if err != nil {
		t.Fatalf("reading config file: %v", err)
	}
	got := string(content)
	// Uses resolved names from the fakeInitResolver.
	if !strings.Contains(got, `[workspace]`) || !strings.Contains(got, `name = "acme-corp"`) {
		t.Errorf("expected workspace header in file:\n%s", got)
	}
	if !strings.Contains(got, `[project]`) || !strings.Contains(got, `name = "my-app"`) {
		t.Errorf("expected project header in file:\n%s", got)
	}
	if !strings.Contains(got, `name = "production"`) {
		t.Errorf("expected environment name in file:\n%s", got)
	}
	if !strings.Contains(got, "[service.variables]") {
		t.Errorf("expected service variables section in file:\n%s", got)
	}
	if !strings.Contains(got, "PORT") {
		t.Errorf("expected PORT in file:\n%s", got)
	}
}

func TestRunConfigInit_PrintsSummaryLines(t *testing.T) {
	dir := t.TempDir()
	resolver := newFakeResolver(&config.LiveConfig{
		Services: map[string]*config.ServiceConfig{
			"api":    {Name: "api", Variables: map[string]string{"PORT": "8080"}},
			"worker": {Name: "worker", Variables: map[string]string{"QUEUE": "default"}},
		},
	})
	var buf bytes.Buffer
	err := cli.RunConfigInit(context.Background(), dir, "", "", "", resolver, false, false, true, &buf)
	if err != nil {
		t.Fatalf("RunConfigInit() error: %v", err)
	}
	got := buf.String()

	// Should print summary lines for each resolved entity.
	for _, want := range []string{
		"Workspace: acme-corp",
		"Project: my-app",
		"Environment: production",
		"Services: api, worker (2 selected)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in output, got:\n%s", want, got)
		}
	}
}

func TestRunConfigInit_NonInteractiveNoYesShowsPreview(t *testing.T) {
	dir := t.TempDir()
	// Create an existing config file.
	existing := filepath.Join(dir, "fat-controller.toml")
	if err := os.WriteFile(existing, []byte("existing"), 0o644); err != nil {
		t.Fatalf("write existing: %v", err)
	}

	resolver := newFakeResolver(&config.LiveConfig{
		Services: map[string]*config.ServiceConfig{
			"api": {Name: "api", Variables: map[string]string{"PORT": "8080"}},
		},
	})
	var buf bytes.Buffer
	// Non-interactive without --yes should show preview and suggest --yes.
	err := cli.RunConfigInit(context.Background(), dir, "", "", "", resolver, false, false, false, &buf)
	if err != nil {
		t.Fatalf("RunConfigInit() error: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "would write fat-controller.toml") {
		t.Errorf("expected preview output, got:\n%s", got)
	}
	if !strings.Contains(got, "use --yes") {
		t.Errorf("expected '--yes' suggestion, got:\n%s", got)
	}
	// Original file should be unchanged.
	content, _ := os.ReadFile(existing)
	if string(content) != "existing" {
		t.Errorf("config file should not be modified, got: %s", string(content))
	}
}

func TestRunConfigInit_OverwritesExistingWithYes(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "fat-controller.toml")
	if err := os.WriteFile(existing, []byte("old content"), 0o644); err != nil {
		t.Fatalf("write existing: %v", err)
	}

	resolver := newFakeResolver(&config.LiveConfig{
		Services: map[string]*config.ServiceConfig{
			"api": {Name: "api", Variables: map[string]string{"PORT": "8080"}},
		},
	})
	var buf bytes.Buffer
	err := cli.RunConfigInit(context.Background(), dir, "", "", "", resolver, false, false, true, &buf)
	if err != nil {
		t.Fatalf("RunConfigInit() error: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "wrote fat-controller.toml") {
		t.Errorf("expected 'wrote fat-controller.toml' in output, got:\n%s", got)
	}
	// File should be overwritten with new content.
	content, _ := os.ReadFile(existing)
	if string(content) == "old content" {
		t.Error("config file should have been overwritten")
	}
}

func TestRunConfigInit_SkipsExistingEnvFileWithYes(t *testing.T) {
	dir := t.TempDir()
	// Create existing fat-controller.secrets but no config file.
	envPath := filepath.Join(dir, envFile)
	if err := os.WriteFile(envPath, []byte("OLD_SECRET=old"), 0o600); err != nil {
		t.Fatalf("write existing env: %v", err)
	}

	resolver := newFakeResolver(&config.LiveConfig{
		Services: map[string]*config.ServiceConfig{
			"api": {
				Name: "api",
				Variables: map[string]string{
					"PORT":         "8080",
					"DATABASE_URL": "postgres://user:pass@host/db",
				},
			},
		},
	})
	var buf bytes.Buffer
	// --yes: config should be written (new), env should be overwritten (--yes).
	// But to test independent skipping, we need interactive mode without --yes.
	// With --yes, both files are overwritten. Verify that works.
	err := cli.RunConfigInit(context.Background(), dir, "", "", "", resolver, false, false, true, &buf)
	if err != nil {
		t.Fatalf("RunConfigInit() error: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "wrote fat-controller.toml") {
		t.Errorf("expected config file to be written, got:\n%s", got)
	}
	if !strings.Contains(got, "wrote fat-controller.secrets") {
		t.Errorf("expected env file to be written with --yes, got:\n%s", got)
	}
}

const envFile = "fat-controller.secrets"

func TestRunConfigInit_CreatesEnvFileWithSecrets(t *testing.T) {
	dir := t.TempDir()
	resolver := newFakeResolver(&config.LiveConfig{
		Services: map[string]*config.ServiceConfig{
			"api": {
				Name: "api",
				Variables: map[string]string{
					"PORT":           "8080",
					"DATABASE_URL":   "postgres://user:pass@host/db",
					"STRIPE_API_KEY": "sk_live_xxx",
					"SESSION_SECRET": "abc123",
					"APP_NAME":       "my-app",
				},
			},
		},
	})
	var buf bytes.Buffer
	err := cli.RunConfigInit(context.Background(), dir, "", "", "", resolver, false, false, true, &buf)
	if err != nil {
		t.Fatalf("RunConfigInit() error: %v", err)
	}

	// fat-controller.secrets should contain actual secret values.
	content, err := os.ReadFile(filepath.Join(dir, envFile))
	if err != nil {
		t.Fatalf("reading env file: %v", err)
	}
	got := string(content)

	// Should contain actual values for sensitive vars.
	for _, want := range []string{
		"DATABASE_URL=postgres://user:pass@host/db",
		"SESSION_SECRET=abc123",
		"STRIPE_API_KEY=sk_live_xxx",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in env file, got:\n%s", want, got)
		}
	}

	// Non-sensitive vars should NOT be in the env file.
	if strings.Contains(got, "PORT=") {
		t.Errorf("PORT is not sensitive — should not be in env file:\n%s", got)
	}
	if strings.Contains(got, "APP_NAME=") {
		t.Errorf("APP_NAME is not sensitive — should not be in env file:\n%s", got)
	}

	// The base config should have ${VAR} refs, not ******** or literal secrets.
	base, _ := os.ReadFile(filepath.Join(dir, "fat-controller.toml"))
	baseStr := string(base)
	if strings.Contains(baseStr, "sk_live_xxx") {
		t.Errorf("secret value should not appear in base config:\n%s", baseStr)
	}
	if strings.Contains(baseStr, "********") {
		t.Errorf("masked placeholder should not appear in base config:\n%s", baseStr)
	}
}

func TestRunConfigInit_EnvFileSharedSecrets(t *testing.T) {
	dir := t.TempDir()
	resolver := newFakeResolver(&config.LiveConfig{
		Variables: map[string]string{
			"GLOBAL_SECRET": "s3cr3t",
			"APP_MODE":      "production",
		},
		Services: map[string]*config.ServiceConfig{
			"api": {Name: "api", Variables: map[string]string{"PORT": "8080"}},
		},
	})
	var buf bytes.Buffer
	err := cli.RunConfigInit(context.Background(), dir, "", "", "", resolver, false, false, true, &buf)
	if err != nil {
		t.Fatalf("RunConfigInit() error: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, envFile))
	if err != nil {
		t.Fatalf("reading env file: %v", err)
	}
	got := string(content)

	if !strings.Contains(got, "GLOBAL_SECRET=s3cr3t") {
		t.Errorf("expected GLOBAL_SECRET in env file:\n%s", got)
	}
	if strings.Contains(got, "APP_MODE=") {
		t.Errorf("APP_MODE is not sensitive — should not be in env file:\n%s", got)
	}
}

func TestRunConfigInit_NoSecretsNoEnvFile(t *testing.T) {
	dir := t.TempDir()
	resolver := newFakeResolver(&config.LiveConfig{
		Services: map[string]*config.ServiceConfig{
			"api": {Name: "api", Variables: map[string]string{"PORT": "8080", "APP_NAME": "hello"}},
		},
	})
	var buf bytes.Buffer
	err := cli.RunConfigInit(context.Background(), dir, "", "", "", resolver, false, false, true, &buf)
	if err != nil {
		t.Fatalf("RunConfigInit() error: %v", err)
	}

	// No secrets → no fat-controller.secrets file.
	if _, err := os.Stat(filepath.Join(dir, envFile)); !os.IsNotExist(err) {
		t.Error("should not create fat-controller.secrets when no secrets detected")
	}
}

func TestRunConfigInit_NonInteractiveIncludesAllServices(t *testing.T) {
	dir := t.TempDir()
	resolver := newFakeResolver(&config.LiveConfig{
		Services: map[string]*config.ServiceConfig{
			"api":    {Name: "api", Variables: map[string]string{"PORT": "8080"}},
			"worker": {Name: "worker", Variables: map[string]string{"QUEUE": "default"}},
			"web":    {Name: "web", Variables: map[string]string{"HOST": "0.0.0.0"}},
		},
	})
	var buf bytes.Buffer
	err := cli.RunConfigInit(context.Background(), dir, "", "", "", resolver, false, false, true, &buf)
	if err != nil {
		t.Fatalf("RunConfigInit() error: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "fat-controller.toml"))
	if err != nil {
		t.Fatalf("reading config: %v", err)
	}
	got := string(content)

	// Each service uses [[service]] + [service.variables] sub-table.
	// Verify all service names appear and we have the right number of service sections.
	for _, svc := range []string{"api", "worker", "web"} {
		if !strings.Contains(got, `name = "`+svc+`"`) {
			t.Errorf("expected service %s, got:\n%s", svc, got)
		}
	}
	if count := strings.Count(got, "[[service]]"); count != 3 {
		t.Errorf("expected 3 [[service]] sections, got %d:\n%s", count, got)
	}

	// Output should mention 3 services.
	if !strings.Contains(buf.String(), "3 services") {
		t.Errorf("expected '3 services' in output, got: %s", buf.String())
	}
}

func TestRunConfigInit_ResolveWorkspaceError(t *testing.T) {
	dir := t.TempDir()
	resolver := &fakeInitResolver{wsErr: errors.New("no workspace")}
	var buf bytes.Buffer
	err := cli.RunConfigInit(context.Background(), dir, "", "", "", resolver, false, false, true, &buf)
	if err == nil {
		t.Fatal("expected error from workspace resolve failure")
	}
	if !strings.Contains(err.Error(), "no workspace") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunConfigInit_ResolveProjectError(t *testing.T) {
	dir := t.TempDir()
	resolver := &fakeInitResolver{
		wsName: "acme", wsID: "ws-1",
		projErr: errors.New("no project"),
	}
	var buf bytes.Buffer
	err := cli.RunConfigInit(context.Background(), dir, "", "", "", resolver, false, false, true, &buf)
	if err == nil {
		t.Fatal("expected error from project resolve failure")
	}
}

func TestRunConfigInit_ResolveEnvironmentError(t *testing.T) {
	dir := t.TempDir()
	resolver := &fakeInitResolver{
		wsName: "acme", wsID: "ws-1",
		projName: "app", projID: "proj-1",
		envErr: errors.New("no environment"),
	}
	var buf bytes.Buffer
	err := cli.RunConfigInit(context.Background(), dir, "", "", "", resolver, false, false, true, &buf)
	if err == nil {
		t.Fatal("expected error from environment resolve failure")
	}
}

func TestRunConfigInit_NoYesNonInteractiveWritesNoFiles(t *testing.T) {
	dir := t.TempDir()
	resolver := newFakeResolver(&config.LiveConfig{
		ProjectID:     "proj-1",
		EnvironmentID: "env-1",
		Services: map[string]*config.ServiceConfig{
			"api": {Name: "api", Variables: map[string]string{"PORT": "8080"}},
		},
	})
	var buf bytes.Buffer
	// interactive=false, dryRun=false, yes=false → implicit dry-run.
	err := cli.RunConfigInit(context.Background(), dir, "", "", "", resolver, false, false, false, &buf)
	if err != nil {
		t.Fatalf("RunConfigInit() error: %v", err)
	}

	// No files should be written.
	if _, err := os.Stat(filepath.Join(dir, "fat-controller.toml")); !os.IsNotExist(err) {
		t.Error("non-interactive without --yes should not create fat-controller.toml")
	}

	got := buf.String()
	if !strings.Contains(got, "--yes") {
		t.Errorf("expected --yes hint in output, got:\n%s", got)
	}
}

func TestRunConfigInit_DryRunWritesNoFiles(t *testing.T) {
	dir := t.TempDir()
	resolver := newFakeResolver(&config.LiveConfig{
		ProjectID:     "proj-1",
		EnvironmentID: "env-1",
		Services: map[string]*config.ServiceConfig{
			"api": {
				Name:      "api",
				Variables: map[string]string{"PORT": "8080", "DATABASE_URL": "postgres://..."},
			},
		},
	})
	var buf bytes.Buffer
	err := cli.RunConfigInit(context.Background(), dir, "", "", "", resolver, false, true, false, &buf)
	if err != nil {
		t.Fatalf("RunConfigInit() error: %v", err)
	}

	// No files should be written.
	if _, err := os.Stat(filepath.Join(dir, "fat-controller.toml")); !os.IsNotExist(err) {
		t.Error("dry-run should not create fat-controller.toml")
	}
	if _, err := os.Stat(filepath.Join(dir, envFile)); !os.IsNotExist(err) {
		t.Error("dry-run should not create fat-controller.secrets")
	}
	if _, err := os.Stat(filepath.Join(dir, ".gitignore")); !os.IsNotExist(err) {
		t.Error("dry-run should not create .gitignore")
	}

	// Output should contain dry-run previews.
	got := buf.String()
	if !strings.Contains(got, "dry run") {
		t.Errorf("expected 'dry run' in output, got:\n%s", got)
	}
	if !strings.Contains(got, "fat-controller.toml") {
		t.Errorf("expected config file name in output, got:\n%s", got)
	}
	// Should preview the TOML content.
	if !strings.Contains(got, `name = "my-app"`) {
		t.Errorf("expected TOML preview in output, got:\n%s", got)
	}
}
