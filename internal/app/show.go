package app

import "github.com/hamishmorgan/fat-controller/internal/config"

// LookupKey retrieves a single value from the config for a fully-qualified path.
func LookupKey(cfg config.LiveConfig, p config.Path) (string, bool) {
	switch p.Section {
	case "variables":
		if p.Service == "shared" {
			val, found := cfg.Variables[p.Key]
			return val, found
		}
		svc, ok := cfg.Services[p.Service]
		if !ok {
			return "", false
		}
		val, found := svc.Variables[p.Key]
		return val, found
	default:
		return "", false
	}
}

// FilterSection returns a copy of cfg containing only the requested section.
func FilterSection(cfg config.LiveConfig, p config.Path) config.LiveConfig {
	filtered := config.LiveConfig{
		ProjectID:     cfg.ProjectID,
		EnvironmentID: cfg.EnvironmentID,
	}
	if p.Section != "variables" {
		return filtered
	}
	if p.Service == "shared" {
		filtered.Variables = cfg.Variables
		return filtered
	}
	if cfg.Services == nil {
		return filtered
	}
	svc, ok := cfg.Services[p.Service]
	if !ok {
		return filtered
	}
	filtered.Services = map[string]*config.ServiceConfig{
		p.Service: {
			ID:        svc.ID,
			Name:      svc.Name,
			Variables: svc.Variables,
		},
	}
	return filtered
}
