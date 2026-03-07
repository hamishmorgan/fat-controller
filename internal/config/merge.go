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
			existing := findService(result.Services, svc)
			if existing == nil {
				existing = &DesiredService{Name: svc.Name}
				result.Services = append(result.Services, existing)
			}
			mergeService(existing, svc)
		}
	}
	return result
}

// findService finds a service by ID (preferred) then name in a slice.
func findService(services []*DesiredService, svc *DesiredService) *DesiredService {
	// Match by ID first if both sides have one.
	if svc.ID != "" {
		for _, s := range services {
			if s.ID != "" && s.ID == svc.ID {
				return s
			}
		}
	}
	// Fall back to name match.
	for _, s := range services {
		if s.Name == svc.Name {
			return s
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
	// Merge identity fields.
	if overlay.Name != "" {
		base.Name = overlay.Name
	}
	if overlay.ID != "" {
		base.ID = overlay.ID
	}
	if overlay.Icon != "" {
		base.Icon = overlay.Icon
	}
	if overlay.Delete {
		base.Delete = true
	}

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

	// Merge deploy (field-level, non-nil pointer overwrites).
	if overlay.Deploy != nil {
		if base.Deploy == nil {
			base.Deploy = &DesiredDeploy{}
		}
		mergeDeploy(base.Deploy, overlay.Deploy)
	}

	// Merge sub-resources.
	if overlay.Scale != nil {
		if base.Scale == nil {
			base.Scale = make(map[string]int, len(overlay.Scale))
		}
		for k, v := range overlay.Scale {
			base.Scale[k] = v
		}
	}
	if overlay.Domains != nil {
		if base.Domains == nil {
			base.Domains = make(map[string]DomainConfig, len(overlay.Domains))
		}
		for k, v := range overlay.Domains {
			base.Domains[k] = v
		}
	}
	if overlay.Volumes != nil {
		if base.Volumes == nil {
			base.Volumes = make(map[string]VolumeConfig, len(overlay.Volumes))
		}
		for k, v := range overlay.Volumes {
			base.Volumes[k] = v
		}
	}
	if overlay.TCPProxies != nil {
		base.TCPProxies = overlay.TCPProxies
	}
	if overlay.Network != nil {
		base.Network = overlay.Network
	}
	if overlay.Triggers != nil {
		base.Triggers = overlay.Triggers
	}
	if overlay.Egress != nil {
		base.Egress = overlay.Egress
	}
}

// mergeDeploy merges all deploy fields (non-nil pointer overwrites).
func mergeDeploy(base, overlay *DesiredDeploy) {
	// Source
	if overlay.Repo != nil {
		base.Repo = overlay.Repo
	}
	if overlay.Image != nil {
		base.Image = overlay.Image
	}
	if overlay.Branch != nil {
		base.Branch = overlay.Branch
	}
	if overlay.RegistryCredentials != nil {
		base.RegistryCredentials = overlay.RegistryCredentials
	}
	// Build
	if overlay.Builder != nil {
		base.Builder = overlay.Builder
	}
	if overlay.BuildCommand != nil {
		base.BuildCommand = overlay.BuildCommand
	}
	if overlay.DockerfilePath != nil {
		base.DockerfilePath = overlay.DockerfilePath
	}
	if overlay.RootDirectory != nil {
		base.RootDirectory = overlay.RootDirectory
	}
	if overlay.NixpacksPlan != nil {
		base.NixpacksPlan = overlay.NixpacksPlan
	}
	if overlay.WatchPatterns != nil {
		base.WatchPatterns = overlay.WatchPatterns
	}
	// Run
	if overlay.StartCommand != nil {
		base.StartCommand = overlay.StartCommand
	}
	if overlay.PreDeployCommand != nil {
		base.PreDeployCommand = overlay.PreDeployCommand
	}
	if overlay.CronSchedule != nil {
		base.CronSchedule = overlay.CronSchedule
	}
	// Health
	if overlay.HealthcheckPath != nil {
		base.HealthcheckPath = overlay.HealthcheckPath
	}
	if overlay.HealthcheckTimeout != nil {
		base.HealthcheckTimeout = overlay.HealthcheckTimeout
	}
	if overlay.RestartPolicy != nil {
		base.RestartPolicy = overlay.RestartPolicy
	}
	if overlay.RestartPolicyMaxRetries != nil {
		base.RestartPolicyMaxRetries = overlay.RestartPolicyMaxRetries
	}
	// Deploy strategy
	if overlay.DrainingSeconds != nil {
		base.DrainingSeconds = overlay.DrainingSeconds
	}
	if overlay.OverlapSeconds != nil {
		base.OverlapSeconds = overlay.OverlapSeconds
	}
	if overlay.SleepApplication != nil {
		base.SleepApplication = overlay.SleepApplication
	}
	// Placement
	if overlay.NumReplicas != nil {
		base.NumReplicas = overlay.NumReplicas
	}
	if overlay.Region != nil {
		base.Region = overlay.Region
	}
	// Networking
	if overlay.IPv6Egress != nil {
		base.IPv6Egress = overlay.IPv6Egress
	}
}
