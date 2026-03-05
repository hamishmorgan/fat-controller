package auth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"runtime"
)

// errCodeExchange is a sentinel wrapper used to identify token exchange
// failures, which may indicate a stale client ID and warrant a retry.
var errCodeExchange = errors.New("code exchange failed")

// BrowserOpener is a function that opens a URL in the user's browser.
// Injected so tests can simulate the browser redirect without a real browser.
type BrowserOpener func(url string) error

// OpenBrowser opens the given URL in the user's default browser.
// This is the production BrowserOpener.
func OpenBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default: // linux, freebsd, etc.
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait() //nolint:errcheck
	return nil
}

// Login performs the full OAuth login flow:
//  1. Start callback server
//  2. Register client if no client ID stored
//  3. Generate PKCE verifier + state
//  4. Open browser to authorization URL
//  5. Wait for callback
//  6. Exchange code for tokens
//  7. Store tokens
//
// If the token exchange fails and we were using a stored client ID, Login
// retries once with a freshly registered client (handles revoked client IDs).
//
// The openBrowser parameter controls how the authorization URL is opened.
// Pass OpenBrowser for production use, or a fake for testing.
func Login(ctx context.Context, oauth *OAuthClient, store *TokenStore, openBrowser BrowserOpener, out io.Writer) error {
	err := loginAttempt(ctx, oauth, store, openBrowser, false, out)
	if err == nil {
		return nil
	}

	// Only retry with a fresh client registration if the token exchange
	// itself failed — that's the symptom of a stale/revoked client ID.
	// Other errors (user denied, CSRF mismatch, network, etc.) should
	// propagate immediately.
	if !errors.Is(err, errCodeExchange) {
		return err
	}

	slog.Debug("retrying login with fresh client registration")
	fmt.Fprintln(out, "Token exchange failed; retrying with fresh client registration...")
	return loginAttempt(ctx, oauth, store, openBrowser, true, out)
}

func loginAttempt(ctx context.Context, oauth *OAuthClient, store *TokenStore, openBrowser BrowserOpener, forceNewClient bool, out io.Writer) error {
	slog.Debug("starting login attempt", "force_new_client", forceNewClient)
	// Start callback server.
	srv, err := StartCallbackServer()
	if err != nil {
		return fmt.Errorf("starting callback server: %w", err)
	}
	defer srv.Shutdown()

	redirectURI := srv.RedirectURI()
	slog.Debug("callback server started", "port", srv.Port, "redirect_uri", redirectURI)

	// Check for existing client registration.
	clientID, err := loadOrRegisterClient(ctx, oauth, store, redirectURI, forceNewClient)
	if err != nil {
		return fmt.Errorf("client registration: %w", err)
	}

	// Generate PKCE.
	verifier, err := GenerateCodeVerifier()
	if err != nil {
		return fmt.Errorf("generating code verifier: %w", err)
	}
	challenge := CodeChallenge(verifier)

	state, err := GenerateState()
	if err != nil {
		return fmt.Errorf("generating state: %w", err)
	}

	// Build authorization URL and open browser.
	authURL := oauth.AuthorizationURL(clientID, redirectURI, state, challenge)
	fmt.Fprintln(out, "Opening browser to log in...")
	fmt.Fprintf(out, "If the browser doesn't open, visit:\n%s\n\n", authURL)

	if err := openBrowser(authURL); err != nil {
		// Non-fatal — user can copy the URL.
		fmt.Fprintf(out, "Could not open browser: %v\n", err)
	}

	// Wait for callback.
	fmt.Fprintln(out, "Waiting for authorization...")
	var result CallbackResult
	select {
	case result = <-srv.Result:
		slog.Debug("OAuth callback received", "has_error", result.Error != "")
	case <-ctx.Done():
		srv.Shutdown()
		return ctx.Err()
	}

	if result.Error != "" {
		return fmt.Errorf("authorization failed: %s: %s", result.Error, result.ErrorDescription)
	}

	if result.State != state {
		return fmt.Errorf("state mismatch: possible CSRF attack")
	}

	// Exchange code for tokens.
	slog.Debug("exchanging authorization code")
	tokenResp, err := oauth.ExchangeCode(ctx, clientID, result.Code, redirectURI, verifier)
	if err != nil {
		return fmt.Errorf("%w: %w", errCodeExchange, err)
	}

	// Store tokens.
	if err := store.Save(&StoredTokens{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ClientID:     clientID,
	}); err != nil {
		return fmt.Errorf("storing tokens: %w", err)
	}

	slog.Debug("tokens stored successfully")
	fmt.Fprintln(out, "Login successful!")
	return nil
}

// loadOrRegisterClient returns a client ID, registering a new client if needed.
// If forceNew is true, always registers a fresh client (used for retry after
// a stale client ID causes an auth failure).
func loadOrRegisterClient(ctx context.Context, oauth *OAuthClient, store *TokenStore, redirectURI string, forceNew bool) (string, error) {
	if !forceNew {
		existing, err := store.Load()
		if err == nil && existing.ClientID != "" {
			slog.Debug("using existing client ID")
			return existing.ClientID, nil
		}
	}

	// Register a new client.
	slog.Debug("registering new OAuth client")
	reg, err := oauth.RegisterClient(ctx, redirectURI)
	if err != nil {
		return "", err
	}
	return reg.ClientID, nil
}
