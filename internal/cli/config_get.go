package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/hamishmorgan/fat-controller/internal/config"
	"github.com/hamishmorgan/fat-controller/internal/railway"
)

// configFetcher allows injection for tests.
type configFetcher interface {
	Resolve(ctx context.Context, workspace, project, environment string) (string, string, error)
	Fetch(ctx context.Context, projectID, environmentID, service string) (*config.LiveConfig, error)
}

type defaultConfigFetcher struct {
	client *railway.Client
}

func (d *defaultConfigFetcher) Resolve(ctx context.Context, workspace, project, environment string) (string, string, error) {
	return railway.ResolveProjectEnvironment(ctx, d.client, workspace, project, environment)
}

func (d *defaultConfigFetcher) Fetch(ctx context.Context, projectID, environmentID, service string) (*config.LiveConfig, error) {
	return railway.FetchLiveConfig(ctx, d.client, projectID, environmentID, service)
}

// SetOutput overrides the output writer (for testing).
func (c *ConfigGetCmd) SetOutput(w io.Writer) {
	c.output = w
}

// Run implements `config get`.
func (c *ConfigGetCmd) Run(globals *Globals) error {
	slog.Warn("'config get' is deprecated; use 'show' instead")
	ctx, cancel := c.TimeoutContext(globals.BaseCtx)
	defer cancel()
	client, err := newClient(&c.ApiFlags, globals.BaseCtx)
	if err != nil {
		return err
	}
	fetcher := &defaultConfigFetcher{client: client}
	return RunConfigGet(ctx, globals, c.Workspace, c.Project, c.Environment, c.Path, c.Full, c.Service, c.ShowSecrets, fetcher, c.output)
}

// RunConfigGet is the testable core of `config get`.
func RunConfigGet(ctx context.Context, globals *Globals, workspace, project, environment, path string, full bool, service string, showSecrets bool, fetcher configFetcher, out io.Writer) error {
	slog.Debug("starting config get", "path", path)
	if out == nil {
		out = os.Stdout
	}
	projID, envID, err := fetcher.Resolve(ctx, workspace, project, environment)
	if err != nil {
		return err
	}
	fetchService := service
	var parsed config.Path
	if path != "" {
		parsed, err = config.ParsePath(path)
		if err != nil {
			return err
		}
		if parsed.Service != "" {
			fetchService = parsed.Service
		}
	}
	cfg, err := fetcher.Fetch(ctx, projID, envID, fetchService)
	if err != nil {
		return err
	}
	if cfg == nil {
		return errors.New("no config returned")
	}

	// Single key lookup: output just the raw value.
	if parsed.Key != "" {
		val, ok := lookupKey(*cfg, parsed)
		if !ok {
			return fmt.Errorf("key %q not found in %s.%s", parsed.Key, parsed.Service, parsed.Section)
		}
		if !showSecrets {
			masker := config.NewMasker(nil, nil)
			val = masker.MaskValue(parsed.Key, val)
		}
		_, err = fmt.Fprintln(out, val)
		return err
	}

	if globals.Output == "raw" {
		return errors.New("raw output requires a single scalar value (e.g. show api.variables.PORT)")
	}

	// Section-level lookup: filter config to just that section.
	if parsed.Section != "" {
		filtered := filterSection(*cfg, parsed)
		cfg = &filtered
	}

	output, err := config.Render(*cfg, config.RenderOptions{
		Format:      globals.Output,
		Full:        full,
		ShowSecrets: showSecrets,
	})
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, output)
	return err
}

// lookupKey retrieves a single value from the config for a fully-qualified path.
func lookupKey(cfg config.LiveConfig, p config.Path) (string, bool) {
	switch p.Section {
	case "variables":
		if p.Service == "shared" {
			val, found := cfg.Variables[p.Key]
			return val, found
		}
		svc, ok := cfg.Services[p.Service]
		if !ok {
			return "", false
		}
		val, found := svc.Variables[p.Key]
		return val, found
	default:
		return "", false
	}
}

// filterSection returns a copy of cfg containing only the requested section.
func filterSection(cfg config.LiveConfig, p config.Path) config.LiveConfig {
	filtered := config.LiveConfig{
		ProjectID:     cfg.ProjectID,
		EnvironmentID: cfg.EnvironmentID,
	}
	switch p.Section {
	case "variables":
		if p.Service == "shared" {
			filtered.Variables = cfg.Variables
			return filtered
		}
		svc, ok := cfg.Services[p.Service]
		if !ok {
			return filtered
		}
		filtered.Services = map[string]*config.ServiceConfig{
			p.Service: {
				ID:        svc.ID,
				Name:      svc.Name,
				Variables: svc.Variables,
			},
		}
	}
	return filtered
}
