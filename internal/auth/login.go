package auth

import (
	"errors"
	"fmt"
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
		cmd = browserCommand("open", url)
	case "windows":
		cmd = browserCommand("rundll32", "url.dll,FileProtocolHandler", url)
	default: // linux, freebsd, etc.
		cmd = browserCommand("xdg-open", url)
	}
	return cmd.Start()
}

var browserCommand = exec.Command

// BrowserCommand returns the current command factory used by OpenBrowser.
func BrowserCommand() func(name string, arg ...string) *exec.Cmd {
	return browserCommand
}

// SetBrowserCommand overrides the command factory used by OpenBrowser.
// Intended for tests.
func SetBrowserCommand(cmd func(name string, arg ...string) *exec.Cmd) {
	if cmd == nil {
		return
	}
	browserCommand = cmd
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
func Login(oauth *OAuthClient, store *TokenStore, openBrowser BrowserOpener) error {
	err := loginAttempt(oauth, store, openBrowser, false)
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

	fmt.Println("Token exchange failed; retrying with fresh client registration...")
	return loginAttempt(oauth, store, openBrowser, true)
}

func loginAttempt(oauth *OAuthClient, store *TokenStore, openBrowser BrowserOpener, forceNewClient bool) error {
	// Start callback server.
	srv, err := StartCallbackServer()
	if err != nil {
		return fmt.Errorf("starting callback server: %w", err)
	}
	defer srv.Shutdown()

	redirectURI := srv.RedirectURI()

	// Check for existing client registration.
	clientID, err := loadOrRegisterClient(oauth, store, redirectURI, forceNewClient)
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
	fmt.Println("Opening browser to log in...")
	fmt.Printf("If the browser doesn't open, visit:\n%s\n\n", authURL)

	if err := openBrowser(authURL); err != nil {
		// Non-fatal — user can copy the URL.
		fmt.Printf("Could not open browser: %v\n", err)
	}

	// Wait for callback.
	fmt.Println("Waiting for authorization...")
	result := <-srv.Result

	if result.Error != "" {
		return fmt.Errorf("authorization failed: %s: %s", result.Error, result.ErrorDescription)
	}

	if result.State != state {
		return fmt.Errorf("state mismatch: possible CSRF attack")
	}

	// Exchange code for tokens.
	tokenResp, err := oauth.ExchangeCode(clientID, result.Code, redirectURI, verifier)
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

	fmt.Println("Login successful!")
	return nil
}

// loadOrRegisterClient returns a client ID, registering a new client if needed.
// If forceNew is true, always registers a fresh client (used for retry after
// a stale client ID causes an auth failure).
func loadOrRegisterClient(oauth *OAuthClient, store *TokenStore, redirectURI string, forceNew bool) (string, error) {
	if !forceNew {
		existing, err := store.Load()
		if err == nil && existing.ClientID != "" {
			return existing.ClientID, nil
		}
	}

	// Register a new client.
	reg, err := oauth.RegisterClient(redirectURI)
	if err != nil {
		return "", err
	}
	return reg.ClientID, nil
}
