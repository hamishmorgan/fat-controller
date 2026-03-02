package cli

import (
	"fmt"

	"github.com/hamishmorgan/fat-controller/internal/auth"
	"github.com/hamishmorgan/fat-controller/internal/platform"
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
	case "env:RAILWAY_TOKEN":
		fmt.Println("Using RAILWAY_TOKEN environment variable (project access token).")
		return nil
	case "env:RAILWAY_API_TOKEN":
		fmt.Println("Using RAILWAY_API_TOKEN environment variable (account/workspace token).")
		return nil
	case "flag":
		fmt.Println("Using --token flag.")
		return nil
	}

	// For stored OAuth tokens, fetch user info.
	// Note: if the access token is expired (>1hr), this will fail with a 401.
	// M2 will add a refresh-aware HTTP client that handles this transparently.
	// For now, we show a helpful message.
	oauth := auth.NewOAuthClient()
	info, err := oauth.FetchUserInfo(resolved.Token)
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
