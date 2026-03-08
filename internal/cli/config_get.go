package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/hamishmorgan/fat-controller/internal/app"
	"github.com/hamishmorgan/fat-controller/internal/config"
	"github.com/hamishmorgan/fat-controller/internal/railway"
)

type defaultConfigFetcher struct {
	client *railway.Client
	// cachedServices stores services fetched during Resolve (from
	// ProjectsResolution), so Fetch can skip a separate ProjectServices call.
	cachedServices []railway.ServiceInfo
}

func (d *defaultConfigFetcher) Resolve(ctx context.Context, workspace, project, environment string) (*app.ResolvedIdentity, error) {
	r, err := railway.ResolveProjectEnvironment(ctx, d.client, workspace, project, environment, interactivePicker)
	if err != nil {
		return nil, err
	}
	identity := &app.ResolvedIdentity{
		ProjectID:       r.ProjectID,
		EnvironmentID:   r.EnvironmentID,
		WorkspaceName:   r.WorkspaceName,
		ProjectName:     r.ProjectName,
		EnvironmentName: r.EnvironmentName,
	}
	for _, svc := range r.Services {
		identity.Services = append(identity.Services, app.ServiceRef{
			ID: svc.ID, Name: svc.Name, Icon: svc.Icon,
		})
	}
	// Cache services for use in Fetch (avoids separate ProjectServices query).
	d.cachedServices = r.Services
	return identity, nil
}

func (d *defaultConfigFetcher) Fetch(ctx context.Context, projectID, environmentID string, services []string) (*config.LiveConfig, error) {
	return railway.FetchLiveConfig(ctx, d.client, projectID, environmentID, services, d.cachedServices)
}

// RunConfigGet is the testable core of `show` (formerly `config get`).
func RunConfigGet(ctx context.Context, globals *Globals, workspace, project, environment, path string, full bool, service string, showSecrets bool, fetcher app.ConfigFetcher, out io.Writer) error {
	slog.Debug("starting config get", "path", path)
	if out == nil {
		out = os.Stdout
	}
	resolved, err := fetcher.Resolve(ctx, workspace, project, environment)
	if err != nil {
		return err
	}
	projID, envID := resolved.ProjectID, resolved.EnvironmentID
	var fetchServices []string
	if service != "" {
		fetchServices = []string{service}
	}
	var parsed config.Path
	if path != "" {
		parsed, err = config.ParsePath(path)
		if err != nil {
			return err
		}
		if parsed.Service != "" {
			fetchServices = []string{parsed.Service}
		}
	}
	cfg, err := fetcher.Fetch(ctx, projID, envID, fetchServices)
	if err != nil {
		return err
	}
	if cfg == nil {
		return errors.New("no config returned")
	}

	// Single key lookup: output just the raw value.
	if parsed.Key != "" {
		val, ok := app.LookupKey(*cfg, parsed)
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
		filtered := app.FilterSection(*cfg, parsed)
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
