package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/config"
)

func TestLoadCascade_ConfigFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "custom.toml")
	writeFile(t, path, "name = \"custom\"")

	result, err := config.LoadCascade(config.LoadOptions{ConfigFile: path})
	if err != nil {
		t.Fatal(err)
	}
	if result.Config.Name != "custom" {
		t.Errorf("Name = %q, want custom", result.Config.Name)
	}
	if result.PrimaryFile != path {
		t.Errorf("PrimaryFile = %q, want %q", result.PrimaryFile, path)
	}
}

func TestLoadCascade_Discovery(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	deep := filepath.Join(root, "envs", "prod")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}

	writeFile(t, filepath.Join(root, "fat-controller.toml"), "[workspace]\nname = \"Acme\"")
	writeFile(t, filepath.Join(deep, "fat-controller.toml"), "name = \"production\"")

	result, err := config.LoadCascade(config.LoadOptions{WorkDir: deep})
	if err != nil {
		t.Fatal(err)
	}
	if result.Config.Name != "production" {
		t.Errorf("Name = %q, want production", result.Config.Name)
	}
	if result.Config.Workspace == nil || result.Config.Workspace.Name != "Acme" {
		t.Errorf("Workspace not merged from parent")
	}
	if len(result.Files) != 2 {
		t.Errorf("Files count = %d, want 2", len(result.Files))
	}
}

func TestLoadCascade_LocalOverride(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "fat-controller.toml"), "name = \"base\"\nvariables = { A = \"1\" }")
	writeFile(t, filepath.Join(dir, "fat-controller.local.toml"), "variables = { A = \"override\" }")

	result, err := config.LoadCascade(config.LoadOptions{WorkDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if result.Config.Variables["A"] != "override" {
		t.Errorf("A = %q, want override", result.Config.Variables["A"])
	}
}

func TestLoadCascade_NoConfigError(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := config.LoadCascade(config.LoadOptions{WorkDir: dir})
	if err == nil {
		t.Fatal("expected error when no config files found")
	}
}

func TestLoadCascade_EnvFileLoading(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "fat-controller.toml"), "name = \"test\"\n[tool]\nenv_file = \".env\"")
	writeFile(t, filepath.Join(dir, ".env"), "SECRET=hunter2\nAPI_KEY=abc123")

	result, err := config.LoadCascade(config.LoadOptions{WorkDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if result.EnvVars == nil {
		t.Fatal("EnvVars is nil")
	}
	if result.EnvVars["SECRET"] != "hunter2" {
		t.Errorf("SECRET = %q, want hunter2", result.EnvVars["SECRET"])
	}
	if result.EnvVars["API_KEY"] != "abc123" {
		t.Errorf("API_KEY = %q, want abc123", result.EnvVars["API_KEY"])
	}
}

func TestLoadCascade_ConfigFileMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "deploy", "staging.toml")

	result, err := config.LoadCascade(config.LoadOptions{ConfigFile: path})
	if err != nil {
		t.Fatalf("missing --config-file should not error, got: %v", err)
	}
	if result.Config == nil {
		t.Fatal("Config should be non-nil (empty)")
	}
	if result.PrimaryFile != path {
		t.Errorf("PrimaryFile = %q, want %q", result.PrimaryFile, path)
	}
	if len(result.Files) != 1 || result.Files[0] != path {
		t.Errorf("Files = %v, want [%s]", result.Files, path)
	}
}

func TestLoadCascade_EnvFileMissing(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "fat-controller.toml"), "name = \"test\"\n[tool]\nenv_file = \".env.missing\"")

	result, err := config.LoadCascade(config.LoadOptions{WorkDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	// Missing env file should be silently skipped.
	if result.EnvVars != nil {
		t.Errorf("EnvVars = %v, want nil (missing env file should be skipped)", result.EnvVars)
	}
}
