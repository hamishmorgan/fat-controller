package app

import (
	"sort"
	"strings"

	"github.com/hamishmorgan/fat-controller/internal/config"
)

// ScopeLiveByPath narrows a LiveConfig to only include the service or
// section specified by path.
func ScopeLiveByPath(live *config.LiveConfig, path string) *config.LiveConfig {
	if live == nil || path == "" {
		return live
	}
	if path == "variables" {
		return &config.LiveConfig{
			ProjectID:     live.ProjectID,
			EnvironmentID: live.EnvironmentID,
			Variables:     live.Variables,
			Services:      map[string]*config.ServiceConfig{},
		}
	}
	// Check for service name match.
	parts := splitDotPath(path)
	svcName := parts[0]
	if svc, ok := live.Services[svcName]; ok {
		return &config.LiveConfig{
			ProjectID:     live.ProjectID,
			EnvironmentID: live.EnvironmentID,
			Variables:     nil,
			Services:      map[string]*config.ServiceConfig{svcName: svc},
		}
	}
	// Unrecognized path — return empty (no services matched).
	return &config.LiveConfig{
		ProjectID:     live.ProjectID,
		EnvironmentID: live.EnvironmentID,
		Variables:     nil,
		Services:      map[string]*config.ServiceConfig{},
	}
}

// AdoptMerge merges live state into an existing desired config, producing
// a LiveConfig suitable for rendering. MergeFlags control what gets included:
//   - create: add services/variables that exist in live but not config
//   - update: overwrite services/variables that exist in both
//   - delete: remove services/variables from config that don't exist in live
//
// When path is set, merge flags only apply within the scoped section;
// everything outside the path is preserved from the existing config.
func AdoptMerge(desired *config.DesiredConfig, live *config.LiveConfig, create, update, del bool, path string) *config.LiveConfig {
	result := &config.LiveConfig{
		ProjectID:     live.ProjectID,
		EnvironmentID: live.EnvironmentID,
		Variables:     make(map[string]string),
		Services:      make(map[string]*config.ServiceConfig),
	}

	// Build set of desired service names.
	desiredSvcNames := make(map[string]bool, len(desired.Services))
	for _, svc := range desired.Services {
		desiredSvcNames[svc.Name] = true
	}

	// Determine if variables are in scope.
	varsInScope := path == "" || path == "variables"
	// Determine which services are in scope.
	// When path == "variables", no services are in scope.
	// When path == "", all services are in scope (scopedService stays "").
	// When path == "api", only "api" is in scope.
	svcsInScope := path != "variables"
	scopedService := ""
	if path != "" && path != "variables" {
		parts := splitDotPath(path)
		scopedService = parts[0]
	}

	// --- Variables ---
	if varsInScope {
		// Start with desired variables (converted to live format).
		desiredVars := make(map[string]bool, len(desired.Variables))
		for k := range desired.Variables {
			desiredVars[k] = true
		}

		// Add/update from live.
		for k, v := range live.Variables {
			inDesired := desiredVars[k]
			if inDesired && update {
				result.Variables[k] = v
			} else if inDesired && !update {
				// Keep existing desired value.
				result.Variables[k] = desired.Variables[k]
			} else if !inDesired && create {
				result.Variables[k] = v
			}
			// else: not in desired and !create → skip
		}

		// Keep desired variables not in live (unless delete).
		if !del {
			for k, v := range desired.Variables {
				if _, inLive := live.Variables[k]; !inLive {
					result.Variables[k] = v
				}
			}
		}
	} else {
		// Variables not in scope — preserve from desired as-is.
		for k, v := range desired.Variables {
			result.Variables[k] = v
		}
	}

	// --- Services ---
	if svcsInScope {
		for svcName, liveSvc := range live.Services {
			inScope := scopedService == "" || scopedService == svcName
			inDesired := desiredSvcNames[svcName]

			if inDesired && inScope && update {
				// Update: take live state.
				result.Services[svcName] = liveSvc
			} else if inDesired && inScope && !update {
				// Keep existing (convert desired → live for rendering).
				result.Services[svcName] = DesiredServiceToLive(FindDesiredService(desired, svcName), liveSvc)
			} else if inDesired && !inScope {
				// Out of scope — use live if available (for rendering), preserve existing.
				result.Services[svcName] = liveSvc
			} else if !inDesired && inScope && create {
				// Create: add from live.
				result.Services[svcName] = liveSvc
			}
			// else: not in desired, not creating → skip
		}

		// Keep desired services not in live (unless delete and in scope).
		for _, svc := range desired.Services {
			if _, inLive := live.Services[svc.Name]; inLive {
				continue // already handled above
			}
			inScope := scopedService == "" || scopedService == svc.Name
			if del && inScope {
				continue // delete: remove from result
			}
			// Preserve the service (convert desired → live for rendering).
			result.Services[svc.Name] = DesiredServiceToLive(svc, nil)
		}
	} else {
		// Services not in scope — preserve from desired as-is.
		for _, svc := range desired.Services {
			liveSvc := live.Services[svc.Name]
			result.Services[svc.Name] = DesiredServiceToLive(svc, liveSvc)
		}
	}

	return result
}

