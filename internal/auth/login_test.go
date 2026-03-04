package auth_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/auth"
	"github.com/zalando/go-keyring"
)

func TestLogin_FullFlow(t *testing.T) {
	keyring.MockInit()

	// Mock registration endpoint.
	regServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(auth.RegistrationResponse{
			ClientID: "test-client-id",
		}); err != nil {
			t.Fatal(err)
		}
	}))
	defer regServer.Close()

	// Mock token endpoint.
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(auth.TokenResponse{
			AccessToken:  "test-access-token",
			RefreshToken: "test-refresh-token",
			ExpiresIn:    3600,
		}); err != nil {
			t.Fatal(err)
		}
	}))
	defer tokenServer.Close()

	oauth := &auth.OAuthClient{
		AuthEndpoint:    "https://example.com/oauth/auth",
		TokenEndpoint:   tokenServer.URL,
		RegistrationURL: regServer.URL,
	}

	store := auth.NewTokenStore(
		auth.WithKeyringService("fat-controller-test"),
		auth.WithFallbackPath(filepath.Join(t.TempDir(), "auth.json")),
	)

	// Simulate the browser: parse the auth URL, extract state, hit the
	// callback server with a fake auth code and the correct state.
	fakeBrowser := func(authURL string) error {
		parsed, err := url.Parse(authURL)
		if err != nil {
			return err
		}
		state := parsed.Query().Get("state")
		redirectURI := parsed.Query().Get("redirect_uri")

		callbackURL := fmt.Sprintf("%s?code=fake-auth-code&state=%s", redirectURI, state)
		resp, err := http.Get(callbackURL) //nolint:errcheck,noctx
		if err != nil {
			return err
		}
		resp.Body.Close() //nolint:errcheck
		return nil
	}

	err := auth.Login(oauth, store, fakeBrowser)
	if err != nil {
		t.Fatalf("Login() error: %v", err)
	}

	// Verify tokens were stored.
	tokens, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if tokens.AccessToken != "test-access-token" {
		t.Errorf("AccessToken = %q, want %q", tokens.AccessToken, "test-access-token")
	}
	if tokens.RefreshToken != "test-refresh-token" {
		t.Errorf("RefreshToken = %q, want %q", tokens.RefreshToken, "test-refresh-token")
	}
	if tokens.ClientID != "test-client-id" {
		t.Errorf("ClientID = %q, want %q", tokens.ClientID, "test-client-id")
	}
}

func TestLogin_AuthorizationDenied(t *testing.T) {
	keyring.MockInit()

	regServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(auth.RegistrationResponse{
			ClientID: "test-client-id",
		}); err != nil {
			t.Fatal(err)
		}
	}))
	defer regServer.Close()

	oauth := &auth.OAuthClient{
		AuthEndpoint:    "https://example.com/oauth/auth",
		TokenEndpoint:   "https://example.com/token",
		RegistrationURL: regServer.URL,
	}

	store := auth.NewTokenStore(
		auth.WithKeyringService("fat-controller-test"),
		auth.WithFallbackPath(filepath.Join(t.TempDir(), "auth.json")),
	)

	// Simulate the browser returning an error.
	fakeBrowser := func(authURL string) error {
		parsed, _ := url.Parse(authURL)
		redirectURI := parsed.Query().Get("redirect_uri")
		callbackURL := fmt.Sprintf("%s?error=access_denied&error_description=User+denied", redirectURI)
		resp, err := http.Get(callbackURL) //nolint:noctx
		if err != nil {
			return err
		}
		resp.Body.Close() //nolint:errcheck
		return nil
	}

	err := auth.Login(oauth, store, fakeBrowser)
	if err == nil {
		t.Fatal("Login() should have returned an error")
	}
	// Should contain the original error, not a retry-related error.
	if got := err.Error(); !strings.Contains(got, "access_denied") {
		t.Errorf("error should mention access_denied, got: %s", got)
	}
}

