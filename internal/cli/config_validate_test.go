package cli_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/cli"
)

func TestRunConfigValidate_ShowsWarnings(t *testing.T) {
	dir := t.TempDir()
	// Config with a lowercase var name (W030) and empty string delete (W012).
	writeTOML(t, dir, "fat-controller.toml", `
[[service]]
name = "api"
variables = { myVar = "" }
`)

	var buf bytes.Buffer
	globals := &cli.Globals{}
	err := cli.RunConfigValidate(globals, dir, nil, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "W030") {
		t.Errorf("expected W030 warning in output, got:\n%s", out)
	}
	if !strings.Contains(out, "W012") {
		t.Errorf("expected W012 warning in output, got:\n%s", out)
	}
}

func TestRunConfigValidate_NoWarnings(t *testing.T) {
	dir := t.TempDir()
	writeTOML(t, dir, "fat-controller.toml", `
[[service]]
name = "api"
variables = { PORT = "8080" }
`)

	var buf bytes.Buffer
	globals := &cli.Globals{}
	err := cli.RunConfigValidate(globals, dir, nil, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "No warnings found") {
		t.Errorf("expected 'No warnings found', got:\n%s", out)
	}
}

func TestRunConfigValidate_MissingConfigFile(t *testing.T) {
	dir := t.TempDir()
	var buf bytes.Buffer
	globals := &cli.Globals{}
	err := cli.RunConfigValidate(globals, dir, nil, &buf)
	if err == nil {
		t.Fatal("expected error for missing config file")
	}
}

func TestRunConfigValidate_ExitCleanlyWithWarnings(t *testing.T) {
	dir := t.TempDir()
	// Empty service block triggers W003.
	writeTOML(t, dir, "fat-controller.toml", `
[[service]]
name = "api"
`)

	var buf bytes.Buffer
	globals := &cli.Globals{}
	err := cli.RunConfigValidate(globals, dir, nil, &buf)
	// Warnings are advisory — should return nil, not an error.
	if err != nil {
		t.Fatalf("expected no error (warnings are advisory), got: %v", err)
	}
}
