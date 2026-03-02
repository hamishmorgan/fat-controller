package cli

import (
	"fmt"
	"net/http"

	"github.com/hamishmorgan/fat-controller/internal/auth"
	"github.com/hamishmorgan/fat-controller/internal/platform"
	"github.com/hamishmorgan/fat-controller/internal/railway"
)

func (c *AuthLoginCmd) Run(globals *Globals) error {
	oauth := auth.NewOAuthClient()
	store := auth.NewTokenStore(
		auth.WithFallbackPath(platform.AuthFilePath()),
	)
	return auth.Login(oauth, store, auth.OpenBrowser)
}

func (c *AuthLogoutCmd) Run(globals *Globals) error {
	store := auth.NewTokenStore(
		auth.WithFallbackPath(platform.AuthFilePath()),
	)
	if err := store.Delete(); err != nil {
		return fmt.Errorf("clearing credentials: %w", err)
	}
	fmt.Println("Logged out successfully.")
	return nil
}

func (c *AuthStatusCmd) Run(globals *Globals) error {
	store := auth.NewTokenStore(
		auth.WithFallbackPath(platform.AuthFilePath()),
	)

	resolved, err := auth.ResolveAuth(globals.Token, store)
	if err != nil {
		fmt.Println("Not authenticated.")
		fmt.Println("Run 'fat-controller auth login' or set RAILWAY_TOKEN.")
		return nil
	}

	fmt.Printf("Authenticated via: %s\n", resolved.Source)

	switch resolved.Source {
	case auth.SourceEnvToken:
		fmt.Println("Using RAILWAY_TOKEN environment variable (project access token).")
		return nil
	case auth.SourceEnvAPIToken:
		fmt.Println("Using RAILWAY_API_TOKEN environment variable (account/workspace token).")
		return nil
	case auth.SourceFlag:
		fmt.Println("Using --token flag.")
		return nil
	}

	// For stored OAuth tokens, use the refresh-aware transport so
	// expired tokens get refreshed transparently on 401.
	//
	// The refresher uses a separate OAuthClient with the default HTTP client
	// so that token refresh requests don't go through AuthTransport (which
	// would cause infinite recursion if the token endpoint also returned 401).
	refreshOAuth := auth.NewOAuthClient()
	refresher := railway.NewOAuthRefresher(refreshOAuth)
	transport := railway.NewAuthTransport(resolved, store, refresher)

	oauth := auth.NewOAuthClient()
	oauth.HTTPClient = &http.Client{Transport: transport}

	info, err := oauth.FetchUserInfo()
	if err != nil {
		fmt.Println("Authenticated (stored OAuth token).")
		fmt.Printf("Could not fetch user info: %v\n", err)
		fmt.Println("If your session expired, run 'fat-controller auth login' to re-authenticate.")
		return nil
	}

	fmt.Printf("User: %s\n", info.Name)
	fmt.Printf("Email: %s\n", info.Email)
	return nil
}
