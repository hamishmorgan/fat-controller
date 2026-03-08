package cli_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/cli"
	"github.com/zalando/go-keyring"
)

func TestRunAuthLogout_PrintsSuccess(t *testing.T) {
	keyring.MockInit()

	var buf bytes.Buffer
	// Pass yes=true to skip confirmation prompt.
	if err := cli.RunAuthLogout(false, true, &buf); err != nil {
		t.Fatalf("RunAuthLogout() error: %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "Logged out successfully") {
		t.Errorf("output = %q, want it to contain %q", got, "Logged out successfully")
	}
}

func TestRunAuthLogout_RefusesNonInteractive(t *testing.T) {
	keyring.MockInit()

	var buf bytes.Buffer
	// Non-interactive without --yes should fail.
	err := cli.RunAuthLogout(false, false, &buf)
	if err == nil {
		t.Fatal("expected error for non-interactive logout without --yes")
	}
	if !strings.Contains(err.Error(), "non-interactive") {
		t.Errorf("error = %q, want it to mention non-interactive", err.Error())
	}
}
