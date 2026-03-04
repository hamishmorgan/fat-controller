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
)

// NOTE: These tests reuse fakeFetcher from config_get_test.go (same package).

func TestRunConfigInit_WritesConfigFile(t *testing.T) {
	dir := t.TempDir()
	fetcher := &fakeFetcher{
		cfg: &config.LiveConfig{
			ProjectID:     "proj-1",
			EnvironmentID: "env-1",
			Services: map[string]*config.ServiceConfig{
				"api": {
					Name:      "api",
					Variables: map[string]string{"PORT": "8080", "APP_ENV": "production"},
				},
			},
		},
	}
	var buf bytes.Buffer
	err := cli.RunConfigInit(context.Background(), dir, "", "my-app", "production", fetcher, &buf)
	if err != nil {
		t.Fatalf("RunConfigInit() error: %v", err)
	}

	// Verify the file was written.
	content, err := os.ReadFile(filepath.Join(dir, "fat-controller.toml"))
	if err != nil {
		t.Fatalf("reading config file: %v", err)
	}
	got := string(content)
	if strings.Contains(got, `workspace = `) {
		t.Errorf("did not expect workspace header when not provided, got:\n%s", got)
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

func TestRunConfigInit_RefusesToOverwrite(t *testing.T) {
	dir := t.TempDir()
	// Create an existing config file.
	existing := filepath.Join(dir, "fat-controller.toml")
	if err := os.WriteFile(existing, []byte("existing"), 0o644); err != nil {
		t.Fatalf("write existing: %v", err)
	}

	fetcher := &fakeFetcher{
		cfg: &config.LiveConfig{Services: map[string]*config.ServiceConfig{}},
	}
	var buf bytes.Buffer
	err := cli.RunConfigInit(context.Background(), dir, "", "proj", "env", fetcher, &buf)
	if err == nil {
		t.Fatal("expected error when config file already exists")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error should mention 'already exists': %v", err)
	}
}

func TestRunConfigInit_CreatesLocalTOMLStub(t *testing.T) {
	dir := t.TempDir()
	fetcher := &fakeFetcher{
		cfg: &config.LiveConfig{
			Services: map[string]*config.ServiceConfig{
				"api": {Name: "api", Variables: map[string]string{"PORT": "8080"}},
			},
		},
	}
	var buf bytes.Buffer
	err := cli.RunConfigInit(context.Background(), dir, "", "proj", "env", fetcher, &buf)
	if err != nil {
		t.Fatalf("RunConfigInit() error: %v", err)
	}

	// Verify .local.toml stub was created.
	localPath := filepath.Join(dir, "fat-controller.local.toml")
	content, err := os.ReadFile(localPath)
	if err != nil {
		t.Fatalf("reading local config: %v", err)
	}
	if !strings.Contains(string(content), "Local overrides") {
		t.Errorf("expected comment in local stub:\n%s", string(content))
	}
}

func TestRunConfigInit_PrintsSummary(t *testing.T) {
	dir := t.TempDir()
	fetcher := &fakeFetcher{
		cfg: &config.LiveConfig{
			Services: map[string]*config.ServiceConfig{
				"api": {Name: "api", Variables: map[string]string{"PORT": "8080"}},
			},
		},
	}
	var buf bytes.Buffer
	err := cli.RunConfigInit(context.Background(), dir, "", "proj", "env", fetcher, &buf)
	if err != nil {
		t.Fatalf("RunConfigInit() error: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "fat-controller.toml") {
		t.Errorf("expected filename in output:\n%s", got)
	}
}

func TestRunConfigInit_ResolveError(t *testing.T) {
	dir := t.TempDir()
	fetcher := &fakeFetcher{resolveErr: errors.New("no project")}
	var buf bytes.Buffer
	err := cli.RunConfigInit(context.Background(), dir, "", "proj", "env", fetcher, &buf)
	if err == nil {
		t.Fatal("expected error from resolve failure")
	}
}