func TestLogin_RetriesOnStaleClientID(t *testing.T) {
	keyring.MockInit()

	regCalls := 0
	regServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		regCalls++
		if err := json.NewEncoder(w).Encode(auth.RegistrationResponse{
			ClientID: fmt.Sprintf("client-%d", regCalls),
		}); err != nil {
			t.Fatal(err)
		}
	}))
	defer regServer.Close()

	tokenCalls := 0
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenCalls++
		if tokenCalls == 1 {
			// First attempt: reject (simulates stale client ID).
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		// Second attempt: succeed.
		if err := json.NewEncoder(w).Encode(auth.TokenResponse{
			AccessToken:  "retry-access-token",
			RefreshToken: "retry-refresh-token",
			ExpiresIn:    3600,
		}); err != nil {
			t.Fatal(err)
		}
	}))
	defer tokenServer.Close()

	oauth := &auth.OAuthClient{
		AuthEndpoint:    "https://example.com/oauth/auth",
		TokenEndpoint:   tokenServer.URL,
		RegistrationURL: regServer.URL,
	}

	store := auth.NewTokenStore(
		auth.WithKeyringService("fat-controller-test"),
		auth.WithFallbackPath(filepath.Join(t.TempDir(), "auth.json")),
	)

	fakeBrowser := func(authURL string) error {
		parsed, err := url.Parse(authURL)
		if err != nil {
			return err
		}
		state := parsed.Query().Get("state")
		redirectURI := parsed.Query().Get("redirect_uri")
		callbackURL := fmt.Sprintf("%s?code=fake-code&state=%s", redirectURI, state)
		resp, err := http.Get(callbackURL) //nolint:noctx
		if err != nil {
			return err
		}
		resp.Body.Close() //nolint:errcheck
		return nil
	}

	err := auth.Login(oauth, store, fakeBrowser)
	if err != nil {
		t.Fatalf("Login() error: %v", err)
	}

	// Should have registered twice (once for initial, once for retry).
	if regCalls != 2 {
		t.Errorf("registration calls = %d, want 2", regCalls)
	}
	// Should have hit token endpoint twice.
	if tokenCalls != 2 {
		t.Errorf("token calls = %d, want 2", tokenCalls)
	}

	tokens, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if tokens.AccessToken != "retry-access-token" {
		t.Errorf("AccessToken = %q, want %q", tokens.AccessToken, "retry-access-token")
	}
}

func TestLogin_BrowserOpenError(t *testing.T) {
	keyring.MockInit()

	regServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(auth.RegistrationResponse{
			ClientID: "test-client-id",
		}); err != nil {
			t.Fatal(err)
		}
	}))
	defer regServer.Close()

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(auth.TokenResponse{
			AccessToken:  "test-access-token",
			RefreshToken: "test-refresh-token",
			ExpiresIn:    3600,
		}); err != nil {
			t.Fatal(err)
		}
	}))
	defer tokenServer.Close()

	oauth := &auth.OAuthClient{
		AuthEndpoint:    "https://example.com/oauth/auth",
		TokenEndpoint:   tokenServer.URL,
		RegistrationURL: regServer.URL,
	}

	store := auth.NewTokenStore(
		auth.WithKeyringService("fat-controller-test"),
		auth.WithFallbackPath(filepath.Join(t.TempDir(), "auth.json")),
	)

	// Browser opener that returns an error but still hits the callback
	// (simulates browser failure with user manually navigating).
	fakeBrowser := func(authURL string) error {
		parsed, err := url.Parse(authURL)
		if err != nil {
			return err
		}
		state := parsed.Query().Get("state")
		redirectURI := parsed.Query().Get("redirect_uri")

		// Hit the callback in a goroutine (simulates user copying URL).
		go func() {
			callbackURL := fmt.Sprintf("%s?code=fake-auth-code&state=%s", redirectURI, state)
			resp, err := http.Get(callbackURL) //nolint:noctx
			if err != nil {
				return
			}
			resp.Body.Close() //nolint:errcheck
		}()

		return errors.New("browser open failed")
	}

	err := auth.Login(oauth, store, fakeBrowser)
	if err != nil {
		t.Fatalf("Login() should succeed despite browser error, got: %v", err)
	}
}

func TestLogin_StateMismatch(t *testing.T) {
	keyring.MockInit()

	regServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(auth.RegistrationResponse{
			ClientID: "test-client-id",
		}); err != nil {
			t.Fatal(err)
		}
	}))
	defer regServer.Close()

	oauth := &auth.OAuthClient{
		AuthEndpoint:    "https://example.com/oauth/auth",
		TokenEndpoint:   "https://example.com/token",
		RegistrationURL: regServer.URL,
	}

	store := auth.NewTokenStore(
		auth.WithKeyringService("fat-controller-test"),
		auth.WithFallbackPath(filepath.Join(t.TempDir(), "auth.json")),
	)

	// Return a different state than what was sent.
	fakeBrowser := func(authURL string) error {
		parsed, _ := url.Parse(authURL)
		redirectURI := parsed.Query().Get("redirect_uri")

		callbackURL := fmt.Sprintf("%s?code=fake-code&state=wrong-state", redirectURI)
		resp, err := http.Get(callbackURL) //nolint:noctx
		if err != nil {
			return err
		}
		resp.Body.Close() //nolint:errcheck
		return nil
	}

	err := auth.Login(oauth, store, fakeBrowser)
	if err == nil {
		t.Fatal("Login() should have returned an error for state mismatch")
	}
	if !strings.Contains(err.Error(), "state mismatch") {
		t.Errorf("error should mention state mismatch, got: %s", err)
	}
}

