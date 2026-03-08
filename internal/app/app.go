// Package app provides application-level orchestration between the CLI
// interface layer and the domain packages (config, diff, apply). It owns
// the shared load → interpolate → resolve → fetch → filter pipeline and
// domain-logic helpers that have no CLI/terminal dependency.
package app

import (
	"context"
	"log/slog"

	"github.com/hamishmorgan/fat-controller/internal/config"
)

// ConfigFetcher abstracts the resolve + fetch operations so that app logic
// does not depend on the railway package directly.
type ConfigFetcher interface {
	Resolve(ctx context.Context, workspace, project, environment string) (string, string, error)
	Fetch(ctx context.Context, projectID, environmentID string, services []string) (*config.LiveConfig, error)
}

// ConfigPair bundles the desired and live config together with resolved IDs,
// produced by the shared load → interpolate → resolve → fetch → filter pipeline.
type ConfigPair struct {
	// RawDesired is the config as loaded from disk (before ${VAR} interpolation).
	// Used for emitting advisory warnings that should reflect the authored config.
	RawDesired    *config.DesiredConfig
	Desired       *config.DesiredConfig
	Live          *config.LiveConfig
	ProjectID     string
	EnvironmentID string
	EnvVars       map[string]string
}

// LoadAndFetch runs the shared pipeline used by both diff and apply:
//  1. Load config files (cascade or single --config-file)
//  2. Interpolate ${VAR} references (env files → process env)
//  3. Fall back to config-file project/environment when flags are empty
//  4. Resolve project and environment IDs
//  5. Fetch live state
//  6. Filter desired config by --service if set
func LoadAndFetch(ctx context.Context, flagWorkspace, flagProject, flagEnvironment, configDir string, configFile string, service string, fetcher ConfigFetcher) (*ConfigPair, error) {
	// 1. Load config files (cascade or single --config-file).
	slog.Debug("loading config", "dir", configDir, "config_file", configFile)
	result, err := config.LoadCascade(config.LoadOptions{
		WorkDir:    configDir,
		ConfigFile: configFile,
	})
	if err != nil {
		return nil, err
	}
	desired := result.Config
	rawDesired := cloneDesiredForWarnings(desired)

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

	return &ConfigPair{
		RawDesired:    rawDesired,
		Desired:       desired,
		Live:          live,
		ProjectID:     projID,
		EnvironmentID: envID,
		EnvVars:       result.EnvVars,
	}, nil
}

// cloneDesiredForWarnings makes a copy of the parts of DesiredConfig that are
// mutated by interpolation (variables and certain nested string fields). This
// keeps warning output focused on what the user wrote in the config.
func cloneDesiredForWarnings(in *config.DesiredConfig) *config.DesiredConfig {
	if in == nil {
		return nil
	}
	out := *in

	if in.Variables != nil {
		vars := make(config.Variables, len(in.Variables))
		for k, v := range in.Variables {
			vars[k] = v
		}
		out.Variables = vars
	}

	if len(in.Services) > 0 {
		services := make([]*config.DesiredService, 0, len(in.Services))
		for _, svc := range in.Services {
			if svc == nil {
				services = append(services, nil)
				continue
			}
			copiedSvc := *svc
			if svc.Variables != nil {
				svcVars := make(config.Variables, len(svc.Variables))
				for k, v := range svc.Variables {
					svcVars[k] = v
				}
				copiedSvc.Variables = svcVars
			}
			if svc.Deploy != nil {
				copiedDeploy := *svc.Deploy
				if svc.Deploy.RegistryCredentials != nil {
					copiedCreds := *svc.Deploy.RegistryCredentials
					copiedDeploy.RegistryCredentials = &copiedCreds
				}
				copiedSvc.Deploy = &copiedDeploy
			}
			services = append(services, &copiedSvc)
		}
		out.Services = services
	}

	return &out
}

// ScopeDesiredByPath narrows a DesiredConfig to only include the service or
// section specified by path. A path of "api" keeps only the "api" service.
// A path of "variables" keeps only shared variables. An unrecognized path
// returns the config unchanged.
func ScopeDesiredByPath(cfg *config.DesiredConfig, path string) *config.DesiredConfig {
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
