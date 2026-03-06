package railway

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"

	"github.com/hamishmorgan/fat-controller/internal/auth"
	"github.com/hamishmorgan/fat-controller/internal/prompt"
)

// uuidPattern matches Railway-style UUIDs (e.g. "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx").
var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// ResolveProjectEnvironment returns project/environment IDs for the active auth.
// For project tokens, it uses the ProjectToken query. For account tokens, it
// resolves the provided project/environment names (or passes through IDs).
// An optional PickOpts controls picker behaviour (e.g. ForcePrompt for init).
func ResolveProjectEnvironment(ctx context.Context, client *Client, workspace, project, environment string, opts ...prompt.PickOpts) (string, string, error) {
	slog.Debug("resolving project and environment", "workspace", workspace, "project", project, "environment", environment)
	pickOpts := prompt.PickOpts{}
	if len(opts) > 0 {
		pickOpts = opts[0]
	}
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
	projID, err := resolveProjectID(ctx, client, workspace, project, pickOpts)
	if err != nil {
		return "", "", err
	}
	envID, err := resolveEnvironmentID(ctx, client, projID, environment, pickOpts)
	if err != nil {
		return "", "", err
	}
	return projID, envID, nil
}

func resolveProjectID(ctx context.Context, client *Client, workspace, project string, pickOpts prompt.PickOpts) (string, error) {
	if project != "" && uuidPattern.MatchString(project) {
		slog.Debug("project is UUID, skipping resolution", "project_id", project)
		return project, nil
	}
	workspaceID, err := resolveWorkspaceID(ctx, client, workspace, pickOpts)
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
	return prompt.PickProject(items, prompt.StdinIsInteractive(), pickOpts)
}

func resolveWorkspaceID(ctx context.Context, client *Client, workspace string, pickOpts prompt.PickOpts) (*string, error) {
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
				slog.Debug("resolved workspace", "name", workspace, "id", id)
				return &id, nil
			}
		}
		return nil, fmt.Errorf("workspace not found: %s", workspace)
	}

	items := make([]prompt.Item, len(resp.ApiToken.Workspaces))
	for i, ws := range resp.ApiToken.Workspaces {
		items[i] = prompt.Item{Name: ws.Name, ID: ws.Id}
	}
	selected, err := prompt.PickWorkspace(items, prompt.StdinIsInteractive(), pickOpts)
	if err != nil {
		return nil, err
	}
	return &selected, nil
}

// ResolveWorkspaceID returns a workspace ID, prompting or auto-selecting when missing.
func ResolveWorkspaceID(ctx context.Context, client *Client, workspace string) (string, error) {
	id, err := resolveWorkspaceID(ctx, client, workspace, prompt.PickOpts{})
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
	return resolveProjectID(ctx, client, workspace, project, prompt.PickOpts{})
}

// ResolveServiceID maps a service name to its ID within a project.
// If the name is already a UUID, it is returned as-is.
func ResolveServiceID(ctx context.Context, client *Client, projectID, service string) (string, error) {
	if service == "" {
		return "", nil // shared scope
	}
	if uuidPattern.MatchString(service) {
		return service, nil
	}
	resp, err := ProjectServices(ctx, client.GQL(), projectID)
	if err != nil {
		return "", err
	}
	for _, edge := range resp.Project.Services.Edges {
		if edge.Node.Name == service {
			return edge.Node.Id, nil
		}
	}
	return "", fmt.Errorf("service not found: %s", service)
}

// ResolvedEntity holds both the name and ID of a resolved Railway entity,
// so callers can display which workspace/project/environment was chosen.
type ResolvedEntity struct {
	Name string
	ID   string
}

