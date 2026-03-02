package railway_test

import (
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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer my-flag-token" {
			t.Errorf("Authorization = %q, want %q", got, "Bearer my-flag-token")
		}
		w.Header().Set("Content-Type", "application/json")
		// Return a valid GraphQL response.
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

	keyring.MockInit()

	resolved := &auth.ResolvedAuth{
		Token:       "my-flag-token",
		HeaderName:  "Authorization",
		HeaderValue: "Bearer my-flag-token",
		Source:      "flag",
	}
	store := auth.NewTokenStore(
		auth.WithKeyringService("test-client"),
		auth.WithFallbackPath(filepath.Join(t.TempDir(), "auth.json")),
	)
	oauth := &auth.OAuthClient{TokenEndpoint: "http://unused"}

	client := railway.NewClient(server.URL, resolved, store, oauth)

	if client == nil {
		t.Fatal("NewClient returned nil")
	}
}
