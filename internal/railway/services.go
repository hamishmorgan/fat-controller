package railway

import (
	"context"
	"fmt"
	"log/slog"
)

// EntityInfo is a lightweight name+ID pair returned by list operations.
// Used for workspaces, projects, environments, and services.
type EntityInfo struct {
	ID   string `json:"id" toml:"id"`
	Name string `json:"name" toml:"name"`
}

// ListServices returns the name/ID pairs for all services in a project.
func ListServices(ctx context.Context, client *Client, projectID string) ([]EntityInfo, error) {
	resp, err := ProjectServices(ctx, client.GQL(), projectID)
	if err != nil {
		return nil, err
	}
	services := make([]EntityInfo, len(resp.Project.Services.Edges))
	for i, edge := range resp.Project.Services.Edges {
		services[i] = EntityInfo{Name: edge.Node.Name, ID: edge.Node.Id}
	}
	return services, nil
}

// CreateService creates a new service in a project.
// Returns the service ID on success.
func CreateService(ctx context.Context, client *Client, projectID, name string) (string, error) {
	slog.Debug("creating service", "project_id", projectID, "name", name)
	input := ServiceCreateInput{
		ProjectId: projectID,
		Name:      &name,
	}
	resp, err := ServiceCreate(ctx, client.GQL(), input)
	if err != nil {
		return "", fmt.Errorf("creating service %q: %w", name, err)
	}
	return resp.ServiceCreate.Id, nil
}

// DeleteService deletes a service by ID.
func DeleteService(ctx context.Context, client *Client, id string) error {
	slog.Debug("deleting service", "id", id)
	_, err := ServiceDelete(ctx, client.GQL(), id)
	if err != nil {
		return fmt.Errorf("deleting service %q: %w", id, err)
	}
	return nil
}

// UpdateService updates a service's name and/or icon.
func UpdateService(ctx context.Context, client *Client, id string, input ServiceUpdateInput) error {
	slog.Debug("updating service", "id", id)
	_, err := ServiceUpdate(ctx, client.GQL(), id, input)
	if err != nil {
		return fmt.Errorf("updating service %q: %w", id, err)
	}
	return nil
}

// ConnectService connects a service to a source repo or image.
func ConnectService(ctx context.Context, client *Client, id string, input ServiceConnectInput) error {
	slog.Debug("connecting service", "id", id)
	_, err := ServiceConnect(ctx, client.GQL(), id, input)
	if err != nil {
		return fmt.Errorf("connecting service %q: %w", id, err)
	}
	return nil
}