// ResolveWorkspaceNamed resolves a workspace and returns both name and ID.
// When workspace is provided by name, it is looked up. When empty and
// interactive, the user picks from a list.
func ResolveWorkspaceNamed(ctx context.Context, client *Client, workspace string, pickOpts prompt.PickOpts) (ResolvedEntity, error) {
	if workspace != "" && uuidPattern.MatchString(workspace) {
		return ResolvedEntity{Name: workspace, ID: workspace}, nil
	}
	resp, err := ApiToken(ctx, client.GQL())
	if err != nil {
		return ResolvedEntity{}, err
	}
	if len(resp.ApiToken.Workspaces) == 0 {
		return ResolvedEntity{}, errors.New("no workspaces found")
	}
	if workspace != "" {
		for _, ws := range resp.ApiToken.Workspaces {
			if ws.Name == workspace {
				slog.Debug("resolved workspace", "name", workspace, "id", ws.Id)
				return ResolvedEntity{Name: ws.Name, ID: ws.Id}, nil
			}
		}
		return ResolvedEntity{}, fmt.Errorf("workspace not found: %s", workspace)
	}

	items := make([]prompt.Item, len(resp.ApiToken.Workspaces))
	for i, ws := range resp.ApiToken.Workspaces {
		items[i] = prompt.Item{Name: ws.Name, ID: ws.Id}
	}
	id, err := prompt.PickWorkspace(items, prompt.StdinIsInteractive(), pickOpts)
	if err != nil {
		return ResolvedEntity{}, err
	}
	// Look up name from ID.
	for _, ws := range resp.ApiToken.Workspaces {
		if ws.Id == id {
			return ResolvedEntity{Name: ws.Name, ID: id}, nil
		}
	}
	return ResolvedEntity{Name: id, ID: id}, nil
}

// ResolveProjectNamed resolves a project within a workspace and returns both name and ID.
func ResolveProjectNamed(ctx context.Context, client *Client, workspaceID, project string, pickOpts prompt.PickOpts) (ResolvedEntity, error) {
	if project != "" && uuidPattern.MatchString(project) {
		return ResolvedEntity{Name: project, ID: project}, nil
	}
	resp, err := Projects(ctx, client.GQL(), &workspaceID)
	if err != nil {
		return ResolvedEntity{}, err
	}
	if project != "" {
		for _, edge := range resp.Projects.Edges {
			if edge.Node.Name == project {
				return ResolvedEntity{Name: edge.Node.Name, ID: edge.Node.Id}, nil
			}
		}
		return ResolvedEntity{}, fmt.Errorf("project not found: %s", project)
	}

	items := make([]prompt.Item, len(resp.Projects.Edges))
	for i, edge := range resp.Projects.Edges {
		items[i] = prompt.Item{Name: edge.Node.Name, ID: edge.Node.Id}
	}
	id, err := prompt.PickProject(items, prompt.StdinIsInteractive(), pickOpts)
	if err != nil {
		return ResolvedEntity{}, err
	}
	for _, edge := range resp.Projects.Edges {
		if edge.Node.Id == id {
			return ResolvedEntity{Name: edge.Node.Name, ID: id}, nil
		}
	}
	return ResolvedEntity{Name: id, ID: id}, nil
}

// ResolveEnvironmentNamed resolves an environment within a project and returns both name and ID.
func ResolveEnvironmentNamed(ctx context.Context, client *Client, projectID, env string, pickOpts prompt.PickOpts) (ResolvedEntity, error) {
	if env != "" && uuidPattern.MatchString(env) {
		return ResolvedEntity{Name: env, ID: env}, nil
	}
	resp, err := Environments(ctx, client.GQL(), projectID)
	if err != nil {
		return ResolvedEntity{}, err
	}
	if env != "" {
		for _, edge := range resp.Environments.Edges {
			if edge.Node.Name == env {
				slog.Debug("resolved environment", "name", env, "id", edge.Node.Id)
				return ResolvedEntity{Name: edge.Node.Name, ID: edge.Node.Id}, nil
			}
		}
		return ResolvedEntity{}, fmt.Errorf("environment not found: %s", env)
	}

	items := make([]prompt.Item, len(resp.Environments.Edges))
	for i, edge := range resp.Environments.Edges {
		items[i] = prompt.Item{Name: edge.Node.Name, ID: edge.Node.Id}
	}
	id, err := prompt.PickEnvironment(items, prompt.StdinIsInteractive(), pickOpts)
	if err != nil {
		return ResolvedEntity{}, err
	}
	for _, edge := range resp.Environments.Edges {
		if edge.Node.Id == id {
			return ResolvedEntity{Name: edge.Node.Name, ID: id}, nil
		}
	}
	return ResolvedEntity{Name: id, ID: id}, nil
}

func resolveEnvironmentID(ctx context.Context, client *Client, projectID, env string, pickOpts prompt.PickOpts) (string, error) {
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
				slog.Debug("resolved environment", "name", env, "id", edge.Node.Id)
				return edge.Node.Id, nil
			}
		}
		return "", fmt.Errorf("environment not found: %s", env)
	}

	items := make([]prompt.Item, len(resp.Environments.Edges))
	for i, edge := range resp.Environments.Edges {
		items[i] = prompt.Item{Name: edge.Node.Name, ID: edge.Node.Id}
	}
	return prompt.PickEnvironment(items, prompt.StdinIsInteractive(), pickOpts)
}
