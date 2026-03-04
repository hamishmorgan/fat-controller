package auth_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/auth"
)

func TestOAuthClient_RegisterClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected application/json content type")
		}

		var req auth.RegistrationRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if req.ClientName != "Fat Controller CLI" {
			t.Errorf("ClientName = %q", req.ClientName)
		}
		if req.TokenEndpointAuthMethod != "none" {
			t.Errorf("TokenEndpointAuthMethod = %q", req.TokenEndpointAuthMethod)
		}
		if req.ApplicationType != "native" {
			t.Errorf("ApplicationType = %q", req.ApplicationType)
		}

		if err := json.NewEncoder(w).Encode(auth.RegistrationResponse{
			ClientID:   "test-client-id",
			ClientName: "Fat Controller CLI",
		}); err != nil {
			t.Fatal(err)
		}
	}))
	defer server.Close()

	client := &auth.OAuthClient{
		RegistrationURL: server.URL,
	}
	resp, err := client.RegisterClient(context.Background(), "http://127.0.0.1:12345/callback")
	if err != nil {
		t.Fatal(err)
	}
	if resp.ClientID != "test-client-id" {
		t.Errorf("ClientID = %q", resp.ClientID)
	}
}

func TestOAuthClient_AuthorizationURL(t *testing.T) {
	client := &auth.OAuthClient{
		AuthEndpoint: "https://example.com/oauth/auth",
	}

	authURL := client.AuthorizationURL("client-123", "http://127.0.0.1:8080/callback", "state-abc", "challenge-xyz")

	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatal(err)
	}

	tests := map[string]string{
		"response_type":         "code",
		"client_id":             "client-123",
		"redirect_uri":          "http://127.0.0.1:8080/callback",
		"state":                 "state-abc",
		"code_challenge":        "challenge-xyz",
		"code_challenge_method": "S256",
		"prompt":                "consent",
	}
	for key, want := range tests {
		got := parsed.Query().Get(key)
		if got != want {
			t.Errorf("param %q = %q, want %q", key, got, want)
		}
	}

	// Verify scope contains required values.
	scope := parsed.Query().Get("scope")
	for _, required := range []string{"openid", "offline_access"} {
		if !strings.Contains(scope, required) {
			t.Errorf("scope missing %q, got %q", required, scope)
		}
	}
}

func TestOAuthClient_ExchangeCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.Form.Get("grant_type") != "authorization_code" {
			t.Errorf("grant_type = %q", r.Form.Get("grant_type"))
		}
		if r.Form.Get("code") != "auth-code-123" {
			t.Errorf("code = %q", r.Form.Get("code"))
		}
		if r.Form.Get("code_verifier") != "verifier-abc" {
			t.Errorf("code_verifier = %q", r.Form.Get("code_verifier"))
		}
		// Native client: client_id in body, no secret.
		if r.Form.Get("client_id") != "client-123" {
			t.Errorf("client_id = %q", r.Form.Get("client_id"))
		}

		if err := json.NewEncoder(w).Encode(auth.TokenResponse{
			AccessToken:  "new-access-token",
			RefreshToken: "new-refresh-token",
			ExpiresIn:    3600,
			TokenType:    "Bearer",
		}); err != nil {
			t.Fatal(err)
		}
	}))
	defer server.Close()

	client := &auth.OAuthClient{TokenEndpoint: server.URL}

	resp, err := client.ExchangeCode(context.Background(), "client-123", "auth-code-123", "http://127.0.0.1:8080/callback", "verifier-abc")
	if err != nil {
		t.Fatal(err)
	}
	if resp.AccessToken != "new-access-token" {
		t.Errorf("AccessToken = %q", resp.AccessToken)
	}
	if resp.RefreshToken != "new-refresh-token" {
		t.Errorf("RefreshToken = %q", resp.RefreshToken)
	}
}

func TestOAuthClient_RegisterClient_ErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	client := &auth.OAuthClient{RegistrationURL: server.URL}
	_, err := client.RegisterClient(context.Background(), "http://127.0.0.1:12345/callback")
	if err == nil {
		t.Fatal("expected error for 403 response")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error should mention status 403, got: %s", err)
	}
}

func TestOAuthClient_ExchangeCode_ErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	client := &auth.OAuthClient{TokenEndpoint: server.URL}
	_, err := client.ExchangeCode(context.Background(), "client-123", "bad-code", "http://127.0.0.1:8080/callback", "verifier")
	if err == nil {
		t.Fatal("expected error for 400 response")
	}
	if !strings.Contains(err.Error(), "400") {
		t.Errorf("error should mention status 400, got: %s", err)
	}
}

