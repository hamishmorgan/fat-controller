package config

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// Warning represents a single advisory diagnostic from config validation.
type Warning struct {
	Code    string
	Message string
	Path    string // dot-path to the problematic config element, e.g. "api.variables.PORT"
}

// suspiciousRefRe matches ${word.word} patterns that likely should be ${{word.word}}.
var suspiciousRefRe = regexp.MustCompile(`\$\{([a-zA-Z_][a-zA-Z0-9_]*\.[a-zA-Z_][a-zA-Z0-9_]*)\}`)

// doubleRefRe matches legitimate ${{...}} Railway references.
var doubleRefRe = regexp.MustCompile(`\$\{\{[^}]+\}\}`)

// validVarNameRe matches valid variable names: starts with letter or underscore, then alphanumeric or underscore.
var validVarNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// serviceRefRe matches ${{service.VAR}} Railway cross-service references.
var serviceRefRe = regexp.MustCompile(`\$\{\{([a-zA-Z_][a-zA-Z0-9_-]*)\.[a-zA-Z_][a-zA-Z0-9_]*\}\}`)

// Validate runs advisory checks on a DesiredConfig and returns warnings.
// liveServiceNames is the list of service names that actually exist in Railway;
// pass nil to skip W040 checks (e.g. for offline validation).
// Warnings whose codes appear in tool.suppress_warnings are filtered out.
func Validate(cfg *DesiredConfig, liveServiceNames []string) []Warning {
	return ValidateWithOptions(cfg, ValidateOptions{LiveServiceNames: liveServiceNames})
}

// ValidateOptions controls optional validation inputs.
type ValidateOptions struct {
	LiveServiceNames []string
	// EnvFileVars are the merged key/value pairs loaded from tool.env_file.
	// Used for W080 orphan env-file key warnings.
	EnvFileVars map[string]string
}

