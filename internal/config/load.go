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
	// DefaultEnvFile is the default secrets file written by init/adopt.
	DefaultEnvFile = ".secrets.fat-controller"
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
			".secrets.fat-controller for secret values",
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

// LoadOptions controls how LoadCascade discovers and loads config files.
type LoadOptions struct {
	ConfigFile string // --config-file override; disables walk + local override
	WorkDir    string // starting dir for discovery (defaults to cwd)
}

// LoadResult holds the merged config and metadata about which files were loaded.
type LoadResult struct {
	Config      *DesiredConfig
	PrimaryFile string            // deepest discovered file
	Files       []string          // all loaded files in merge order
	EnvVars     map[string]string // merged env file variables
}

// LoadCascade discovers and merges config files from the cascade:
//  1. Global config (~/.config/fat-controller/config.toml)
//  2. Discovered configs (walk upward from WorkDir to git root)
//  3. Local override (primary.local.toml)
//  4. Env files referenced by [tool] env_file
//
// If ConfigFile is set, only that file is loaded (no walk, no local override).
func LoadCascade(opts LoadOptions) (*LoadResult, error) {
	if opts.WorkDir == "" {
		var err error
		opts.WorkDir, err = os.Getwd()
		if err != nil {
			return nil, err
		}
	}

	result := &LoadResult{}
	var configs []*DesiredConfig

	// If --config-file is set, load only that file.
	if opts.ConfigFile != "" {
		cfg, err := ParseFile(opts.ConfigFile)
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", opts.ConfigFile, err)
		}
		result.Config = cfg
		result.PrimaryFile = opts.ConfigFile
		result.Files = []string{opts.ConfigFile}
		result.EnvVars = loadEnvFiles(cfg, opts.ConfigFile)
		return result, nil
	}

	// 1. Global config (XDG).
	globalPath := globalConfigPath()
	if globalPath != "" {
		if _, err := os.Stat(globalPath); err == nil {
			slog.Debug("loading global config", "path", globalPath)
			cfg, err := ParseFile(globalPath)
			if err != nil {
				return nil, fmt.Errorf("parsing global config %s: %w", globalPath, err)
			}
			configs = append(configs, cfg)
			result.Files = append(result.Files, globalPath)
		}
	}

	// 2. Discovered configs (walk upward, shallowest-first).
	discovered, err := DiscoverConfigs(opts.WorkDir)
	if err != nil {
		return nil, err
	}
	for _, path := range discovered {
		slog.Debug("loading discovered config", "path", path)
		cfg, err := ParseFile(path)
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", path, err)
		}
		configs = append(configs, cfg)
		result.Files = append(result.Files, path)
	}

	// Primary is the deepest (last) discovered file.
	if len(discovered) > 0 {
		result.PrimaryFile = discovered[len(discovered)-1]
	}

	// 3. Local override.
	if result.PrimaryFile != "" {
		localPath := LocalOverridePath(result.PrimaryFile)
		if _, err := os.Stat(localPath); err == nil {
			slog.Debug("loading local override", "path", localPath)
			cfg, err := ParseFile(localPath)
			if err != nil {
				return nil, fmt.Errorf("parsing %s: %w", localPath, err)
			}
			configs = append(configs, cfg)
			result.Files = append(result.Files, localPath)
		}
	}

	if len(configs) == 0 {
		return nil, fmt.Errorf("no config files found (searched from %s)", opts.WorkDir)
	}

	result.Config = Merge(configs...)

	// 4. Load env files referenced by [tool] env_file.
	if result.PrimaryFile != "" {
		result.EnvVars = loadEnvFiles(result.Config, result.PrimaryFile)
	}

	return result, nil
}

// loadEnvFiles reads env files referenced by the config's [tool] env_file setting.
// Paths are resolved relative to the config file's directory.
func loadEnvFiles(cfg *DesiredConfig, configPath string) map[string]string {
	if cfg.Tool == nil {
		return nil
	}
	envFiles := cfg.Tool.EnvFiles()
	if len(envFiles) == 0 {
		return nil
	}

	baseDir := filepath.Dir(configPath)
	merged := make(map[string]string)
	for _, ef := range envFiles {
		path := ef
		if !filepath.IsAbs(path) {
			path = filepath.Join(baseDir, path)
		}
		vars, err := ParseEnvFile(path)
		if err != nil {
			slog.Debug("skipping env file", "path", path, "error", err)
			continue
		}
		slog.Debug("loaded env file", "path", path, "vars", len(vars))
		for k, v := range vars {
			merged[k] = v
		}
	}
	if len(merged) == 0 {
		return nil
	}
	return merged
}

func globalConfigPath() string {
	configDir := os.Getenv("XDG_CONFIG_HOME")
	if configDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		configDir = filepath.Join(home, ".config")
	}
	return filepath.Join(configDir, "fat-controller", "config.toml")
}

// findOverrides detects variables in overlay that override variables in base.
func findOverrides(base, overlay *DesiredConfig, sourceName string) []Override {
	var overrides []Override
	if base.Variables != nil && overlay.Variables != nil {
		for k := range overlay.Variables {
			if _, ok := base.Variables[k]; ok {
				overrides = append(overrides, Override{
					Path: "variables." + k, Source: sourceName,
				})
			}
		}
	}
	for _, overlaySvc := range overlay.Services {
		if overlaySvc == nil {
			continue
		}
		baseSvc := findService(base.Services, overlaySvc)
		if baseSvc == nil {
			continue
		}
		for k := range overlaySvc.Variables {
			if _, ok := baseSvc.Variables[k]; ok {
				overrides = append(overrides, Override{
					Path: overlaySvc.Name + ".variables." + k, Source: sourceName,
				})
			}
		}
	}
	return overrides
}
