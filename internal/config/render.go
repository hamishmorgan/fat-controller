package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

// ---------- TOML render structs ----------
// These structs exist solely for toml.Marshal output. They mirror the config
// model but use toml struct tags and omitempty to produce clean output.

// tomlShowConfig is the top-level struct for `show --toml`.
type tomlShowConfig struct {
	ProjectID     string            `toml:"project_id,omitempty"`
	EnvironmentID string            `toml:"environment_id,omitempty"`
	Variables     map[string]string `toml:"variables,omitempty"`
	Service       []tomlService     `toml:"service,omitempty"`
}

// tomlService represents one [[service]] entry in rendered TOML.
type tomlService struct {
	Name       string                `toml:"name"`
	ID         string                `toml:"id,omitempty"`
	Icon       string                `toml:"icon,omitempty"`
	Variables  map[string]string     `toml:"variables,omitempty"`
	Deploy     *tomlDeploy           `toml:"deploy,omitempty"`
	Resources  *tomlResources        `toml:"resources,omitempty"`
	Domains    map[string]tomlDomain `toml:"domains,omitempty"`
	Volumes    map[string]tomlVolume `toml:"volumes,omitempty"`
	TCPProxies []tomlTCPProxy        `toml:"tcp_proxies,omitempty"`
	Network    *tomlNetwork          `toml:"network,omitempty"`
	Triggers   []tomlTrigger         `toml:"triggers,omitempty"`
	Egress     []tomlEgress          `toml:"egress,omitempty"`
}

// tomlDeploy mirrors Deploy with toml tags for marshalling.
type tomlDeploy struct {
	Builder                 string   `toml:"builder,omitempty"`
	Repo                    *string  `toml:"repo,omitempty"`
	Image                   *string  `toml:"image,omitempty"`
	BuildCommand            *string  `toml:"build_command,omitempty"`
	DockerfilePath          *string  `toml:"dockerfile_path,omitempty"`
	RootDirectory           *string  `toml:"root_directory,omitempty"`
	StartCommand            *string  `toml:"start_command,omitempty"`
	CronSchedule            *string  `toml:"cron_schedule,omitempty"`
	HealthcheckPath         *string  `toml:"healthcheck_path,omitempty"`
	HealthcheckTimeout      *int     `toml:"healthcheck_timeout,omitempty"`
	RestartPolicy           string   `toml:"restart_policy,omitempty"`
	RestartPolicyMaxRetries *int     `toml:"restart_policy_max_retries,omitempty"`
	DrainingSeconds         *int     `toml:"draining_seconds,omitempty"`
	OverlapSeconds          *int     `toml:"overlap_seconds,omitempty"`
	WatchPatterns           []string `toml:"watch_patterns,omitempty"`
	PreDeployCommand        []string `toml:"pre_deploy_command,omitempty"`
	SleepApplication        *bool    `toml:"sleep_application,omitempty"`
	NumReplicas             *int     `toml:"num_replicas,omitempty"`
	Region                  *string  `toml:"region,omitempty"`
	IPv6Egress              *bool    `toml:"ipv6_egress,omitempty"`
}

type tomlResources struct {
	VCPUs    *float64 `toml:"vcpus,omitempty"`
	MemoryGB *float64 `toml:"memory_gb,omitempty"`
}

type tomlDomain struct {
	Port   *int   `toml:"port,omitempty"`
	Suffix string `toml:"suffix,omitempty"`
}

type tomlVolume struct {
	Mount  string `toml:"mount"`
	Region string `toml:"region,omitempty"`
}

type tomlTrigger struct {
	Repository string `toml:"repository"`
	Branch     string `toml:"branch"`
	Provider   string `toml:"provider,omitempty"`
}

type tomlTCPProxy struct {
	ApplicationPort int    `toml:"application_port"`
	ProxyPort       int    `toml:"proxy_port,omitempty"`
	Domain          string `toml:"domain,omitempty"`
}

type tomlNetwork struct {
	Enabled bool   `toml:"enabled"`
	DNSName string `toml:"dns_name,omitempty"`
}

type tomlEgress struct {
	Region string `toml:"region"`
	IPv4   string `toml:"ipv4,omitempty"`
}

