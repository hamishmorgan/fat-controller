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

// workspaceInfoFromFields converts a generated WorkspaceFields fragment
// into the public WorkspaceInfo type.
func workspaceInfoFromFields(w *WorkspaceFields) WorkspaceInfo {
	return WorkspaceInfo{ID: w.Id, Name: w.Name}
}

// ListWorkspaces returns the name/ID pairs for all workspaces the token has access to.
func ListWorkspaces(ctx context.Context, client *Client) ([]WorkspaceInfo, error) {
	resp, err := ApiToken(ctx, client.gql())
	if err != nil {
		return nil, err
	}
	workspaces := make([]WorkspaceInfo, len(resp.ApiToken.Workspaces))
	for i, ws := range resp.ApiToken.Workspaces {
		workspaces[i] = workspaceInfoFromFields(&ws.WorkspaceFields)
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

// ResolveResult holds the IDs and human-readable names produced by resolution.
type ResolveResult struct {
	ProjectID       string
	EnvironmentID   string
	WorkspaceName   string
	ProjectName     string
	EnvironmentName string
	// Services is populated when resolution fetches the service list in the
	// same query (via ProjectsResolution). Nil when not available (e.g.
	// project-token auth or UUID-based resolution).
	Services []ServiceInfo
}

// ResolveProjectEnvironment returns project/environment IDs for the active auth.
// For project tokens, it uses the ProjectToken query. For account tokens, it
// resolves the provided project/environment names (or passes through IDs).
// The picker is called when interactive selection is needed; pass nil for
// non-interactive mode.
func ResolveProjectEnvironment(ctx context.Context, client *Client, workspace, project, environment string, picker Picker) (*ResolveResult, error) {
	slog.Debug("resolving project and environment", "workspace", workspace, "project", project, "environment", environment)
	if client == nil || client.Auth() == nil {
		return nil, errors.New("missing auth")
	}
	if client.Auth().Source == auth.SourceEnvToken {
		resp, err := ProjectToken(ctx, client.gql())
		if err != nil {
			return nil, err
		}
		return &ResolveResult{
			ProjectID:     resp.ProjectToken.ProjectId,
			EnvironmentID: resp.ProjectToken.EnvironmentId,
		}, nil
	}

	// Fast path: if both project and environment are UUIDs, skip all resolution queries.
	if project != "" && uuidPattern.MatchString(project) && environment != "" && uuidPattern.MatchString(environment) {
		return &ResolveResult{
			ProjectID:     project,
			EnvironmentID: environment,
		}, nil
	}

	// Use ProjectsResolution to fetch projects with their environments and
	// services in a single query (replaces separate Projects + Environments
	// + ProjectServices calls).
	result, err := resolveWithBulkQuery(ctx, client, workspace, project, environment, picker)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// resolveWithBulkQuery uses the ProjectsResolution query (projects + environments +
// services in one HTTP request) to resolve project, environment, and services.
func resolveWithBulkQuery(ctx context.Context, client *Client, workspace, project, environment string, picker Picker) (*ResolveResult, error) {
	wsResult, err := resolveWorkspaceID(ctx, client, workspace, picker)
	if err != nil {
		return nil, err
	}

	resp, err := ProjectsResolution(ctx, client.gql(), wsResult.id)
	if err != nil {
		return nil, err
	}

	// Resolve project from the results. Each edge node embeds ProjectSummaryFields.
	type projEdge = ProjectsResolutionProjectsQueryProjectsConnectionEdgesQueryProjectsConnectionEdgeNodeProject
	var matchedProject *projEdge

	if project != "" && uuidPattern.MatchString(project) {
		for i, edge := range resp.Projects.Edges {
			if edge.Node.Id == project {
				matchedProject = &resp.Projects.Edges[i].Node
				break
			}
		}
		if matchedProject == nil {
			return nil, fmt.Errorf("project not found: %s", project)
		}
	} else if project != "" {
		for i, edge := range resp.Projects.Edges {
			if edge.Node.Name == project {
				matchedProject = &resp.Projects.Edges[i].Node
				break
			}
		}
		if matchedProject == nil {
			return nil, fmt.Errorf("project not found: %s", project)
		}
	} else {
		items := make([]PickCandidate, len(resp.Projects.Edges))
		for i, edge := range resp.Projects.Edges {
			items[i] = PickCandidate{Name: edge.Node.Name, ID: edge.Node.Id}
		}
		id, err := pickOne("project", items, picker)
		if err != nil {
			return nil, err
		}
		for i, edge := range resp.Projects.Edges {
			if edge.Node.Id == id {
				matchedProject = &resp.Projects.Edges[i].Node
				break
			}
		}
	}

	// Resolve environment from the matched project's environments.
	// Each edge node embeds EnvironmentSummaryFields.
	var envID, envName string
	if environment != "" && uuidPattern.MatchString(environment) {
		envID = environment
		for _, edge := range matchedProject.Environments.Edges {
			if edge.Node.Id == environment {
				envName = edge.Node.Name
				break
			}
		}
	} else if environment != "" {
		for _, edge := range matchedProject.Environments.Edges {
			if edge.Node.Name == environment {
				envID = edge.Node.Id
				envName = edge.Node.Name
				break
			}
		}
		if envID == "" {
			return nil, fmt.Errorf("environment not found: %s", environment)
		}
	} else {
		items := make([]PickCandidate, len(matchedProject.Environments.Edges))
		for i, edge := range matchedProject.Environments.Edges {
			items[i] = PickCandidate{Name: edge.Node.Name, ID: edge.Node.Id}
		}
		id, err := pickOne("environment", items, picker)
		if err != nil {
			return nil, err
		}
		envID = id
		for _, edge := range matchedProject.Environments.Edges {
			if edge.Node.Id == id {
				envName = edge.Node.Name
				break
			}
		}
	}

	slog.Debug("resolved environment", "name", envName, "id", envID)

	// Collect services from the matched project using the shared fragment type.
	var services []ServiceInfo
	for _, edge := range matchedProject.Services.Edges {
		services = append(services, serviceInfoFromSummary(&edge.Node.ServiceSummaryFields))
	}

	return &ResolveResult{
		ProjectID:       matchedProject.Id,
		EnvironmentID:   envID,
		WorkspaceName:   wsResult.name,
		ProjectName:     matchedProject.Name,
		EnvironmentName: envName,
		Services:        services,
	}, nil
}

type projectResult struct {
	id            string
	name          string
	workspaceName string
}

func resolveProjectID(ctx context.Context, client *Client, workspace, project string, picker Picker) (projectResult, error) {
	if project != "" && uuidPattern.MatchString(project) {
		slog.Debug("project is UUID, skipping resolution", "project_id", project)
		return projectResult{id: project}, nil
	}
	wsResult, err := resolveWorkspaceID(ctx, client, workspace, picker)
	if err != nil {
		return projectResult{}, err
	}
	resp, err := Projects(ctx, client.gql(), wsResult.id)
	if err != nil {
		return projectResult{}, err
	}
	if project != "" {
		for _, edge := range resp.Projects.Edges {
			if edge.Node.Name == project {
				return projectResult{id: edge.Node.Id, name: edge.Node.Name, workspaceName: wsResult.name}, nil
			}
		}
		return projectResult{}, fmt.Errorf("project not found: %s", project)
	}

	items := make([]PickCandidate, len(resp.Projects.Edges))
	for i, edge := range resp.Projects.Edges {
		items[i] = PickCandidate{Name: edge.Node.Name, ID: edge.Node.Id}
	}
	id, err := pickOne("project", items, picker)
	if err != nil {
		return projectResult{}, err
	}
	// Find the name for the selected ID.
	var name string
	for _, edge := range resp.Projects.Edges {
		if edge.Node.Id == id {
			name = edge.Node.Name
			break
		}
	}
	return projectResult{id: id, name: name, workspaceName: wsResult.name}, nil
}

type workspaceResult struct {
	id   *string
	name string
}

func resolveWorkspaceID(ctx context.Context, client *Client, workspace string, picker Picker) (workspaceResult, error) {
	if workspace != "" && uuidPattern.MatchString(workspace) {
		return workspaceResult{id: &workspace}, nil
	}
	resp, err := ApiToken(ctx, client.gql())
	if err != nil {
		return workspaceResult{}, err
	}
	if len(resp.ApiToken.Workspaces) == 0 {
		return workspaceResult{}, errors.New("no workspaces found")
	}
	if workspace != "" {
		for _, ws := range resp.ApiToken.Workspaces {
			if ws.Name == workspace {
				id := ws.Id
				slog.Debug("resolved workspace", "name", workspace, "id", id)
				return workspaceResult{id: &id, name: ws.Name}, nil
			}
		}
		return workspaceResult{}, fmt.Errorf("workspace not found: %s", workspace)
	}

	items := make([]PickCandidate, len(resp.ApiToken.Workspaces))
	for i, ws := range resp.ApiToken.Workspaces {
		items[i] = PickCandidate{Name: ws.Name, ID: ws.Id}
	}
	selected, err := pickOne("workspace", items, picker)
	if err != nil {
		return workspaceResult{}, err
	}
	// Find the name for the selected ID.
	var name string
	for _, ws := range resp.ApiToken.Workspaces {
		if ws.Id == selected {
			name = ws.Name
			break
		}
	}
	return workspaceResult{id: &selected, name: name}, nil
}

// ResolveWorkspaceID returns a workspace ID, resolving a name or auto-selecting.
// Pass nil picker for non-interactive mode.
func ResolveWorkspaceID(ctx context.Context, client *Client, workspace string) (string, error) {
	result, err := resolveWorkspaceID(ctx, client, workspace, nil)
	if err != nil {
		return "", err
	}
	if result.id == nil {
		return "", errors.New("workspace required")
	}
	return *result.id, nil
}

// ResolveProjectID returns a project ID, resolving a name or auto-selecting.
// Pass nil picker for non-interactive mode.
func ResolveProjectID(ctx context.Context, client *Client, workspace, project string) (string, error) {
	result, err := resolveProjectID(ctx, client, workspace, project, nil)
	if err != nil {
		return "", err
	}
	return result.id, nil
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

type environmentResult struct {
	id   string
	name string
}

func resolveEnvironmentID(ctx context.Context, client *Client, projectID, env string, picker Picker) (environmentResult, error) {
	if env != "" && uuidPattern.MatchString(env) {
		return environmentResult{id: env}, nil
	}
	resp, err := Environments(ctx, client.gql(), projectID)
	if err != nil {
		return environmentResult{}, err
	}
	if env != "" {
		for _, edge := range resp.Environments.Edges {
			if edge.Node.Name == env {
				slog.Debug("resolved environment", "name", env, "id", edge.Node.Id)
				return environmentResult{id: edge.Node.Id, name: edge.Node.Name}, nil
			}
		}
		return environmentResult{}, fmt.Errorf("environment not found: %s", env)
	}

	items := make([]PickCandidate, len(resp.Environments.Edges))
	for i, edge := range resp.Environments.Edges {
		items[i] = PickCandidate{Name: edge.Node.Name, ID: edge.Node.Id}
	}
	id, err := pickOne("environment", items, picker)
	if err != nil {
		return environmentResult{}, err
	}
	// Find the name for the selected ID.
	var name string
	for _, edge := range resp.Environments.Edges {
		if edge.Node.Id == id {
			name = edge.Node.Name
			break
		}
	}
	return environmentResult{id: id, name: name}, nil
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
