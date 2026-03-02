package railway_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/auth"
	"github.com/hamishmorgan/fat-controller/internal/railway"
	"github.com/zalando/go-keyring"
)

// fakeRefresher implements railway.Refresher for testing.
type fakeRefresher struct {
	calls     atomic.Int32
	called    atomic.Bool
	returnTok *auth.TokenResponse
	returnErr error
}

func (f *fakeRefresher) Refresh(_ context.Context, clientID, refreshToken string) (*auth.TokenResponse, error) {
	f.calls.Add(1)
	f.called.Store(true)
	return f.returnTok, f.returnErr
}

func TestAuthTransport_InjectsHeader(t *testing.T) {
	var gotHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	resolved := &auth.ResolvedAuth{
		Token:       "test-token",
		HeaderName:  "Authorization",
		HeaderValue: "Bearer test-token",
		Source:      auth.SourceFlag,
	}
	transport := railway.NewAuthTransport(resolved, nil, nil)
	client := &http.Client{Transport: transport}

	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if gotHeader != "Bearer test-token" {
		t.Errorf("Authorization header = %q, want %q", gotHeader, "Bearer test-token")
	}
}

func TestAuthTransport_ProjectAccessTokenHeader(t *testing.T) {
	var gotHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("Project-Access-Token")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	resolved := &auth.ResolvedAuth{
		Token:       "proj-token",
		HeaderName:  "Project-Access-Token",
		HeaderValue: "proj-token",
		Source:      auth.SourceEnvToken,
	}
	transport := railway.NewAuthTransport(resolved, nil, nil)
	client := &http.Client{Transport: transport}

	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if gotHeader != "proj-token" {
		t.Errorf("Project-Access-Token header = %q, want %q", gotHeader, "proj-token")
	}
}

func TestAuthTransport_NoRefreshForNonStoredTokens(t *testing.T) {
	// First request returns 401. Transport should NOT attempt refresh
	// because the token source is "flag", not "stored".
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	resolved := &auth.ResolvedAuth{
		Token:       "flag-token",
		HeaderName:  "Authorization",
		HeaderValue: "Bearer flag-token",
		Source:      auth.SourceFlag,
	}
	transport := railway.NewAuthTransport(resolved, nil, nil)
	client := &http.Client{Transport: transport}

	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close() //nolint:errcheck

	// Should return 401 directly — no refresh attempted.
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("StatusCode = %d, want 401", resp.StatusCode)
	}
}

func TestAuthTransport_RefreshesOnUnauthorized(t *testing.T) {
	keyring.MockInit()

	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := requestCount.Add(1)
		if n == 1 {
			// First request: return 401 (token expired).
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		// Second request (after refresh): check new token and return 200.
		if got := r.Header.Get("Authorization"); got != "Bearer refreshed-token" {
			t.Errorf("retry Authorization = %q, want %q", got, "Bearer refreshed-token")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	store := auth.NewTokenStore(
		auth.WithKeyringService("test-refresh"),
		auth.WithFallbackPath(filepath.Join(t.TempDir(), "auth.json")),
	)
	// Pre-populate stored tokens so the transport can load them for refresh.
	if err := store.Save(&auth.StoredTokens{
		AccessToken:  "expired-token",
		RefreshToken: "valid-refresh-token",
		ClientID:     "client-123",
	}); err != nil {
		t.Fatal(err)
	}

	resolved := &auth.ResolvedAuth{
		Token:       "expired-token",
		HeaderName:  "Authorization",
		HeaderValue: "Bearer expired-token",
		Source:      auth.SourceStored,
	}

	refresher := &fakeRefresher{
		returnTok: &auth.TokenResponse{
			AccessToken:  "refreshed-token",
			RefreshToken: "new-refresh-token",
		},
	}

	transport := railway.NewAuthTransport(resolved, store, refresher)
	client := &http.Client{Transport: transport}

	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want 200", resp.StatusCode)
	}
	if !refresher.called.Load() {
		t.Error("refresher should have been called")
	}
	if requestCount.Load() != 2 {
		t.Errorf("request count = %d, want 2", requestCount.Load())
	}

	// Verify new tokens were saved.
	saved, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if saved.AccessToken != "refreshed-token" {
		t.Errorf("saved AccessToken = %q, want %q", saved.AccessToken, "refreshed-token")
	}
	if saved.RefreshToken != "new-refresh-token" {
		t.Errorf("saved RefreshToken = %q, want %q", saved.RefreshToken, "new-refresh-token")
	}
}

func TestAuthTransport_ConcurrentRefresh(t *testing.T) {
	keyring.MockInit()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got == "Bearer refreshed-token" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	store := auth.NewTokenStore(
		auth.WithKeyringService("test-refresh-concurrent"),
		auth.WithFallbackPath(filepath.Join(t.TempDir(), "auth.json")),
	)
	if err := store.Save(&auth.StoredTokens{
		AccessToken:  "expired-token",
		RefreshToken: "valid-refresh-token",
		ClientID:     "client-123",
	}); err != nil {
		t.Fatal(err)
	}

	resolved := &auth.ResolvedAuth{
		Token:       "expired-token",
		HeaderName:  "Authorization",
		HeaderValue: "Bearer expired-token",
		Source:      auth.SourceStored,
	}

	refresher := &fakeRefresher{
		returnTok: &auth.TokenResponse{
			AccessToken:  "refreshed-token",
			RefreshToken: "new-refresh-token",
		},
	}

	transport := railway.NewAuthTransport(resolved, store, refresher)
	client := &http.Client{Transport: transport}

	const workers = 5
	errCh := make(chan error, workers)
	for i := 0; i < workers; i++ {
		go func() {
			resp, err := client.Get(server.URL)
			if err != nil {
				errCh <- err
				return
			}
			defer resp.Body.Close() //nolint:errcheck
			if resp.StatusCode != http.StatusOK {
				errCh <- fmt.Errorf("StatusCode = %d, want 200", resp.StatusCode)
				return
			}
			errCh <- nil
		}()
	}

	for i := 0; i < workers; i++ {
		if err := <-errCh; err != nil {
			t.Fatal(err)
		}
	}

	if refresher.calls.Load() != 1 {
		t.Errorf("refresh calls = %d, want 1", refresher.calls.Load())
	}
}

func TestAuthTransport_RefreshFailsReturnsOriginal401(t *testing.T) {
	keyring.MockInit()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	store := auth.NewTokenStore(
		auth.WithKeyringService("test-refresh-fail"),
		auth.WithFallbackPath(filepath.Join(t.TempDir(), "auth.json")),
	)
	if err := store.Save(&auth.StoredTokens{
		AccessToken:  "expired-token",
		RefreshToken: "bad-refresh-token",
		ClientID:     "client-123",
	}); err != nil {
		t.Fatal(err)
	}

	resolved := &auth.ResolvedAuth{
		Token:       "expired-token",
		HeaderName:  "Authorization",
		HeaderValue: "Bearer expired-token",
		Source:      auth.SourceStored,
	}

	refresher := &fakeRefresher{
		returnErr: fmt.Errorf("refresh token revoked"),
	}

	transport := railway.NewAuthTransport(resolved, store, refresher)
	client := &http.Client{Transport: transport}

	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close() //nolint:errcheck

	// Should return original 401 since refresh failed.
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("StatusCode = %d, want 401", resp.StatusCode)
	}
}
