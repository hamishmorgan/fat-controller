package railway

import (
	"net/http"
	"sync"

	"github.com/hamishmorgan/fat-controller/internal/auth"
)

// Refresher abstracts token refresh so transport doesn't depend on OAuthClient directly.
type Refresher interface {
	Refresh(clientID, refreshToken string) (*auth.TokenResponse, error)
}

// AuthTransport is an http.RoundTripper that injects auth headers and
// transparently refreshes expired OAuth tokens.
type AuthTransport struct {
	mu       sync.Mutex
	resolved *auth.ResolvedAuth
	store    *auth.TokenStore
	refresh  Refresher
	base     http.RoundTripper
}

// NewAuthTransport creates a transport that injects auth headers from resolved.
// If store and refresh are non-nil AND the token source is "stored", the
// transport will attempt a token refresh on 401 responses.
func NewAuthTransport(resolved *auth.ResolvedAuth, store *auth.TokenStore, refresh Refresher) *AuthTransport {
	return &AuthTransport{
		resolved: resolved,
		store:    store,
		refresh:  refresh,
		base:     http.DefaultTransport,
	}
}

// RoundTrip implements http.RoundTripper.
func (t *AuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.mu.Lock()
	headerName := t.resolved.HeaderName
	headerValue := t.resolved.HeaderValue
	t.mu.Unlock()

	clone := req.Clone(req.Context())
	clone.Header.Set(headerName, headerValue)

	resp, err := t.base.RoundTrip(clone)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusUnauthorized && t.canRefresh() {
		newTokens, refreshErr := t.tryRefresh()
		if refreshErr != nil {
			return resp, nil
		}

		resp.Body.Close() //nolint:errcheck

		t.mu.Lock()
		t.resolved.Token = newTokens.AccessToken
		t.resolved.HeaderValue = "Bearer " + newTokens.AccessToken
		headerValue = t.resolved.HeaderValue
		t.mu.Unlock()

		retry := req.Clone(req.Context())
		retry.Header.Set(headerName, headerValue)
		return t.base.RoundTrip(retry)
	}

	return resp, nil
}

func (t *AuthTransport) canRefresh() bool {
	return t.resolved.Source == "stored" && t.refresh != nil && t.store != nil
}

func (t *AuthTransport) tryRefresh() (*auth.TokenResponse, error) {
	stored, err := t.store.Load()
	if err != nil {
		return nil, err
	}

	newTokens, err := t.refresh.Refresh(stored.ClientID, stored.RefreshToken)
	if err != nil {
		return nil, err
	}

	// Railway rotates refresh tokens — save the new pair.
	if err := t.store.Save(&auth.StoredTokens{
		AccessToken:  newTokens.AccessToken,
		RefreshToken: newTokens.RefreshToken,
		ClientID:     stored.ClientID,
	}); err != nil {
		return nil, err
	}

	return newTokens, nil
}