// tomlInitConfig is the top-level struct for init/adopt output.
type tomlInitConfig struct {
	Name      string            `toml:"name"`
	ID        string            `toml:"id,omitempty"`
	Tool      *tomlToolSettings `toml:"tool,omitempty"`
	Workspace *tomlContextBlock `toml:"workspace,omitempty"`
	Project   *tomlContextBlock `toml:"project,omitempty"`
	Variables map[string]string `toml:"variables,omitempty"`
	Service   []tomlService     `toml:"service,omitempty"`
}

type tomlToolSettings struct {
	EnvFile string `toml:"env_file,omitempty"`
}

type tomlContextBlock struct {
	Name string `toml:"name"`
	ID   string `toml:"id,omitempty"`
}

// ---------- conversion helpers ----------

func deployToTOML(d Deploy) *tomlDeploy {
	td := &tomlDeploy{
		Builder:                 d.Builder,
		Repo:                    d.Repo,
		Image:                   d.Image,
		BuildCommand:            d.BuildCommand,
		DockerfilePath:          d.DockerfilePath,
		RootDirectory:           d.RootDirectory,
		StartCommand:            d.StartCommand,
		CronSchedule:            d.CronSchedule,
		HealthcheckPath:         d.HealthcheckPath,
		HealthcheckTimeout:      d.HealthcheckTimeout,
		RestartPolicy:           d.RestartPolicy,
		RestartPolicyMaxRetries: d.RestartPolicyMaxRetries,
		DrainingSeconds:         d.DrainingSeconds,
		OverlapSeconds:          d.OverlapSeconds,
		WatchPatterns:           d.WatchPatterns,
		PreDeployCommand:        d.PreDeployCommand,
		SleepApplication:        d.SleepApplication,
		NumReplicas:             d.NumReplicas,
		Region:                  d.Region,
		IPv6Egress:              d.IPv6Egress,
	}
	// Return nil if all fields are zero so omitempty drops the section.
	if td.Builder == "" && td.Repo == nil && td.Image == nil &&
		td.BuildCommand == nil && td.DockerfilePath == nil &&
		td.RootDirectory == nil && td.StartCommand == nil &&
		td.CronSchedule == nil && td.HealthcheckPath == nil &&
		td.HealthcheckTimeout == nil && td.RestartPolicy == "" &&
		td.RestartPolicyMaxRetries == nil && td.DrainingSeconds == nil &&
		td.OverlapSeconds == nil && td.WatchPatterns == nil &&
		td.PreDeployCommand == nil && td.SleepApplication == nil &&
		td.NumReplicas == nil && td.Region == nil && td.IPv6Egress == nil {
		return nil
	}
	return td
}

func liveToTOMLServices(cfg LiveConfig, full bool) []tomlService {
	names := sortedServiceNames(cfg.Services)
	services := make([]tomlService, 0, len(names))
	for _, name := range names {
		svc := cfg.Services[name]
		ts := tomlService{
			Name:      name,
			ID:        svc.ID,
			Icon:      svc.Icon,
			Variables: svc.Variables,
		}
		if full {
			ts.Deploy = deployToTOML(svc.Deploy)
			if svc.VCPUs != nil || svc.MemoryGB != nil {
				ts.Resources = &tomlResources{VCPUs: svc.VCPUs, MemoryGB: svc.MemoryGB}
			}
			if len(svc.Domains) > 0 {
				ts.Domains = make(map[string]tomlDomain, len(svc.Domains))
				for _, domain := range sortedDomains(svc.Domains) {
					ts.Domains[domain.Domain] = tomlDomain{Port: domain.TargetPort, Suffix: domain.Suffix}
				}
			}
			if len(svc.Volumes) > 0 {
				ts.Volumes = make(map[string]tomlVolume, len(svc.Volumes))
				for _, volume := range sortedVolumes(svc.Volumes) {
					ts.Volumes[volume.Name] = tomlVolume{Mount: volume.MountPath, Region: volume.Region}
				}
			}
			if len(svc.TCPProxies) > 0 {
				ts.TCPProxies = make([]tomlTCPProxy, 0, len(svc.TCPProxies))
				for _, proxy := range sortedTCPProxies(svc.TCPProxies) {
					ts.TCPProxies = append(ts.TCPProxies, tomlTCPProxy{
						ApplicationPort: proxy.ApplicationPort,
						ProxyPort:       proxy.ProxyPort,
						Domain:          proxy.Domain,
					})
				}
			}
			if svc.Network != nil {
				ts.Network = &tomlNetwork{Enabled: true, DNSName: svc.Network.DNSName}
			}
			if len(svc.Triggers) > 0 {
				ts.Triggers = make([]tomlTrigger, 0, len(svc.Triggers))
				for _, trigger := range sortedTriggers(svc.Triggers) {
					ts.Triggers = append(ts.Triggers, tomlTrigger{
						Repository: trigger.Repository,
						Branch:     trigger.Branch,
						Provider:   trigger.Provider,
					})
				}
			}
			if len(svc.Egress) > 0 {
				ts.Egress = make([]tomlEgress, 0, len(svc.Egress))
				for _, gateway := range sortedEgress(svc.Egress) {
					ts.Egress = append(ts.Egress, tomlEgress(gateway))
				}
			}
		}
		services = append(services, ts)
	}
	return services
}