func TestOAuthClient_RefreshToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.Form.Get("grant_type") != "refresh_token" {
			t.Errorf("grant_type = %q", r.Form.Get("grant_type"))
		}
		if r.Form.Get("refresh_token") != "old-refresh" {
			t.Errorf("refresh_token = %q", r.Form.Get("refresh_token"))
		}
		if r.Form.Get("client_id") != "client-123" {
			t.Errorf("client_id = %q", r.Form.Get("client_id"))
		}

		if err := json.NewEncoder(w).Encode(auth.TokenResponse{
			AccessToken:  "refreshed-access",
			RefreshToken: "rotated-refresh",
			ExpiresIn:    3600,
		}); err != nil {
			t.Fatal(err)
		}
	}))
	defer server.Close()

	client := &auth.OAuthClient{TokenEndpoint: server.URL}

	resp, err := client.RefreshToken(t.Context(), "client-123", "old-refresh")
	if err != nil {
		t.Fatal(err)
	}
	if resp.AccessToken != "refreshed-access" {
		t.Errorf("AccessToken = %q", resp.AccessToken)
	}
	if resp.RefreshToken != "rotated-refresh" {
		t.Errorf("RefreshToken = %q", resp.RefreshToken)
	}
}

func TestOAuthClient_RefreshToken_ErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	client := &auth.OAuthClient{TokenEndpoint: server.URL}
	_, err := client.RefreshToken(t.Context(), "client-123", "expired-refresh")
	if err == nil {
		t.Fatal("expected error for 401 response")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error should mention status 401, got: %s", err)
	}
}

func TestNewOAuthClient(t *testing.T) {
	client := auth.NewOAuthClient()
	if client.AuthEndpoint == "" {
		t.Error("AuthEndpoint should be set")
	}
	if client.TokenEndpoint == "" {
		t.Error("TokenEndpoint should be set")
	}
	if client.RegistrationURL == "" {
		t.Error("RegistrationURL should be set")
	}
	if client.UserinfoURL == "" {
		t.Error("UserinfoURL should be set")
	}
	if client.HTTPClient == nil {
		t.Error("HTTPClient should be set")
	}
}

func TestOAuthClient_RegisterClient_NetworkError(t *testing.T) {
	// Use a server that's been closed to trigger a network error.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	serverURL := server.URL
	server.Close()

	client := &auth.OAuthClient{RegistrationURL: serverURL}
	_, err := client.RegisterClient(context.Background(), "http://127.0.0.1:12345/callback")
	if err == nil {
		t.Fatal("expected network error")
	}
	if !strings.Contains(err.Error(), "registration request failed") {
		t.Errorf("error should mention registration request failed, got: %s", err)
	}
}

func TestOAuthClient_RegisterClient_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("not valid json"))
	}))
	defer server.Close()

	client := &auth.OAuthClient{RegistrationURL: server.URL}
	_, err := client.RegisterClient(context.Background(), "http://127.0.0.1:12345/callback")
	if err == nil {
		t.Fatal("expected JSON decode error")
	}
	if !strings.Contains(err.Error(), "decoding registration response") {
		t.Errorf("error should mention decoding, got: %s", err)
	}
}

func TestOAuthClient_ExchangeCode_NetworkError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	serverURL := server.URL
	server.Close()

	client := &auth.OAuthClient{TokenEndpoint: serverURL}
	_, err := client.ExchangeCode(context.Background(), "client-123", "code", "http://127.0.0.1:8080/callback", "verifier")
	if err == nil {
		t.Fatal("expected network error")
	}
	if !strings.Contains(err.Error(), "token exchange request failed") {
		t.Errorf("error should mention token exchange request failed, got: %s", err)
	}
}

func TestOAuthClient_ExchangeCode_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("not valid json"))
	}))
	defer server.Close()

	client := &auth.OAuthClient{TokenEndpoint: server.URL}
	_, err := client.ExchangeCode(context.Background(), "client-123", "code", "http://127.0.0.1:8080/callback", "verifier")
	if err == nil {
		t.Fatal("expected JSON decode error")
	}
	if !strings.Contains(err.Error(), "decoding token response") {
		t.Errorf("error should mention decoding token response, got: %s", err)
	}
}

func TestOAuthClient_RefreshToken_NetworkError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	serverURL := server.URL
	server.Close()

	client := &auth.OAuthClient{TokenEndpoint: serverURL}
	_, err := client.RefreshToken(t.Context(), "client-123", "refresh-token")
	if err == nil {
		t.Fatal("expected network error")
	}
	if !strings.Contains(err.Error(), "refresh request failed") {
		t.Errorf("error should mention refresh request failed, got: %s", err)
	}
}

func TestOAuthClient_RefreshToken_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("not valid json"))
	}))
	defer server.Close()

	client := &auth.OAuthClient{TokenEndpoint: server.URL}
	_, err := client.RefreshToken(t.Context(), "client-123", "refresh-token")
	if err == nil {
		t.Fatal("expected JSON decode error")
	}
	if !strings.Contains(err.Error(), "decoding refresh response") {
		t.Errorf("error should mention decoding refresh response, got: %s", err)
	}
}
