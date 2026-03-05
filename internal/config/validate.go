package config

import (
	"fmt"
	"log/slog"
	"regexp"
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

// Validate runs advisory checks on a DesiredConfig and returns warnings.
// liveServiceNames is the list of service names that actually exist in Railway;
// pass nil to skip W040 checks (e.g. for offline validation).
// Warnings whose codes appear in cfg.SuppressWarnings are filtered out.
func Validate(cfg *DesiredConfig, liveServiceNames []string) []Warning {
	slog.Debug("validating config", "services", len(cfg.Services), "has_live_names", liveServiceNames != nil)
	var warnings []Warning

	// Build suppression set.
	suppressed := make(map[string]bool, len(cfg.SuppressWarnings))
	for _, code := range cfg.SuppressWarnings {
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
	if cfg.Shared != nil {
		for name, value := range cfg.Shared.Vars {
			path := "shared.variables." + name
			warnings = append(warnings, checkVarName(name, path)...)
			warnings = append(warnings, checkVarValue(value, path)...)
		}
	}

	// Check each service.
	for svcName, svc := range cfg.Services {
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

			// W020: Variable in both shared and service.
			if cfg.Shared != nil {
				if _, ok := cfg.Shared.Vars[name]; ok {
					warnings = append(warnings, Warning{
						Code:    "W020",
						Message: fmt.Sprintf("variable %q defined in both shared and service %q (service wins)", name, svcName),
						Path:    path,
					})
				}
			}
		}
	}

	// W040: Unknown service name.
	if liveServiceNames != nil {
		liveSet := make(map[string]bool, len(liveServiceNames))
		for _, name := range liveServiceNames {
			liveSet[name] = true
		}
		for svcName := range cfg.Services {
			if !liveSet[svcName] {
				warnings = append(warnings, Warning{
					Code:    "W040",
					Message: fmt.Sprintf("service %q not found in Railway project", svcName),
					Path:    svcName,
				})
			}
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

// hasAnythingActionable returns true if the config defines at least one service
// or at least one shared variable.
func hasAnythingActionable(cfg *DesiredConfig) bool {
	if cfg.Shared != nil && len(cfg.Shared.Vars) > 0 {
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
