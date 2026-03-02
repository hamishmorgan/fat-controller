package platform_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/adrg/xdg"
	"github.com/hamishmorgan/fat-controller/internal/platform"
)

// setXDGConfigHome overrides XDG_CONFIG_HOME for the duration of a test.
// adrg/xdg caches env vars at init time, so we must call xdg.Reload()
// after changing the env and again on cleanup to restore the original.
func setXDGConfigHome(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", dir)
	xdg.Reload()
	t.Cleanup(func() { xdg.Reload() })
}

func TestConfigDir(t *testing.T) {
	tmp := t.TempDir()
	setXDGConfigHome(t, tmp)

	dir := platform.ConfigDir()
	want := filepath.Join(tmp, "fat-controller")
	if dir != want {
		t.Errorf("ConfigDir() = %q, want %q", dir, want)
	}
}

func TestAuthFilePath(t *testing.T) {
	tmp := t.TempDir()
	setXDGConfigHome(t, tmp)

	path := platform.AuthFilePath()
	want := filepath.Join(tmp, "fat-controller", "auth.json")
	if path != want {
		t.Errorf("AuthFilePath() = %q, want %q", path, want)
	}
}

func TestConfigFilePath(t *testing.T) {
	tmp := t.TempDir()
	setXDGConfigHome(t, tmp)

	path := platform.ConfigFilePath()
	want := filepath.Join(tmp, "fat-controller", "config.toml")
	if path != want {
		t.Errorf("ConfigFilePath() = %q, want %q", path, want)
	}
}

func TestEnsureConfigDir(t *testing.T) {
	tmp := t.TempDir()
	setXDGConfigHome(t, tmp)

	dir, err := platform.EnsureConfigDir()
	if err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("directory not created: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("%q is not a directory", dir)
	}
}
