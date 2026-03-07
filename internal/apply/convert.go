package apply

import (
	"fmt"
	"strings"

	"github.com/hamishmorgan/fat-controller/internal/config"
	"github.com/hamishmorgan/fat-controller/internal/railway"
)

// ToServiceInstanceUpdateInput converts a DesiredDeploy to the Railway
// GraphQL input type. Only non-nil fields are set; others remain nil
// (Railway treats nil as "don't change").
func ToServiceInstanceUpdateInput(desired *config.DesiredDeploy) (railway.ServiceInstanceUpdateInput, error) {
	var input railway.ServiceInstanceUpdateInput
	if desired == nil {
		return input, nil
	}

	// Builder
	if desired.Builder != nil {
		b, ok := parseBuilder(*desired.Builder)
		if !ok {
			return input, fmt.Errorf("unknown builder: %q (valid: NIXPACKS, RAILPACK, PAKETO, HEROKU)", *desired.Builder)
		}
		input.Builder = &b
	}

	// Source
	if desired.Repo != nil || desired.Image != nil {
		source := &railway.ServiceSourceInput{
			Repo:  desired.Repo,
			Image: desired.Image,
		}
		input.Source = source
	}
	if desired.RegistryCredentials != nil {
		input.RegistryCredentials = &railway.RegistryCredentialsInput{
			Username: desired.RegistryCredentials.Username,
			Password: desired.RegistryCredentials.Password,
		}
	}

	// Build
	input.BuildCommand = desired.BuildCommand
	input.DockerfilePath = desired.DockerfilePath
	input.RootDirectory = desired.RootDirectory
	if desired.WatchPatterns != nil {
		input.WatchPatterns = desired.WatchPatterns
	}

	// Run
	input.StartCommand = desired.StartCommand
	input.CronSchedule = desired.CronSchedule
	if desired.PreDeployCommand != nil {
		input.PreDeployCommand = toPreDeployCommand(desired.PreDeployCommand)
	}

	// Health
	input.HealthcheckPath = desired.HealthcheckPath
	input.HealthcheckTimeout = desired.HealthcheckTimeout
	if desired.RestartPolicy != nil {
		rp, ok := parseRestartPolicy(*desired.RestartPolicy)
		if !ok {
			return input, fmt.Errorf("unknown restart_policy: %q (valid: ALWAYS, NEVER, ON_FAILURE)", *desired.RestartPolicy)
		}
		input.RestartPolicyType = &rp
	}
	input.RestartPolicyMaxRetries = desired.RestartPolicyMaxRetries

	// Deploy strategy
	input.DrainingSeconds = desired.DrainingSeconds
	input.OverlapSeconds = desired.OverlapSeconds
	input.SleepApplication = desired.SleepApplication

	// Placement
	input.NumReplicas = desired.NumReplicas
	input.Region = desired.Region

	// Networking
	input.Ipv6EgressEnabled = desired.IPv6Egress

	return input, nil
}

func parseBuilder(value string) (railway.Builder, bool) {
	switch strings.ToUpper(value) {
	case "NIXPACKS":
		return railway.BuilderNixpacks, true
	case "RAILPACK":
		return railway.BuilderRailpack, true
	case "PAKETO":
		return railway.BuilderPaketo, true
	case "HEROKU":
		return railway.BuilderHeroku, true
	default:
		return "", false
	}
}

func parseRestartPolicy(value string) (railway.RestartPolicyType, bool) {
	switch strings.ToUpper(value) {
	case "ALWAYS":
		return railway.RestartPolicyTypeAlways, true
	case "NEVER":
		return railway.RestartPolicyTypeNever, true
	case "ON_FAILURE":
		return railway.RestartPolicyTypeOnFailure, true
	default:
		return "", false
	}
}

// toPreDeployCommand converts the DesiredDeploy.PreDeployCommand (any) to []string.
// TOML allows string or array; we normalize to []string for the API.
func toPreDeployCommand(v any) []string {
	switch val := v.(type) {
	case string:
		if val == "" {
			return nil
		}
		return []string{val}
	case []any:
		result := make([]string, 0, len(val))
		for _, item := range val {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	case []string:
		return val
	default:
		return nil
	}
}
