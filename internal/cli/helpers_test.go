package cli_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

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

// --- Shared mutation fakes ---

// recordingMutator implements both configSetter and configDeleter,
// recording the arguments it was called with. Set err to make it fail.
type recordingMutator struct {
	called  bool
	service string
	key     string
	value   string // populated only by SetVar
	err     error
}

func (r *recordingMutator) SetVar(_ context.Context, service, key, value string) error {
	r.called = true
	r.service = service
	r.key = key
	r.value = value
	return r.err
}

func (r *recordingMutator) DeleteVar(_ context.Context, service, key string) error {
	r.called = true
	r.service = service
	r.key = key
	return r.err
}

// --- Shared fetcher fakes ---

// fakeFetcher implements cli.ConfigFetcher for testing.
type fakeFetcher struct {
	cfg        *config.LiveConfig
	resolveErr error
	fetchErr   error
}

func (f *fakeFetcher) Resolve(_ context.Context, _, _, _ string) (string, string, error) {
	if f.resolveErr != nil {
		return "", "", f.resolveErr
	}
	return "proj-1", "env-1", nil
}

func (f *fakeFetcher) Fetch(_ context.Context, _, _, _ string) (*config.LiveConfig, error) {
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

func (f *capturingFetcher) Resolve(_ context.Context, _, project, environment string) (string, string, error) {
	f.project = project
	f.environment = environment
	return "proj-1", "env-1", nil
}

func (f *capturingFetcher) Fetch(_ context.Context, _, _, _ string) (*config.LiveConfig, error) {
	return f.cfg, nil
}
