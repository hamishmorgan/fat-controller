package railway

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	"github.com/hamishmorgan/fat-controller/internal/auth"
	"github.com/hamishmorgan/fat-controller/internal/prompt"
)

// uuidPattern matches Railway-style UUIDs (e.g. "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx").
var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// ResolveProjectEnvironment returns project/environment IDs for the active auth.
// For project tokens, it uses the ProjectToken query. For account tokens, it
// resolves the provided project/environment names (or passes through IDs).
func ResolveProjectEnvironment(ctx context.Context, client *Client, workspace, project, environment string) (string, string, error) {
	if client == nil || client.Auth() == nil {
		return "", "", errors.New("missing auth")
	}
	if client.Auth().Source == auth.SourceEnvToken {
		resp, err := ProjectToken(ctx, client.GQL())
		if err != nil {
			return "", "", err
		}
		return resp.ProjectToken.ProjectId, resp.ProjectToken.EnvironmentId, nil
	}
	projID, err := resolveProjectID(ctx, client, workspace, project)
	if err != nil {
		return "", "", err
	}
	envID, err := resolveEnvironmentID(ctx, client, projID, environment)
	if err != nil {
		return "", "", err
	}
	return projID, envID, nil
}

func resolveProjectID(ctx context.Context, client *Client, workspace, project string) (string, error) {
	if project != "" && uuidPattern.MatchString(project) {
		return project, nil
	}
	workspaceID, err := resolveWorkspaceID(ctx, client, workspace)
	if err != nil {
		return "", err
	}
	resp, err := Projects(ctx, client.GQL(), workspaceID)
	if err != nil {
		return "", err
	}
	if project != "" {
		for _, edge := range resp.Projects.Edges {
			if edge.Node.Name == project {
				return edge.Node.Id, nil
			}
		}
		return "", fmt.Errorf("project not found: %s", project)
	}

	items := make([]prompt.Item, len(resp.Projects.Edges))
	for i, edge := range resp.Projects.Edges {
		items[i] = prompt.Item{Name: edge.Node.Name, ID: edge.Node.Id}
	}
	return prompt.PickProject(items, prompt.StdinIsInteractive())
}

func resolveWorkspaceID(ctx context.Context, client *Client, workspace string) (*string, error) {
	if workspace != "" && uuidPattern.MatchString(workspace) {
		return &workspace, nil
	}
	resp, err := ApiToken(ctx, client.GQL())
	if err != nil {
		return nil, err
	}
	if len(resp.ApiToken.Workspaces) == 0 {
		return nil, errors.New("no workspaces found")
	}
	if workspace != "" {
		for _, ws := range resp.ApiToken.Workspaces {
			if ws.Name == workspace {
				id := ws.Id
				return &id, nil
			}
		}
		return nil, fmt.Errorf("workspace not found: %s", workspace)
	}

	items := make([]prompt.Item, len(resp.ApiToken.Workspaces))
	for i, ws := range resp.ApiToken.Workspaces {
		items[i] = prompt.Item{Name: ws.Name, ID: ws.Id}
	}
	selected, err := prompt.PickWorkspace(items, prompt.StdinIsInteractive())
	if err != nil {
		return nil, err
	}
	return &selected, nil
}

// ResolveWorkspaceID returns a workspace ID, prompting or auto-selecting when missing.
func ResolveWorkspaceID(ctx context.Context, client *Client, workspace string) (string, error) {
	id, err := resolveWorkspaceID(ctx, client, workspace)
	if err != nil {
		return "", err
	}
	if id == nil {
		return "", errors.New("workspace required")
	}
	return *id, nil
}

// ResolveProjectID returns a project ID, prompting or auto-selecting when missing.
func ResolveProjectID(ctx context.Context, client *Client, workspace, project string) (string, error) {
	return resolveProjectID(ctx, client, workspace, project)
}

func resolveEnvironmentID(ctx context.Context, client *Client, projectID, env string) (string, error) {
	if env != "" && uuidPattern.MatchString(env) {
		return env, nil
	}
	resp, err := Environments(ctx, client.GQL(), projectID)
	if err != nil {
		return "", err
	}
	if env != "" {
		for _, edge := range resp.Environments.Edges {
			if edge.Node.Name == env {
				return edge.Node.Id, nil
			}
		}
		return "", fmt.Errorf("environment not found: %s", env)
	}

	items := make([]prompt.Item, len(resp.Environments.Edges))
	for i, edge := range resp.Environments.Edges {
		items[i] = prompt.Item{Name: edge.Node.Name, ID: edge.Node.Id}
	}
	return prompt.PickEnvironment(items, prompt.StdinIsInteractive())
}
