package cli

import (
	"context"
	"log/slog"

	"github.com/hamishmorgan/fat-controller/internal/auth"
	"github.com/hamishmorgan/fat-controller/internal/platform"
	"github.com/hamishmorgan/fat-controller/internal/prompt"
	"github.com/hamishmorgan/fat-controller/internal/railway"
)

func newClient(api *ApiFlags, baseCtx context.Context) (*railway.Client, error) {
	slog.Debug("creating Railway client")
	ctx, cancel := api.TimeoutContext(baseCtx)
	defer cancel()
	store := auth.NewTokenStore(auth.WithFallbackPath(platform.AuthFilePath()))
	resolved, err := auth.ResolveAuth(ctx, api.Token, store)
	if err != nil {
		return nil, err
	}
	slog.Debug("Railway client created", "auth_source", resolved.Source)
	return railway.NewClient(railway.Endpoint, resolved, store, auth.NewOAuthClient()), nil
}

// interactivePicker bridges railway.Picker to prompt.PickItem for interactive
// CLI use. It checks whether stdin is a TTY and delegates to the prompt package.
func interactivePicker(label string, items []railway.PickCandidate) (string, error) {
	promptItems := make([]prompt.Item, len(items))
	for i, c := range items {
		promptItems[i] = prompt.Item{Name: c.Name, ID: c.ID}
	}
	return prompt.PickItem(label, promptItems, prompt.StdinIsInteractive(), prompt.PickOpts{})
}
