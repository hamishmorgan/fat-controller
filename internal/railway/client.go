package railway

import (
	"net/http"

	"github.com/Khan/genqlient/graphql"
	"github.com/hamishmorgan/fat-controller/internal/auth"
)

// Client wraps the genqlient GraphQL client with Railway-specific auth.
type Client struct {
	gql graphql.Client
}

// NewClient creates a Railway GraphQL client with authenticated transport.
// The transport injects the correct auth header and handles token refresh
// for stored OAuth tokens.
func NewClient(endpoint string, resolved *auth.ResolvedAuth, store *auth.TokenStore, oauth *auth.OAuthClient) *Client {
	refresher := NewOAuthRefresher(oauth)
	transport := NewAuthTransport(resolved, store, refresher)

	httpClient := &http.Client{Transport: transport}
	gql := graphql.NewClient(endpoint, httpClient)

	return &Client{gql: gql}
}

// GQL returns the underlying genqlient client for making queries.
// Callers use the generated functions directly:
//
//	resp, err := ProjectToken(ctx, client.GQL())
func (c *Client) GQL() graphql.Client {
	return c.gql
}
