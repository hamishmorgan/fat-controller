package cli

import (
	"context"
	"fmt"
	"log/slog"

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
	slog.Debug("loading config", "dir", configDir)
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
	slog.Debug("resolving project and environment", "project", project, "environment", environment)
	projID, envID, err := fetcher.Resolve(ctx, workspace, project, environment)
	if err != nil {
		return nil, err
	}

	// 5. Fetch live state.
	slog.Debug("fetching live state", "project_id", projID, "environment_id", envID)
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

// emitWarnings runs validation on the config pair and emits warnings to stderr via slog.
// Respects --quiet to suppress warnings. Callers that always want warnings (e.g. config validate)
// should call config.Validate directly.
func emitWarnings(pair *configPair, globals *Globals, configDir string) {
	if globals.Quiet {
		return
	}
	// Extract live service names for W040.
	var liveNames []string
	for name := range pair.Live.Services {
		liveNames = append(liveNames, name)
	}

	warnings := config.Validate(pair.Desired, liveNames)
	warnings = append(warnings, config.ValidateFiles(configDir)...)

	// Filter suppressed warnings (Validate already filters, but ValidateFiles warnings need it too).
	suppressed := make(map[string]bool, len(pair.Desired.SuppressWarnings))
	for _, code := range pair.Desired.SuppressWarnings {
		suppressed[code] = true
	}

	for _, w := range warnings {
		if suppressed[w.Code] {
			continue
		}
		path := ""
		if w.Path != "" {
			path = " (" + w.Path + ")"
		}
		slog.Warn(fmt.Sprintf("[%s]%s %s", w.Code, path, w.Message))
	}
}
