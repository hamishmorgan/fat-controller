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
[api.variables]
PORT = "8080"
`), 0o644); err != nil {
		t.Fatalf("write base: %v", err)
	}

	cfg, err := config.LoadConfigs(dir, nil)
	if err != nil {
		t.Fatalf("LoadConfigs() error: %v", err)
	}
	if cfg.Services["api"].Variables["PORT"] != "8080" {
		t.Error("expected api PORT=8080")
	}
}

func TestLoadConfigs_IgnoresLocalFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "fat-controller.toml"), []byte(`
[api.variables]
PORT = "8080"
APP_ENV = "staging"
`), 0o644); err != nil {
		t.Fatalf("write base: %v", err)
	}
	// Create a local file — it should be ignored.
	if err := os.WriteFile(filepath.Join(dir, "fat-controller.local.toml"), []byte(`
[api.variables]
APP_ENV = "production"
`), 0o644); err != nil {
		t.Fatalf("write local: %v", err)
	}

	cfg, err := config.LoadConfigs(dir, nil)
	if err != nil {
		t.Fatalf("LoadConfigs() error: %v", err)
	}
	// Local file should NOT be merged — APP_ENV should remain "staging".
	if cfg.Services["api"].Variables["APP_ENV"] != "staging" {
		t.Errorf("APP_ENV = %q, want staging (local file should be ignored)",
			cfg.Services["api"].Variables["APP_ENV"])
	}
}

func TestLoadConfigs_WithExtraFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "fat-controller.toml"), []byte(`
[api.variables]
PORT = "8080"
`), 0o644); err != nil {
		t.Fatalf("write base: %v", err)
	}
	extra := filepath.Join(dir, "extra.toml")
	if err := os.WriteFile(extra, []byte(`
[api.variables]
PORT = "9090"
`), 0o644); err != nil {
		t.Fatalf("write extra: %v", err)
	}

	cfg, err := config.LoadConfigs(dir, []string{extra})
	if err != nil {
		t.Fatalf("LoadConfigs() error: %v", err)
	}
	if cfg.Services["api"].Variables["PORT"] != "9090" {
		t.Errorf("PORT = %q, want 9090 (from extra override)", cfg.Services["api"].Variables["PORT"])
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
[api.variables]
PORT = "8080"
`), 0o644); err != nil {
		t.Fatalf("write base: %v", err)
	}

	_, err := config.LoadConfigs(dir, []string{"/tmp/nonexistent-fc.toml"})
	if err == nil {
		t.Fatal("expected error for nonexistent extra config file")
	}
}
