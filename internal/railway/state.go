package railway

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/hamishmorgan/fat-controller/internal/config"
)

// FetchLiveConfig loads shared + per-service variables and basic settings.
func FetchLiveConfig(ctx context.Context, client *Client, projectID, environmentID, serviceFilter string) (*config.LiveConfig, error) {
	slog.Debug("fetching live config", "project_id", projectID, "environment_id", environmentID, "service_filter", serviceFilter)
	cfg := &config.LiveConfig{
		ProjectID:     projectID,
		EnvironmentID: environmentID,
		Variables:     map[string]string{},
		Services:      map[string]*config.ServiceConfig{},
	}

	shared, err := Variables(ctx, client.GQL(), projectID, environmentID, nil)
	if err != nil {
		return nil, err
	}
	// Variables returns EnvironmentVariables which genqlient maps to
	// map[string]interface{} — convert values to strings.
	for k, v := range shared.Variables {
		cfg.Variables[k] = fmt.Sprint(v)
	}
	slog.Debug("fetched shared variables", "count", len(cfg.Variables))

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

		instance, err := ServiceInstance(ctx, client.GQL(), environmentID, edge.Node.Id)
		if err != nil {
			return nil, fmt.Errorf("fetching deploy settings for %s: %w", edge.Node.Name, err)
		}
		si := instance.ServiceInstance
		svc.Deploy = config.Deploy{
			Builder:         string(si.Builder),
			DockerfilePath:  si.DockerfilePath,
			RootDirectory:   si.RootDirectory,
			StartCommand:    si.StartCommand,
			HealthcheckPath: si.HealthcheckPath,
		}

		// Fetch resource limits (non-fatal — may not be available for all services).
		limits, limitsErr := ServiceInstanceLimits(ctx, client.GQL(), environmentID, edge.Node.Id)
		if limitsErr != nil {
			slog.Debug("could not fetch resource limits", "service", edge.Node.Name, "error", limitsErr)
		} else if limits.ServiceInstanceLimits != nil {
			if v, ok := limits.ServiceInstanceLimits["vCPUs"]; ok {
				if f, ok := toFloat64(v); ok {
					svc.VCPUs = &f
				}
			}
			if v, ok := limits.ServiceInstanceLimits["memoryGB"]; ok {
				if f, ok := toFloat64(v); ok {
					svc.MemoryGB = &f
				}
			}
		}

		slog.Debug("fetched service state", "service", edge.Node.Name, "variables", len(svc.Variables))
		cfg.Services[edge.Node.Name] = svc
	}

	return cfg, nil
}

// toFloat64 attempts to convert an interface{} value to float64.
// Handles float64, int64, and json.Number from GraphQL JSON responses.
func toFloat64(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}
