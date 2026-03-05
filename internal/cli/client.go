package cli

import (
	"context"
	"log/slog"

	"github.com/hamishmorgan/fat-controller/internal/auth"
	"github.com/hamishmorgan/fat-controller/internal/platform"
	"github.com/hamishmorgan/fat-controller/internal/railway"
)

func newClient(globals *Globals) (*railway.Client, error) {
	slog.Debug("creating Railway client")
	store := auth.NewTokenStore(auth.WithFallbackPath(platform.AuthFilePath()))
	resolved, err := auth.ResolveAuth(context.Background(), globals.Token, store)
	if err != nil {
		return nil, err
	}
	slog.Debug("Railway client created", "auth_source", resolved.Source)
	return railway.NewClient(railway.Endpoint, resolved, store, auth.NewOAuthClient()), nil
}
