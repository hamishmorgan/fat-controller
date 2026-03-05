package railway

import (
	"context"
	"log/slog"
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
	_, err := VariableUpsert(ctx, client.GQL(), input)
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
	_, err := VariableCollectionUpsert(ctx, client.GQL(), input)
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
	_, err := VariableDelete(ctx, client.GQL(), input)
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
	_, err := ServiceInstanceLimitsUpdate(ctx, client.GQL(), input)
	return err
}

// UpdateServiceSettings updates deploy/build settings.
// The generated ServiceInstanceUpdate function takes serviceId as a separate
// argument (matching the GraphQL schema where it's a top-level mutation arg).
func UpdateServiceSettings(ctx context.Context, client *Client, serviceID string, input ServiceInstanceUpdateInput) error {
	slog.Debug("updating service settings", "service_id", serviceID)
	_, err := ServiceInstanceUpdate(ctx, client.GQL(), serviceID, input)
	return err
}
