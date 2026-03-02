package auth_test

import (
	"path/filepath"
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
	if resolved.Source != "flag" {
		t.Errorf("Source = %q, want %q", resolved.Source, "flag")
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
	if resolved.Source != "env:RAILWAY_API_TOKEN" {
		t.Errorf("Source = %q, want %q", resolved.Source, "env:RAILWAY_API_TOKEN")
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
	if resolved.Source != "env:RAILWAY_TOKEN" {
		t.Errorf("Source = %q, want %q", resolved.Source, "env:RAILWAY_TOKEN")
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
	if resolved.Source != "stored" {
		t.Errorf("Source = %q, want %q", resolved.Source, "stored")
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
