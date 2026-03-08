package cli

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"

	"github.com/hamishmorgan/fat-controller/internal/auth"
	"github.com/hamishmorgan/fat-controller/internal/platform"
	"github.com/hamishmorgan/fat-controller/internal/prompt"
	"github.com/hamishmorgan/fat-controller/internal/railway"
)

func (c *AuthLoginCmd) Run(globals *Globals) error {
	ctx, cancel := c.TimeoutContext(globals.BaseCtx)
	defer cancel()
	return RunAuthLogin(ctx, globals, os.Stdout)
}

// RunAuthLogin is the testable core of `auth login`.
func RunAuthLogin(ctx context.Context, globals *Globals, out io.Writer) error {
	slog.Debug("starting auth login")
	oauth := auth.NewOAuthClient()
	store := auth.NewTokenStore(
		auth.WithFallbackPath(platform.AuthFilePath()),
	)
	return auth.Login(ctx, oauth, store, auth.OpenBrowser, out)
}

func (c *AuthLogoutCmd) Run(globals *Globals) error {
	interactive := prompt.StdinIsInteractive()
	return RunAuthLogout(interactive, c.Yes, os.Stdout)
}

// RunAuthLogout is the testable core of `auth logout`.
func RunAuthLogout(interactive, yes bool, out io.Writer) error {
	slog.Debug("starting auth logout")
	store := auth.NewTokenStore(
		auth.WithFallbackPath(platform.AuthFilePath()),
	)

	// Confirm before deleting credentials.
	if !yes {
		if !interactive {
			return fmt.Errorf("refusing to log out in non-interactive mode without --yes")
		}
		confirmed, err := prompt.Confirm("Clear stored credentials?", false)
		if err != nil {
			return fmt.Errorf("reading confirmation: %w", err)
		}
		if !confirmed {
			_, _ = fmt.Fprintln(out, "Logout cancelled.")
			return nil
		}
	}

	// TODO: revoke token when Railway supports RFC 7009 (OAuth token revocation).
	if err := store.Delete(); err != nil {
		return fmt.Errorf("clearing credentials: %w", err)
	}
	_, _ = fmt.Fprintln(out, "Logged out successfully.")
	return nil
}

func (c *AuthStatusCmd) Run(globals *Globals) error {
	ctx, cancel := c.TimeoutContext(globals.BaseCtx)
	defer cancel()
	return RunAuthStatus(ctx, c.Token, globals, os.Stdout)
}

// RunAuthStatus is the testable core of `auth status`.
func RunAuthStatus(ctx context.Context, token string, globals *Globals, out io.Writer) error {
	slog.Debug("checking auth status")
	store := auth.NewTokenStore(
		auth.WithFallbackPath(platform.AuthFilePath()),
	)

	resolved, err := auth.ResolveAuth(ctx, token, store)
	if err != nil {
		if isStructuredOutput(globals) {
			payload := struct {
				Authenticated bool   `json:"authenticated" toml:"authenticated"`
				Error         string `json:"error" toml:"error"`
			}{Authenticated: false, Error: err.Error()}
			return writeStructured(out, globals.Output, payload)
		}
		_, _ = fmt.Fprintln(out, "Not authenticated.")
		_, _ = fmt.Fprintln(out, "Run 'fat-controller auth login' or set RAILWAY_TOKEN.")
		return nil
	}

	if isStructuredOutput(globals) {
		payload := struct {
			Authenticated bool   `json:"authenticated" toml:"authenticated"`
			Source        string `json:"source" toml:"source"`
			UserName      string `json:"user_name,omitempty" toml:"user_name"`
			UserEmail     string `json:"user_email,omitempty" toml:"user_email"`
			UserInfoError string `json:"user_info_error,omitempty" toml:"user_info_error"`
		}{Authenticated: true, Source: resolved.Source}

		// For stored OAuth, try to fetch user info (best-effort).
		if resolved.Source == auth.SourceStored {
			refreshOAuth := auth.NewOAuthClient()
			refresher := railway.NewOAuthRefresher(refreshOAuth)
			transport := railway.NewAuthTransport(resolved, store, refresher)
			oauthClient := auth.NewOAuthClient()
			oauthClient.HTTPClient = &http.Client{Transport: transport}
			info, err := oauthClient.FetchUserInfo(ctx)
			if err != nil {
				payload.UserInfoError = err.Error()
			} else {
				payload.UserName = info.Name
				payload.UserEmail = info.Email
			}
		}

		return writeStructured(out, globals.Output, payload)
	}

	_, _ = fmt.Fprintf(out, "Authenticated via: %s\n", resolved.Source)

	switch resolved.Source {
	case auth.SourceEnvToken:
		_, _ = fmt.Fprintln(out, "Using RAILWAY_TOKEN environment variable (project access token).")
		return nil
	case auth.SourceEnvAPIToken:
		_, _ = fmt.Fprintln(out, "Using RAILWAY_API_TOKEN environment variable (account/workspace token).")
		return nil
	case auth.SourceFlag:
		_, _ = fmt.Fprintln(out, "Using --token flag.")
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

	slog.Debug("fetching user info via refresh-aware transport")
	info, err := oauth.FetchUserInfo(ctx)
	if err != nil {
		_, _ = fmt.Fprintln(out, "Authenticated (stored OAuth token).")
		_, _ = fmt.Fprintf(out, "Could not fetch user info: %v\n", err)
		_, _ = fmt.Fprintln(out, "If your session expired, run 'fat-controller auth login' to re-authenticate.")
		return nil
	}

	_, _ = fmt.Fprintf(out, "User: %s\n", info.Name)
	_, _ = fmt.Fprintf(out, "Email: %s\n", info.Email)
	return nil
}
