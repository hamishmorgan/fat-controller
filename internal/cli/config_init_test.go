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

func (f *fakeInitResolver) FetchLiveState(_ context.Context, _, _ string) (*config.LiveConfig, error) {
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
	err := cli.RunConfigInit(context.Background(), dir, "", "", "", resolver, false, false, &buf)
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
	if !strings.Contains(got, `workspace = "acme-corp"`) {
		t.Errorf("expected workspace header in file:\n%s", got)
	}
	if !strings.Contains(got, `project = "my-app"`) {
		t.Errorf("expected project header in file:\n%s", got)
	}
	if !strings.Contains(got, `environment = "production"`) {
		t.Errorf("expected environment header in file:\n%s", got)
	}
	if !strings.Contains(got, "[api.variables]") {
		t.Errorf("expected service section in file:\n%s", got)
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
	err := cli.RunConfigInit(context.Background(), dir, "", "", "", resolver, false, false, &buf)
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

func TestRunConfigInit_RefusesToOverwrite(t *testing.T) {
	dir := t.TempDir()
	// Create an existing config file.
	existing := filepath.Join(dir, "fat-controller.toml")
	if err := os.WriteFile(existing, []byte("existing"), 0o644); err != nil {
		t.Fatalf("write existing: %v", err)
	}

	resolver := newFakeResolver(&config.LiveConfig{Services: map[string]*config.ServiceConfig{}})
	var buf bytes.Buffer
	err := cli.RunConfigInit(context.Background(), dir, "", "", "", resolver, false, false, &buf)
	if err == nil {
		t.Fatal("expected error when config file already exists")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error should mention 'already exists': %v", err)
	}
}

func TestRunConfigInit_CreatesLocalTOMLWithSecrets(t *testing.T) {
	dir := t.TempDir()
	resolver := newFakeResolver(&config.LiveConfig{
		Services: map[string]*config.ServiceConfig{
			"api": {
				Name: "api",
				Variables: map[string]string{
					"PORT":           "8080",
					"DATABASE_URL":   "postgres://...",
					"STRIPE_API_KEY": "sk_live_xxx",
					"SESSION_SECRET": "abc123",
					"APP_NAME":       "my-app",
				},
			},
		},
	})
	var buf bytes.Buffer
	err := cli.RunConfigInit(context.Background(), dir, "", "", "", resolver, false, false, &buf)
	if err != nil {
		t.Fatalf("RunConfigInit() error: %v", err)
	}

	// Verify .local.toml was created with interpolation refs for secrets.
	localPath := filepath.Join(dir, "fat-controller.local.toml")
	content, err := os.ReadFile(localPath)
	if err != nil {
		t.Fatalf("reading local config: %v", err)
	}
	got := string(content)

	// Should contain interpolation refs for sensitive vars.
	for _, want := range []string{
		"[api.variables]",
		`DATABASE_URL = "${DATABASE_URL}"`,
		`SESSION_SECRET = "${SESSION_SECRET}"`,
		`STRIPE_API_KEY = "${STRIPE_API_KEY}"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in local config, got:\n%s", want, got)
		}
	}

	// Should NOT contain non-sensitive vars.
	if strings.Contains(got, "PORT") {
		t.Errorf("PORT is not sensitive — should not be in local config:\n%s", got)
	}
	if strings.Contains(got, "APP_NAME") {
		t.Errorf("APP_NAME is not sensitive — should not be in local config:\n%s", got)
	}
}

func TestRunConfigInit_LocalTOMLSharedSecrets(t *testing.T) {
	dir := t.TempDir()
	resolver := newFakeResolver(&config.LiveConfig{
		Shared: map[string]string{
			"GLOBAL_SECRET": "s3cr3t",
			"APP_MODE":      "production",
		},
		Services: map[string]*config.ServiceConfig{
			"api": {Name: "api", Variables: map[string]string{"PORT": "8080"}},
		},
	})
	var buf bytes.Buffer
	err := cli.RunConfigInit(context.Background(), dir, "", "", "", resolver, false, false, &buf)
	if err != nil {
		t.Fatalf("RunConfigInit() error: %v", err)
	}

	localPath := filepath.Join(dir, "fat-controller.local.toml")
	content, err := os.ReadFile(localPath)
	if err != nil {
		t.Fatalf("reading local config: %v", err)
	}
	got := string(content)

	if !strings.Contains(got, "[shared.variables]") {
		t.Errorf("expected shared section in local config:\n%s", got)
	}
	if !strings.Contains(got, `GLOBAL_SECRET = "${GLOBAL_SECRET}"`) {
		t.Errorf("expected GLOBAL_SECRET interpolation ref:\n%s", got)
	}
	if strings.Contains(got, "APP_MODE") {
		t.Errorf("APP_MODE is not sensitive — should not be in local config:\n%s", got)
	}
}

func TestRunConfigInit_LocalTOMLNoSecretsFallsBack(t *testing.T) {
	dir := t.TempDir()
	resolver := newFakeResolver(&config.LiveConfig{
		Services: map[string]*config.ServiceConfig{
			"api": {Name: "api", Variables: map[string]string{"PORT": "8080", "APP_NAME": "hello"}},
		},
	})
	var buf bytes.Buffer
	err := cli.RunConfigInit(context.Background(), dir, "", "", "", resolver, false, false, &buf)
	if err != nil {
		t.Fatalf("RunConfigInit() error: %v", err)
	}

	localPath := filepath.Join(dir, "fat-controller.local.toml")
	content, err := os.ReadFile(localPath)
	if err != nil {
		t.Fatalf("reading local config: %v", err)
	}
	got := string(content)

	// No secrets found — should contain the fallback stub.
	if !strings.Contains(got, "No secrets detected") {
		t.Errorf("expected fallback stub when no secrets, got:\n%s", got)
	}
	if !strings.Contains(got, "STRIPE_KEY") {
		t.Errorf("expected example in fallback stub:\n%s", got)
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
	err := cli.RunConfigInit(context.Background(), dir, "", "", "", resolver, false, false, &buf)
	if err != nil {
		t.Fatalf("RunConfigInit() error: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "fat-controller.toml"))
	if err != nil {
		t.Fatalf("reading config: %v", err)
	}
	got := string(content)

	for _, svc := range []string{"api", "worker", "web"} {
		if !strings.Contains(got, "["+svc+".variables]") {
			t.Errorf("expected [%s.variables] section, got:\n%s", svc, got)
		}
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
	err := cli.RunConfigInit(context.Background(), dir, "", "", "", resolver, false, false, &buf)
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
	err := cli.RunConfigInit(context.Background(), dir, "", "", "", resolver, false, false, &buf)
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
	err := cli.RunConfigInit(context.Background(), dir, "", "", "", resolver, false, false, &buf)
	if err == nil {
		t.Fatal("expected error from environment resolve failure")
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
	err := cli.RunConfigInit(context.Background(), dir, "", "", "", resolver, false, true, &buf)
	if err != nil {
		t.Fatalf("RunConfigInit() error: %v", err)
	}

	// No files should be written.
	if _, err := os.Stat(filepath.Join(dir, "fat-controller.toml")); !os.IsNotExist(err) {
		t.Error("dry-run should not create fat-controller.toml")
	}
	if _, err := os.Stat(filepath.Join(dir, "fat-controller.local.toml")); !os.IsNotExist(err) {
		t.Error("dry-run should not create fat-controller.local.toml")
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
	if !strings.Contains(got, `project = "my-app"`) {
		t.Errorf("expected TOML preview in output, got:\n%s", got)
	}
}