// ValidateWithOptions runs advisory checks on a DesiredConfig and returns warnings.
// It is the implementation behind Validate.
func ValidateWithOptions(cfg *DesiredConfig, opts ValidateOptions) []Warning {
	slog.Debug("validating config", "services", len(cfg.Services), "has_live_names", opts.LiveServiceNames != nil)
	var warnings []Warning

	// Extract tool settings.
	var suppressWarnings, sensitiveKeywords, sensitiveAllowlist []string
	if cfg.Tool != nil {
		suppressWarnings = cfg.Tool.SuppressWarnings
		sensitiveKeywords = cfg.Tool.SensitiveKeywords
		sensitiveAllowlist = cfg.Tool.SensitiveAllowlist
	}

	// Build suppression set.
	suppressed := make(map[string]bool, len(suppressWarnings))
	for _, code := range suppressWarnings {
		suppressed[code] = true
	}

	// W041: No services or shared variables defined.
	if !hasAnythingActionable(cfg) {
		warnings = append(warnings, Warning{
			Code:    "W041",
			Message: "config defines no services or shared variables",
			Path:    "",
		})
	}

	// Check shared variables.
	if cfg.Variables != nil {
		for name, value := range cfg.Variables {
			path := "variables." + name
			warnings = append(warnings, checkVarName(name, path)...)
			warnings = append(warnings, checkVarValue(value, path)...)
		}
	}

	// W050: Hardcoded secret detection.
	masker := NewMasker(sensitiveKeywords, sensitiveAllowlist)

	// Check shared variables for W050 and W060.
	if cfg.Variables != nil {
		for name, value := range cfg.Variables {
			path := "variables." + name
			warnings = append(warnings, checkSecret(masker, name, value, path)...)
		}
	}

	// W060: Reference to unknown service — build known service set.
	knownServices := make(map[string]bool, len(cfg.Services))
	for _, svc := range cfg.Services {
		knownServices[svc.Name] = true
	}
	// Railway treats environment-level variables as belonging to a pseudo-service
	// named "shared" for the purpose of ${{shared.VAR}} references. In this tool,
	// those variables are represented by the top-level [variables] table.
	knownServices["shared"] = true

	// Check shared vars for W060.
	if cfg.Variables != nil {
		for varName, value := range cfg.Variables {
			warnings = append(warnings, checkServiceRefs(value, "variables."+varName, knownServices)...)
		}
	}

	// Check each service.
	for _, svc := range cfg.Services {
		svcName := svc.Name

		// W003: Empty service block.
		if isEmptyService(svc) {
			warnings = append(warnings, Warning{
				Code:    "W003",
				Message: fmt.Sprintf("service %q has no variables, resources, or deploy settings", svcName),
				Path:    svcName,
			})
		}

		for name, value := range svc.Variables {
			path := svcName + ".variables." + name
			warnings = append(warnings, checkVarName(name, path)...)
			warnings = append(warnings, checkVarValue(value, path)...)
			warnings = append(warnings, checkSecret(masker, name, value, path)...)
			warnings = append(warnings, checkServiceRefs(value, path, knownServices)...)

			// W020: Variable in both shared and service.
			if cfg.Variables != nil {
				if _, ok := cfg.Variables[name]; ok {
					warnings = append(warnings, Warning{
						Code:    "W020",
						Message: fmt.Sprintf("variable %q defined in both shared and service %q (service wins)", name, svcName),
						Path:    path,
					})
				}
			}
		}
	}

	// W021: Variable overridden by local file.
	for _, ov := range cfg.Overrides {
		warnings = append(warnings, Warning{
			Code:    "W021",
			Message: fmt.Sprintf("%s is overridden by %s", ov.Path, ov.Source),
			Path:    ov.Path,
		})
	}

	// W040: Unknown service name.
	if opts.LiveServiceNames != nil {
		liveSet := make(map[string]bool, len(opts.LiveServiceNames))
		for _, name := range opts.LiveServiceNames {
			liveSet[name] = true
		}
		for _, svc := range cfg.Services {
			if !liveSet[svc.Name] {
				warnings = append(warnings, Warning{
					Code:    "W040",
					Message: fmt.Sprintf("service %q not found in Railway project", svc.Name),
					Path:    svc.Name,
				})
			}
		}
	}

	// W070: Duplicate service name.
	{
		seen := make(map[string]int, len(cfg.Services))
		for i, svc := range cfg.Services {
			if prev, ok := seen[svc.Name]; ok {
				warnings = append(warnings, Warning{
					Code:    "W070",
					Message: fmt.Sprintf("service name %q appears at index %d and %d — names must be unique", svc.Name, prev, i),
					Path:    svc.Name,
				})
			}
			seen[svc.Name] = i
		}
	}

	// W071: Mutually exclusive repo + image.
	for _, svc := range cfg.Services {
		if svc.Deploy != nil && svc.Deploy.Repo != nil && svc.Deploy.Image != nil {
			if *svc.Deploy.Repo != "" && *svc.Deploy.Image != "" {
				warnings = append(warnings, Warning{
					Code:    "W071",
					Message: fmt.Sprintf("service %q sets both deploy.repo and deploy.image — only one source is allowed", svc.Name),
					Path:    svc.Name + ".deploy",
				})
			}
		}
	}

	// W072: scale + deploy.region both set.
	for _, svc := range cfg.Services {
		if len(svc.Scale) > 0 && svc.Deploy != nil && svc.Deploy.Region != nil && *svc.Deploy.Region != "" {
			warnings = append(warnings, Warning{
				Code:    "W072",
				Message: fmt.Sprintf("service %q sets both scale and deploy.region — use scale for multi-region placement instead", svc.Name),
				Path:    svc.Name + ".deploy.region",
			})
		}
	}

	// W090: Unresolvable ${VAR} references.
	// Checks that all ${VAR} references in the raw (pre-interpolation) config
	// can be resolved from either the env file or the process environment.
	{
		allRefs := collectEnvReferences(cfg)
		for varName := range allRefs {
			// Check env file first, then process environment.
			if opts.EnvFileVars != nil {
				if _, ok := opts.EnvFileVars[varName]; ok {
					continue
				}
			}
			if _, ok := os.LookupEnv(varName); ok {
				continue
			}
			warnings = append(warnings, Warning{
				Code:    "W090",
				Message: fmt.Sprintf("${%s} is referenced but not defined in env file or process environment", varName),
				Path:    "",
			})
		}
	}

	// W080: Orphaned env-file key.
	if cfg.Tool != nil && len(cfg.Tool.EnvFiles()) > 0 && len(opts.EnvFileVars) > 0 {
		references := collectEnvReferences(cfg)
		keys := make([]string, 0, len(opts.EnvFileVars))
		for k := range opts.EnvFileVars {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if !validVarNameRe.MatchString(k) {
				continue
			}
			if references[k] {
				continue
			}
			warnings = append(warnings, Warning{
				Code:    "W080",
				Message: fmt.Sprintf("env file key %q is set but not referenced in config (no ${%s})", k, k),
				Path:    "tool.env_file",
			})
		}
	}

	// Filter suppressed warnings.
	if len(suppressed) > 0 {
		filtered := warnings[:0]
		for _, w := range warnings {
			if !suppressed[w.Code] {
				filtered = append(filtered, w)
			}
		}
		warnings = filtered
	}

	slog.Debug("validation complete", "warnings", len(warnings))
	return warnings
}

func collectEnvReferences(cfg *DesiredConfig) map[string]bool {
	refs := make(map[string]bool)
	add := func(value string) {
		for _, m := range localEnvPattern.FindAllStringSubmatch(value, -1) {
			if len(m) >= 2 {
				refs[m[1]] = true
			}
		}
	}
	for _, v := range cfg.Variables {
		add(v)
	}
	for _, svc := range cfg.Services {
		if svc == nil {
			continue
		}
		for _, v := range svc.Variables {
			add(v)
		}
		if svc.Deploy != nil && svc.Deploy.RegistryCredentials != nil {
			add(svc.Deploy.RegistryCredentials.Password)
		}
	}
	return refs
}

