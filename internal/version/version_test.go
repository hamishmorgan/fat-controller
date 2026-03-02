package version_test

import (
	"strings"
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/version"
)

func TestString_Defaults(t *testing.T) {
	// With no ldflags, defaults should be used.
	got := version.String()
	if got == "" {
		t.Fatal("String() should not be empty")
	}
	if !strings.Contains(got, "dev") {
		t.Errorf("default version should contain 'dev', got %q", got)
	}
}

func TestString_Format(t *testing.T) {
	// The output should be a single line with version, commit, date.
	got := version.String()
	// Should not contain newlines.
	if strings.Contains(got, "\n") {
		t.Errorf("String() should be single line, got %q", got)
	}
}
