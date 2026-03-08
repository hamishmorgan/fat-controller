package railway

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"golang.org/x/sync/errgroup"

	"github.com/hamishmorgan/fat-controller/internal/config"
)

// serviceInstanceNode is a type alias to shorten references to the generated
// batched service-instance node type from the EnvironmentBulk query.
type serviceInstanceNode = EnvironmentBulkEnvironmentServiceInstancesEnvironmentServiceInstancesConnectionEdgesEnvironmentServiceInstancesConnectionEdgeNodeServiceInstance

// FetchLiveConfig loads shared + per-service variables and settings.
// serviceFilter limits which services are fetched — empty means all.
// prefetchedServices, when non-nil, provides service name/ID/icon data from a
// prior resolution query, skipping the separate ProjectServices API call.
// Per-service queries run concurrently via errgroup.
func FetchLiveConfig(ctx context.Context, client *Client, projectID, environmentID string, serviceFilter []string, prefetchedServices []ServiceInfo) (*config.LiveConfig, error) {
	slog.Debug("fetching live config", "project_id", projectID, "environment_id", environmentID, "service_filter", serviceFilter, "prefetched", len(prefetchedServices) > 0)
	cfg := &config.LiveConfig{
		ProjectID:     projectID,
		EnvironmentID: environmentID,
		Variables:     map[string]string{},
		Services:      map[string]*config.ServiceConfig{},
	}

	filterSet := make(map[string]bool, len(serviceFilter))
	for _, name := range serviceFilter {
		filterSet[name] = true
	}

	shared, err := Variables(ctx, client.gql(), projectID, environmentID, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range shared.Variables {
		cfg.Variables[k] = fmt.Sprint(v)
	}
	slog.Debug("fetched shared variables", "count", len(cfg.Variables))

	// Collect the services to fetch, using pre-fetched data if available,
	// otherwise querying ProjectServices.
	type svcRef struct{ name, id, icon string }
	var toFetch []svcRef

	if len(prefetchedServices) > 0 {
		slog.Debug("using pre-fetched services list", "count", len(prefetchedServices))
		for _, svc := range prefetchedServices {
			if len(filterSet) > 0 && !filterSet[svc.Name] {
				continue
			}
			toFetch = append(toFetch, svcRef{name: svc.Name, id: svc.ID, icon: svc.Icon})
		}
	} else {
		services, err := ProjectServices(ctx, client.gql(), projectID)
		if err != nil {
			return nil, err
		}
		for _, edge := range services.Project.Services.Edges {
			if len(filterSet) > 0 && !filterSet[edge.Node.Name] {
				continue
			}
			icon := ""
			if edge.Node.Icon != nil {
				icon = *edge.Node.Icon
			}
			toFetch = append(toFetch, svcRef{name: edge.Node.Name, id: edge.Node.Id, icon: icon})
		}
	}

	if len(toFetch) == 0 {
		return cfg, nil
	}

	// Pre-fetch all environment-wide data in a single bulk query (4 requests → 1).
	instancesByService, triggersByService, volumesByService, networks := fetchEnvironmentBulk(ctx, client, projectID, environmentID)

	// Fetch per-service details concurrently.
	type svcResult struct {
		name string
		svc  *config.ServiceConfig
	}
	results := make([]svcResult, len(toFetch))

	g, gCtx := errgroup.WithContext(ctx)
	for i, ref := range toFetch {
		g.Go(func() error {
			svc, err := fetchServiceState(gCtx, client, projectID, environmentID, ref.id, ref.name, ref.icon, volumesByService, networks, instancesByService, triggersByService)
			if err != nil {
				return err
			}
			results[i] = svcResult{name: ref.name, svc: svc}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}

	for _, r := range results {
		cfg.Services[r.name] = r.svc
	}
	return cfg, nil
}

// fetchEnvironmentBulk fetches all environment-wide data in a single query:
// service instances, deployment triggers, volume instances, and private networks.
// Non-fatal: returns empty/nil maps on error.
func fetchEnvironmentBulk(ctx context.Context, client *Client, projectID, environmentID string) (
	instancesByService map[string]*serviceInstanceNode,
	triggersByService map[string][]config.LiveTrigger,
	volumesByService map[string][]config.LiveVolume,
	networks []EnvironmentBulkPrivateNetworksPrivateNetwork,
) {
	instancesByService = make(map[string]*serviceInstanceNode)
	triggersByService = make(map[string][]config.LiveTrigger)
	volumesByService = make(map[string][]config.LiveVolume)

	resp, err := EnvironmentBulk(ctx, client.gql(), environmentID, &projectID)
	if err != nil {
		slog.Debug("could not fetch environment bulk data (will fall back to per-service queries)", "error", err)
		return
	}

	// Service instances — keyed by serviceId.
	for i := range resp.Environment.ServiceInstances.Edges {
		node := &resp.Environment.ServiceInstances.Edges[i].Node
		instancesByService[node.ServiceId] = node
	}
	slog.Debug("fetched environment service instances", "count", len(instancesByService))

	// Deployment triggers — grouped by serviceId.
	for _, edge := range resp.Environment.DeploymentTriggers.Edges {
		t := edge.Node
		if t.ServiceId == nil {
			continue
		}
		triggersByService[*t.ServiceId] = append(triggersByService[*t.ServiceId], config.LiveTrigger{
			ID:         t.Id,
			Branch:     t.Branch,
			Repository: t.Repository,
			Provider:   t.Provider,
		})
	}
	slog.Debug("fetched environment deployment triggers", "count", len(triggersByService))

	// Volume instances — grouped by serviceId.
	for _, edge := range resp.Environment.VolumeInstances.Edges {
		vi := edge.Node
		if vi.ServiceId == nil {
			continue
		}
		region := ""
		if vi.Region != nil {
			region = *vi.Region
		}
		volumesByService[*vi.ServiceId] = append(volumesByService[*vi.ServiceId], config.LiveVolume{
			ID:        vi.Id,
			VolumeID:  vi.VolumeId,
			Name:      vi.Volume.Name,
			MountPath: vi.MountPath,
			Region:    region,
		})
	}

	// Private networks.
	networks = resp.PrivateNetworks

	return
}

// fetchServiceState fetches all state for a single service. Called concurrently.
func fetchServiceState(ctx context.Context, client *Client, projectID, environmentID, serviceID, serviceName, icon string, volumesByService map[string][]config.LiveVolume, networks []EnvironmentBulkPrivateNetworksPrivateNetwork, instancesByService map[string]*serviceInstanceNode, triggersByService map[string][]config.LiveTrigger) (*config.ServiceConfig, error) {
	svc := &config.ServiceConfig{
		ID:        serviceID,
		Name:      serviceName,
		Variables: map[string]string{},
	}
	svc.Icon = icon

	// Variables are required — fetch now.
	vars, err := Variables(ctx, client.gql(), projectID, environmentID, &serviceID)
	if err != nil {
		return nil, err
	}
	for k, v := range vars.Variables {
		svc.Variables[k] = fmt.Sprint(v)
	}

	// Apply pre-fetched service instance settings.
	if si, ok := instancesByService[serviceID]; ok {
		svc.Deploy = config.Deploy{
			Builder:                 string(si.Builder),
			BuildCommand:            si.BuildCommand,
			DockerfilePath:          si.DockerfilePath,
			RootDirectory:           si.RootDirectory,
			WatchPatterns:           si.WatchPatterns,
			PreDeployCommand:        parsePreDeployCommand(si.PreDeployCommand),
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
	} else {
		slog.Debug("no pre-fetched service instance, falling back to per-service query", "service", serviceName)
		instance, err := ServiceInstance(ctx, client.gql(), environmentID, serviceID)
		if err != nil {
			return nil, fmt.Errorf("fetching deploy settings for %s: %w", serviceName, err)
		}
		si := instance.ServiceInstance
		svc.Deploy = config.Deploy{
			Builder:                 string(si.Builder),
			BuildCommand:            si.BuildCommand,
			DockerfilePath:          si.DockerfilePath,
			RootDirectory:           si.RootDirectory,
			WatchPatterns:           si.WatchPatterns,
			PreDeployCommand:        parsePreDeployCommand(si.PreDeployCommand),
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
	}

	// Apply pre-fetched triggers.
	if triggers, ok := triggersByService[serviceID]; ok {
		svc.Triggers = triggers
	}

	// Non-fatal sub-resource queries — run concurrently.
	subG, subCtx := errgroup.WithContext(ctx)
	subG.Go(func() error {
		limits, err := ServiceInstanceLimits(subCtx, client.gql(), environmentID, serviceID)
		if err != nil {
			slog.Debug("could not fetch resource limits", "service", serviceName, "error", err)
			return nil
		}
		if limits.ServiceInstanceLimits != nil {
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
		return nil
	})
	subG.Go(func() error {
		fetchTCPProxies(subCtx, client, environmentID, serviceID, svc)
		return nil
	})
	subG.Go(func() error {
		fetchEgress(subCtx, client, environmentID, serviceID, svc)
		return nil
	})
	subG.Go(func() error {
		fetchNetworkEndpoint(subCtx, client, environmentID, serviceID, networks, svc)
		return nil
	})
	_ = subG.Wait() // all non-fatal

	// Attach pre-fetched volumes.
	svc.Volumes = volumesByService[serviceID]

	slog.Debug("fetched service state", "service", serviceName, "variables", len(svc.Variables))
	return svc, nil
}

// fetchTCPProxies populates TCP proxies on the service config. Non-fatal.
func fetchTCPProxies(ctx context.Context, client *Client, environmentID, serviceID string, svc *config.ServiceConfig) {
	resp, err := TCPProxies(ctx, client.gql(), environmentID, serviceID)
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
	resp, err := EgressGateways(ctx, client.gql(), environmentID, serviceID)
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

// fetchNetworkEndpoint checks if a service has a private network endpoint. Non-fatal.
func fetchNetworkEndpoint(ctx context.Context, client *Client, environmentID, serviceID string, networks []EnvironmentBulkPrivateNetworksPrivateNetwork, svc *config.ServiceConfig) {
	for _, net := range networks {
		resp, err := PrivateNetworkEndpoint(ctx, client.gql(), environmentID, net.PublicId, serviceID)
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

// parsePreDeployCommand extracts a []string from Railway's preDeployCommand field.
// The GraphQL response returns it as *map[string]interface{} or nil.
func parsePreDeployCommand(raw *map[string]interface{}) []string {
	if raw == nil {
		return nil
	}
	// Railway returns preDeployCommand as a JSON object; extract string values.
	var result []string
	for _, v := range *raw {
		if s, ok := v.(string); ok && s != "" {
			result = append(result, s)
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
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
