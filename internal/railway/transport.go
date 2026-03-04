package railway

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"github.com/hamishmorgan/fat-controller/internal/auth"
)

// Refresher abstracts token refresh so transport doesn't depend on OAuthClient directly.
type Refresher interface {
	Refresh(ctx context.Context, clientID, refreshToken string) (*auth.TokenResponse, error)
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
	source := t.resolved.Source
	t.mu.Unlock()

	clone := req.Clone(req.Context())
	clone.Header.Set(headerName, headerValue)

	resp, err := t.base.RoundTrip(clone)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusUnauthorized {
		return resp, nil
	}

	if !t.canRefresh(source) {
		return resp, nil
	}

	// Serialize refresh attempts — only one goroutine refreshes at a time.
	// If another goroutine already refreshed, we'll pick up the new token.
	t.mu.Lock()
	defer t.mu.Unlock()

	// Re-check: another goroutine may have refreshed while we waited.
	if t.resolved.HeaderValue != headerValue {
		// Token was already refreshed — retry with the new value.
		resp.Body.Close() //nolint:errcheck
		retry := req.Clone(req.Context())
		retry.Header.Set(headerName, t.resolved.HeaderValue)
		return t.base.RoundTrip(retry)
	}

	newTokens, refreshErr := t.tryRefresh(req.Context())
	if refreshErr != nil {
		resp.Body.Close() //nolint:errcheck
		return nil, fmt.Errorf("authentication failed (token refresh error: %w)", refreshErr)
	}

	resp.Body.Close() //nolint:errcheck

	t.resolved.SetToken(newTokens.AccessToken)

	retry := req.Clone(req.Context())
	retry.Header.Set(headerName, t.resolved.HeaderValue)
	return t.base.RoundTrip(retry)
}

func (t *AuthTransport) canRefresh(source string) bool {
	return source == auth.SourceStored && t.refresh != nil && t.store != nil
}

func (t *AuthTransport) tryRefresh(ctx context.Context) (*auth.TokenResponse, error) {
	stored, err := t.store.Load()
	if err != nil {
		return nil, err
	}

	newTokens, err := t.refresh.Refresh(ctx, stored.ClientID, stored.RefreshToken)
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
