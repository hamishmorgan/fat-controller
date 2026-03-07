package railway

import (
	"context"
	"fmt"
	"log/slog"
)

// CreateDeploymentTrigger creates a deployment trigger that links a GitHub repo
// and branch to a service. Returns the trigger ID on success.
func CreateDeploymentTrigger(ctx context.Context, client *Client, envID, projectID, serviceID, repo, branch string) (string, error) {
	slog.Debug("creating deployment trigger", "service_id", serviceID, "repo", repo, "branch", branch)
	input := DeploymentTriggerCreateInput{
		EnvironmentId: envID,
		ProjectId:     projectID,
		ServiceId:     serviceID,
		Repository:    repo,
		Branch:        branch,
		Provider:      "github",
	}
	resp, err := DeploymentTriggerCreate(ctx, client.GQL(), input)
	if err != nil {
		return "", fmt.Errorf("creating deployment trigger for %s@%s: %w", repo, branch, err)
	}
	return resp.DeploymentTriggerCreate.Id, nil
}

// DeleteDeploymentTrigger deletes a deployment trigger by ID.
func DeleteDeploymentTrigger(ctx context.Context, client *Client, id string) error {
	slog.Debug("deleting deployment trigger", "id", id)
	_, err := DeploymentTriggerDelete(ctx, client.GQL(), id)
	if err != nil {
		return fmt.Errorf("deleting deployment trigger %q: %w", id, err)
	}
	return nil
}
