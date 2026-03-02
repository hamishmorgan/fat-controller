package railway_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/auth"
	"github.com/hamishmorgan/fat-controller/internal/railway"
)

func TestOAuthRefresher_Refresh(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.Form.Get("grant_type") != "refresh_token" {
			t.Errorf("grant_type = %q", r.Form.Get("grant_type"))
		}
		if r.Form.Get("client_id") != "client-abc" {
			t.Errorf("client_id = %q", r.Form.Get("client_id"))
		}
		if r.Form.Get("refresh_token") != "refresh-xyz" {
			t.Errorf("refresh_token = %q", r.Form.Get("refresh_token"))
		}
		if err := json.NewEncoder(w).Encode(auth.TokenResponse{
			AccessToken:  "new-access",
			RefreshToken: "new-refresh",
			ExpiresIn:    3600,
		}); err != nil {
			t.Fatal(err)
		}
	}))
	defer server.Close()

	oauth := &auth.OAuthClient{
		TokenEndpoint: server.URL,
	}
	refresher := railway.NewOAuthRefresher(oauth)

	tok, err := refresher.Refresh(t.Context(), "client-abc", "refresh-xyz")
	if err != nil {
		t.Fatal(err)
	}
	if tok.AccessToken != "new-access" {
		t.Errorf("AccessToken = %q", tok.AccessToken)
	}
	if tok.RefreshToken != "new-refresh" {
		t.Errorf("RefreshToken = %q", tok.RefreshToken)
	}
}

func TestOAuthRefresher_RefreshError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	oauth := &auth.OAuthClient{
		TokenEndpoint: server.URL,
	}
	refresher := railway.NewOAuthRefresher(oauth)

	_, err := refresher.Refresh(t.Context(), "client-abc", "bad-refresh")
	if err == nil {
		t.Fatal("expected error for 400 response")
	}
	if !strings.Contains(err.Error(), "400") {
		t.Errorf("error should mention status 400, got: %s", err)
	}
}
