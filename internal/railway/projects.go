package railway

import (
	"context"
	"fmt"
	"log/slog"
)

// ListProjects returns the name/ID pairs for all projects in a workspace.
func ListProjects(ctx context.Context, client *Client, workspaceID string) ([]EntityInfo, error) {
	resp, err := Projects(ctx, client.GQL(), &workspaceID)
	if err != nil {
		return nil, err
	}
	projects := make([]EntityInfo, len(resp.Projects.Edges))
	for i, edge := range resp.Projects.Edges {
		projects[i] = EntityInfo{Name: edge.Node.Name, ID: edge.Node.Id}
	}
	return projects, nil
}

// CreateProject creates a new project.
// Returns the project ID on success.
func CreateProject(ctx context.Context, client *Client, name string) (string, error) {
	slog.Debug("creating project", "name", name)
	input := ProjectCreateInput{
		Name: &name,
	}
	resp, err := ProjectCreate(ctx, client.GQL(), input)
	if err != nil {
		return "", fmt.Errorf("creating project %q: %w", name, err)
	}
	return resp.ProjectCreate.Id, nil
}

// DeleteProject deletes a project by ID.
func DeleteProject(ctx context.Context, client *Client, id string) error {
	slog.Debug("deleting project", "id", id)
	_, err := ProjectDelete(ctx, client.GQL(), id)
	if err != nil {
		return fmt.Errorf("deleting project %q: %w", id, err)
	}
	return nil
}
