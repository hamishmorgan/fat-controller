package auth

import (
	"fmt"
	"os/exec"
	"runtime"
)

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
	return cmd.Start()
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
	if err != nil {
		// If the first attempt failed, retry with a fresh client registration.
		// This handles the case where a stored client ID was revoked.
		fmt.Println("Retrying with fresh client registration...")
		return loginAttempt(oauth, store, openBrowser, true)
	}
	return nil
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
		return fmt.Errorf("exchanging authorization code: %w", err)
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
