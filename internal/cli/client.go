package cli

import (
	"log/slog"

	"github.com/hamishmorgan/fat-controller/internal/auth"
	"github.com/hamishmorgan/fat-controller/internal/platform"
	"github.com/hamishmorgan/fat-controller/internal/railway"
)

func newClient(globals *Globals) (*railway.Client, error) {
	slog.Debug("creating Railway client")
	ctx, cancel := globals.TimeoutContext(globals.BaseCtx)
	defer cancel()
	store := auth.NewTokenStore(auth.WithFallbackPath(platform.AuthFilePath()))
	resolved, err := auth.ResolveAuth(ctx, globals.Token, store)
	if err != nil {
		return nil, err
	}
	slog.Debug("Railway client created", "auth_source", resolved.Source)
	return railway.NewClient(railway.Endpoint, resolved, store, auth.NewOAuthClient()), nil
}
