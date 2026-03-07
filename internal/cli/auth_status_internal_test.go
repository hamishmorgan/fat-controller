package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/hamishmorgan/fat-controller/internal/platform"
	"github.com/zalando/go-keyring"
)

func TestRunAuthStatus_JSON_Unauthenticated(t *testing.T) {
	keyring.MockInit()
	_ = keyring.Delete("fat-controller", "oauth-token")

	// Ensure no env tokens and no stored token file.
	t.Setenv("RAILWAY_API_TOKEN", "")
	t.Setenv("RAILWAY_TOKEN", "")

	// Point XDG config home at temp so platform.AuthFilePath() is empty.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	_ = os.Remove(platform.AuthFilePath())

	globals := &Globals{Output: "json"}
	var buf bytes.Buffer
	if err := RunAuthStatus(context.Background(), "", globals, &buf); err != nil {
		t.Fatalf("RunAuthStatus() error: %v", err)
	}

	var payload struct {
		Authenticated bool   `json:"authenticated"`
		Error         string `json:"error"`
	}
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, buf.String())
	}
	if payload.Authenticated {
		t.Fatalf("authenticated = true, want false")
	}
	if payload.Error == "" {
		t.Fatalf("error is empty")
	}
}

func TestRunAuthStatus_TOML_EnvToken(t *testing.T) {
	keyring.MockInit()
	_ = keyring.Delete("fat-controller", "oauth-token")

	t.Setenv("RAILWAY_TOKEN", "tok-123")
	t.Setenv("RAILWAY_API_TOKEN", "")

	// platform.AuthFilePath() should exist but won't be used in this resolution path.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := os.MkdirAll(filepath.Dir(platform.AuthFilePath()), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	globals := &Globals{Output: "toml"}
	var buf bytes.Buffer
	if err := RunAuthStatus(context.Background(), "", globals, &buf); err != nil {
		t.Fatalf("RunAuthStatus() error: %v", err)
	}

	var payload struct {
		Authenticated bool   `toml:"authenticated"`
		Source        string `toml:"source"`
	}
	if err := toml.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatalf("output is not valid TOML: %v\n%s", err, buf.String())
	}
	if !payload.Authenticated {
		t.Fatalf("authenticated = false, want true")
	}
	if payload.Source == "" {
		t.Fatalf("source is empty")
	}
}
