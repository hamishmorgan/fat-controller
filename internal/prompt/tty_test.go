package prompt

import (
	"os"
	"testing"
)

func TestIsInteractive_ReturnsBool(t *testing.T) {
	// In test environment, stdin is typically not a TTY.
	got := IsInteractive(os.Stdin)
	// We don't assert true/false since it depends on environment,
	// but verify it doesn't panic and returns a bool.
	_ = got
}
