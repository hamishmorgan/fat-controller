package railway

import (
	"context"
	"fmt"
	"log/slog"
)

// ProjectInfo holds the name and ID of a Railway project.
type ProjectInfo struct {
	ID   string `json:"id" toml:"id"`
	Name string `json:"name" toml:"name"`
}

// projectInfoFromSummary converts a generated ProjectSummaryFields
// fragment into the public ProjectInfo type.
func projectInfoFromSummary(p *ProjectSummaryFields) ProjectInfo {
	return ProjectInfo{ID: p.Id, Name: p.Name}
}

// ListProjects returns the name/ID pairs for all projects in a workspace.
func ListProjects(ctx context.Context, client *Client, workspaceID string) ([]ProjectInfo, error) {
	resp, err := Projects(ctx, client.gql(), &workspaceID)
	if err != nil {
		return nil, err
	}
	projects := make([]ProjectInfo, len(resp.Projects.Edges))
	for i, edge := range resp.Projects.Edges {
		projects[i] = projectInfoFromSummary(&edge.Node.ProjectSummaryFields)
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
	resp, err := ProjectCreate(ctx, client.gql(), input)
	if err != nil {
		return "", fmt.Errorf("creating project %q: %w", name, err)
	}
	return resp.ProjectCreate.Id, nil
}

// DeleteProject deletes a project by ID.
func DeleteProject(ctx context.Context, client *Client, id string) error {
	slog.Debug("deleting project", "id", id)
	_, err := ProjectDelete(ctx, client.gql(), id)
	if err != nil {
		return fmt.Errorf("deleting project %q: %w", id, err)
	}
	return nil
}
