package config

import (
	"fmt"
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
//  2. fat-controller.local.toml from dir (optional, auto-discovered)
//  3. Extra files from --config flags (in order)
//
// Returns the merged DesiredConfig. Returns an error if the base file
// is missing or any explicitly-specified file fails to parse. If you
// want to support --config without a base file, relax this requirement
// and document the behavior.
func LoadConfigs(dir string, extraFiles []string) (*DesiredConfig, error) {
	basePath := filepath.Join(dir, BaseConfigFile)
	if _, err := os.Stat(basePath); err != nil {
		return nil, fmt.Errorf("config file not found: %s", basePath)
	}

	var configs []*DesiredConfig

	base, err := ParseFile(basePath)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", basePath, err)
	}
	configs = append(configs, base)

	localPath := filepath.Join(dir, LocalConfigFile)
	if _, err := os.Stat(localPath); err == nil {
		local, err := ParseFile(localPath)
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", localPath, err)
		}
		configs = append(configs, local)
	}

	for _, path := range extraFiles {
		extra, err := ParseFile(path)
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", path, err)
		}
		configs = append(configs, extra)
	}

	return Merge(configs...), nil
}
