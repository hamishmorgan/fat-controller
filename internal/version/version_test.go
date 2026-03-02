package version_test

import (
	"strings"
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/version"
)

func TestString_Defaults(t *testing.T) {
	// In `go test`, BuildInfo.Main.Version is "(devel)", so version falls
	// back to "dev". The commit and date may come from VCS settings if
	// running in a git repo, or fall back to "unknown".
	got := version.String()
	if got == "" {
		t.Fatal("String() should not be empty")
	}
	if !strings.Contains(got, "dev") {
		t.Errorf("default version should contain 'dev', got %q", got)
	}
}

func TestString_Format(t *testing.T) {
	got := version.String()
	if strings.Contains(got, "\n") {
		t.Errorf("String() should be single line, got %q", got)
	}
	// Should always have the three-part format.
	if !strings.Contains(got, "(commit ") {
		t.Errorf("String() should contain '(commit ', got %q", got)
	}
	if !strings.Contains(got, ", built ") {
		t.Errorf("String() should contain ', built ', got %q", got)
	}
}

func TestVersion_Default(t *testing.T) {
	got := version.Version()
	if got != "dev" {
		t.Errorf("Version() should be 'dev' in test, got %q", got)
	}
}
