package railway

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// LogEntry represents a single log line.
type LogEntry struct {
	Message   string
	Severity  *string
	Timestamp string
}

// GetDeploymentLogs fetches logs for a deployment.
// limit defaults to 100 on the server side; max is 5000.
func GetDeploymentLogs(ctx context.Context, client *Client, deploymentID string, limit *int, startDate, endDate *time.Time, filter *string) ([]LogEntry, error) {
	slog.Debug("fetching deployment logs", "deployment_id", deploymentID, "limit", limit)
	resp, err := DeploymentLogs(ctx, client.GQL(), deploymentID, limit, startDate, endDate, filter)
	if err != nil {
		return nil, fmt.Errorf("fetching deployment logs for %q: %w", deploymentID, err)
	}
	entries := make([]LogEntry, len(resp.DeploymentLogs))
	for i, l := range resp.DeploymentLogs {
		entries[i] = LogEntry{
			Message:   l.Message,
			Severity:  l.Severity,
			Timestamp: l.Timestamp,
		}
	}
	return entries, nil
}

// GetBuildLogs fetches build logs for a deployment.
// limit defaults to 100 on the server side; max is 5000.
func GetBuildLogs(ctx context.Context, client *Client, deploymentID string, limit *int, startDate, endDate *time.Time, filter *string) ([]LogEntry, error) {
	slog.Debug("fetching build logs", "deployment_id", deploymentID, "limit", limit)
	resp, err := BuildLogs(ctx, client.GQL(), deploymentID, limit, startDate, endDate, filter)
	if err != nil {
		return nil, fmt.Errorf("fetching build logs for %q: %w", deploymentID, err)
	}
	entries := make([]LogEntry, len(resp.BuildLogs))
	for i, l := range resp.BuildLogs {
		entries[i] = LogEntry{
			Message:   l.Message,
			Severity:  l.Severity,
			Timestamp: l.Timestamp,
		}
	}
	return entries, nil
}

// GetEnvironmentLogs fetches logs for a project environment.
// Build logs are excluded unless a snapshot ID is explicitly provided in the filter.
func GetEnvironmentLogs(ctx context.Context, client *Client, environmentID string, limit *int, filter *string) ([]LogEntry, error) {
	slog.Debug("fetching environment logs", "environment_id", environmentID, "limit", limit)
	resp, err := EnvironmentLogs(ctx, client.GQL(), environmentID, nil, nil, limit, nil, nil, filter)
	if err != nil {
		return nil, fmt.Errorf("fetching environment logs for %q: %w", environmentID, err)
	}
	entries := make([]LogEntry, len(resp.EnvironmentLogs))
	for i, l := range resp.EnvironmentLogs {
		entries[i] = LogEntry{
			Message:   l.Message,
			Severity:  l.Severity,
			Timestamp: l.Timestamp,
		}
	}
	return entries, nil
}
