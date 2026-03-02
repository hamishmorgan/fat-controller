package auth_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
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
}
