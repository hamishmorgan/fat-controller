package auth_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/auth"
	"github.com/zalando/go-keyring"
)

func TestTokenStore_SaveAndLoad_Keyring(t *testing.T) {
	keyring.MockInit()

	store := auth.NewTokenStore(
		auth.WithKeyringService("fat-controller-test"),
		auth.WithFallbackPath(filepath.Join(t.TempDir(), "auth.json")),
	)

	tokens := &auth.StoredTokens{
		AccessToken:  "access-123",
		RefreshToken: "refresh-456",
		ClientID:     "client-789",
	}

	if err := store.Save(tokens); err != nil {
		t.Fatal(err)
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}

	if loaded.AccessToken != tokens.AccessToken {
		t.Errorf("AccessToken = %q, want %q", loaded.AccessToken, tokens.AccessToken)
	}
	if loaded.RefreshToken != tokens.RefreshToken {
		t.Errorf("RefreshToken = %q, want %q", loaded.RefreshToken, tokens.RefreshToken)
	}
	if loaded.ClientID != tokens.ClientID {
		t.Errorf("ClientID = %q, want %q", loaded.ClientID, tokens.ClientID)
	}
}

func TestTokenStore_SaveAndLoad_FileFallback(t *testing.T) {
	keyring.MockInitWithError(os.ErrPermission)

	fallbackPath := filepath.Join(t.TempDir(), "auth.json")
	store := auth.NewTokenStore(
		auth.WithKeyringService("fat-controller-test"),
		auth.WithFallbackPath(fallbackPath),
	)

	tokens := &auth.StoredTokens{
		AccessToken:  "access-abc",
		RefreshToken: "refresh-def",
		ClientID:     "client-ghi",
	}

	if err := store.Save(tokens); err != nil {
		t.Fatal(err)
	}

	// Verify file was created with correct permissions.
	info, err := os.Stat(fallbackPath)
	if err != nil {
		t.Fatalf("fallback file not created: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("file permissions = %o, want 0600", perm)
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}

	if loaded.AccessToken != tokens.AccessToken {
		t.Errorf("AccessToken = %q, want %q", loaded.AccessToken, tokens.AccessToken)
	}
}

func TestTokenStore_Delete_Keyring(t *testing.T) {
	keyring.MockInit()

	store := auth.NewTokenStore(
		auth.WithKeyringService("fat-controller-test"),
		auth.WithFallbackPath(filepath.Join(t.TempDir(), "auth.json")),
	)

	tokens := &auth.StoredTokens{
		AccessToken: "access-123",
		ClientID:    "client-789",
	}
	if err := store.Save(tokens); err != nil {
		t.Fatal(err)
	}

	if err := store.Delete(); err != nil {
		t.Fatal(err)
	}

	_, err := store.Load()
	if err != auth.ErrNoStoredTokens {
		t.Errorf("expected ErrNoStoredTokens, got %v", err)
	}
}

func TestTokenStore_Delete_FileFallback(t *testing.T) {
	keyring.MockInitWithError(os.ErrPermission)

	fallbackPath := filepath.Join(t.TempDir(), "auth.json")
	store := auth.NewTokenStore(
		auth.WithKeyringService("fat-controller-test"),
		auth.WithFallbackPath(fallbackPath),
	)

	tokens := &auth.StoredTokens{
		AccessToken: "access-abc",
		ClientID:    "client-ghi",
	}
	if err := store.Save(tokens); err != nil {
		t.Fatal(err)
	}

	if err := store.Delete(); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(fallbackPath); !os.IsNotExist(err) {
		t.Errorf("fallback file should be deleted")
	}
}

func TestTokenStore_Load_Empty(t *testing.T) {
	keyring.MockInit()

	store := auth.NewTokenStore(
		auth.WithKeyringService("fat-controller-test"),
		auth.WithFallbackPath(filepath.Join(t.TempDir(), "auth.json")),
	)

	_, err := store.Load()
	if err != auth.ErrNoStoredTokens {
		t.Errorf("expected ErrNoStoredTokens, got %v", err)
	}
}