func TestLogin_StoreSaveError(t *testing.T) {
	// Use a keyring that errors on Set and no fallback path, so Save fails.
	keyring.MockInitWithError(os.ErrPermission)

	regServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(auth.RegistrationResponse{
			ClientID: "test-client-id",
		}); err != nil {
			t.Fatal(err)
		}
	}))
	defer regServer.Close()

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(auth.TokenResponse{
			AccessToken:  "test-access-token",
			RefreshToken: "test-refresh-token",
			ExpiresIn:    3600,
		}); err != nil {
			t.Fatal(err)
		}
	}))
	defer tokenServer.Close()

	oauth := &auth.OAuthClient{
		AuthEndpoint:    "https://example.com/oauth/auth",
		TokenEndpoint:   tokenServer.URL,
		RegistrationURL: regServer.URL,
	}

	// No fallback path — Save will fail.
	store := auth.NewTokenStore(
		auth.WithKeyringService("fat-controller-test"),
	)

	fakeBrowser := func(authURL string) error {
		parsed, err := url.Parse(authURL)
		if err != nil {
			return err
		}
		state := parsed.Query().Get("state")
		redirectURI := parsed.Query().Get("redirect_uri")

		callbackURL := fmt.Sprintf("%s?code=fake-auth-code&state=%s", redirectURI, state)
		resp, err := http.Get(callbackURL) //nolint:noctx
		if err != nil {
			return err
		}
		resp.Body.Close() //nolint:errcheck
		return nil
	}

	err := auth.Login(oauth, store, fakeBrowser)
	if err == nil {
		t.Fatal("Login() should have returned an error when store.Save fails")
	}
	if !strings.Contains(err.Error(), "storing tokens") {
		t.Errorf("error should mention storing tokens, got: %s", err)
	}
}

func TestLogin_UsesStoredClientID(t *testing.T) {
	keyring.MockInit()

	regCalls := 0
	regServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		regCalls++
		if err := json.NewEncoder(w).Encode(auth.RegistrationResponse{
			ClientID: "new-client-id",
		}); err != nil {
			t.Fatal(err)
		}
	}))
	defer regServer.Close()

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(auth.TokenResponse{
			AccessToken:  "test-access-token",
			RefreshToken: "test-refresh-token",
			ExpiresIn:    3600,
		}); err != nil {
			t.Fatal(err)
		}
	}))
	defer tokenServer.Close()

	oauth := &auth.OAuthClient{
		AuthEndpoint:    "https://example.com/oauth/auth",
		TokenEndpoint:   tokenServer.URL,
		RegistrationURL: regServer.URL,
	}

	store := auth.NewTokenStore(
		auth.WithKeyringService("fat-controller-test"),
		auth.WithFallbackPath(filepath.Join(t.TempDir(), "auth.json")),
	)

	// Pre-store a client ID so loadOrRegisterClient returns it.
	if err := store.Save(&auth.StoredTokens{
		ClientID:    "stored-client-id",
		AccessToken: "old-token",
	}); err != nil {
		t.Fatal(err)
	}

	fakeBrowser := func(authURL string) error {
		parsed, err := url.Parse(authURL)
		if err != nil {
			return err
		}
		state := parsed.Query().Get("state")
		redirectURI := parsed.Query().Get("redirect_uri")

		// Verify we're using the stored client ID, not a new one.
		clientID := parsed.Query().Get("client_id")
		if clientID != "stored-client-id" {
			t.Errorf("should use stored client ID, got %q", clientID)
		}

		callbackURL := fmt.Sprintf("%s?code=fake-auth-code&state=%s", redirectURI, state)
		resp, err := http.Get(callbackURL) //nolint:noctx
		if err != nil {
			return err
		}
		resp.Body.Close() //nolint:errcheck
		return nil
	}

	err := auth.Login(oauth, store, fakeBrowser)
	if err != nil {
		t.Fatalf("Login() error: %v", err)
	}

	// Should NOT have called registration endpoint.
	if regCalls != 0 {
		t.Errorf("registration calls = %d, want 0 (should use stored client ID)", regCalls)
	}
}

func TestLogin_RegistrationError(t *testing.T) {
	keyring.MockInit()

	// Registration server returns 500.
	regServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer regServer.Close()

	oauth := &auth.OAuthClient{
		AuthEndpoint:    "https://example.com/oauth/auth",
		TokenEndpoint:   "https://example.com/token",
		RegistrationURL: regServer.URL,
	}

	store := auth.NewTokenStore(
		auth.WithKeyringService("fat-controller-test"),
		auth.WithFallbackPath(filepath.Join(t.TempDir(), "auth.json")),
	)

	fakeBrowser := func(authURL string) error {
		return nil
	}

	err := auth.Login(oauth, store, fakeBrowser)
	if err == nil {
		t.Fatal("Login() should have returned an error when registration fails")
	}
	if !strings.Contains(err.Error(), "client registration") {
		t.Errorf("error should mention client registration, got: %s", err)
	}
}
