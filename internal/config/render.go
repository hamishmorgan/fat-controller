package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

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

// renderTOML builds valid TOML output using [[service]] array-of-tables syntax
// with TOML v1.1 multiline inline tables for sub-fields (variables, deploy).
func renderTOML(cfg LiveConfig, full bool) string {
	var out strings.Builder
	if full {
		out.WriteString("project_id = " + tomlQuote(cfg.ProjectID) + "\n")
		out.WriteString("environment_id = " + tomlQuote(cfg.EnvironmentID) + "\n\n")
	}
	if len(cfg.Variables) > 0 {
		writeTOMLInlineVars(&out, "variables", cfg.Variables)
		out.WriteString("\n")
	}
	serviceNames := sortedServiceNames(cfg.Services)
	for _, name := range serviceNames {
		svc := cfg.Services[name]
		out.WriteString("[[service]]\n")
		out.WriteString("name = " + tomlQuote(name) + "\n")
		if full && svc.ID != "" {
			out.WriteString("id = " + tomlQuote(svc.ID) + "\n")
		}

		if len(svc.Variables) > 0 {
			writeTOMLInlineVars(&out, "variables", svc.Variables)
		}
		if full {
			writeTOMLInlineDeploy(&out, svc.Deploy)
		}
		out.WriteString("\n")
	}
	return strings.TrimRight(out.String(), "\n")
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
// ${VAR} env references for secrets, and excludes deploy settings and IDs
// (those are operational, not config).
func RenderInitTOML(workspace, project, environment string, cfg LiveConfig) string {
	replaced := envRefConfig(cfg)

	var out strings.Builder
	out.WriteString("name = " + tomlQuote(environment) + "\n\n")
	if workspace != "" {
		out.WriteString("[workspace]\n")
		out.WriteString("name = " + tomlQuote(workspace) + "\n\n")
	}
	out.WriteString("[project]\n")
	out.WriteString("name = " + tomlQuote(project) + "\n")

	// Render service sections using the existing TOML renderer (without
	// IDs or deploy settings — those are fetched live, not managed in config).
	body := renderTOML(replaced, false)
	if body != "" {
		out.WriteString("\n")
		out.WriteString(body)
	}

	return out.String()
}

// writeTOMLInlineVars writes a multiline inline table of key-value string pairs.
// Example output:
//
//	variables = {
//	    PORT = "8080",
//	    NODE_ENV = "production",
//	}
func writeTOMLInlineVars(out *strings.Builder, tableName string, vars map[string]string) {
	keys := sortedKeys(vars)
	out.WriteString(tableName + " = {\n")
	for _, k := range keys {
		out.WriteString("    " + tomlKey(k) + " = " + tomlQuote(vars[k]) + ",\n")
	}
	out.WriteString("}\n")
}

// writeTOMLInlineDeploy writes deploy settings as a multiline inline table.
func writeTOMLInlineDeploy(out *strings.Builder, d Deploy) {
	lines := deployLines(d)
	if len(lines) > 0 {
		out.WriteString("deploy = {\n")
		for _, line := range lines {
			out.WriteString("    " + line + ",\n")
		}
		out.WriteString("}\n")
	}
}

// deployLines returns TOML key=value lines for all non-zero deploy fields.
func deployLines(d Deploy) []string {
	var lines []string
	if d.Builder != "" {
		lines = append(lines, "builder = "+tomlQuote(d.Builder))
	}
	if d.Repo != nil && *d.Repo != "" {
		lines = append(lines, "repo = "+tomlQuote(*d.Repo))
	}
	if d.Image != nil && *d.Image != "" {
		lines = append(lines, "image = "+tomlQuote(*d.Image))
	}
	if d.BuildCommand != nil {
		lines = append(lines, "build_command = "+tomlQuote(*d.BuildCommand))
	}
	if d.DockerfilePath != nil {
		lines = append(lines, "dockerfile_path = "+tomlQuote(*d.DockerfilePath))
	}
	if d.RootDirectory != nil {
		lines = append(lines, "root_directory = "+tomlQuote(*d.RootDirectory))
	}
	if d.StartCommand != nil {
		lines = append(lines, "start_command = "+tomlQuote(*d.StartCommand))
	}
	if d.CronSchedule != nil {
		lines = append(lines, "cron_schedule = "+tomlQuote(*d.CronSchedule))
	}
	if d.HealthcheckPath != nil {
		lines = append(lines, "healthcheck_path = "+tomlQuote(*d.HealthcheckPath))
	}
	if d.HealthcheckTimeout != nil {
		lines = append(lines, fmt.Sprintf("healthcheck_timeout = %d", *d.HealthcheckTimeout))
	}
	if d.RestartPolicy != "" {
		lines = append(lines, "restart_policy = "+tomlQuote(d.RestartPolicy))
	}
	if d.RestartPolicyMaxRetries != nil {
		lines = append(lines, fmt.Sprintf("restart_policy_max_retries = %d", *d.RestartPolicyMaxRetries))
	}
	if d.DrainingSeconds != nil {
		lines = append(lines, fmt.Sprintf("draining_seconds = %d", *d.DrainingSeconds))
	}
	if d.OverlapSeconds != nil {
		lines = append(lines, fmt.Sprintf("overlap_seconds = %d", *d.OverlapSeconds))
	}
	if d.SleepApplication != nil {
		lines = append(lines, fmt.Sprintf("sleep_application = %t", *d.SleepApplication))
	}
	if d.NumReplicas != nil {
		lines = append(lines, fmt.Sprintf("num_replicas = %d", *d.NumReplicas))
	}
	if d.Region != nil {
		lines = append(lines, "region = "+tomlQuote(*d.Region))
	}
	if d.IPv6Egress != nil {
		lines = append(lines, fmt.Sprintf("ipv6_egress = %t", *d.IPv6Egress))
	}
	return lines
}

// tomlKey returns a bare TOML key if it contains only safe characters
// (A-Z, a-z, 0-9, -, _), otherwise returns a quoted key.
func tomlKey(key string) string {
	for _, r := range key {
		if (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' && r != '-' {
			return tomlQuote(key)
		}
	}
	return key
}

// tomlQuote returns a TOML basic string with special characters escaped.
// All C0 control characters (U+0000–U+001F) and DEL (U+007F) are escaped
// per the TOML spec.
func tomlQuote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r <= 0x1F || r == 0x7F {
				fmt.Fprintf(&b, `\u%04X`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
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
			lines := deployLines(svc.Deploy)
			if len(lines) > 0 {
				out.WriteString("[service.deploy]\n")
				for _, line := range lines {
					out.WriteString(line + "\n")
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
