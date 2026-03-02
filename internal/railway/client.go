package railway

import (
	"net/http"

	"github.com/Khan/genqlient/graphql"
	"github.com/hamishmorgan/fat-controller/internal/auth"
)

// Endpoint is the Railway GraphQL API URL.
const Endpoint = "https://backboard.railway.com/graphql/v2"

// Client wraps the genqlient GraphQL client with Railway-specific auth.
type Client struct {
	gql  graphql.Client
	auth *auth.ResolvedAuth
}

// NewClient creates a Railway GraphQL client with authenticated transport.
// The transport injects the correct auth header and handles token refresh
// for stored OAuth tokens.
//
// The oauth parameter is used only for token refresh — its HTTPClient is NOT
// modified and should use the default transport (not AuthTransport) to avoid
// circular refresh calls.
func NewClient(endpoint string, resolved *auth.ResolvedAuth, store *auth.TokenStore, oauth *auth.OAuthClient) *Client {
	var refresher Refresher
	if oauth != nil {
		refresher = NewOAuthRefresher(oauth)
	}
	transport := NewAuthTransport(resolved, store, refresher)

	httpClient := &http.Client{Transport: transport}
	gql := graphql.NewClient(endpoint, httpClient)

	return &Client{gql: gql, auth: resolved}
}

// Auth returns the resolved auth info (used by resolve.go to branch on token type).
func (c *Client) Auth() *auth.ResolvedAuth {
	return c.auth
}

// GQL returns the underlying genqlient client for making queries.
// Callers use the generated functions directly:
//
//	resp, err := ProjectToken(ctx, client.GQL())
func (c *Client) GQL() graphql.Client {
	return c.gql
}
