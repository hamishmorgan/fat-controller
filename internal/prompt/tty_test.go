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
