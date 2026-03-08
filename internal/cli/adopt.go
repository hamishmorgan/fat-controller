package cli

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hamishmorgan/fat-controller/internal/config"
	"github.com/hamishmorgan/fat-controller/internal/prompt"
)

// AdoptCmd implements the `adopt` command.
type AdoptCmd struct {
	ServiceFlags    `kong:"embed"`
	MergeFlags      `kong:"embed"`
	PromptFlags     `kong:"embed"`
	ConfigFileFlags `kong:"embed"`
	DryRun          bool   `help:"Force preview of mutations." name:"dry-run" env:"FAT_CONTROLLER_DRY_RUN"`
	ShowSecrets     bool   `help:"Show secret values instead of masking." name:"show-secrets" env:"FAT_CONTROLLER_SHOW_SECRETS"`
	Path            string `arg:"" optional:"" help:"Dot-path to scope adoption (e.g. api)."`
}

// Run implements `adopt`.
func (c *AdoptCmd) Run(globals *Globals) error {
	ctx, cancel := c.TimeoutContext(globals.BaseCtx)
	defer cancel()
	client, err := newClient(&c.ApiFlags, globals.BaseCtx)
	if err != nil {
		return err
	}

	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	interactive := prompt.StdinIsInteractive()
	if c.PromptMode() == "all" {
		interactive = true
	} else if c.PromptMode() == "none" {
		interactive = false
	}

	// Try to load an existing config. If none exists, fall back to the
	// wizard-style init flow for first-time bootstrap.
	result, loadErr := config.LoadCascade(config.LoadOptions{WorkDir: wd})
	if loadErr != nil || result == nil || result.Config == nil {
		slog.Debug("no existing config found, using init wizard")
		resolver := &railwayInitResolver{client: client}
		return RunConfigInit(ctx, wd, c.Workspace, c.Project, c.Environment, resolver, interactive, c.DryRun, c.Yes, os.Stdout)
	}

	// Existing config found — run the merge-based adopt flow.
	desired := result.Config
	out := os.Stdout

	// Interpolate ${VAR} references so we can resolve names.
	if err := config.Interpolate(desired, result.EnvVars); err != nil {
		return err
	}

	// Resolve workspace/project/environment from flags → config fallback.
	project := c.Project
	if project == "" && desired.Project != nil {
		project = desired.Project.Name
	}
	environment := c.Environment
	if environment == "" {
		environment = desired.Name
	}
	workspace := c.Workspace
	if workspace == "" && desired.Workspace != nil {
		workspace = desired.Workspace.Name
	}

	fetcher := &defaultConfigFetcher{client: client}
	projID, envID, err := fetcher.Resolve(ctx, workspace, project, environment)
	if err != nil {
		return err
	}

	// Fetch live state.
	slog.Debug("fetching live state", "project_id", projID, "environment_id", envID)
	var adoptFilter []string
	if c.Service != "" {
		adoptFilter = []string{c.Service}
	}
	live, err := fetcher.Fetch(ctx, projID, envID, adoptFilter)
	if err != nil {
		return err
	}

	// Scope by path if provided.
	scopedLive := live
	if c.Path != "" {
		scopedLive = scopeLiveByPath(live, c.Path)
	}

	// Build the adopted config by merging live into existing desired,
	// respecting MergeFlags (create/update/delete).
	adopted := adoptMerge(desired, scopedLive, c.Create, c.Update, c.Delete, c.Path)

	// Use the workspace/project names from the resolved context.
	wsName := workspace
	if desired.Workspace != nil && desired.Workspace.Name != "" {
		wsName = desired.Workspace.Name
	}
	projName := project
	if desired.Project != nil && desired.Project.Name != "" {
		projName = desired.Project.Name
	}
	envName := environment
	if desired.Name != "" {
		envName = desired.Name
	}

	// Render the adopted config.
	content := config.RenderInitTOML(wsName, projName, envName, *adopted)

	// Summarize what changed.
	_, _ = fmt.Fprintf(out, "  Workspace: %s\n", wsName)
	_, _ = fmt.Fprintf(out, "  Project: %s\n", projName)
	_, _ = fmt.Fprintf(out, "  Environment: %s\n", envName)
	_, _ = fmt.Fprintf(out, "  Services: %s (%d)\n", joinServiceNames(adopted), len(adopted.Services))
	_, _ = fmt.Fprintln(out)

	envFileName := config.DefaultEnvFile
	envContent := renderEnvFile(adopted)

	if c.DryRun {
		_, _ = fmt.Fprintf(out, "dry run: would write %s (%d services)\n\n%s\n",
			config.BaseConfigFile, len(adopted.Services), content)
		if envContent != "" {
			_, _ = fmt.Fprintf(out, "\ndry run: would write %s\n\n%s\n",
				envFileName, envContent)
		}
		return nil
	}

	if !c.Yes {
		if !interactive {
			_, _ = fmt.Fprintf(out, "would write %s (%d services)\n\n%s\n",
				config.BaseConfigFile, len(adopted.Services), content)
			if envContent != "" {
				_, _ = fmt.Fprintf(out, "\nwould write %s\n\n%s\n", envFileName, envContent)
			}
			_, _ = fmt.Fprintln(out, "use --yes to write files")
			return nil
		}

		_, _ = fmt.Fprintf(out, "Will write %s (%d services):\n\n%s\n\n",
			config.BaseConfigFile, len(adopted.Services), content)
		confirmed, err := prompt.Confirm("Write changes?", true)
		if err != nil {
			return fmt.Errorf("reading confirmation: %w", err)
		}
		if !confirmed {
			_, _ = fmt.Fprintln(out, "Adopt cancelled.")
			return nil
		}
	}

	// Write the config file.
	configPath := result.PrimaryFile
	if configPath == "" {
		configPath = filepath.Join(wd, config.BaseConfigFile)
	}
	if err := os.WriteFile(configPath, []byte(content+"\n"), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", config.BaseConfigFile, err)
	}
	_, _ = fmt.Fprintf(out, "wrote %s (%d services)\n", config.BaseConfigFile, len(adopted.Services))

	// Write env file if there are secrets.
	if envContent != "" {
		envPath := filepath.Join(wd, envFileName)
		writeEnv, err := confirmWrite(envPath, envFileName, c.Yes, interactive)
		if err != nil {
			return err
		}
		if writeEnv {
			if err := os.WriteFile(envPath, []byte(envContent), 0o600); err != nil {
				return fmt.Errorf("writing %s: %w", envFileName, err)
			}
			_, _ = fmt.Fprintf(out, "wrote %s (secret values — do not commit)\n", envFileName)

			added, err := ensureGitignoreHasLine(wd, envFileName)
			if err != nil {
				return fmt.Errorf("updating .gitignore: %w", err)
			}
			if added {
				_, _ = fmt.Fprintf(out, "updated .gitignore (added %s)\n", envFileName)
			}
		}
	}

	return nil
}

