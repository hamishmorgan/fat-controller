package cli_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
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

func TestRunConfigValidate_JSONOutput(t *testing.T) {
	dir := t.TempDir()
	// Empty service block triggers W003.
	writeTOML(t, dir, "fat-controller.toml", `
[[service]]
name = "api"
`)

	var buf bytes.Buffer
	globals := &cli.Globals{Output: "json"}
	if err := cli.RunConfigValidate(globals, dir, nil, &buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var payload struct {
		Warnings []struct {
			Code    string `json:"code"`
			Message string `json:"message"`
			Path    string `json:"path"`
		} `json:"warnings"`
	}
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, buf.String())
	}
	if len(payload.Warnings) == 0 {
		t.Fatalf("expected at least one warning, got 0")
	}
}

func TestRunConfigValidate_TOMLOutput(t *testing.T) {
	dir := t.TempDir()
	// Empty service block triggers W003.
	writeTOML(t, dir, "fat-controller.toml", `
[[service]]
name = "api"
`)

	var buf bytes.Buffer
	globals := &cli.Globals{Output: "toml"}
	if err := cli.RunConfigValidate(globals, dir, nil, &buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var payload struct {
		Warnings []struct {
			Code    string `toml:"code"`
			Message string `toml:"message"`
			Path    string `toml:"path"`
		} `toml:"warnings"`
	}
	if _, err := toml.Decode(buf.String(), &payload); err != nil {
		t.Fatalf("output is not valid TOML: %v\n%s", err, buf.String())
	}
	if len(payload.Warnings) == 0 {
		t.Fatalf("expected at least one warning, got 0")
	}
}
