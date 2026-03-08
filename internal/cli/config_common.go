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
//  1. Load and merge config files via cascade
//  2. Interpolate ${VAR} references (env files → process env)
//  3. Fall back to config-file project/environment when flags are empty
//  4. Resolve project and environment IDs
//  5. Fetch live state
//  6. Filter desired config by --service if set
func loadAndFetch(ctx context.Context, flagWorkspace, flagProject, flagEnvironment, configDir string, extraFiles []string, service string, fetcher configFetcher) (*configPair, error) {
	// 1. Load and merge config files via cascade.
	slog.Debug("loading config", "dir", configDir)
	result, err := config.LoadCascade(config.LoadOptions{WorkDir: configDir})
	if err != nil {
		return nil, err
	}
	desired := result.Config

	// Merge extra config files (--file flags) on top.
	for _, f := range extraFiles {
		extra, err := config.ParseFile(f)
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", f, err)
		}
		desired = config.Merge(desired, extra)
	}

	// 2. Interpolate ${VAR} references (env files → process env).
	if err := config.Interpolate(desired, result.EnvVars); err != nil {
		return nil, err
	}

	// 3. Use config-file project/environment/workspace as fallback for resolution.
	project := flagProject
	if project == "" && desired.Project != nil {
		project = desired.Project.Name
	}
	environment := flagEnvironment
	if environment == "" {
		environment = desired.Name
	}
	workspace := flagWorkspace
	if workspace == "" && desired.Workspace != nil {
		workspace = desired.Workspace.Name
	}

	// 4. Resolve project and environment IDs.
	slog.Debug("resolving project and environment", "project", project, "environment", environment)
	projID, envID, err := fetcher.Resolve(ctx, workspace, project, environment)
	if err != nil {
		return nil, err
	}

	// 5. Fetch live state.
	slog.Debug("fetching live state", "project_id", projID, "environment_id", envID)
	var svcFilter []string
	if service != "" {
		svcFilter = []string{service}
	}
	live, err := fetcher.Fetch(ctx, projID, envID, svcFilter)
	if err != nil {
		return nil, err
	}

	// 6. Filter desired config by --service if set.
	if service != "" {
		filtered := &config.DesiredConfig{
			Variables: desired.Variables,
		}
		for _, svc := range desired.Services {
			if svc.Name == service {
				filtered.Services = append(filtered.Services, svc)
				break
			}
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

// scopeDesiredByPath narrows a DesiredConfig to only include the service or
// section specified by path. A path of "api" keeps only the "api" service.
// A path of "variables" keeps only shared variables. An unrecognized path
// returns the config unchanged.
func scopeDesiredByPath(cfg *config.DesiredConfig, path string) *config.DesiredConfig {
	if cfg == nil || path == "" {
		return cfg
	}
	// If path matches "variables" (shared), strip services.
	if path == "variables" {
		return &config.DesiredConfig{
			Variables: cfg.Variables,
		}
	}
	// If path matches a service name, keep only that service.
	for _, svc := range cfg.Services {
		if svc.Name == path {
			return &config.DesiredConfig{
				Services: []*config.DesiredService{svc},
			}
		}
	}
	// If path is "service.section" (e.g. "api.variables"), keep just the service.
	parts := splitDotPath(path)
	if len(parts) >= 1 {
		for _, svc := range cfg.Services {
			if svc.Name == parts[0] {
				return &config.DesiredConfig{
					Services: []*config.DesiredService{svc},
				}
			}
		}
	}
	// Unrecognized path — return as-is.
	return cfg
}

// splitDotPath splits a dot-separated path into parts.
func splitDotPath(path string) []string {
	if path == "" {
		return nil
	}
	var parts []string
	start := 0
	for i, c := range path {
		if c == '.' {
			parts = append(parts, path[start:i])
			start = i + 1
		}
	}
	parts = append(parts, path[start:])
	return parts
}

// emitWarnings runs validation on the config pair and emits warnings to stderr via slog.
// Respects --quiet to suppress warnings. Callers that always want warnings (e.g. config validate)
// should call config.Validate directly.
func emitWarnings(pair *configPair, quiet int, configDir string) {
	if quiet > 0 {
		return
	}
	// Extract live service names for W040.
	var liveNames []string
	for name := range pair.Live.Services {
		liveNames = append(liveNames, name)
	}

	warnings := config.ValidateWithOptions(pair.Desired, config.ValidateOptions{LiveServiceNames: liveNames, EnvFileVars: nil})
	warnings = append(warnings, config.ValidateFiles(configDir)...)

	// Filter suppressed warnings (Validate already filters, but ValidateFiles warnings need it too).
	var suppressWarnings []string
	if pair.Desired.Tool != nil {
		suppressWarnings = pair.Desired.Tool.SuppressWarnings
	}
	suppressed := make(map[string]bool, len(suppressWarnings))
	for _, code := range suppressWarnings {
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
