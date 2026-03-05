package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/zalando/go-keyring"
)

// ErrNoStoredTokens is returned when no tokens are found in keyring or file.
var ErrNoStoredTokens = errors.New("no stored tokens found")

// StoredTokens holds the persisted OAuth tokens and client registration.
type StoredTokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ClientID     string `json:"client_id"`
}

// TokenStore handles persisting OAuth tokens to OS keyring with file fallback.
type TokenStore struct {
	keyringService string
	keyringUser    string
	fallbackPath   string
}

// TokenStoreOption configures a TokenStore.
type TokenStoreOption func(*TokenStore)

// WithKeyringService sets a custom keyring service name (useful for tests).
func WithKeyringService(service string) TokenStoreOption {
	return func(s *TokenStore) { s.keyringService = service }
}

// WithFallbackPath sets a custom fallback file path (useful for tests).
func WithFallbackPath(path string) TokenStoreOption {
	return func(s *TokenStore) { s.fallbackPath = path }
}

// NewTokenStore creates a TokenStore with default settings.
// Pass options to override keyring service or fallback path.
func NewTokenStore(opts ...TokenStoreOption) *TokenStore {
	s := &TokenStore{
		keyringService: "fat-controller",
		keyringUser:    "oauth-token",
		// fallbackPath should be set by caller or via option.
		// Default empty — Load/Save will skip file fallback if unset.
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Save persists tokens. Tries keyring first, falls back to file.
func (s *TokenStore) Save(tokens *StoredTokens) error {
	data, err := json.Marshal(tokens)
	if err != nil {
		return fmt.Errorf("marshalling tokens: %w", err)
	}

	if err := keyring.Set(s.keyringService, s.keyringUser, string(data)); err != nil {
		// Keyring unavailable — fall back to file.
		slog.Debug("keyring unavailable, falling back to file", "path", s.fallbackPath)
		return s.saveToFile(data)
	}
	slog.Debug("tokens saved to keyring")
	return nil
}

// Load retrieves stored tokens. Tries keyring first, then file.
// Returns ErrNoStoredTokens if nothing is stored anywhere.
func (s *TokenStore) Load() (*StoredTokens, error) {
	// Try keyring.
	data, err := keyring.Get(s.keyringService, s.keyringUser)
	if err == nil {
		var tokens StoredTokens
		if err := json.Unmarshal([]byte(data), &tokens); err != nil {
			return nil, fmt.Errorf("unmarshalling keyring data: %w", err)
		}
		slog.Debug("tokens loaded from keyring")
		return &tokens, nil
	}

	// Keyring miss — try file fallback.
	slog.Debug("keyring miss, trying file fallback", "path", s.fallbackPath)
	return s.loadFromFile()
}

// Delete removes stored tokens from both keyring and file.
func (s *TokenStore) Delete() error {
	slog.Debug("deleting stored tokens")
	// Delete from keyring (ignore ErrNotFound and other keyring errors).
	// Keyring failures are not fatal — we still clean up the file fallback.
	_ = keyring.Delete(s.keyringService, s.keyringUser)

	// Delete file if it exists.
	if s.fallbackPath != "" {
		if err := os.Remove(s.fallbackPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("removing fallback file: %w", err)
		}
	}
	return nil
}

func (s *TokenStore) saveToFile(data []byte) error {
	if s.fallbackPath == "" {
		return fmt.Errorf("keyring unavailable and no fallback path configured")
	}

	dir := filepath.Dir(s.fallbackPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	if err := os.WriteFile(s.fallbackPath, data, 0o600); err != nil {
		return fmt.Errorf("writing fallback file: %w", err)
	}
	return nil
}

func (s *TokenStore) loadFromFile() (*StoredTokens, error) {
	if s.fallbackPath == "" {
		return nil, ErrNoStoredTokens
	}

	data, err := os.ReadFile(s.fallbackPath)
	if err != nil {
		if os.IsNotExist(err) {
			slog.Debug("no fallback file found", "path", s.fallbackPath)
			return nil, ErrNoStoredTokens
		}
		return nil, fmt.Errorf("reading fallback file: %w", err)
	}

	var tokens StoredTokens
	if err := json.Unmarshal(data, &tokens); err != nil {
		return nil, fmt.Errorf("unmarshalling fallback file: %w", err)
	}
	return &tokens, nil
}
