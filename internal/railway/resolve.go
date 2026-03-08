package railway

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/hamishmorgan/fat-controller/internal/auth"
)

// WorkspaceInfo holds the name and ID of a Railway workspace.
type WorkspaceInfo struct {
	ID   string `json:"id" toml:"id"`
	Name string `json:"name" toml:"name"`
}

// ListWorkspaces returns the name/ID pairs for all workspaces the token has access to.
func ListWorkspaces(ctx context.Context, client *Client) ([]WorkspaceInfo, error) {
	resp, err := ApiToken(ctx, client.gql())
	if err != nil {
		return nil, err
	}
	workspaces := make([]WorkspaceInfo, len(resp.ApiToken.Workspaces))
	for i, ws := range resp.ApiToken.Workspaces {
		workspaces[i] = WorkspaceInfo{Name: ws.Name, ID: ws.Id}
	}
	return workspaces, nil
}

// Picker selects an ID from a list of candidates. Called when resolution
// finds multiple matches and no explicit name was given.
//
// Pass nil for non-interactive mode: single results are auto-selected,
// multiple results produce an error listing the candidates.
type Picker func(label string, items []PickCandidate) (id string, err error)

// PickCandidate is a name+ID pair presented to an interactive picker.
type PickCandidate struct {
	Name string
	ID   string
}

// uuidPattern matches Railway-style UUIDs (e.g. "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx").
var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// ResolveProjectEnvironment returns project/environment IDs for the active auth.
// For project tokens, it uses the ProjectToken query. For account tokens, it
// resolves the provided project/environment names (or passes through IDs).
// The picker is called when interactive selection is needed; pass nil for
// non-interactive mode.
func ResolveProjectEnvironment(ctx context.Context, client *Client, workspace, project, environment string, picker Picker) (string, string, error) {
	slog.Debug("resolving project and environment", "workspace", workspace, "project", project, "environment", environment)
	if client == nil || client.Auth() == nil {
		return "", "", errors.New("missing auth")
	}
	if client.Auth().Source == auth.SourceEnvToken {
		resp, err := ProjectToken(ctx, client.gql())
		if err != nil {
			return "", "", err
		}
		return resp.ProjectToken.ProjectId, resp.ProjectToken.EnvironmentId, nil
	}
	projID, err := resolveProjectID(ctx, client, workspace, project, picker)
	if err != nil {
		return "", "", err
	}
	envID, err := resolveEnvironmentID(ctx, client, projID, environment, picker)
	if err != nil {
		return "", "", err
	}
	return projID, envID, nil
}

func resolveProjectID(ctx context.Context, client *Client, workspace, project string, picker Picker) (string, error) {
	if project != "" && uuidPattern.MatchString(project) {
		slog.Debug("project is UUID, skipping resolution", "project_id", project)
		return project, nil
	}
	workspaceID, err := resolveWorkspaceID(ctx, client, workspace, picker)
	if err != nil {
		return "", err
	}
	resp, err := Projects(ctx, client.gql(), workspaceID)
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

	items := make([]PickCandidate, len(resp.Projects.Edges))
	for i, edge := range resp.Projects.Edges {
		items[i] = PickCandidate{Name: edge.Node.Name, ID: edge.Node.Id}
	}
	return pickOne("project", items, picker)
}

func resolveWorkspaceID(ctx context.Context, client *Client, workspace string, picker Picker) (*string, error) {
	if workspace != "" && uuidPattern.MatchString(workspace) {
		return &workspace, nil
	}
	resp, err := ApiToken(ctx, client.gql())
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

	items := make([]PickCandidate, len(resp.ApiToken.Workspaces))
	for i, ws := range resp.ApiToken.Workspaces {
		items[i] = PickCandidate{Name: ws.Name, ID: ws.Id}
	}
	selected, err := pickOne("workspace", items, picker)
	if err != nil {
		return nil, err
	}
	return &selected, nil
}

// ResolveWorkspaceID returns a workspace ID, resolving a name or auto-selecting.
// Pass nil picker for non-interactive mode.
func ResolveWorkspaceID(ctx context.Context, client *Client, workspace string) (string, error) {
	id, err := resolveWorkspaceID(ctx, client, workspace, nil)
	if err != nil {
		return "", err
	}
	if id == nil {
		return "", errors.New("workspace required")
	}
	return *id, nil
}

// ResolveProjectID returns a project ID, resolving a name or auto-selecting.
// Pass nil picker for non-interactive mode.
func ResolveProjectID(ctx context.Context, client *Client, workspace, project string) (string, error) {
	return resolveProjectID(ctx, client, workspace, project, nil)
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
	resp, err := ProjectServices(ctx, client.gql(), projectID)
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

func resolveEnvironmentID(ctx context.Context, client *Client, projectID, env string, picker Picker) (string, error) {
	if env != "" && uuidPattern.MatchString(env) {
		return env, nil
	}
	resp, err := Environments(ctx, client.gql(), projectID)
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

	items := make([]PickCandidate, len(resp.Environments.Edges))
	for i, edge := range resp.Environments.Edges {
		items[i] = PickCandidate{Name: edge.Node.Name, ID: edge.Node.Id}
	}
	return pickOne("environment", items, picker)
}

// pickOne selects from candidates:
//   - 0 items: error
//   - 1 item: auto-select
//   - multiple + picker: delegate to picker
//   - multiple + nil picker: error listing candidates
func pickOne(label string, items []PickCandidate, picker Picker) (string, error) {
	if len(items) == 0 {
		return "", fmt.Errorf("no %ss found", label)
	}
	if len(items) == 1 {
		return items[0].ID, nil
	}
	if picker != nil {
		return picker(label, items)
	}
	// Non-interactive: list candidates in the error message.
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "multiple %ss available — specify with --%s flag:", label, label)
	for _, item := range items {
		_, _ = fmt.Fprintf(&b, "\n  %s (%s)", item.Name, item.ID)
	}
	return "", errors.New(b.String())
}
