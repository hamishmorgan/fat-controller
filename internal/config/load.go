package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

const (
	// BaseConfigFile is the default config file name.
	BaseConfigFile = "fat-controller.toml"
	// LocalConfigFile is the auto-discovered local override.
	LocalConfigFile = "fat-controller.local.toml"
)

// LoadConfigs loads and merges config files:
//  1. fat-controller.toml from dir (required)
//  2. Extra files from --config flags (in order)
//
// If fat-controller.local.toml exists, a deprecation warning is logged.
// Migrate secrets to ${VAR} references in the base config file.
// Returns the merged DesiredConfig. Returns an error if the base file
// is missing or any explicitly-specified file fails to parse. If you
// want to support --config without a base file, relax this requirement
// and document the behavior.
func LoadConfigs(dir string, extraFiles []string) (*DesiredConfig, error) {
	basePath := filepath.Join(dir, BaseConfigFile)
	if _, err := os.Stat(basePath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("config file not found: %s", basePath)
		}
		return nil, fmt.Errorf("checking config file: %w", err)
	}

	var configs []*DesiredConfig

	slog.Debug("loading config file", "path", basePath)
	base, err := ParseFile(basePath)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", basePath, err)
	}
	configs = append(configs, base)

	var overrides []Override

	localPath := filepath.Join(dir, LocalConfigFile)
	if _, err := os.Stat(localPath); err == nil {
		slog.Warn("fat-controller.local.toml is deprecated — move secrets "+
			"to ${VAR} references in fat-controller.toml and use "+
			".env.fat-controller for secret values",
			"path", localPath)
	}

	for _, path := range extraFiles {
		slog.Debug("loading extra config", "path", path)
		extra, err := ParseFile(path)
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", path, err)
		}
		// Compare against the accumulated merge so far.
		accumulated := Merge(configs...)
		overrides = append(overrides, findOverrides(accumulated, extra, filepath.Base(path))...)
		configs = append(configs, extra)
	}

	slog.Debug("merged config files", "count", len(configs))
	result := Merge(configs...)
	result.Overrides = overrides
	return result, nil
}

// findOverrides detects variables in overlay that override variables in base.
func findOverrides(base, overlay *DesiredConfig, sourceName string) []Override {
	var overrides []Override
	if base.Shared != nil && overlay.Shared != nil {
		for k := range overlay.Shared.Vars {
			if _, ok := base.Shared.Vars[k]; ok {
				overrides = append(overrides, Override{
					Path: "shared.variables." + k, Source: sourceName,
				})
			}
		}
	}
	for svcName, overlaySvc := range overlay.Services {
		if overlaySvc == nil {
			continue
		}
		baseSvc, ok := base.Services[svcName]
		if !ok || baseSvc == nil {
			continue
		}
		for k := range overlaySvc.Variables {
			if _, ok := baseSvc.Variables[k]; ok {
				overrides = append(overrides, Override{
					Path: svcName + ".variables." + k, Source: sourceName,
				})
			}
		}
	}
	return overrides
}
