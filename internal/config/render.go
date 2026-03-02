package config

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"
)

// Render renders the live config in the requested output format.
// When full is true, IDs and deploy settings are included.
func Render(cfg LiveConfig, format string, full bool) (string, error) {
	switch format {
	case "json":
		buf, err := json.MarshalIndent(toJSONMap(cfg, full), "", "  ")
		if err != nil {
			return "", err
		}
		return string(buf), nil
	case "toml":
		return renderTOML(cfg, full), nil
	case "text", "":
		return renderText(cfg, full), nil
	default:
		return "", errors.New("unsupported output format")
	}
}

// toJSONMap builds a clean map for JSON output.
// When full is true, includes service IDs and deploy settings.
func toJSONMap(cfg LiveConfig, full bool) map[string]any {
	m := map[string]any{}
	if full {
		m["project_id"] = cfg.ProjectID
		m["environment_id"] = cfg.EnvironmentID
	}
	if len(cfg.Shared) > 0 {
		m["shared"] = map[string]any{"variables": cfg.Shared}
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

// deployMap converts Deploy to a clean map, omitting nil fields.
func deployMap(d Deploy) map[string]any {
	m := map[string]any{}
	if d.Builder != "" {
		m["builder"] = d.Builder
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
	if d.HealthcheckPath != nil {
		m["healthcheck_path"] = *d.HealthcheckPath
	}
	return m
}

// renderTOML builds valid TOML output with properly escaped string values.
func renderTOML(cfg LiveConfig, full bool) string {
	var out strings.Builder
	if full {
		out.WriteString("project_id = " + tomlQuote(cfg.ProjectID) + "\n")
		out.WriteString("environment_id = " + tomlQuote(cfg.EnvironmentID) + "\n\n")
	}
	if len(cfg.Shared) > 0 {
		out.WriteString("[shared.variables]\n")
		keys := sortedKeys(cfg.Shared)
		for _, k := range keys {
			out.WriteString(k + " = " + tomlQuote(cfg.Shared[k]) + "\n")
		}
		out.WriteString("\n")
	}
	serviceNames := sortedServiceNames(cfg.Services)
	for _, name := range serviceNames {
		svc := cfg.Services[name]
		if full {
			out.WriteString("[" + name + "]\n")
			out.WriteString("id = " + tomlQuote(svc.ID) + "\n\n")
		}
		if len(svc.Variables) > 0 {
			out.WriteString("[" + name + ".variables]\n")
			keys := sortedKeys(svc.Variables)
			for _, k := range keys {
				out.WriteString(k + " = " + tomlQuote(svc.Variables[k]) + "\n")
			}
			out.WriteString("\n")
		}
		if full {
			writeTOMLDeploy(&out, name, svc.Deploy)
		}
	}
	return strings.TrimRight(out.String(), "\n")
}

func writeTOMLDeploy(out *strings.Builder, name string, d Deploy) {
	// Only write deploy section if there's something to show.
	var lines []string
	if d.Builder != "" {
		lines = append(lines, "builder = "+tomlQuote(d.Builder))
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
	if d.HealthcheckPath != nil {
		lines = append(lines, "healthcheck_path = "+tomlQuote(*d.HealthcheckPath))
	}
	if len(lines) > 0 {
		out.WriteString("[" + name + ".deploy]\n")
		for _, line := range lines {
			out.WriteString(line + "\n")
		}
		out.WriteString("\n")
	}
}

// tomlQuote returns a TOML basic string with special characters escaped.
func tomlQuote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteRune(r)
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

	if len(cfg.Shared) > 0 {
		out.WriteString("[shared.variables]\n")
		keys := sortedKeys(cfg.Shared)
		for _, k := range keys {
			out.WriteString(k + " = " + cfg.Shared[k] + "\n")
		}
		out.WriteString("\n")
	}

	serviceNames := sortedServiceNames(cfg.Services)
	for _, name := range serviceNames {
		svc := cfg.Services[name]
		if full {
			out.WriteString("[" + name + "]\n")
			out.WriteString("id = " + svc.ID + "\n\n")
		}
		if len(svc.Variables) > 0 {
			out.WriteString("[" + name + ".variables]\n")
			keys := sortedKeys(svc.Variables)
			for _, k := range keys {
				out.WriteString(k + " = " + svc.Variables[k] + "\n")
			}
			out.WriteString("\n")
		}
		if full {
			writeTextDeploy(&out, name, svc.Deploy)
		}
	}

	return strings.TrimRight(out.String(), "\n")
}

func writeTextDeploy(out *strings.Builder, name string, d Deploy) {
	var lines []string
	if d.Builder != "" {
		lines = append(lines, "builder = "+d.Builder)
	}
	if d.DockerfilePath != nil {
		lines = append(lines, "dockerfile_path = "+*d.DockerfilePath)
	}
	if d.RootDirectory != nil {
		lines = append(lines, "root_directory = "+*d.RootDirectory)
	}
	if d.StartCommand != nil {
		lines = append(lines, "start_command = "+*d.StartCommand)
	}
	if d.HealthcheckPath != nil {
		lines = append(lines, "healthcheck_path = "+*d.HealthcheckPath)
	}
	if len(lines) > 0 {
		out.WriteString("[" + name + ".deploy]\n")
		for _, line := range lines {
			out.WriteString(line + "\n")
		}
		out.WriteString("\n")
	}
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
