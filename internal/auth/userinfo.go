package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// UserInfo represents the OIDC userinfo response.
type UserInfo struct {
	Sub     string `json:"sub"`
	Email   string `json:"email"`
	Name    string `json:"name"`
	Picture string `json:"picture"`
}

// FetchUserInfo calls the OIDC userinfo endpoint.
// Auth is handled by the OAuthClient's HTTPClient transport —
// callers must set HTTPClient to a client with an auth-injecting transport.
func (c *OAuthClient) FetchUserInfo(ctx context.Context) (*UserInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.UserinfoURL, nil)
	if err != nil {
		return nil, fmt.Errorf("building userinfo request: %w", err)
	}

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("userinfo request failed: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("userinfo failed with status %d", resp.StatusCode)
	}

	var info UserInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("decoding userinfo: %w", err)
	}
	return &info, nil
}
