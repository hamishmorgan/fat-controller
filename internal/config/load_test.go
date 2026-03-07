package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/config"
)

func TestLoadConfigs_BaseOnly(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "fat-controller.toml")
	if err := os.WriteFile(base, []byte(`
[[service]]
name = "api"
variables = { PORT = "8080" }
`), 0o644); err != nil {
		t.Fatalf("write base: %v", err)
	}

	cfg, err := config.LoadConfigs(dir, nil)
	if err != nil {
		t.Fatalf("LoadConfigs() error: %v", err)
	}
	if cfg.Services[0].Variables["PORT"] != "8080" {
		t.Error("expected api PORT=8080")
	}
}

func TestLoadConfigs_IgnoresLocalFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "fat-controller.toml"), []byte(`
[[service]]
name = "api"
variables = { PORT = "8080", APP_ENV = "staging" }
`), 0o644); err != nil {
		t.Fatalf("write base: %v", err)
	}
	// Create a local file — it should be ignored.
	if err := os.WriteFile(filepath.Join(dir, "fat-controller.local.toml"), []byte(`
[[service]]
name = "api"
variables = { APP_ENV = "production" }
`), 0o644); err != nil {
		t.Fatalf("write local: %v", err)
	}

	cfg, err := config.LoadConfigs(dir, nil)
	if err != nil {
		t.Fatalf("LoadConfigs() error: %v", err)
	}
	// Local file should NOT be merged — APP_ENV should remain "staging".
	if cfg.Services[0].Variables["APP_ENV"] != "staging" {
		t.Errorf("APP_ENV = %q, want staging (local file should be ignored)",
			cfg.Services[0].Variables["APP_ENV"])
	}
}

func TestLoadConfigs_WithExtraFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "fat-controller.toml"), []byte(`
[[service]]
name = "api"
variables = { PORT = "8080" }
`), 0o644); err != nil {
		t.Fatalf("write base: %v", err)
	}
	extra := filepath.Join(dir, "extra.toml")
	if err := os.WriteFile(extra, []byte(`
[[service]]
name = "api"
variables = { PORT = "9090" }
`), 0o644); err != nil {
		t.Fatalf("write extra: %v", err)
	}

	cfg, err := config.LoadConfigs(dir, []string{extra})
	if err != nil {
		t.Fatalf("LoadConfigs() error: %v", err)
	}
	if cfg.Services[0].Variables["PORT"] != "9090" {
		t.Errorf("PORT = %q, want 9090 (from extra override)", cfg.Services[0].Variables["PORT"])
	}
}

func TestLoadConfigs_NoBaseFile(t *testing.T) {
	dir := t.TempDir()
	_, err := config.LoadConfigs(dir, nil)
	if err == nil {
		t.Fatal("expected error when no base config file exists")
	}
}

func TestLoadConfigs_ExtraFileNotFound(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "fat-controller.toml"), []byte(`
[[service]]
name = "api"
variables = { PORT = "8080" }
`), 0o644); err != nil {
		t.Fatalf("write base: %v", err)
	}

	_, err := config.LoadConfigs(dir, []string{"/tmp/nonexistent-fc.toml"})
	if err == nil {
		t.Fatal("expected error for nonexistent extra config file")
	}
}

// --- LoadCascade tests ---

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
