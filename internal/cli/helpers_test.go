package cli_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/app"
	"github.com/hamishmorgan/fat-controller/internal/config"
)

// --- Shared TOML file helper ---

// writeTOML creates a file with the given name and content in dir.
func writeTOML(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// --- Shared fetcher fakes ---

// fakeFetcher implements cli.ConfigFetcher for testing.
type fakeFetcher struct {
	cfg        *config.LiveConfig
	resolveErr error
	fetchErr   error
}

func (f *fakeFetcher) Resolve(_ context.Context, _, _, _ string) (*app.ResolvedIdentity, error) {
	if f.resolveErr != nil {
		return nil, f.resolveErr
	}
	return &app.ResolvedIdentity{ProjectID: "proj-1", EnvironmentID: "env-1"}, nil
}

func (f *fakeFetcher) Fetch(_ context.Context, _, _ string, _ []string) (*config.LiveConfig, error) {
	if f.fetchErr != nil {
		return nil, f.fetchErr
	}
	return f.cfg, nil
}

// capturingFetcher records the project/environment passed to Resolve.
type capturingFetcher struct {
	cfg         *config.LiveConfig
	project     string
	environment string
}

func (f *capturingFetcher) Resolve(_ context.Context, _, project, environment string) (*app.ResolvedIdentity, error) {
	f.project = project
	f.environment = environment
	return &app.ResolvedIdentity{ProjectID: "proj-1", EnvironmentID: "env-1"}, nil
}

func (f *capturingFetcher) Fetch(_ context.Context, _, _ string, _ []string) (*config.LiveConfig, error) {
	return f.cfg, nil
}
