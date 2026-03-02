package auth

import (
	"errors"
	"fmt"
	"os"
)

// ErrNotAuthenticated is returned when no token is available from any source.
var ErrNotAuthenticated = errors.New("not authenticated: run 'fat-controller auth login' or set RAILWAY_TOKEN")

// ResolvedAuth contains the resolved token and the HTTP header to use.
type ResolvedAuth struct {
	Token       string
	HeaderName  string
	HeaderValue string
	Source      string // "flag", "env:RAILWAY_API_TOKEN", "env:RAILWAY_TOKEN", "stored"
}

// ResolveAuth determines the active auth token using the precedence:
//  1. flagToken (from --token flag)
//  2. RAILWAY_API_TOKEN env var (account/workspace-scoped)
//  3. RAILWAY_TOKEN env var (project-scoped)
//  4. Stored OAuth token (from keyring or file)
func ResolveAuth(flagToken string, store *TokenStore) (*ResolvedAuth, error) {
	// 1. --token flag
	if flagToken != "" {
		return &ResolvedAuth{
			Token:       flagToken,
			HeaderName:  "Authorization",
			HeaderValue: "Bearer " + flagToken,
			Source:      "flag",
		}, nil
	}

	// 2. RAILWAY_API_TOKEN env var
	if token := os.Getenv("RAILWAY_API_TOKEN"); token != "" {
		return &ResolvedAuth{
			Token:       token,
			HeaderName:  "Authorization",
			HeaderValue: "Bearer " + token,
			Source:      "env:RAILWAY_API_TOKEN",
		}, nil
	}

	// 3. RAILWAY_TOKEN env var (project-scoped)
	if token := os.Getenv("RAILWAY_TOKEN"); token != "" {
		return &ResolvedAuth{
			Token:       token,
			HeaderName:  "Project-Access-Token",
			HeaderValue: token,
			Source:      "env:RAILWAY_TOKEN",
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

	return &ResolvedAuth{
		Token:       tokens.AccessToken,
		HeaderName:  "Authorization",
		HeaderValue: "Bearer " + tokens.AccessToken,
		Source:      "stored",
	}, nil
}