func marshalTOML(v any) string {
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(v); err != nil {
		// Struct marshalling should never fail for our types.
		panic(fmt.Sprintf("toml.Encode: %v", err))
	}
	return strings.TrimRight(buf.String(), "\n")
}

// RenderOptions controls how config is rendered.
type RenderOptions struct {
	Format      string   // "text", "json", "toml"
	Full        bool     // Include IDs and deploy settings
	ShowSecrets bool     // Show all values unmasked
	Keywords    []string // Custom sensitive keywords (nil = defaults)
	Allowlist   []string // Custom allowlist (nil = defaults)
}

// Render renders the live config in the requested output format.
// Variable values are masked by default unless ShowSecrets is true.
func Render(cfg LiveConfig, opts RenderOptions) (string, error) {
	var masker *Masker
	if !opts.ShowSecrets {
		masker = NewMasker(opts.Keywords, opts.Allowlist)
	}
	masked := maskConfig(cfg, masker)

	switch opts.Format {
	case "json":
		buf, err := json.MarshalIndent(toJSONMap(masked, opts.Full), "", "  ")
		if err != nil {
			return "", err
		}
		return string(buf), nil
	case "toml":
		return renderTOML(masked, opts.Full), nil
	case "text", "":
		return renderText(masked, opts.Full), nil
	case "raw":
		return "", errors.New("raw output is only supported for single values")
	default:
		return "", errors.New("unsupported output format")
	}
}

// maskConfig returns a copy of cfg with variable values masked.
// If masker is nil (ShowSecrets mode), returns cfg unchanged.
func maskConfig(cfg LiveConfig, masker *Masker) LiveConfig {
	if masker == nil {
		return cfg
	}
	out := LiveConfig{
		ProjectID:     cfg.ProjectID,
		EnvironmentID: cfg.EnvironmentID,
		Variables:     maskVars(cfg.Variables, masker),
		Services:      make(map[string]*ServiceConfig, len(cfg.Services)),
	}
	for name, svc := range cfg.Services {
		out.Services[name] = &ServiceConfig{
			ID:         svc.ID,
			Name:       svc.Name,
			Icon:       svc.Icon,
			Variables:  maskVars(svc.Variables, masker),
			Deploy:     svc.Deploy,
			VCPUs:      svc.VCPUs,
			MemoryGB:   svc.MemoryGB,
			Domains:    svc.Domains,
			Volumes:    svc.Volumes,
			TCPProxies: svc.TCPProxies,
			Triggers:   svc.Triggers,
			Egress:     svc.Egress,
			Network:    svc.Network,
		}
	}
	return out
}

// maskVars returns a new map with values masked as needed.
func maskVars(vars map[string]string, masker *Masker) map[string]string {
	if len(vars) == 0 {
		return vars
	}
	out := make(map[string]string, len(vars))
	for k, v := range vars {
		out[k] = masker.MaskValue(k, v)
	}
	return out
}

