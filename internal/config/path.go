package config

import (
	"errors"
	"strings"
)

// Path is a parsed dot-path like "service.section.key".
type Path struct {
	Service string
	Section string
	Key     string
}

// IsTopLevel returns true for workspace, project, tool, or variables paths
// that don't refer to a specific service.
func (p Path) IsTopLevel() bool {
	switch p.Service {
	case "workspace", "project", "tool", "variables":
		return true
	}
	return false
}

// ParsePath parses a dot-path into components.
// Top-level paths: "workspace", "workspace.name", "project", "project.id",
// "tool", "tool.api_timeout", "variables", "variables.KEY".
// Service paths: "api", "api.variables", "api.variables.PORT", "api.deploy.builder".
func ParsePath(input string) (Path, error) {
	if strings.TrimSpace(input) == "" {
		return Path{}, errors.New("path cannot be empty")
	}
	parts := strings.Split(input, ".")
	if len(parts) > 3 {
		return Path{}, errors.New("path must have 1 to 3 segments")
	}
	for _, p := range parts {
		if p == "" {
			return Path{}, errors.New("path segments cannot be empty")
		}
	}
	path := Path{Service: parts[0]}
	if len(parts) > 1 {
		path.Section = parts[1]
	}
	if len(parts) > 2 {
		path.Key = parts[2]
	}
	return path, nil
}

// ResolveValue looks up a value in a DesiredConfig by path.
// Returns the value and true if found, or "" and false if not.
func ResolveValue(cfg *DesiredConfig, p Path) (string, bool) {
	if cfg == nil {
		return "", false
	}
	switch p.Service {
	case "workspace":
		if cfg.Workspace == nil {
			return "", false
		}
		return resolveContextBlock(cfg.Workspace, p.Section)
	case "project":
		if cfg.Project == nil {
			return "", false
		}
		return resolveContextBlock(cfg.Project, p.Section)
	case "variables":
		if p.Section != "" {
			v, ok := cfg.Variables[p.Section]
			return v, ok
		}
		return "", false
	case "tool":
		return "", false // tool settings are complex; not scalar-resolvable
	default:
		// Service path.
		for _, svc := range cfg.Services {
			if svc.Name == p.Service {
				return resolveServiceValue(svc, p.Section, p.Key)
			}
		}
		return "", false
	}
}

func resolveContextBlock(cb *ContextBlock, field string) (string, bool) {
	switch field {
	case "", "name":
		return cb.Name, cb.Name != ""
	case "id":
		return cb.ID, cb.ID != ""
	default:
		return "", false
	}
}

func resolveServiceValue(svc *DesiredService, section, key string) (string, bool) {
	switch section {
	case "variables":
		if key != "" {
			v, ok := svc.Variables[key]
			return v, ok
		}
	case "":
		return svc.Name, true
	}
	return "", false
}