// DesiredServiceToLive converts a DesiredService to a ServiceConfig for
// rendering, using liveSvc to fill in ID and other live-only fields when available.
func DesiredServiceToLive(ds *config.DesiredService, liveSvc *config.ServiceConfig) *config.ServiceConfig {
	if ds == nil {
		return nil
	}
	sc := &config.ServiceConfig{
		Name:      ds.Name,
		ID:        ds.ID,
		Icon:      ds.Icon,
		Variables: make(map[string]string, len(ds.Variables)),
	}
	for k, v := range ds.Variables {
		sc.Variables[k] = v
	}
	if ds.Deploy != nil {
		sc.Deploy = DesiredDeployToLive(ds.Deploy)
	}
	if ds.Resources != nil {
		sc.VCPUs = ds.Resources.VCPUs
		sc.MemoryGB = ds.Resources.MemoryGB
	}
	// Use live ID if desired doesn't have one.
	if sc.ID == "" && liveSvc != nil {
		sc.ID = liveSvc.ID
	}
	// Carry forward live sub-resources (domains, volumes, etc.) if available.
	if liveSvc != nil {
		sc.Domains = liveSvc.Domains
		sc.Volumes = liveSvc.Volumes
		sc.TCPProxies = liveSvc.TCPProxies
		sc.Triggers = liveSvc.Triggers
		sc.Egress = liveSvc.Egress
		sc.Network = liveSvc.Network
	}
	return sc
}

// DesiredDeployToLive converts a DesiredDeploy to a live Deploy struct.
func DesiredDeployToLive(dd *config.DesiredDeploy) config.Deploy {
	d := config.Deploy{}
	if dd.Builder != nil {
		d.Builder = *dd.Builder
	}
	d.Repo = dd.Repo
	d.Image = dd.Image
	d.BuildCommand = dd.BuildCommand
	d.DockerfilePath = dd.DockerfilePath
	d.RootDirectory = dd.RootDirectory
	d.WatchPatterns = dd.WatchPatterns
	d.StartCommand = dd.StartCommand
	d.CronSchedule = dd.CronSchedule
	d.HealthcheckPath = dd.HealthcheckPath
	d.HealthcheckTimeout = dd.HealthcheckTimeout
	if dd.RestartPolicy != nil {
		d.RestartPolicy = *dd.RestartPolicy
	}
	d.RestartPolicyMaxRetries = dd.RestartPolicyMaxRetries
	d.DrainingSeconds = dd.DrainingSeconds
	d.OverlapSeconds = dd.OverlapSeconds
	d.SleepApplication = dd.SleepApplication
	d.NumReplicas = dd.NumReplicas
	d.Region = dd.Region
	d.IPv6Egress = dd.IPv6Egress
	return d
}

// FindDesiredService returns the DesiredService with the given name, or nil.
func FindDesiredService(cfg *config.DesiredConfig, name string) *config.DesiredService {
	for _, svc := range cfg.Services {
		if svc.Name == name {
			return svc
		}
	}
	return nil
}

// JoinServiceNames returns a comma-separated list of service names from a LiveConfig.
func JoinServiceNames(cfg *config.LiveConfig) string {
	names := make([]string, 0, len(cfg.Services))
	for name := range cfg.Services {
		names = append(names, name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}
