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

	// Pre-fetch environment-wide volume instances (keyed by serviceId).
	volumesByService := fetchVolumesByService(ctx, client, projectID, environmentID)

	// Pre-fetch private networks for the environment (needed for per-service endpoint check).
	networks := fetchPrivateNetworks(ctx, client, environmentID)

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
			Builder:                 string(si.Builder),
			BuildCommand:            si.BuildCommand,
			DockerfilePath:          si.DockerfilePath,
			RootDirectory:           si.RootDirectory,
			WatchPatterns:           si.WatchPatterns,
			StartCommand:            si.StartCommand,
			CronSchedule:            si.CronSchedule,
			HealthcheckPath:         si.HealthcheckPath,
			HealthcheckTimeout:      si.HealthcheckTimeout,
			RestartPolicy:           string(si.RestartPolicyType),
			RestartPolicyMaxRetries: intPtrNonZero(si.RestartPolicyMaxRetries),
			DrainingSeconds:         si.DrainingSeconds,
			OverlapSeconds:          si.OverlapSeconds,
			SleepApplication:        si.SleepApplication,
			NumReplicas:             si.NumReplicas,
			Region:                  si.Region,
			IPv6Egress:              si.Ipv6EgressEnabled,
		}
		if si.Source != nil {
			svc.Deploy.Repo = si.Source.Repo
			svc.Deploy.Image = si.Source.Image
		}

		// Map domains into LiveDomain slice.
		for _, cd := range si.Domains.CustomDomains {
			svc.Domains = append(svc.Domains, config.LiveDomain{
				ID: cd.Id, Domain: cd.Domain, TargetPort: cd.TargetPort,
			})
		}
		for _, sd := range si.Domains.ServiceDomains {
			suffix := ""
			if sd.Suffix != nil {
				suffix = *sd.Suffix
			}
			svc.Domains = append(svc.Domains, config.LiveDomain{
				ID: sd.Id, Domain: sd.Domain, TargetPort: sd.TargetPort,
				IsService: true, Suffix: suffix,
			})
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

		// Attach pre-fetched volumes for this service.
		svc.Volumes = volumesByService[edge.Node.Id]

		// Fetch TCP proxies for this service (non-fatal).
		fetchTCPProxies(ctx, client, environmentID, edge.Node.Id, svc)

		// Fetch egress gateways for this service (non-fatal).
		fetchEgress(ctx, client, environmentID, edge.Node.Id, svc)

		// Fetch deployment triggers for this service (non-fatal).
		fetchTriggers(ctx, client, environmentID, projectID, edge.Node.Id, svc)

		// Check private network endpoint for this service (non-fatal).
		fetchNetworkEndpoint(ctx, client, environmentID, edge.Node.Id, networks, svc)

		slog.Debug("fetched service state", "service", edge.Node.Name, "variables", len(svc.Variables))
		cfg.Services[edge.Node.Name] = svc
	}

	return cfg, nil
}

// fetchVolumesByService fetches all volume instances for the environment and
// groups them by serviceId. Non-fatal: returns empty map on error.
func fetchVolumesByService(ctx context.Context, client *Client, projectID, environmentID string) map[string][]config.LiveVolume {
	result := map[string][]config.LiveVolume{}
	resp, err := EnvironmentVolumes(ctx, client.GQL(), environmentID, &projectID)
	if err != nil {
		slog.Debug("could not fetch volumes", "error", err)
		return result
	}
	for _, edge := range resp.Environment.VolumeInstances.Edges {
		vi := edge.Node
		if vi.ServiceId == nil {
			continue
		}
		region := ""
		if vi.Region != nil {
			region = *vi.Region
		}
		result[*vi.ServiceId] = append(result[*vi.ServiceId], config.LiveVolume{
			ID:        vi.Id,
			VolumeID:  vi.VolumeId,
			Name:      vi.Volume.Name,
			MountPath: vi.MountPath,
			Region:    region,
		})
	}
	return result
}

// fetchPrivateNetworks returns the list of private networks for the environment.
func fetchPrivateNetworks(ctx context.Context, client *Client, environmentID string) []PrivateNetworksPrivateNetworksPrivateNetwork {
	resp, err := PrivateNetworks(ctx, client.GQL(), environmentID)
	if err != nil {
		slog.Debug("could not fetch private networks", "error", err)
		return nil
	}
	return resp.PrivateNetworks
}

// fetchTCPProxies populates TCP proxies on the service config. Non-fatal.
func fetchTCPProxies(ctx context.Context, client *Client, environmentID, serviceID string, svc *config.ServiceConfig) {
	resp, err := TCPProxies(ctx, client.GQL(), environmentID, serviceID)
	if err != nil {
		slog.Debug("could not fetch TCP proxies", "service", svc.Name, "error", err)
		return
	}
	for _, p := range resp.TcpProxies {
		svc.TCPProxies = append(svc.TCPProxies, config.LiveTCPProxy{
			ID:              p.Id,
			ApplicationPort: p.ApplicationPort,
			ProxyPort:       p.ProxyPort,
			Domain:          p.Domain,
		})
	}
}

// fetchEgress populates egress gateways on the service config. Non-fatal.
func fetchEgress(ctx context.Context, client *Client, environmentID, serviceID string, svc *config.ServiceConfig) {
	resp, err := EgressGateways(ctx, client.GQL(), environmentID, serviceID)
	if err != nil {
		slog.Debug("could not fetch egress gateways", "service", svc.Name, "error", err)
		return
	}
	for _, g := range resp.EgressGateways {
		svc.Egress = append(svc.Egress, config.LiveEgressGateway{
			Region: g.Region,
			IPv4:   g.Ipv4,
		})
	}
}

// fetchTriggers populates deployment triggers on the service config. Non-fatal.
func fetchTriggers(ctx context.Context, client *Client, environmentID, projectID, serviceID string, svc *config.ServiceConfig) {
	resp, err := DeploymentTriggers(ctx, client.GQL(), environmentID, projectID, serviceID)
	if err != nil {
		slog.Debug("could not fetch triggers", "service", svc.Name, "error", err)
		return
	}
	for _, edge := range resp.DeploymentTriggers.Edges {
		t := edge.Node
		svc.Triggers = append(svc.Triggers, config.LiveTrigger{
			ID:         t.Id,
			Branch:     t.Branch,
			Repository: t.Repository,
			Provider:   t.Provider,
		})
	}
}

// fetchNetworkEndpoint checks if a service has a private network endpoint. Non-fatal.
func fetchNetworkEndpoint(ctx context.Context, client *Client, environmentID, serviceID string, networks []PrivateNetworksPrivateNetworksPrivateNetwork, svc *config.ServiceConfig) {
	for _, net := range networks {
		resp, err := PrivateNetworkEndpoint(ctx, client.GQL(), environmentID, net.PublicId, serviceID)
		if err != nil {
			slog.Debug("could not check network endpoint", "service", svc.Name, "network", net.Name, "error", err)
			continue
		}
		if resp.PrivateNetworkEndpoint != nil {
			svc.Network = &config.LiveNetworkEndpoint{
				ID:      resp.PrivateNetworkEndpoint.PublicId,
				DNSName: resp.PrivateNetworkEndpoint.DnsName,
			}
			return // one endpoint is enough to know it's enabled
		}
	}
}

// intPtrNonZero converts an int to *int, returning nil for zero values.
func intPtrNonZero(v int) *int {
	if v == 0 {
		return nil
	}
	return &v
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
