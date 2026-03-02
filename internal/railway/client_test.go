package railway_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/auth"
	"github.com/hamishmorgan/fat-controller/internal/railway"
	"github.com/zalando/go-keyring"
)

func TestNewClient_WithFlagToken(t *testing.T) {
	keyring.MockInit()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer my-flag-token" {
			t.Errorf("Authorization = %q, want %q", got, "Bearer my-flag-token")
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"projectToken": map[string]interface{}{
					"projectId":     "proj-123",
					"environmentId": "env-456",
				},
			},
		}); err != nil {
			t.Fatal(err)
		}
	}))
	defer server.Close()

	resolved := &auth.ResolvedAuth{
		Token:       "my-flag-token",
		HeaderName:  "Authorization",
		HeaderValue: "Bearer my-flag-token",
		Source:      auth.SourceFlag,
	}
	store := auth.NewTokenStore(
		auth.WithKeyringService("test-client"),
		auth.WithFallbackPath(filepath.Join(t.TempDir(), "auth.json")),
	)
	oauth := &auth.OAuthClient{TokenEndpoint: "http://unused"}

	client := railway.NewClient(server.URL, resolved, store, oauth)

	// Exercise the client to verify auth header is injected.
	resp, err := railway.ProjectToken(context.Background(), client.GQL())
	if err != nil {
		t.Fatalf("ProjectToken() error: %v", err)
	}
	if resp.ProjectToken.ProjectId != "proj-123" {
		t.Errorf("ProjectId = %q, want %q", resp.ProjectToken.ProjectId, "proj-123")
	}
}

func TestNewClient_NilOAuth(t *testing.T) {
	keyring.MockInit()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"projectToken": map[string]interface{}{
					"projectId":     "proj-123",
					"environmentId": "env-456",
				},
			},
		}); err != nil {
			t.Fatal(err)
		}
	}))
	defer server.Close()

	resolved := &auth.ResolvedAuth{
		Token:       "test-token",
		HeaderName:  "Authorization",
		HeaderValue: "Bearer test-token",
		Source:      auth.SourceFlag,
	}

	// nil oauth should not panic.
	client := railway.NewClient(server.URL, resolved, nil, nil)
	if client == nil {
		t.Fatal("NewClient returned nil")
	}

	resp, err := railway.ProjectToken(context.Background(), client.GQL())
	if err != nil {
		t.Fatalf("ProjectToken() error: %v", err)
	}
	if resp.ProjectToken.ProjectId != "proj-123" {
		t.Errorf("ProjectId = %q, want %q", resp.ProjectToken.ProjectId, "proj-123")
	}
}

func TestClient_ProjectToken_EndToEnd(t *testing.T) {
	keyring.MockInit()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify auth header is present.
		if r.Header.Get("Authorization") == "" {
			t.Error("missing Authorization header")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		// Verify it's a POST to the GraphQL endpoint.
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}

		// Return a valid projectToken response.
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"projectToken": map[string]interface{}{
					"projectId":     "proj-abc-123",
					"environmentId": "env-def-456",
				},
			},
		}); err != nil {
			t.Fatal(err)
		}
	}))
	defer server.Close()

	resolved := &auth.ResolvedAuth{
		Token:       "test-token",
		HeaderName:  "Authorization",
		HeaderValue: "Bearer test-token",
		Source:      auth.SourceFlag,
	}
	store := auth.NewTokenStore(
		auth.WithKeyringService("test-e2e"),
		auth.WithFallbackPath(filepath.Join(t.TempDir(), "auth.json")),
	)
	oauth := &auth.OAuthClient{TokenEndpoint: "http://unused"}

	client := railway.NewClient(server.URL, resolved, store, oauth)

	resp, err := railway.ProjectToken(context.Background(), client.GQL())
	if err != nil {
		t.Fatalf("ProjectToken() error: %v", err)
	}

	if resp.ProjectToken.ProjectId != "proj-abc-123" {
		t.Errorf("ProjectId = %q, want %q", resp.ProjectToken.ProjectId, "proj-abc-123")
	}
	if resp.ProjectToken.EnvironmentId != "env-def-456" {
		t.Errorf("EnvironmentId = %q, want %q", resp.ProjectToken.EnvironmentId, "env-def-456")
	}
}
