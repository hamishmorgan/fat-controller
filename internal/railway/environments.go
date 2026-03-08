package railway

import (
	"context"
	"fmt"
	"log/slog"
)

// EnvironmentInfo holds the name and ID of a Railway environment.
type EnvironmentInfo struct {
	ID   string `json:"id" toml:"id"`
	Name string `json:"name" toml:"name"`
}

// environmentInfoFromSummary converts a generated EnvironmentSummaryFields
// fragment into the public EnvironmentInfo type.
func environmentInfoFromSummary(e *EnvironmentSummaryFields) EnvironmentInfo {
	return EnvironmentInfo{ID: e.Id, Name: e.Name}
}

// ListEnvironments returns the name/ID pairs for all environments in a project.
func ListEnvironments(ctx context.Context, client *Client, projectID string) ([]EnvironmentInfo, error) {
	resp, err := Environments(ctx, client.gql(), projectID)
	if err != nil {
		return nil, err
	}
	envs := make([]EnvironmentInfo, len(resp.Environments.Edges))
	for i, edge := range resp.Environments.Edges {
		envs[i] = environmentInfoFromSummary(&edge.Node.EnvironmentSummaryFields)
	}
	return envs, nil
}

// CreateEnvironment creates a new environment in a project.
// Returns the environment ID on success.
func CreateEnvironment(ctx context.Context, client *Client, projectID, name string) (string, error) {
	slog.Debug("creating environment", "project_id", projectID, "name", name)
	input := EnvironmentCreateInput{
		ProjectId: projectID,
		Name:      name,
	}
	resp, err := EnvironmentCreate(ctx, client.gql(), input)
	if err != nil {
		return "", fmt.Errorf("creating environment %q: %w", name, err)
	}
	return resp.EnvironmentCreate.Id, nil
}

// DeleteEnvironment deletes an environment by ID.
func DeleteEnvironment(ctx context.Context, client *Client, id string) error {
	slog.Debug("deleting environment", "id", id)
	_, err := EnvironmentDelete(ctx, client.gql(), id)
	if err != nil {
		return fmt.Errorf("deleting environment %q: %w", id, err)
	}
	return nil
}
