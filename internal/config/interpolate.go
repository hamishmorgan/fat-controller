package config

import (
	"fmt"
	"log/slog"
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
// envFileVars is checked first, then the process environment. Pass nil to
// skip env file lookup. Returns an error if any referenced env var is not set.
func Interpolate(cfg *DesiredConfig, envFileVars map[string]string) error {
	slog.Debug("interpolating config variables")

	lookupVar := func(name string) (string, bool) {
		if envFileVars != nil {
			if v, ok := envFileVars[name]; ok {
				return v, true
			}
		}
		return os.LookupEnv(name)
	}

	if cfg.Variables != nil {
		if err := interpolateVars(cfg.Variables, "variables", lookupVar); err != nil {
			return err
		}
	}
	for _, svc := range cfg.Services {
		if svc.Variables != nil {
			if err := interpolateVars(svc.Variables, svc.Name+".variables", lookupVar); err != nil {
				return err
			}
		}
		// Interpolate registry credentials password if present.
		if svc.Deploy != nil && svc.Deploy.RegistryCredentials != nil {
			resolved, err := interpolateValue(svc.Deploy.RegistryCredentials.Password, lookupVar)
			if err != nil {
				return fmt.Errorf("%s.deploy.registry_credentials.password: %w", svc.Name, err)
			}
			svc.Deploy.RegistryCredentials.Password = resolved
		}
	}
	return nil
}

func interpolateVars(vars map[string]string, section string, lookup func(string) (string, bool)) error {
	for key, val := range vars {
		resolved, err := interpolateValue(val, lookup)
		if err != nil {
			return fmt.Errorf("%s.%s: %w", section, key, err)
		}
		vars[key] = resolved
	}
	return nil
}

func interpolateValue(val string, lookup func(string) (string, bool)) (string, error) {
	var missing []string
	result := localEnvPattern.ReplaceAllStringFunc(val, func(match string) string {
		// Extract the variable name from ${VAR}.
		sub := localEnvPattern.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		varName := sub[1]

		envVal, ok := lookup(varName)
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
