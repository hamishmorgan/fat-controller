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
	if err := cli.RunAuthLogout(&buf); err != nil {
		t.Fatalf("RunAuthLogout() error: %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "Logged out successfully") {
		t.Errorf("output = %q, want it to contain %q", got, "Logged out successfully")
	}
}
