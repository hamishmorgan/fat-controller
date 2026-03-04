package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
)

// ErrNotAuthenticated is returned when no token is available from any source.
var ErrNotAuthenticated = errors.New("not authenticated: run 'fat-controller auth login' or set RAILWAY_TOKEN")

// Source constants identify how the active token was resolved.
const (
	SourceFlag        = "flag"
	SourceEnvAPIToken = "env:RAILWAY_API_TOKEN"
	SourceEnvToken    = "env:RAILWAY_TOKEN"
	SourceStored      = "stored"
)

// ResolvedAuth contains the resolved token and the HTTP header to use.
type ResolvedAuth struct {
	mu          sync.Mutex
	Token       string
	HeaderName  string
	HeaderValue string
	Source      string // One of the Source* constants.
}

// GetToken returns the current token in a thread-safe manner.
func (r *ResolvedAuth) GetToken() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.Token
}

// SetToken updates the token and header value in a thread-safe manner.
func (r *ResolvedAuth) SetToken(token string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Token = token
	r.HeaderValue = "Bearer " + token
}

// ResolveAuth determines the active auth token using the precedence:
//  1. flagToken (from --token flag)
//  2. RAILWAY_API_TOKEN env var (account/workspace-scoped)
//  3. RAILWAY_TOKEN env var (project-scoped)
//  4. Stored OAuth token (from keyring or file)
func ResolveAuth(ctx context.Context, flagToken string, store *TokenStore) (*ResolvedAuth, error) {
	// 1. --token flag
	if flagToken != "" {
		slog.Debug("auth resolved", "source", SourceFlag)
		return &ResolvedAuth{
			Token:       flagToken,
			HeaderName:  "Authorization",
			HeaderValue: "Bearer " + flagToken,
			Source:      SourceFlag,
		}, nil
	}

	// 2. RAILWAY_API_TOKEN env var
	if token := os.Getenv("RAILWAY_API_TOKEN"); token != "" {
		slog.Debug("auth resolved", "source", SourceEnvAPIToken)
		return &ResolvedAuth{
			Token:       token,
			HeaderName:  "Authorization",
			HeaderValue: "Bearer " + token,
			Source:      SourceEnvAPIToken,
		}, nil
	}

	// 3. RAILWAY_TOKEN env var (project-scoped)
	if token := os.Getenv("RAILWAY_TOKEN"); token != "" {
		slog.Debug("auth resolved", "source", SourceEnvToken)
		return &ResolvedAuth{
			Token:       token,
			HeaderName:  "Project-Access-Token",
			HeaderValue: token,
			Source:      SourceEnvToken,
		}, nil
	}

	// 4. Stored OAuth token
	tokens, err := store.Load()
	if err != nil {
		if errors.Is(err, ErrNoStoredTokens) {
			return nil, ErrNotAuthenticated
		}
		return nil, fmt.Errorf("loading stored tokens: %w", err)
	}

	if tokens.AccessToken == "" {
		return nil, ErrNotAuthenticated
	}

	slog.Debug("auth resolved", "source", SourceStored)
	return &ResolvedAuth{
		Token:       tokens.AccessToken,
		HeaderName:  "Authorization",
		HeaderValue: "Bearer " + tokens.AccessToken,
		Source:      SourceStored,
	}, nil
}
