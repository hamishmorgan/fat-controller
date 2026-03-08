package railway

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/hamishmorgan/fat-controller/internal/config"
)

// UpsertVariable sets a single variable for shared or service scope.
func UpsertVariable(ctx context.Context, client *Client, projectID, environmentID, serviceID, name, value string, skipDeploys bool) error {
	slog.Debug("upserting variable", "service_id", serviceID, "name", name, "skip_deploys", skipDeploys)
	input := VariableUpsertInput{
		ProjectId:     projectID,
		EnvironmentId: environmentID,
		Name:          name,
		Value:         value,
		SkipDeploys:   &skipDeploys,
	}
	if serviceID != "" {
		input.ServiceId = &serviceID
	}
	_, err := VariableUpsert(ctx, client.gql(), input)
	return err
}

// UpsertVariableCollection upserts multiple variables in a single API call.
func UpsertVariableCollection(ctx context.Context, client *Client, projectID, environmentID, serviceID string, variables map[string]string, skipDeploys bool) error {
	slog.Debug("upserting variable collection", "service_id", serviceID, "count", len(variables), "skip_deploys", skipDeploys)
	vars := make(map[string]interface{}, len(variables))
	for k, v := range variables {
		vars[k] = v
	}
	input := VariableCollectionUpsertInput{
		ProjectId:     projectID,
		EnvironmentId: environmentID,
		SkipDeploys:   &skipDeploys,
		Variables:     vars,
	}
	if serviceID != "" {
		input.ServiceId = &serviceID
	}
	_, err := VariableCollectionUpsert(ctx, client.gql(), input)
	return err
}

// DeleteVariable deletes a single variable.
func DeleteVariable(ctx context.Context, client *Client, projectID, environmentID, serviceID, name string) error {
	slog.Debug("deleting variable", "service_id", serviceID, "name", name)
	input := VariableDeleteInput{
		ProjectId:     projectID,
		EnvironmentId: environmentID,
		Name:          name,
	}
	if serviceID != "" {
		input.ServiceId = &serviceID
	}
	_, err := VariableDelete(ctx, client.gql(), input)
	return err
}

// UpdateServiceIcon updates the icon of a service. Icon is project-scoped
// (shared across all environments for this service).
func UpdateServiceIcon(ctx context.Context, client *Client, serviceID, icon string) error {
	slog.Debug("updating service icon", "service_id", serviceID, "icon", icon)
	input := ServiceUpdateInput{Icon: &icon}
	_, err := ServiceUpdate(ctx, client.gql(), serviceID, input)
	return err
}

// UpdateServiceLimits updates vCPU and/or memory limits.
// Nil pointers mean "don't change".
func UpdateServiceLimits(ctx context.Context, client *Client, environmentID, serviceID string, vcpus, memoryGB *float64) error {
	slog.Debug("updating service limits", "service_id", serviceID, "vcpus", vcpus, "memory_gb", memoryGB)
	input := ServiceInstanceLimitsUpdateInput{
		EnvironmentId: environmentID,
		ServiceId:     serviceID,
		VCPUs:         vcpus,
		MemoryGB:      memoryGB,
	}
	_, err := ServiceInstanceLimitsUpdate(ctx, client.gql(), input)
	return err
}

// UpdateServiceSettings converts desired deploy settings to the GraphQL input
// type and updates the service instance. Only non-nil fields in desired are
// set; Railway treats nil as "don't change".
func UpdateServiceSettings(ctx context.Context, client *Client, serviceID string, desired *config.DesiredDeploy) error {
	if desired == nil {
		return nil
	}
	slog.Debug("updating service settings", "service_id", serviceID)
	input, err := buildServiceInstanceInput(desired)
	if err != nil {
		return err
	}
	_, err = ServiceInstanceUpdate(ctx, client.gql(), serviceID, input)
	return err
}

// buildServiceInstanceInput converts a DesiredDeploy to the generated GraphQL
// input type. Kept package-private so the generated types don't leak.
func buildServiceInstanceInput(desired *config.DesiredDeploy) (ServiceInstanceUpdateInput, error) {
	var input ServiceInstanceUpdateInput

	// Builder
	if desired.Builder != nil {
		b, err := parseBuilder(*desired.Builder)
		if err != nil {
			return input, err
		}
		input.Builder = &b
	}

	// Source
	if desired.Repo != nil || desired.Image != nil {
		input.Source = &ServiceSourceInput{
			Repo:  desired.Repo,
			Image: desired.Image,
		}
	}
	if desired.RegistryCredentials != nil {
		input.RegistryCredentials = &RegistryCredentialsInput{
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
		rp, err := parseRestartPolicy(*desired.RestartPolicy)
		if err != nil {
			return input, err
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

// parseBuilder maps a string to the generated Builder enum.
func parseBuilder(value string) (Builder, error) {
	switch strings.ToUpper(value) {
	case "NIXPACKS":
		return BuilderNixpacks, nil
	case "RAILPACK":
		return BuilderRailpack, nil
	case "PAKETO":
		return BuilderPaketo, nil
	case "HEROKU":
		return BuilderHeroku, nil
	default:
		return "", fmt.Errorf("unknown builder: %q (valid: NIXPACKS, RAILPACK, PAKETO, HEROKU)", value)
	}
}

// parseRestartPolicy maps a string to the generated RestartPolicyType enum.
func parseRestartPolicy(value string) (RestartPolicyType, error) {
	switch strings.ToUpper(value) {
	case "ALWAYS":
		return RestartPolicyTypeAlways, nil
	case "NEVER":
		return RestartPolicyTypeNever, nil
	case "ON_FAILURE":
		return RestartPolicyTypeOnFailure, nil
	default:
		return "", fmt.Errorf("unknown restart_policy: %q (valid: ALWAYS, NEVER, ON_FAILURE)", value)
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
