package auth_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/auth"
)

func TestFetchUserInfo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}

		if err := json.NewEncoder(w).Encode(auth.UserInfo{
			Sub:   "user_abc123",
			Email: "test@example.com",
			Name:  "Test User",
		}); err != nil {
			t.Fatal(err)
		}
	}))
	defer server.Close()

	client := &auth.OAuthClient{
		UserinfoURL: server.URL,
		HTTPClient:  http.DefaultClient,
	}

	info, err := client.FetchUserInfo("test-token")
	if err != nil {
		t.Fatal(err)
	}
	if info.Email != "test@example.com" {
		t.Errorf("Email = %q", info.Email)
	}
	if info.Name != "Test User" {
		t.Errorf("Name = %q", info.Name)
	}
}
