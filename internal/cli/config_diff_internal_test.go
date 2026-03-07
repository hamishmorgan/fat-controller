package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/hamishmorgan/fat-controller/internal/config"
)

type fakeConfigFetcher struct {
	cfg *config.LiveConfig
}

func (f *fakeConfigFetcher) Resolve(_ context.Context, _, _, _ string) (string, string, error) {
	return "proj-1", "env-1", nil
}

func (f *fakeConfigFetcher) Fetch(_ context.Context, _, _, _ string) (*config.LiveConfig, error) {
	return f.cfg, nil
}

func TestRunConfigDiff_JSON(t *testing.T) {
	dir := t.TempDir()
	writeTOML := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	writeTOML("fat-controller.toml", `
[[service]]
name = "api"
variables = { PORT = "9090" }
`)

	fetcher := &fakeConfigFetcher{cfg: &config.LiveConfig{ProjectID: "proj-1", EnvironmentID: "env-1", Services: map[string]*config.ServiceConfig{"api": {Name: "api", Variables: map[string]string{"PORT": "8080"}}}}}
	var buf bytes.Buffer
	globals := &Globals{Output: "json"}
	if err := RunConfigDiff(context.Background(), globals, "", "", "", dir, nil, "", false, fetcher, &buf); err != nil {
		t.Fatalf("RunConfigDiff() error: %v", err)
	}
	var payload DiffOutput
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, buf.String())
	}
}

func TestRunConfigDiff_TOML(t *testing.T) {
	dir := t.TempDir()
	writeTOML := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	writeTOML("fat-controller.toml", `
[[service]]
name = "api"
variables = { PORT = "9090" }
`)

	fetcher := &fakeConfigFetcher{cfg: &config.LiveConfig{ProjectID: "proj-1", EnvironmentID: "env-1", Services: map[string]*config.ServiceConfig{"api": {Name: "api", Variables: map[string]string{"PORT": "8080"}}}}}
	var buf bytes.Buffer
	globals := &Globals{Output: "toml"}
	if err := RunConfigDiff(context.Background(), globals, "", "", "", dir, nil, "", false, fetcher, &buf); err != nil {
		t.Fatalf("RunConfigDiff() error: %v", err)
	}
	var payload DiffOutput
	if err := toml.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatalf("output is not valid TOML: %v\n%s", err, buf.String())
	}
}

func TestRunConfigDiff_StructuredMasksDesiredSecrets(t *testing.T) {
	dir := t.TempDir()
	writeTOML := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	writeTOML("fat-controller.toml", `
[[service]]
name = "api"
variables = { DATABASE_PASSWORD = "newpass" }
`)

	fetcher := &fakeConfigFetcher{cfg: &config.LiveConfig{ProjectID: "proj-1", EnvironmentID: "env-1", Services: map[string]*config.ServiceConfig{"api": {Name: "api", Variables: map[string]string{"DATABASE_PASSWORD": "hunter2"}}}}}
	var buf bytes.Buffer
	globals := &Globals{Output: "json"}
	if err := RunConfigDiff(context.Background(), globals, "", "", "", dir, nil, "", false, fetcher, &buf); err != nil {
		t.Fatalf("RunConfigDiff() error: %v", err)
	}
	var payload DiffOutput
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, buf.String())
	}
	if payload.Services == nil || payload.Services["api"] == nil || len(payload.Services["api"].Variables) != 1 {
		t.Fatalf("expected one variable change in payload, got: %#v", payload)
	}
	ch := payload.Services["api"].Variables[0]
	if ch.LiveValue != "********" {
		t.Fatalf("live_value=%q, want masked", ch.LiveValue)
	}
	if ch.DesiredValue != "********" {
		t.Fatalf("desired_value=%q, want masked", ch.DesiredValue)
	}
}