// scopeLiveByPath narrows a LiveConfig to only include the service or
// section specified by path.
func scopeLiveByPath(live *config.LiveConfig, path string) *config.LiveConfig {
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

// adoptMerge merges live state into an existing desired config, producing
// a LiveConfig suitable for rendering. MergeFlags control what gets included:
//   - create: add services/variables that exist in live but not config
//   - update: overwrite services/variables that exist in both
//   - delete: remove services/variables from config that don't exist in live
//
// When path is set, merge flags only apply within the scoped section;
// everything outside the path is preserved from the existing config.
func adoptMerge(desired *config.DesiredConfig, live *config.LiveConfig, create, update, del bool, path string) *config.LiveConfig {
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
				result.Services[svcName] = desiredServiceToLive(findDesiredService(desired, svcName), liveSvc)
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
			result.Services[svc.Name] = desiredServiceToLive(svc, nil)
		}
	} else {
		// Services not in scope — preserve from desired as-is.
		for _, svc := range desired.Services {
			liveSvc := live.Services[svc.Name]
			result.Services[svc.Name] = desiredServiceToLive(svc, liveSvc)
		}
	}

	return result
}

// desiredServiceToLive converts a DesiredService to a ServiceConfig for
// rendering, using liveSvc to fill in ID and other live-only fields when available.
func desiredServiceToLive(ds *config.DesiredService, liveSvc *config.ServiceConfig) *config.ServiceConfig {
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
		sc.Deploy = desiredDeployToLive(ds.Deploy)
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

// desiredDeployToLive converts a DesiredDeploy to a live Deploy struct.
func desiredDeployToLive(dd *config.DesiredDeploy) config.Deploy {
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

// findDesiredService returns the DesiredService with the given name, or nil.
func findDesiredService(cfg *config.DesiredConfig, name string) *config.DesiredService {
	for _, svc := range cfg.Services {
		if svc.Name == name {
			return svc
		}
	}
	return nil
}

// joinServiceNames returns a comma-separated list of service names from a LiveConfig.
func joinServiceNames(cfg *config.LiveConfig) string {
	names := make([]string, 0, len(cfg.Services))
	for name := range cfg.Services {
		names = append(names, name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// splitDotPath splits a dot-separated path into parts.
// TODO: Remove when adopt merge logic moves to app package (Task 2).
func splitDotPath(path string) []string {
	if path == "" {
		return nil
	}
	var parts []string
	start := 0
	for i, c := range path {
		if c == '.' {
			parts = append(parts, path[start:i])
			start = i + 1
		}
	}
	parts = append(parts, path[start:])
	return parts
}
