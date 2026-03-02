package auth_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/auth"
	"github.com/zalando/go-keyring"
)

func TestResolveAuth_FlagTakesPrecedence(t *testing.T) {
	keyring.MockInit()

	store := auth.NewTokenStore(
		auth.WithKeyringService("fat-controller-test"),
		auth.WithFallbackPath(filepath.Join(t.TempDir(), "auth.json")),
	)
	// Store an OAuth token.
	if err := store.Save(&auth.StoredTokens{AccessToken: "stored-token"}); err != nil {
		t.Fatal(err)
	}
	// Set env vars too.
	t.Setenv("RAILWAY_API_TOKEN", "api-token")
	t.Setenv("RAILWAY_TOKEN", "project-token")

	// Flag should win.
	resolved, err := auth.ResolveAuth("flag-token", store)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Token != "flag-token" {
		t.Errorf("Token = %q, want %q", resolved.Token, "flag-token")
	}
	if resolved.HeaderName != "Authorization" {
		t.Errorf("HeaderName = %q, want %q", resolved.HeaderName, "Authorization")
	}
	if resolved.Source != auth.SourceFlag {
		t.Errorf("Source = %q, want %q", resolved.Source, auth.SourceFlag)
	}
}

func TestResolveAuth_APITokenEnvVar(t *testing.T) {
	keyring.MockInit()

	store := auth.NewTokenStore(
		auth.WithKeyringService("fat-controller-test"),
		auth.WithFallbackPath(filepath.Join(t.TempDir(), "auth.json")),
	)
	t.Setenv("RAILWAY_API_TOKEN", "api-token")
	t.Setenv("RAILWAY_TOKEN", "project-token")

	resolved, err := auth.ResolveAuth("", store)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Token != "api-token" {
		t.Errorf("Token = %q, want %q", resolved.Token, "api-token")
	}
	if resolved.HeaderName != "Authorization" {
		t.Errorf("HeaderName = %q, want %q", resolved.HeaderName, "Authorization")
	}
	if resolved.Source != auth.SourceEnvAPIToken {
		t.Errorf("Source = %q, want %q", resolved.Source, auth.SourceEnvAPIToken)
	}
}

func TestResolveAuth_ProjectTokenEnvVar(t *testing.T) {
	keyring.MockInit()

	store := auth.NewTokenStore(
		auth.WithKeyringService("fat-controller-test"),
		auth.WithFallbackPath(filepath.Join(t.TempDir(), "auth.json")),
	)
	t.Setenv("RAILWAY_API_TOKEN", "")
	t.Setenv("RAILWAY_TOKEN", "project-token")

	resolved, err := auth.ResolveAuth("", store)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Token != "project-token" {
		t.Errorf("Token = %q, want %q", resolved.Token, "project-token")
	}
	if resolved.HeaderName != "Project-Access-Token" {
		t.Errorf("HeaderName = %q, want %q", resolved.HeaderName, "Project-Access-Token")
	}
	if resolved.Source != auth.SourceEnvToken {
		t.Errorf("Source = %q, want %q", resolved.Source, auth.SourceEnvToken)
	}
}

func TestResolveAuth_FallsBackToStore(t *testing.T) {
	keyring.MockInit()

	store := auth.NewTokenStore(
		auth.WithKeyringService("fat-controller-test"),
		auth.WithFallbackPath(filepath.Join(t.TempDir(), "auth.json")),
	)
	if err := store.Save(&auth.StoredTokens{AccessToken: "stored-token"}); err != nil {
		t.Fatal(err)
	}

	t.Setenv("RAILWAY_API_TOKEN", "")
	t.Setenv("RAILWAY_TOKEN", "")

	resolved, err := auth.ResolveAuth("", store)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Token != "stored-token" {
		t.Errorf("Token = %q, want %q", resolved.Token, "stored-token")
	}
	if resolved.HeaderName != "Authorization" {
		t.Errorf("HeaderName = %q, want %q", resolved.HeaderName, "Authorization")
	}
	if resolved.Source != auth.SourceStored {
		t.Errorf("Source = %q, want %q", resolved.Source, auth.SourceStored)
	}
}

func TestResolveAuth_NothingAvailable(t *testing.T) {
	keyring.MockInit()

	store := auth.NewTokenStore(
		auth.WithKeyringService("fat-controller-test"),
		auth.WithFallbackPath(filepath.Join(t.TempDir(), "auth.json")),
	)

	t.Setenv("RAILWAY_API_TOKEN", "")
	t.Setenv("RAILWAY_TOKEN", "")

	_, err := auth.ResolveAuth("", store)
	if err != auth.ErrNotAuthenticated {
		t.Errorf("expected ErrNotAuthenticated, got %v", err)
	}
}

func TestResolveAuth_LoadError(t *testing.T) {
	// Trigger a non-ErrNoStoredTokens error from store.Load.
	// Use a keyring error + a fallback file that's a directory (causes read error).
	keyring.MockInitWithError(os.ErrPermission)

	tmpDir := t.TempDir()
	fallbackPath := filepath.Join(tmpDir, "auth.json")
	// Create a directory where the file should be — ReadFile will fail.
	if err := os.Mkdir(fallbackPath, 0o700); err != nil {
		t.Fatal(err)
	}

	store := auth.NewTokenStore(
		auth.WithKeyringService("fat-controller-test"),
		auth.WithFallbackPath(fallbackPath),
	)

	t.Setenv("RAILWAY_API_TOKEN", "")
	t.Setenv("RAILWAY_TOKEN", "")

	_, err := auth.ResolveAuth("", store)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "loading stored tokens") {
		t.Errorf("error should mention loading stored tokens, got: %s", err)
	}
}

func TestResolveAuth_EmptyAccessToken(t *testing.T) {
	keyring.MockInit()

	store := auth.NewTokenStore(
		auth.WithKeyringService("fat-controller-test"),
		auth.WithFallbackPath(filepath.Join(t.TempDir(), "auth.json")),
	)

	// Store tokens with empty access token but non-empty client ID.
	if err := store.Save(&auth.StoredTokens{
		AccessToken: "",
		ClientID:    "client-123",
	}); err != nil {
		t.Fatal(err)
	}

	t.Setenv("RAILWAY_API_TOKEN", "")
	t.Setenv("RAILWAY_TOKEN", "")

	_, err := auth.ResolveAuth("", store)
	if err != auth.ErrNotAuthenticated {
		t.Errorf("expected ErrNotAuthenticated, got %v", err)
	}
}
