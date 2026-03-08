package railway

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/Khan/genqlient/graphql"
	"github.com/hamishmorgan/fat-controller/internal/auth"
)

// Endpoint is the Railway GraphQL API URL.
const Endpoint = "https://backboard.railway.com/graphql/v2"

// Client wraps the genqlient GraphQL client with Railway-specific auth.
type Client struct {
	gqlClient graphql.Client
	auth      *auth.ResolvedAuth
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
	gql = &loggingClient{inner: gql}

	return &Client{gqlClient: gql, auth: resolved}
}

// Auth returns the resolved auth info (used by resolve.go to branch on token type).
func (c *Client) Auth() *auth.ResolvedAuth {
	return c.auth
}

// gql returns the underlying genqlient client for making queries.
// Package-private: callers outside this package should use the exported
// wrapper functions (ListServices, UpdateServiceSettings, etc.) instead.
func (c *Client) gql() graphql.Client {
	return c.gqlClient
}

// loggingClient wraps a genqlient graphql.Client and logs each operation.
type loggingClient struct {
	inner graphql.Client
}

func (c *loggingClient) MakeRequest(ctx context.Context, req *graphql.Request, resp *graphql.Response) error {
	start := time.Now()
	err := c.inner.MakeRequest(ctx, req, resp)
	duration := time.Since(start)

	attrs := []slog.Attr{
		slog.String("op", req.OpName),
		slog.Duration("duration", duration),
	}

	if req.Variables != nil {
		attrs = append(attrs, slog.Any("vars", RedactVariables(req.Variables)))
	}

	if err != nil {
		attrs = append(attrs, slog.String("error", err.Error()))
		slog.LogAttrs(ctx, slog.LevelDebug, "graphql request failed", attrs...)
		return err
	}

	if resp != nil && len(resp.Errors) > 0 {
		attrs = append(attrs, slog.Int("graphql_errors", len(resp.Errors)))
	}

	slog.LogAttrs(ctx, slog.LevelDebug, "graphql request", attrs...)
	return nil
}

// redactedValue is the placeholder for redacted fields.
const redactedValue = "[REDACTED]"

// secretKeys are JSON field names whose values should be redacted in logs.
var secretKeys = map[string]bool{
	"value":     true, // VariableUpsertInput.Value
	"variables": true, // VariableCollectionUpsertInput.Variables (map of key→value)
}

// RedactVariables converts a variables struct to a map and redacts secret fields.
// If marshaling fails, it returns the original value unmodified.
func RedactVariables(v interface{}) interface{} {
	b, err := json.Marshal(v)
	if err != nil {
		return v
	}
	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		return v
	}
	redactMap(m)
	return m
}

// redactMap recursively walks a map and replaces secret field values.
func redactMap(m map[string]interface{}) {
	for k, v := range m {
		if secretKeys[k] {
			m[k] = redactedValue
			continue
		}
		if sub, ok := v.(map[string]interface{}); ok {
			redactMap(sub)
		}
	}
}
