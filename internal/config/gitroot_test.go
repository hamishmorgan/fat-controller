package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/config"
)

func TestFindGitRoot_InRepo(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := config.FindGitRoot(sub)
	if err != nil {
		t.Fatal(err)
	}
	if got != root {
		t.Errorf("FindGitRoot(%q) = %q, want %q", sub, got, root)
	}
}

func TestFindGitRoot_NotInRepo(t *testing.T) {
	dir := t.TempDir()
	_, err := config.FindGitRoot(dir)
	if err == nil {
		t.Fatal("expected error when not in a git repo")
	}
}

func TestFindGitRoot_AtRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := config.FindGitRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	if got != root {
		t.Errorf("FindGitRoot(%q) = %q, want %q", root, got, root)
	}
}
