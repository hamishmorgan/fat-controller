package prompt_test

import (
	"os"
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/prompt"
)

func TestIsInteractive_ReturnsFalseInTestEnv(t *testing.T) {
	// In test environments, stdin is not a TTY.
	if prompt.IsInteractive(os.Stdin) {
		t.Skip("stdin is a TTY in this environment — skipping")
	}
}

func TestStdinIsInteractive_ReturnsFalseInTestEnv(t *testing.T) {
	// StdinIsInteractive is a convenience wrapper; verify it doesn't panic.
	if prompt.StdinIsInteractive() {
		t.Skip("stdin is a TTY in this environment — skipping")
	}
}

func TestIsCI_True(t *testing.T) {
	t.Setenv("CI", "true")
	if !prompt.IsCI() {
		t.Error("IsCI() = false, want true")
	}
}

func TestIsCI_False(t *testing.T) {
	t.Setenv("CI", "")
	if prompt.IsCI() {
		t.Error("IsCI() = true, want false")
	}
}

func TestIsCI_Unset(t *testing.T) {
	// CI env var may or may not be set in the test runner.
	// Use Setenv with empty to explicitly clear, then check.
	t.Setenv("CI", "")
	_ = os.Unsetenv("CI")
	if prompt.IsCI() {
		t.Error("IsCI() = true, want false when unset")
	}
}
