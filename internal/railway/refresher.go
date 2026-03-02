package railway

import (
	"context"

	"github.com/hamishmorgan/fat-controller/internal/auth"
)

// OAuthRefresher implements Refresher by delegating to auth.OAuthClient.
type OAuthRefresher struct {
	oauth *auth.OAuthClient
}

// NewOAuthRefresher creates a Refresher that uses the given OAuthClient.
func NewOAuthRefresher(oauth *auth.OAuthClient) *OAuthRefresher {
	return &OAuthRefresher{oauth: oauth}
}

// Refresh exchanges a refresh token for new tokens via the OAuth token endpoint.
func (r *OAuthRefresher) Refresh(ctx context.Context, clientID, refreshToken string) (*auth.TokenResponse, error) {
	return r.oauth.RefreshToken(ctx, clientID, refreshToken)
}