// hasAnythingActionable returns true if the config defines at least one service
// or at least one shared variable.
func hasAnythingActionable(cfg *DesiredConfig) bool {
	if len(cfg.Variables) > 0 {
		return true
	}
	return len(cfg.Services) > 0
}

// isEmptyService returns true if a service block has no variables, resources, or deploy.
func isEmptyService(svc *DesiredService) bool {
	if svc == nil {
		return true
	}
	if len(svc.Variables) > 0 {
		return false
	}
	if svc.Resources != nil {
		return false
	}
	if svc.Deploy != nil {
		return false
	}
	return true
}

// checkVarName checks a variable name for naming warnings.
func checkVarName(name, path string) []Warning {
	var warnings []Warning

	// W030: Lowercase variable name.
	if hasLowercase(name) {
		warnings = append(warnings, Warning{
			Code:    "W030",
			Message: fmt.Sprintf("variable name %q contains lowercase letters (convention is UPPER_SNAKE_CASE)", name),
			Path:    path,
		})
	}

	// W031: Invalid variable name characters.
	if !validVarNameRe.MatchString(name) {
		warnings = append(warnings, Warning{
			Code:    "W031",
			Message: fmt.Sprintf("variable name %q contains invalid characters (expected [A-Za-z0-9_])", name),
			Path:    path,
		})
	}

	return warnings
}

// checkSecret checks whether a variable value looks like a hardcoded secret.
func checkSecret(masker *Masker, name, value, path string) []Warning {
	// W050: Hardcoded secret in config.
	// Skip empty values, values containing interpolation (${...}), and Railway references (${{...}}).
	if value != "" && !strings.Contains(value, "${") && masker.MaskValue(name, value) == MaskedValue {
		return []Warning{{
			Code:    "W050",
			Message: fmt.Sprintf("variable %q appears to contain a hardcoded secret — consider using ${ENV_VAR} interpolation", name),
			Path:    path,
		}}
	}
	return nil
}

// checkServiceRefs checks variable values for ${{service.VAR}} references to unknown services.
func checkServiceRefs(value, path string, knownServices map[string]bool) []Warning {
	var warnings []Warning
	for _, ref := range serviceRefRe.FindAllStringSubmatch(value, -1) {
		if !knownServices[ref[1]] {
			warnings = append(warnings, Warning{
				Code:    "W060",
				Message: fmt.Sprintf("reference ${{%s...}} refers to service %q not defined in config", ref[1], ref[1]),
				Path:    path,
			})
		}
	}
	return warnings
}

// checkVarValue checks a variable value for value warnings.
func checkVarValue(value, path string) []Warning {
	var warnings []Warning

	// W012: Empty string = delete.
	if value == "" {
		warnings = append(warnings, Warning{
			Code:    "W012",
			Message: fmt.Sprintf("%s: empty string will delete this variable in Railway", path),
			Path:    path,
		})
	}

	// W011: Suspicious ${word.word} syntax (should be ${{...}}).
	// Only flag if the value doesn't already contain ${{...}} around that same ref.
	if suspiciousRefRe.MatchString(value) {
		// Strip legitimate ${{...}} refs, then check what remains.
		stripped := doubleRefRe.ReplaceAllString(value, "")
		if suspiciousRefRe.MatchString(stripped) {
			matches := suspiciousRefRe.FindAllString(stripped, -1)
			for _, m := range matches {
				warnings = append(warnings, Warning{
					Code:    "W011",
					Message: fmt.Sprintf("%s: %s looks like a Railway reference — did you mean ${{%s}}?", path, m, strings.TrimSuffix(strings.TrimPrefix(m, "${"), "}")),
					Path:    path,
				})
			}
		}
	}

	return warnings
}

// hasLowercase returns true if the string contains any lowercase letter.
func hasLowercase(s string) bool {
	for _, r := range s {
		if unicode.IsLower(r) {
			return true
		}
	}
	return false
}

// ValidateFiles checks filesystem conditions that can't be detected from
// the parsed config alone.
func ValidateFiles(dir string) []Warning {
	var warnings []Warning
	localPath := filepath.Join(dir, LocalConfigFile)
	if _, err := os.Stat(localPath); err == nil {
		warnings = append(warnings, Warning{
			Code: "W052",
			Message: fmt.Sprintf("%s is deprecated — move secrets to ${VAR} "+
				"references in %s and use fat-controller.secrets for secret values",
				LocalConfigFile, BaseConfigFile),
			Path: LocalConfigFile,
		})
	}
	return warnings
}
