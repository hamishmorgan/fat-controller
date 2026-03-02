package auth_test

import (
	"os"
	"path/filepath"
	"strings"
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

func TestTokenStore_Load_KeyringUnmarshalError(t *testing.T) {
	keyring.MockInit()

	// Write invalid JSON directly into the mock keyring.
	if err := keyring.Set("fat-controller-test", "oauth-token", "not valid json"); err != nil {
		t.Fatal(err)
	}

	store := auth.NewTokenStore(
		auth.WithKeyringService("fat-controller-test"),
		auth.WithFallbackPath(filepath.Join(t.TempDir(), "auth.json")),
	)

	_, err := store.Load()
	if err == nil {
		t.Fatal("expected unmarshal error")
	}
	if !strings.Contains(err.Error(), "unmarshalling keyring data") {
		t.Errorf("error should mention unmarshalling keyring data, got: %s", err)
	}
}

func TestTokenStore_Delete_FileRemoveError(t *testing.T) {
	// Point fallback to a path inside a non-existent directory that isn't
	// simply "file does not exist" — use a path where the parent is a file,
	// so os.Remove returns a non-ENOENT error.
	tmpDir := t.TempDir()
	blockingFile := filepath.Join(tmpDir, "blocker")
	if err := os.WriteFile(blockingFile, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	// fallbackPath points to a child of a regular file, so Remove will fail
	// with ENOTDIR, which is not ENOENT.
	fallbackPath := filepath.Join(blockingFile, "auth.json")

	keyring.MockInit()
	store := auth.NewTokenStore(
		auth.WithKeyringService("fat-controller-test"),
		auth.WithFallbackPath(fallbackPath),
	)

	err := store.Delete()
	if err == nil {
		t.Fatal("expected error from os.Remove on invalid path")
	}
	if !strings.Contains(err.Error(), "removing fallback file") {
		t.Errorf("error should mention removing fallback file, got: %s", err)
	}
}

func TestTokenStore_SaveToFile_EmptyFallbackPath(t *testing.T) {
	// When keyring fails and no fallback path is set, Save should return error.
	keyring.MockInitWithError(os.ErrPermission)

	store := auth.NewTokenStore(
		auth.WithKeyringService("fat-controller-test"),
		// No fallback path set.
	)

	err := store.Save(&auth.StoredTokens{AccessToken: "test"})
	if err == nil {
		t.Fatal("expected error when no fallback path configured")
	}
	if !strings.Contains(err.Error(), "no fallback path configured") {
		t.Errorf("error should mention no fallback path, got: %s", err)
	}
}

func TestTokenStore_SaveToFile_MkdirAllError(t *testing.T) {
	keyring.MockInitWithError(os.ErrPermission)

	// Point fallback path inside a regular file so MkdirAll fails.
	tmpDir := t.TempDir()
	blockingFile := filepath.Join(tmpDir, "blocker")
	if err := os.WriteFile(blockingFile, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	fallbackPath := filepath.Join(blockingFile, "subdir", "auth.json")

	store := auth.NewTokenStore(
		auth.WithKeyringService("fat-controller-test"),
		auth.WithFallbackPath(fallbackPath),
	)

	err := store.Save(&auth.StoredTokens{AccessToken: "test"})
	if err == nil {
		t.Fatal("expected MkdirAll error")
	}
	if !strings.Contains(err.Error(), "creating config directory") {
		t.Errorf("error should mention creating config directory, got: %s", err)
	}
}

func TestTokenStore_LoadFromFile_ReadError(t *testing.T) {
	keyring.MockInitWithError(os.ErrPermission)

	// Create a directory where the fallback file should be — ReadFile on a
	// directory returns an error that is NOT os.IsNotExist.
	tmpDir := t.TempDir()
	fallbackPath := filepath.Join(tmpDir, "auth.json")
	if err := os.Mkdir(fallbackPath, 0o700); err != nil {
		t.Fatal(err)
	}

	store := auth.NewTokenStore(
		auth.WithKeyringService("fat-controller-test"),
		auth.WithFallbackPath(fallbackPath),
	)

	_, err := store.Load()
	if err == nil {
		t.Fatal("expected read error")
	}
	if !strings.Contains(err.Error(), "reading fallback file") {
		t.Errorf("error should mention reading fallback file, got: %s", err)
	}
}

func TestTokenStore_SaveToFile_WriteFileError(t *testing.T) {
	keyring.MockInitWithError(os.ErrPermission)

	// Use a directory as the fallback path — WriteFile to a directory fails.
	tmpDir := t.TempDir()
	fallbackDir := filepath.Join(tmpDir, "authdir")
	if err := os.Mkdir(fallbackDir, 0o700); err != nil {
		t.Fatal(err)
	}

	store := auth.NewTokenStore(
		auth.WithKeyringService("fat-controller-test"),
		auth.WithFallbackPath(fallbackDir),
	)

	err := store.Save(&auth.StoredTokens{AccessToken: "test"})
	if err == nil {
		t.Fatal("expected WriteFile error")
	}
	if !strings.Contains(err.Error(), "writing fallback file") {
		t.Errorf("error should mention writing fallback file, got: %s", err)
	}
}

func TestTokenStore_LoadFromFile_UnmarshalError(t *testing.T) {
	keyring.MockInitWithError(os.ErrPermission)

	tmpDir := t.TempDir()
	fallbackPath := filepath.Join(tmpDir, "auth.json")
	if err := os.WriteFile(fallbackPath, []byte("not valid json"), 0o600); err != nil {
		t.Fatal(err)
	}

	store := auth.NewTokenStore(
		auth.WithKeyringService("fat-controller-test"),
		auth.WithFallbackPath(fallbackPath),
	)

	_, err := store.Load()
	if err == nil {
		t.Fatal("expected unmarshal error")
	}
	if !strings.Contains(err.Error(), "unmarshalling fallback file") {
		t.Errorf("error should mention unmarshalling fallback file, got: %s", err)
	}
}
