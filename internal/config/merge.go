package config

// Merge combines multiple DesiredConfig values in order (later wins).
// Variable maps are merged at the key level. Resources/Deploy fields are
// merged at the field level (non-nil overrides nil).
func Merge(configs ...*DesiredConfig) *DesiredConfig {
	result := &DesiredConfig{
		Services: make(map[string]*DesiredService),
	}
	for _, cfg := range configs {
		if cfg == nil {
			continue
		}
		result.Shared = mergeVariables(result.Shared, cfg.Shared)
		for name, svc := range cfg.Services {
			existing, ok := result.Services[name]
			if !ok {
				existing = &DesiredService{}
				result.Services[name] = existing
			}
			mergeService(existing, svc)
		}
	}
	return result
}

func mergeVariables(base, overlay *DesiredVariables) *DesiredVariables {
	if overlay == nil {
		return base
	}
	if base == nil {
		base = &DesiredVariables{Vars: make(map[string]string)}
	}
	for k, v := range overlay.Vars {
		base.Vars[k] = v
	}
	return base
}

func mergeService(base, overlay *DesiredService) {
	// Merge variables.
	if overlay.Variables != nil {
		if base.Variables == nil {
			base.Variables = make(map[string]string)
		}
		for k, v := range overlay.Variables {
			base.Variables[k] = v
		}
	}

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
