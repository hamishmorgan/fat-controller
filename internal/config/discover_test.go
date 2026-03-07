package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/config"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFindConfigInDir_FatControllerToml(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "fat-controller.toml"), "name = \"test\"")
	got := config.FindConfigInDir(dir)
	want := filepath.Join(dir, "fat-controller.toml")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFindConfigInDir_DotConfigFatControllerToml(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".config", "fat-controller.toml"), "name = \"test\"")
	got := config.FindConfigInDir(dir)
	want := filepath.Join(dir, ".config", "fat-controller.toml")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFindConfigInDir_DotConfigFatControllerDir(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".config", "fat-controller", "config.toml"), "name = \"test\"")
	got := config.FindConfigInDir(dir)
	want := filepath.Join(dir, ".config", "fat-controller", "config.toml")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFindConfigInDir_PrecedenceOrder(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "fat-controller.toml"), "name = \"visible\"")
	writeFile(t, filepath.Join(dir, ".config", "fat-controller.toml"), "name = \"hidden\"")
	got := config.FindConfigInDir(dir)
	want := filepath.Join(dir, "fat-controller.toml")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFindConfigInDir_NoConfig(t *testing.T) {
	dir := t.TempDir()
	got := config.FindConfigInDir(dir)
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestLocalOverridePath(t *testing.T) {
	tests := []struct{ input, want string }{
		{"fat-controller.toml", "fat-controller.local.toml"},
		{".config/fat-controller.toml", ".config/fat-controller.local.toml"},
		{".config/fat-controller/config.toml", ".config/fat-controller/config.local.toml"},
	}
	for _, tt := range tests {
		got := config.LocalOverridePath(tt.input)
		if got != tt.want {
			t.Errorf("LocalOverridePath(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestDiscoverConfigs_WalkToGitRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	deep := filepath.Join(root, "envs", "production")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "fat-controller.toml"), "[workspace]\nname = \"Acme\"\n")
	writeFile(t, filepath.Join(deep, "fat-controller.toml"), "name = \"production\"\n")

	paths, err := config.DiscoverConfigs(deep)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 {
		t.Fatalf("len = %d, want 2", len(paths))
	}
	if paths[0] != filepath.Join(root, "fat-controller.toml") {
		t.Errorf("paths[0] = %q, want root config", paths[0])
	}
	if paths[1] != filepath.Join(deep, "fat-controller.toml") {
		t.Errorf("paths[1] = %q, want deep config", paths[1])
	}
}

func TestDiscoverConfigs_NotInGitRepo(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "fat-controller.toml"), "name = \"test\"")
	paths, err := config.DiscoverConfigs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 {
		t.Fatalf("len = %d, want 1", len(paths))
	}
}

func TestDiscoverConfigs_NoConfigs(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	paths, err := config.DiscoverConfigs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 0 {
		t.Errorf("len = %d, want 0", len(paths))
	}
}
