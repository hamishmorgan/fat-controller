package railway

import (
	"context"
	"errors"
	"regexp"

	"github.com/hamishmorgan/fat-controller/internal/auth"
)

// uuidPattern matches Railway-style UUIDs (e.g. "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx").
var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// ResolveProjectEnvironment returns project/environment IDs for the active auth.
// For project tokens, it uses the ProjectToken query. For account tokens, it
// resolves the provided project/environment names (or passes through IDs).
func ResolveProjectEnvironment(ctx context.Context, client *Client, project, environment string) (string, string, error) {
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
	if project == "" || environment == "" {
		return "", "", errors.New("project and environment required for account tokens")
	}
	projID, err := resolveProjectID(ctx, client, project)
	if err != nil {
		return "", "", err
	}
	envID, err := resolveEnvironmentID(ctx, client, projID, environment)
	if err != nil {
		return "", "", err
	}
	return projID, envID, nil
}

func resolveProjectID(ctx context.Context, client *Client, project string) (string, error) {
	if uuidPattern.MatchString(project) {
		return project, nil
	}
	resp, err := Projects(ctx, client.GQL())
	if err != nil {
		return "", err
	}
	for _, edge := range resp.Projects.Edges {
		if edge.Node.Name == project {
			return edge.Node.Id, nil
		}
	}
	return "", errors.New("project not found: " + project)
}

func resolveEnvironmentID(ctx context.Context, client *Client, projectID, env string) (string, error) {
	if uuidPattern.MatchString(env) {
		return env, nil
	}
	resp, err := Environments(ctx, client.GQL(), projectID)
	if err != nil {
		return "", err
	}
	for _, edge := range resp.Environments.Edges {
		if edge.Node.Name == env {
			return edge.Node.Id, nil
		}
	}
	return "", errors.New("environment not found: " + env)
}
