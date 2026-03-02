package config

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"
)

// Render renders the live config in the requested output format.
func Render(cfg LiveConfig, format string, full bool) (string, error) {
	switch format {
	case "json":
		buf, err := json.MarshalIndent(toJSONMap(cfg), "", "  ")
		if err != nil {
			return "", err
		}
		return string(buf), nil
	case "toml":
		return renderTOML(cfg), nil
	case "text", "":
		return renderText(cfg, full), nil
	default:
		return "", errors.New("unsupported output format")
	}
}

// toJSONMap builds a clean map for JSON output (no Go struct field names).
func toJSONMap(cfg LiveConfig) map[string]any {
	m := map[string]any{}
	if len(cfg.Shared) > 0 {
		m["shared_variables"] = cfg.Shared
	}
	for name, svc := range cfg.Services {
		m[name] = map[string]any{"variables": svc.Variables}
	}
	return m
}

// renderTOML builds TOML-style output using the same section structure as text.
func renderTOML(cfg LiveConfig) string {
	var out strings.Builder
	if len(cfg.Shared) > 0 {
		out.WriteString("[shared_variables]\n")
		keys := sortedKeys(cfg.Shared)
		for _, k := range keys {
			out.WriteString(k + " = \"" + cfg.Shared[k] + "\"\n")
		}
		out.WriteString("\n")
	}
	serviceNames := make([]string, 0, len(cfg.Services))
	for name := range cfg.Services {
		serviceNames = append(serviceNames, name)
	}
	sort.Strings(serviceNames)
	for _, name := range serviceNames {
		svc := cfg.Services[name]
		if len(svc.Variables) > 0 {
			out.WriteString("[" + svc.Name + ".variables]\n")
			keys := sortedKeys(svc.Variables)
			for _, k := range keys {
				out.WriteString(k + " = \"" + svc.Variables[k] + "\"\n")
			}
			out.WriteString("\n")
		}
	}
	return strings.TrimRight(out.String(), "\n")
}

func renderText(cfg LiveConfig, full bool) string {
	var out strings.Builder

	if len(cfg.Shared) > 0 {
		out.WriteString("[shared_variables]\n")
		keys := sortedKeys(cfg.Shared)
		for _, k := range keys {
			out.WriteString(k + " = \"" + cfg.Shared[k] + "\"\n")
		}
		out.WriteString("\n")
	}

	serviceNames := make([]string, 0, len(cfg.Services))
	for name := range cfg.Services {
		serviceNames = append(serviceNames, name)
	}
	sort.Strings(serviceNames)
	for _, name := range serviceNames {
		svc := cfg.Services[name]
		if len(svc.Variables) > 0 {
			out.WriteString("[" + svc.Name + ".variables]\n")
			keys := sortedKeys(svc.Variables)
			for _, k := range keys {
				out.WriteString(k + " = \"" + svc.Variables[k] + "\"\n")
			}
			out.WriteString("\n")
		}
	}

	return strings.TrimRight(out.String(), "\n")
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
