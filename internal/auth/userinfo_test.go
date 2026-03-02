package auth_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/auth"
)

// roundTripFunc lets us build a one-off RoundTripper from a function.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

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

	// Inject auth via a simple transport that sets the Authorization header.
	client := &auth.OAuthClient{
		UserinfoURL: server.URL,
		HTTPClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				req.Header.Set("Authorization", "Bearer test-token")
				return http.DefaultTransport.RoundTrip(req)
			}),
		},
	}

	info, err := client.FetchUserInfo(t.Context())
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

func TestFetchUserInfo_Unauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	client := &auth.OAuthClient{
		UserinfoURL: server.URL,
		HTTPClient:  http.DefaultClient,
	}

	_, err := client.FetchUserInfo(t.Context())
	if err == nil {
		t.Fatal("expected error for 401 response")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error should mention status 401, got: %s", err)
	}
}

func TestFetchUserInfo_NetworkError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	serverURL := server.URL
	server.Close()

	client := &auth.OAuthClient{
		UserinfoURL: serverURL,
		HTTPClient:  http.DefaultClient,
	}

	_, err := client.FetchUserInfo(t.Context())
	if err == nil {
		t.Fatal("expected network error")
	}
	if !strings.Contains(err.Error(), "userinfo request failed") {
		t.Errorf("error should mention userinfo request failed, got: %s", err)
	}
}

func TestFetchUserInfo_InvalidURL(t *testing.T) {
	client := &auth.OAuthClient{
		UserinfoURL: "://invalid-url",
		HTTPClient:  http.DefaultClient,
	}

	_, err := client.FetchUserInfo(t.Context())
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
}

func TestFetchUserInfo_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("not valid json"))
	}))
	defer server.Close()

	client := &auth.OAuthClient{
		UserinfoURL: server.URL,
		HTTPClient:  http.DefaultClient,
	}

	_, err := client.FetchUserInfo(t.Context())
	if err == nil {
		t.Fatal("expected JSON decode error")
	}
	if !strings.Contains(err.Error(), "decoding userinfo") {
		t.Errorf("error should mention decoding userinfo, got: %s", err)
	}
}