// toJSONMap builds a clean map for JSON output.
// When full is true, includes service IDs and deploy settings.
func toJSONMap(cfg LiveConfig, full bool) map[string]any {
	m := map[string]any{}
	if full {
		m["project_id"] = cfg.ProjectID
		m["environment_id"] = cfg.EnvironmentID
	}
	if len(cfg.Variables) > 0 {
		m["shared"] = map[string]any{"variables": cfg.Variables}
	}
	for name, svc := range cfg.Services {
		svcMap := map[string]any{"variables": svc.Variables}
		if full {
			svcMap["id"] = svc.ID
			if svc.Icon != "" {
				svcMap["icon"] = svc.Icon
			}
			svcMap["deploy"] = deployMap(svc.Deploy)
			if svc.VCPUs != nil || svc.MemoryGB != nil {
				res := map[string]any{}
				if svc.VCPUs != nil {
					res["vcpus"] = *svc.VCPUs
				}
				if svc.MemoryGB != nil {
					res["memory_gb"] = *svc.MemoryGB
				}
				svcMap["resources"] = res
			}
			if len(svc.Domains) > 0 {
				domains := map[string]any{}
				for _, domain := range sortedDomains(svc.Domains) {
					domainMap := map[string]any{}
					if domain.TargetPort != nil {
						domainMap["port"] = *domain.TargetPort
					}
					if domain.Suffix != "" {
						domainMap["suffix"] = domain.Suffix
					}
					domains[domain.Domain] = domainMap
				}
				svcMap["domains"] = domains
			}
			if len(svc.Volumes) > 0 {
				volumes := map[string]any{}
				for _, volume := range sortedVolumes(svc.Volumes) {
					volumeMap := map[string]any{"mount": volume.MountPath}
					if volume.Region != "" {
						volumeMap["region"] = volume.Region
					}
					volumes[volume.Name] = volumeMap
				}
				svcMap["volumes"] = volumes
			}
			if len(svc.TCPProxies) > 0 {
				tcpProxies := make([]map[string]any, 0, len(svc.TCPProxies))
				for _, proxy := range sortedTCPProxies(svc.TCPProxies) {
					proxyMap := map[string]any{"application_port": proxy.ApplicationPort}
					if proxy.ProxyPort != 0 {
						proxyMap["proxy_port"] = proxy.ProxyPort
					}
					if proxy.Domain != "" {
						proxyMap["domain"] = proxy.Domain
					}
					tcpProxies = append(tcpProxies, proxyMap)
				}
				svcMap["tcp_proxies"] = tcpProxies
			}
			if svc.Network != nil {
				network := map[string]any{"enabled": true}
				if svc.Network.DNSName != "" {
					network["dns_name"] = svc.Network.DNSName
				}
				svcMap["network"] = network
			}
			if len(svc.Triggers) > 0 {
				triggers := make([]map[string]any, 0, len(svc.Triggers))
				for _, trigger := range sortedTriggers(svc.Triggers) {
					triggerMap := map[string]any{
						"repository": trigger.Repository,
						"branch":     trigger.Branch,
					}
					if trigger.Provider != "" {
						triggerMap["provider"] = trigger.Provider
					}
					triggers = append(triggers, triggerMap)
				}
				svcMap["triggers"] = triggers
			}
			if len(svc.Egress) > 0 {
				egress := make([]map[string]any, 0, len(svc.Egress))
				for _, gateway := range sortedEgress(svc.Egress) {
					gatewayMap := map[string]any{"region": gateway.Region}
					if gateway.IPv4 != "" {
						gatewayMap["ipv4"] = gateway.IPv4
					}
					egress = append(egress, gatewayMap)
				}
				svcMap["egress"] = egress
			}
		}
		m[name] = svcMap
	}
	return m
}

