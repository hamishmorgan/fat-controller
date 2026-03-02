package auth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

const (
	DefaultAuthEndpoint    = "https://backboard.railway.com/oauth/auth"
	DefaultTokenEndpoint   = "https://backboard.railway.com/oauth/token"
	DefaultRegistrationURL = "https://backboard.railway.com/oauth/register"
	DefaultUserinfoURL     = "https://backboard.railway.com/oauth/me"
	DefaultGraphQLURL      = "https://backboard.railway.com/graphql/v2"

	defaultScope = "openid email profile offline_access"
)

// OAuthClient handles Railway OAuth 2.0 operations.
// Endpoints are configurable for testing.
type OAuthClient struct {
	AuthEndpoint    string
	TokenEndpoint   string
	RegistrationURL string
	UserinfoURL     string

	HTTPClient *http.Client
}

// NewOAuthClient creates an OAuthClient with Railway's production endpoints.
func NewOAuthClient() *OAuthClient {
	return &OAuthClient{
		AuthEndpoint:    DefaultAuthEndpoint,
		TokenEndpoint:   DefaultTokenEndpoint,
		RegistrationURL: DefaultRegistrationURL,
		UserinfoURL:     DefaultUserinfoURL,
		HTTPClient:      http.DefaultClient,
	}
}

// RegistrationRequest is the body for dynamic client registration (RFC 7591).
type RegistrationRequest struct {
	ClientName              string   `json:"client_name"`
	RedirectURIs            []string `json:"redirect_uris"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
	ApplicationType         string   `json:"application_type"`
}

// RegistrationResponse is returned by the registration endpoint.
type RegistrationResponse struct {
	ClientID                string `json:"client_id"`
	ClientName              string `json:"client_name"`
	RegistrationAccessToken string `json:"registration_access_token"`
	RegistrationClientURI   string `json:"registration_client_uri"`
}

// TokenResponse is returned by the token endpoint.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	Scope        string `json:"scope"`
}

// RegisterClient performs dynamic client registration for a native (public) app.
func (c *OAuthClient) RegisterClient(redirectURI string) (*RegistrationResponse, error) {
	reqBody := RegistrationRequest{
		ClientName:              "Fat Controller CLI",
		RedirectURIs:            []string{redirectURI},
		TokenEndpointAuthMethod: "none",
		GrantTypes:              []string{"authorization_code", "refresh_token"},
		ResponseTypes:           []string{"code"},
		ApplicationType:         "native",
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshalling registration request: %w", err)
	}

	resp, err := c.httpClient().Post(c.RegistrationURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("registration request failed: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("registration failed with status %d", resp.StatusCode)
	}

	var reg RegistrationResponse
	if err := json.NewDecoder(resp.Body).Decode(&reg); err != nil {
		return nil, fmt.Errorf("decoding registration response: %w", err)
	}
	return &reg, nil
}

// AuthorizationURL builds the URL the user should visit to authorize.
func (c *OAuthClient) AuthorizationURL(clientID, redirectURI, state, codeChallenge string) string {
	v := url.Values{
		"response_type":         {"code"},
		"client_id":             {clientID},
		"redirect_uri":          {redirectURI},
		"scope":                 {defaultScope},
		"state":                 {state},
		"code_challenge":        {codeChallenge},
		"code_challenge_method": {"S256"},
		"prompt":                {"consent"},
	}
	return c.AuthEndpoint + "?" + v.Encode()
}

// ExchangeCode exchanges an authorization code for tokens.
// Uses PKCE — no client secret (native client).
func (c *OAuthClient) ExchangeCode(clientID, code, redirectURI, codeVerifier string) (*TokenResponse, error) {
	data := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"client_id":     {clientID},
		"code_verifier": {codeVerifier},
	}

	resp, err := c.httpClient().PostForm(c.TokenEndpoint, data)
	if err != nil {
		return nil, fmt.Errorf("token exchange request failed: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token exchange failed with status %d", resp.StatusCode)
	}

	var tok TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		return nil, fmt.Errorf("decoding token response: %w", err)
	}
	return &tok, nil
}

// RefreshToken exchanges a refresh token for a new access + refresh token pair.
// Important: Railway rotates refresh tokens. Always store the new one.
func (c *OAuthClient) RefreshToken(clientID, refreshToken string) (*TokenResponse, error) {
	data := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {clientID},
	}

	resp, err := c.httpClient().PostForm(c.TokenEndpoint, data)
	if err != nil {
		return nil, fmt.Errorf("refresh request failed: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("refresh failed with status %d", resp.StatusCode)
	}

	var tok TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		return nil, fmt.Errorf("decoding refresh response: %w", err)
	}
	return &tok, nil
}

func (c *OAuthClient) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return http.DefaultClient
}
