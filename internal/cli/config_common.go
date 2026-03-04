package cli

import (
	"context"

	"github.com/hamishmorgan/fat-controller/internal/config"
)

// configPair bundles the desired and live config together with resolved IDs,
// produced by the shared load → interpolate → resolve → fetch → filter pipeline.
type configPair struct {
	Desired       *config.DesiredConfig
	Live          *config.LiveConfig
	ProjectID     string
	EnvironmentID string
}

// loadAndFetch runs the shared pipeline used by both `config diff` and `config apply`:
//  1. Load and merge config files
//  2. Interpolate local env vars
//  3. Fall back to config-file project/environment when globals are empty
//  4. Resolve project and environment IDs
//  5. Fetch live state
//  6. Filter desired config by --service if set
func loadAndFetch(ctx context.Context, globals *Globals, configDir string, extraFiles []string, fetcher configFetcher) (*configPair, error) {
	// 1. Load and merge config files.
	debug(globals, "loading config from %s", configDir)
	desired, err := config.LoadConfigs(configDir, extraFiles)
	if err != nil {
		return nil, err
	}

	// 2. Interpolate local env vars.
	if err := config.Interpolate(desired); err != nil {
		return nil, err
	}

	// 3. Use config-file project/environment/workspace as fallback for resolution.
	project := globals.Project
	if project == "" {
		project = desired.Project
	}
	environment := globals.Environment
	if environment == "" {
		environment = desired.Environment
	}
	workspace := globals.Workspace
	if workspace == "" {
		workspace = desired.Workspace
	}

	// 4. Resolve project and environment IDs.
	debug(globals, "resolving project=%q environment=%q", project, environment)
	projID, envID, err := fetcher.Resolve(ctx, workspace, project, environment)
	if err != nil {
		return nil, err
	}

	// 5. Fetch live state.
	debug(globals, "fetching live state for project=%s environment=%s", projID, envID)
	live, err := fetcher.Fetch(ctx, projID, envID, globals.Service)
	if err != nil {
		return nil, err
	}

	// 6. Filter desired config by --service if set.
	if globals.Service != "" {
		filtered := &config.DesiredConfig{
			Shared:   desired.Shared,
			Services: make(map[string]*config.DesiredService),
		}
		if svc, ok := desired.Services[globals.Service]; ok {
			filtered.Services[globals.Service] = svc
		}
		desired = filtered
	}

	return &configPair{
		Desired:       desired,
		Live:          live,
		ProjectID:     projID,
		EnvironmentID: envID,
	}, nil
}