// deployMap converts Deploy to a clean map, omitting nil/zero fields.
func deployMap(d Deploy) map[string]any {
	m := map[string]any{}
	if d.Builder != "" {
		m["builder"] = d.Builder
	}
	if d.Repo != nil && *d.Repo != "" {
		m["repo"] = *d.Repo
	}
	if d.Image != nil && *d.Image != "" {
		m["image"] = *d.Image
	}
	if d.BuildCommand != nil {
		m["build_command"] = *d.BuildCommand
	}
	if d.DockerfilePath != nil {
		m["dockerfile_path"] = *d.DockerfilePath
	}
	if d.RootDirectory != nil {
		m["root_directory"] = *d.RootDirectory
	}
	if d.StartCommand != nil {
		m["start_command"] = *d.StartCommand
	}
	if d.CronSchedule != nil {
		m["cron_schedule"] = *d.CronSchedule
	}
	if d.HealthcheckPath != nil {
		m["healthcheck_path"] = *d.HealthcheckPath
	}
	if d.HealthcheckTimeout != nil {
		m["healthcheck_timeout"] = *d.HealthcheckTimeout
	}
	if d.RestartPolicy != "" {
		m["restart_policy"] = d.RestartPolicy
	}
	if d.RestartPolicyMaxRetries != nil {
		m["restart_policy_max_retries"] = *d.RestartPolicyMaxRetries
	}
	if d.DrainingSeconds != nil {
		m["draining_seconds"] = *d.DrainingSeconds
	}
	if d.OverlapSeconds != nil {
		m["overlap_seconds"] = *d.OverlapSeconds
	}
	if len(d.WatchPatterns) > 0 {
		m["watch_patterns"] = d.WatchPatterns
	}
	if len(d.PreDeployCommand) > 0 {
		m["pre_deploy_command"] = d.PreDeployCommand
	}
	if d.SleepApplication != nil {
		m["sleep_application"] = *d.SleepApplication
	}
	if d.NumReplicas != nil {
		m["num_replicas"] = *d.NumReplicas
	}
	if d.Region != nil {
		m["region"] = *d.Region
	}
	if d.IPv6Egress != nil {
		m["ipv6_egress"] = *d.IPv6Egress
	}
	return m
}

// renderTOML builds valid TOML output via toml.Marshal.
func renderTOML(cfg LiveConfig, full bool) string {
	tc := tomlShowConfig{
		Variables: cfg.Variables,
		Service:   liveToTOMLServices(cfg, full),
	}
	if full {
		tc.ProjectID = cfg.ProjectID
		tc.EnvironmentID = cfg.EnvironmentID
	}
	return marshalTOML(tc)
}

// envRefConfig returns a copy of cfg with sensitive variable values replaced
// by ${VAR_NAME} environment references. Railway references (${{...}}) are
// preserved. Non-sensitive values are left as-is.
func envRefConfig(cfg LiveConfig) LiveConfig {
	masker := NewMasker(nil, nil)
	out := LiveConfig{
		ProjectID:     cfg.ProjectID,
		EnvironmentID: cfg.EnvironmentID,
		Variables:     envRefVars(cfg.Variables, masker),
		Services:      make(map[string]*ServiceConfig, len(cfg.Services)),
	}
	for name, svc := range cfg.Services {
		out.Services[name] = &ServiceConfig{
			ID:         svc.ID,
			Name:       svc.Name,
			Icon:       svc.Icon,
			Variables:  envRefVars(svc.Variables, masker),
			Deploy:     svc.Deploy,
			VCPUs:      svc.VCPUs,
			MemoryGB:   svc.MemoryGB,
			Domains:    svc.Domains,
			Volumes:    svc.Volumes,
			TCPProxies: svc.TCPProxies,
			Triggers:   svc.Triggers,
			Egress:     svc.Egress,
			Network:    svc.Network,
		}
	}
	return out
}

// envRefVars replaces sensitive values with ${VAR_NAME} references.
func envRefVars(vars map[string]string, masker *Masker) map[string]string {
	if len(vars) == 0 {
		return vars
	}
	out := make(map[string]string, len(vars))
	for k, v := range vars {
		if masker.MaskValue(k, v) == MaskedValue {
			out[k] = "${" + k + "}"
		} else {
			out[k] = v
		}
	}
	return out
}

