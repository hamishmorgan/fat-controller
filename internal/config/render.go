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
	Name      string            `toml:"name"`
	ID        string            `toml:"id,omitempty"`
	Variables map[string]string `toml:"variables,omitempty"`
	Deploy    *tomlDeploy       `toml:"deploy,omitempty"`
}

// tomlDeploy mirrors Deploy with toml tags for marshalling.
type tomlDeploy struct {
	Builder                 string  `toml:"builder,omitempty"`
	Repo                    *string `toml:"repo,omitempty"`
	Image                   *string `toml:"image,omitempty"`
	BuildCommand            *string `toml:"build_command,omitempty"`
	DockerfilePath          *string `toml:"dockerfile_path,omitempty"`
	RootDirectory           *string `toml:"root_directory,omitempty"`
	StartCommand            *string `toml:"start_command,omitempty"`
	CronSchedule            *string `toml:"cron_schedule,omitempty"`
	HealthcheckPath         *string `toml:"healthcheck_path,omitempty"`
	HealthcheckTimeout      *int    `toml:"healthcheck_timeout,omitempty"`
	RestartPolicy           string  `toml:"restart_policy,omitempty"`
	RestartPolicyMaxRetries *int    `toml:"restart_policy_max_retries,omitempty"`
	DrainingSeconds         *int    `toml:"draining_seconds,omitempty"`
	OverlapSeconds          *int    `toml:"overlap_seconds,omitempty"`
	SleepApplication        *bool   `toml:"sleep_application,omitempty"`
	NumReplicas             *int    `toml:"num_replicas,omitempty"`
	Region                  *string `toml:"region,omitempty"`
	IPv6Egress              *bool   `toml:"ipv6_egress,omitempty"`
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
		td.OverlapSeconds == nil && td.SleepApplication == nil &&
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
			Variables: svc.Variables,
		}
		if full {
			ts.Deploy = deployToTOML(svc.Deploy)
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
			ID:        svc.ID,
			Name:      svc.Name,
			Variables: maskVars(svc.Variables, masker),
			Deploy:    svc.Deploy,
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
			svcMap["deploy"] = deployMap(svc.Deploy)
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
			ID:        svc.ID,
			Name:      svc.Name,
			Variables: envRefVars(svc.Variables, masker),
			Deploy:    svc.Deploy,
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
