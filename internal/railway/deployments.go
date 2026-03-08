package railway

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// DeployService triggers a deployment for a service instance.
// Returns the deployment ID on success.
func DeployService(ctx context.Context, client *Client, environmentID, serviceID string, commitSha *string) (string, error) {
	slog.Debug("deploying service", "environment_id", environmentID, "service_id", serviceID, "commit_sha", commitSha)
	resp, err := ServiceInstanceDeployV2(ctx, client.gql(), environmentID, serviceID, commitSha)
	if err != nil {
		return "", fmt.Errorf("deploying service %q: %w", serviceID, err)
	}
	return resp.ServiceInstanceDeployV2, nil
}

// RedeployDeployment redeploys an existing deployment.
// Returns the new deployment ID on success.
func RedeployDeployment(ctx context.Context, client *Client, id string) (string, error) {
	slog.Debug("redeploying deployment", "id", id)
	resp, err := DeploymentRedeploy(ctx, client.gql(), id, nil)
	if err != nil {
		return "", fmt.Errorf("redeploying deployment %q: %w", id, err)
	}
	return resp.DeploymentRedeploy.Id, nil
}

// RestartDeployment restarts a deployment.
func RestartDeployment(ctx context.Context, client *Client, id string) error {
	slog.Debug("restarting deployment", "id", id)
	_, err := DeploymentRestart(ctx, client.gql(), id)
	if err != nil {
		return fmt.Errorf("restarting deployment %q: %w", id, err)
	}
	return nil
}

// CancelDeployment cancels a deployment.
func CancelDeployment(ctx context.Context, client *Client, id string) error {
	slog.Debug("cancelling deployment", "id", id)
	_, err := DeploymentCancel(ctx, client.gql(), id)
	if err != nil {
		return fmt.Errorf("cancelling deployment %q: %w", id, err)
	}
	return nil
}

// RollbackDeployment rolls back to a deployment.
func RollbackDeployment(ctx context.Context, client *Client, id string) error {
	slog.Debug("rolling back deployment", "id", id)
	_, err := DeploymentRollback(ctx, client.gql(), id)
	if err != nil {
		return fmt.Errorf("rolling back deployment %q: %w", id, err)
	}
	return nil
}

// DeploymentInfo contains summary information about a deployment.
type DeploymentInfo struct {
	ID        string
	Status    string
	CreatedAt time.Time
	StaticUrl *string
}

// ListDeployments lists deployments for a service with pagination.
// Pass nil for after to start from the beginning.
func ListDeployments(ctx context.Context, client *Client, environmentID, serviceID string, limit int, after *string) ([]DeploymentInfo, bool, error) {
	slog.Debug("listing deployments", "environment_id", environmentID, "service_id", serviceID, "limit", limit)
	input := DeploymentListInput{
		EnvironmentId: &environmentID,
		ServiceId:     &serviceID,
	}
	resp, err := Deployments(ctx, client.gql(), input, &limit, after)
	if err != nil {
		return nil, false, fmt.Errorf("listing deployments for service %q: %w", serviceID, err)
	}
	infos := make([]DeploymentInfo, len(resp.Deployments.Edges))
	for i, edge := range resp.Deployments.Edges {
		infos[i] = DeploymentInfo{
			ID:        edge.Node.Id,
			Status:    string(edge.Node.Status),
			CreatedAt: edge.Node.CreatedAt,
			StaticUrl: edge.Node.StaticUrl,
		}
	}
	return infos, resp.Deployments.PageInfo.HasNextPage, nil
}
