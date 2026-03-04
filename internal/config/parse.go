package config

import (
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
)

// knownTopLevelKeys are keys that are NOT service names.
// These are tool settings or the shared section.
var knownTopLevelKeys = map[string]bool{
	"project":             true,
	"environment":         true,
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

	// Extract project/environment metadata.
	if v, ok := raw["project"]; ok {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("invalid 'project': expected string, got %T", v)
		}
		cfg.Project = s
	}
	if v, ok := raw["environment"]; ok {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("invalid 'environment': expected string, got %T", v)
		}
		cfg.Environment = s
	}

	// Extract sensitive_keywords, sensitive_allowlist, suppress_warnings.
	if v, ok := raw["sensitive_keywords"]; ok {
		sl, err := toStringSlice(v, "sensitive_keywords")
		if err != nil {
			return nil, err
		}
		cfg.SensitiveKeywords = sl
	}
	if v, ok := raw["sensitive_allowlist"]; ok {
		sl, err := toStringSlice(v, "sensitive_allowlist")
		if err != nil {
			return nil, err
		}
		cfg.SensitiveAllowlist = sl
	}
	if v, ok := raw["suppress_warnings"]; ok {
		sl, err := toStringSlice(v, "suppress_warnings")
		if err != nil {
			return nil, err
		}
		cfg.SuppressWarnings = sl
	}

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
			return nil, fmt.Errorf("unrecognised config key %q (not a known setting or service table)", key)
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

// toStringSlice converts an any value to []string.
// Returns an error if the value is not an array of strings.
func toStringSlice(val any, field string) ([]string, error) {
	arr, ok := val.([]any)
	if !ok {
		return nil, fmt.Errorf("invalid '%s': expected array of strings, got %T", field, val)
	}
	result := make([]string, 0, len(arr))
	for i, item := range arr {
		s, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("invalid '%s[%d]': expected string, got %T", field, i, item)
		}
		result = append(result, s)
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
