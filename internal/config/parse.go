package config

import (
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
)

// knownTopLevelKeys are keys that are NOT service names.
// These are tool settings or the shared section.
var knownTopLevelKeys = map[string]bool{
	"shared":              true,
	"sensitive_keywords":  true,
	"sensitive_allowlist": true,
	"suppress_warnings":   true,
}

// ParseFile reads and parses a TOML config file into a DesiredConfig.
func ParseFile(path string) (*DesiredConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(data)
}

// Parse parses TOML bytes into a DesiredConfig.
func Parse(data []byte) (*DesiredConfig, error) {
	var raw map[string]any
	if err := toml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	cfg := &DesiredConfig{}

	// Extract shared section.
	if sharedRaw, ok := raw["shared"]; ok {
		sharedMap, ok := sharedRaw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid [shared] section: expected table")
		}
		if varsRaw, ok := sharedMap["variables"]; ok {
			vars, err := toStringMap(varsRaw, "shared.variables")
			if err != nil {
				return nil, err
			}
			cfg.Shared = &DesiredVariables{Vars: vars}
		}
	}

	// Extract service sections (anything not a known top-level key).
	for key, val := range raw {
		if knownTopLevelKeys[key] {
			continue
		}
		svcMap, ok := val.(map[string]any)
		if !ok {
			continue // skip non-table top-level keys (future tool settings)
		}
		svc, err := parseService(key, svcMap)
		if err != nil {
			return nil, err
		}
		if cfg.Services == nil {
			cfg.Services = make(map[string]*DesiredService)
		}
		cfg.Services[key] = svc
	}

	return cfg, nil
}

func parseService(name string, raw map[string]any) (*DesiredService, error) {
	svc := &DesiredService{}

	if varsRaw, ok := raw["variables"]; ok {
		vars, err := toStringMap(varsRaw, name+".variables")
		if err != nil {
			return nil, err
		}
		svc.Variables = vars
	}

	if resRaw, ok := raw["resources"]; ok {
		resMap, ok := resRaw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid [%s.resources]: expected table", name)
		}
		res := &DesiredResources{}
		if v, ok := toFloat64(resMap["vcpus"]); ok {
			res.VCPUs = &v
		}
		if v, ok := toFloat64(resMap["memory_gb"]); ok {
			res.MemoryGB = &v
		}
		svc.Resources = res
	}

	if deployRaw, ok := raw["deploy"]; ok {
		deployMap, ok := deployRaw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid [%s.deploy]: expected table", name)
		}
		deploy := &DesiredDeploy{}
		if v, ok := deployMap["builder"].(string); ok {
			deploy.Builder = &v
		}
		if v, ok := deployMap["dockerfile_path"].(string); ok {
			deploy.DockerfilePath = &v
		}
		if v, ok := deployMap["root_directory"].(string); ok {
			deploy.RootDirectory = &v
		}
		if v, ok := deployMap["start_command"].(string); ok {
			deploy.StartCommand = &v
		}
		if v, ok := deployMap["healthcheck_path"].(string); ok {
			deploy.HealthcheckPath = &v
		}
		svc.Deploy = deploy
	}

	return svc, nil
}

// toStringMap converts a map[string]any to map[string]string, coercing
// non-string values via fmt.Sprint. Returns an error if the value is not
// a table (map[string]any).
func toStringMap(val any, section string) (map[string]string, error) {
	raw, ok := val.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid [%s]: expected table", section)
	}
	result := make(map[string]string, len(raw))
	for k, v := range raw {
		result[k] = fmt.Sprint(v)
	}
	return result, nil
}

// toFloat64 attempts to convert an any value to float64.
// TOML decodes integers as int64 and floats as float64.
func toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int64:
		return float64(n), true
	default:
		return 0, false
	}
}