// CollectSecrets returns a map of variable names to their actual values for
// all variables classified as secrets. A variable is a secret if MaskValue
// would mask it (sensitive name or high entropy) AND the value is not a
// Railway reference. The returned map is flat — shared and per-service
// variables are merged (last wins if duplicated, which matches the env var
// namespace).
func CollectSecrets(cfg LiveConfig) map[string]string {
	masker := NewMasker(nil, nil)
	secrets := make(map[string]string)
	for k, v := range cfg.Variables {
		if masker.MaskValue(k, v) == MaskedValue {
			secrets[k] = v
		}
	}
	for _, svc := range cfg.Services {
		for k, v := range svc.Variables {
			if masker.MaskValue(k, v) == MaskedValue {
				secrets[k] = v
			}
		}
	}
	return secrets
}

// RenderInitTOML generates a fat-controller.toml for the init command.
// It includes workspace/project/environment header (when provided), uses
// ${VAR} env references for secrets, and records IDs for pinning.
// Deploy settings are excluded (those are operational, fetched live).
// When secrets are detected, a [tool] section with env_file is included
// so the loader knows where to find the secret values.
func RenderInitTOML(workspace, project, environment string, cfg LiveConfig) string {
	return RenderInitTOMLWithEnvFile(workspace, project, environment, cfg, DefaultEnvFile)
}

// RenderInitTOMLWithEnvFile is like RenderInitTOML, but allows callers to
// choose the tool.env_file value written into the config.
func RenderInitTOMLWithEnvFile(workspace, project, environment string, cfg LiveConfig, envFile string) string {
	replaced := envRefConfig(cfg)

	tc := tomlInitConfig{
		Name:      environment,
		ID:        cfg.EnvironmentID,
		Project:   &tomlContextBlock{Name: project, ID: cfg.ProjectID},
		Variables: replaced.Variables,
		Service:   liveToTOMLServices(replaced, false),
	}
	if workspace != "" {
		tc.Workspace = &tomlContextBlock{Name: workspace}
	}
	if envFile == "" {
		envFile = DefaultEnvFile
	}
	// Include [tool] env_file when secrets were replaced with ${VAR} refs.
	if len(CollectSecrets(cfg)) > 0 {
		tc.Tool = &tomlToolSettings{EnvFile: envFile}
	}
	return marshalTOML(tc) + "\n"
}

