package config

import "log/slog"

// Merge combines multiple DesiredConfig values in order (later wins).
// Variable maps are merged at the key level. Resources/Deploy fields are
// merged at the field level (non-nil overrides nil).
func Merge(configs ...*DesiredConfig) *DesiredConfig {
	result := &DesiredConfig{}
	for _, cfg := range configs {
		if cfg == nil {
			continue
		}
		// Merge name: non-empty overrides.
		if cfg.Name != "" {
			slog.Debug("config override", "field", "name", "value", cfg.Name)
			result.Name = cfg.Name
		}
		if cfg.Workspace != nil {
			slog.Debug("config override", "field", "workspace", "value", cfg.Workspace.Name)
			result.Workspace = cfg.Workspace
		}
		if cfg.Project != nil {
			slog.Debug("config override", "field", "project", "value", cfg.Project.Name)
			result.Project = cfg.Project
		}
		if cfg.Tool != nil {
			result.Tool = cfg.Tool
		}
		result.Variables = mergeVarMaps(result.Variables, cfg.Variables)
		for _, svc := range cfg.Services {
			existing := findServiceByName(result.Services, svc.Name)
			if existing == nil {
				existing = &DesiredService{Name: svc.Name}
				result.Services = append(result.Services, existing)
			}
			mergeService(existing, svc)
		}
	}
	return result
}

// findServiceByName finds a service by name in a slice.
func findServiceByName(services []*DesiredService, name string) *DesiredService {
	for _, svc := range services {
		if svc.Name == name {
			return svc
		}
	}
	return nil
}

func mergeVarMaps(base, overlay Variables) Variables {
	if overlay == nil {
		return base
	}
	if base == nil {
		base = make(Variables, len(overlay))
	}
	for k, v := range overlay {
		base[k] = v
	}
	return base
}

func mergeService(base, overlay *DesiredService) {
	// Merge variables.
	base.Variables = mergeVarMaps(base.Variables, overlay.Variables)

	// Merge resources (field-level).
	if overlay.Resources != nil {
		if base.Resources == nil {
			base.Resources = &DesiredResources{}
		}
		if overlay.Resources.VCPUs != nil {
			base.Resources.VCPUs = overlay.Resources.VCPUs
		}
		if overlay.Resources.MemoryGB != nil {
			base.Resources.MemoryGB = overlay.Resources.MemoryGB
		}
	}

	// Merge deploy (field-level).
	if overlay.Deploy != nil {
		if base.Deploy == nil {
			base.Deploy = &DesiredDeploy{}
		}
		if overlay.Deploy.Builder != nil {
			base.Deploy.Builder = overlay.Deploy.Builder
		}
		if overlay.Deploy.DockerfilePath != nil {
			base.Deploy.DockerfilePath = overlay.Deploy.DockerfilePath
		}
		if overlay.Deploy.RootDirectory != nil {
			base.Deploy.RootDirectory = overlay.Deploy.RootDirectory
		}
		if overlay.Deploy.StartCommand != nil {
			base.Deploy.StartCommand = overlay.Deploy.StartCommand
		}
		if overlay.Deploy.HealthcheckPath != nil {
			base.Deploy.HealthcheckPath = overlay.Deploy.HealthcheckPath
		}
	}
}
