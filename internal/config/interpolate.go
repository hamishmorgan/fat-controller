package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// localEnvPattern matches ${VAR_NAME} but NOT ${{...}} Railway references.
// Go's regexp doesn't support lookahead, so we match ${VAR} where VAR
// starts with [A-Za-z_], which excludes `${{...}}`.
var localEnvPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// Interpolate resolves ${VAR} local environment references in all variable
// values in the config. Railway references (${{...}}) are left untouched.
// Returns an error if any referenced env var is not set.
func Interpolate(cfg *DesiredConfig) error {
	if cfg.Shared != nil {
		if err := interpolateVars(cfg.Shared.Vars, "shared.variables"); err != nil {
			return err
		}
	}
	for name, svc := range cfg.Services {
		if svc.Variables != nil {
			if err := interpolateVars(svc.Variables, name+".variables"); err != nil {
				return err
			}
		}
	}
	return nil
}

func interpolateVars(vars map[string]string, section string) error {
	for key, val := range vars {
		resolved, err := interpolateValue(val)
		if err != nil {
			return fmt.Errorf("%s.%s: %w", section, key, err)
		}
		vars[key] = resolved
	}
	return nil
}

func interpolateValue(val string) (string, error) {
	var missing []string
	result := localEnvPattern.ReplaceAllStringFunc(val, func(match string) string {
		// Extract the variable name from ${VAR}.
		sub := localEnvPattern.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		varName := sub[1]

		envVal, ok := os.LookupEnv(varName)
		if !ok {
			missing = append(missing, varName)
			return match
		}
		return envVal
	})
	if len(missing) > 0 {
		return "", fmt.Errorf("unresolved environment variable(s): %s", strings.Join(missing, ", "))
	}
	return result, nil
}