// renderText builds human-readable text output.
// Unlike TOML, this uses bare values (no quoting) for readability.
// When full is true, IDs and deploy settings are included.
func renderText(cfg LiveConfig, full bool) string {
	var out strings.Builder

	if full {
		out.WriteString("project_id:     " + cfg.ProjectID + "\n")
		out.WriteString("environment_id: " + cfg.EnvironmentID + "\n\n")
	}

	if len(cfg.Variables) > 0 {
		out.WriteString("[variables]\n")
		keys := sortedKeys(cfg.Variables)
		for _, k := range keys {
			out.WriteString(k + " = " + cfg.Variables[k] + "\n")
		}
		out.WriteString("\n")
	}

	serviceNames := sortedServiceNames(cfg.Services)
	for _, name := range serviceNames {
		svc := cfg.Services[name]
		out.WriteString("[[service]]\n")
		out.WriteString("name = " + name + "\n")
		if full && svc.ID != "" {
			out.WriteString("id = " + svc.ID + "\n")
		}
		if full && svc.Icon != "" {
			out.WriteString("icon = " + svc.Icon + "\n")
		}
		out.WriteString("\n")
		if len(svc.Variables) > 0 {
			out.WriteString("[service.variables]\n")
			keys := sortedKeys(svc.Variables)
			for _, k := range keys {
				out.WriteString(k + " = " + svc.Variables[k] + "\n")
			}
			out.WriteString("\n")
		}
		if full {
			td := deployToTOML(svc.Deploy)
			if td != nil {
				out.WriteString("[service.deploy]\n")
				out.WriteString(marshalTOML(td))
				out.WriteString("\n\n")
			}
			if svc.VCPUs != nil || svc.MemoryGB != nil {
				out.WriteString("[service.resources]\n")
				if svc.VCPUs != nil {
					_, _ = fmt.Fprintf(&out, "vcpus = %v\n", *svc.VCPUs)
				}
				if svc.MemoryGB != nil {
					_, _ = fmt.Fprintf(&out, "memory_gb = %v\n", *svc.MemoryGB)
				}
				out.WriteString("\n")
			}
			for _, domain := range sortedDomains(svc.Domains) {
				out.WriteString("[service.domains." + domain.Domain + "]\n")
				if domain.TargetPort != nil {
					_, _ = fmt.Fprintf(&out, "port = %d\n", *domain.TargetPort)
				}
				if domain.Suffix != "" {
					out.WriteString("suffix = " + domain.Suffix + "\n")
				}
				out.WriteString("\n")
			}
			for _, volume := range sortedVolumes(svc.Volumes) {
				out.WriteString("[service.volumes." + volume.Name + "]\n")
				out.WriteString("mount = " + volume.MountPath + "\n")
				if volume.Region != "" {
					out.WriteString("region = " + volume.Region + "\n")
				}
				out.WriteString("\n")
			}
			for _, proxy := range sortedTCPProxies(svc.TCPProxies) {
				out.WriteString("[[service.tcp_proxies]]\n")
				_, _ = fmt.Fprintf(&out, "application_port = %d\n", proxy.ApplicationPort)
				if proxy.ProxyPort != 0 {
					_, _ = fmt.Fprintf(&out, "proxy_port = %d\n", proxy.ProxyPort)
				}
				if proxy.Domain != "" {
					out.WriteString("domain = " + proxy.Domain + "\n")
				}
				out.WriteString("\n")
			}
			if svc.Network != nil {
				out.WriteString("[service.network]\n")
				out.WriteString("enabled = true\n")
				if svc.Network.DNSName != "" {
					out.WriteString("dns_name = " + svc.Network.DNSName + "\n")
				}
				out.WriteString("\n")
			}
			for _, trigger := range sortedTriggers(svc.Triggers) {
				out.WriteString("[[service.triggers]]\n")
				out.WriteString("repository = " + trigger.Repository + "\n")
				out.WriteString("branch = " + trigger.Branch + "\n")
				if trigger.Provider != "" {
					out.WriteString("provider = " + trigger.Provider + "\n")
				}
				out.WriteString("\n")
			}
			for _, gateway := range sortedEgress(svc.Egress) {
				out.WriteString("[[service.egress]]\n")
				out.WriteString("region = " + gateway.Region + "\n")
				if gateway.IPv4 != "" {
					out.WriteString("ipv4 = " + gateway.IPv4 + "\n")
				}
				out.WriteString("\n")
			}
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

func sortedServiceNames(services map[string]*ServiceConfig) []string {
	names := make([]string, 0, len(services))
	for name := range services {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func sortedDomains(domains []LiveDomain) []LiveDomain {
	out := append([]LiveDomain(nil), domains...)
	sort.Slice(out, func(i, j int) bool {
		return out[i].Domain < out[j].Domain
	})
	return out
}

func sortedVolumes(volumes []LiveVolume) []LiveVolume {
	out := append([]LiveVolume(nil), volumes...)
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

func sortedTCPProxies(proxies []LiveTCPProxy) []LiveTCPProxy {
	out := append([]LiveTCPProxy(nil), proxies...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].ApplicationPort != out[j].ApplicationPort {
			return out[i].ApplicationPort < out[j].ApplicationPort
		}
		if out[i].ProxyPort != out[j].ProxyPort {
			return out[i].ProxyPort < out[j].ProxyPort
		}
		return out[i].Domain < out[j].Domain
	})
	return out
}

func sortedTriggers(triggers []LiveTrigger) []LiveTrigger {
	out := append([]LiveTrigger(nil), triggers...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Repository != out[j].Repository {
			return out[i].Repository < out[j].Repository
		}
		if out[i].Branch != out[j].Branch {
			return out[i].Branch < out[j].Branch
		}
		return out[i].Provider < out[j].Provider
	})
	return out
}

func sortedEgress(gateways []LiveEgressGateway) []LiveEgressGateway {
	out := append([]LiveEgressGateway(nil), gateways...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Region != out[j].Region {
			return out[i].Region < out[j].Region
		}
		return out[i].IPv4 < out[j].IPv4
	})
	return out
}
