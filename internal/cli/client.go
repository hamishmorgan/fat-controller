package cli

import (
	"github.com/hamishmorgan/fat-controller/internal/auth"
	"github.com/hamishmorgan/fat-controller/internal/platform"
	"github.com/hamishmorgan/fat-controller/internal/railway"
)

func newClient(globals *Globals) (*railway.Client, error) {
	store := auth.NewTokenStore(auth.WithFallbackPath(platform.AuthFilePath()))
	resolved, err := auth.ResolveAuth(globals.Token, store)
	if err != nil {
		return nil, err
	}
	return railway.NewClient(railway.Endpoint, resolved, store, auth.NewOAuthClient()), nil
}
