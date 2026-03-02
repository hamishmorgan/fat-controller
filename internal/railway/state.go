package railway

import (
	"context"
	"fmt"

	"github.com/hamishmorgan/fat-controller/internal/config"
)

// FetchLiveConfig loads shared + per-service variables and basic settings.
func FetchLiveConfig(ctx context.Context, client *Client, projectID, environmentID, serviceFilter string) (*config.LiveConfig, error) {
	cfg := &config.LiveConfig{
		ProjectID:     projectID,
		EnvironmentID: environmentID,
		Shared:        map[string]string{},
		Services:      map[string]*config.ServiceConfig{},
	}

	shared, err := Variables(ctx, client.GQL(), projectID, environmentID, nil)
	if err != nil {
		return nil, err
	}
	// Variables returns EnvironmentVariables which genqlient maps to
	// map[string]interface{} — convert values to strings.
	for k, v := range shared.Variables {
		cfg.Shared[k] = fmt.Sprint(v)
	}

	services, err := ProjectServices(ctx, client.GQL(), projectID)
	if err != nil {
		return nil, err
	}
	for _, edge := range services.Project.Services.Edges {
		if serviceFilter != "" && edge.Node.Name != serviceFilter {
			continue
		}
		svc := &config.ServiceConfig{
			ID:        edge.Node.Id,
			Name:      edge.Node.Name,
			Variables: map[string]string{},
		}
		vars, err := Variables(ctx, client.GQL(), projectID, environmentID, &edge.Node.Id)
		if err != nil {
			return nil, err
		}
		for k, v := range vars.Variables {
			svc.Variables[k] = fmt.Sprint(v)
		}
		cfg.Services[edge.Node.Name] = svc
	}

	return cfg, nil
}
