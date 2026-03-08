package railway

import (
	"context"
	"fmt"
	"log/slog"
)

// ServiceInfo holds the name, ID, and icon of a Railway service.
type ServiceInfo struct {
	ID   string `json:"id" toml:"id"`
	Name string `json:"name" toml:"name"`
	Icon string `json:"icon,omitempty" toml:"icon,omitempty"`
}

// serviceInfoFromSummary converts a generated ServiceSummaryFields fragment
// into the public ServiceInfo type (which carries toml struct tags for CLI output).
func serviceInfoFromSummary(s *ServiceSummaryFields) ServiceInfo {
	icon := ""
	if s.Icon != nil {
		icon = *s.Icon
	}
	return ServiceInfo{ID: s.Id, Name: s.Name, Icon: icon}
}

// ListServices returns the name/ID pairs for all services in a project.
func ListServices(ctx context.Context, client *Client, projectID string) ([]ServiceInfo, error) {
	resp, err := ProjectServices(ctx, client.gql(), projectID)
	if err != nil {
		return nil, err
	}
	services := make([]ServiceInfo, len(resp.Project.Services.Edges))
	for i, edge := range resp.Project.Services.Edges {
		services[i] = serviceInfoFromSummary(&edge.Node)
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
	resp, err := ServiceCreate(ctx, client.gql(), input)
	if err != nil {
		return "", fmt.Errorf("creating service %q: %w", name, err)
	}
	return resp.ServiceCreate.Id, nil
}

// DeleteService deletes a service by ID.
func DeleteService(ctx context.Context, client *Client, id string) error {
	slog.Debug("deleting service", "id", id)
	_, err := ServiceDelete(ctx, client.gql(), id)
	if err != nil {
		return fmt.Errorf("deleting service %q: %w", id, err)
	}
	return nil
}
