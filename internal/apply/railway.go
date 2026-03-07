package apply

import (
	"context"
	"log/slog"
	"sync"

	"github.com/hamishmorgan/fat-controller/internal/config"
	"github.com/hamishmorgan/fat-controller/internal/railway"
)

// RailwayApplier implements Applier using Railway GraphQL mutations.
type RailwayApplier struct {
	Client        *railway.Client
	ProjectID     string
	EnvironmentID string

	mu         sync.Mutex
	serviceIDs map[string]string // cache: service name → service ID
}

// resolveServiceID resolves a service name to an ID, caching the result.
// Empty name = shared scope (returns "").
func (r *RailwayApplier) resolveServiceID(ctx context.Context, name string) (string, error) {
	if name == "" {
		return "", nil
	}

	r.mu.Lock()
	if id, ok := r.serviceIDs[name]; ok {
		r.mu.Unlock()
		slog.Debug("service ID cache hit", "name", name, "id", id)
		return id, nil
	}
	r.mu.Unlock()
	slog.Debug("resolving service ID", "name", name)

	id, err := railway.ResolveServiceID(ctx, r.Client, r.ProjectID, name)
	if err != nil {
		return "", err
	}

	r.mu.Lock()
	if r.serviceIDs == nil {
		r.serviceIDs = make(map[string]string)
	}
	r.serviceIDs[name] = id
	r.mu.Unlock()
	slog.Debug("cached service ID", "name", name, "id", id)
	return id, nil
}

func (r *RailwayApplier) UpsertVariable(ctx context.Context, service, key, value string, skipDeploys bool) error {
	serviceID, err := r.resolveServiceID(ctx, service)
	if err != nil {
		return err
	}
	return railway.UpsertVariable(ctx, r.Client, r.ProjectID, r.EnvironmentID, serviceID, key, value, skipDeploys)
}

func (r *RailwayApplier) UpsertVariables(ctx context.Context, service string, variables map[string]string, skipDeploys bool) error {
	serviceID, err := r.resolveServiceID(ctx, service)
	if err != nil {
		return err
	}
	return railway.UpsertVariableCollection(ctx, r.Client, r.ProjectID, r.EnvironmentID, serviceID, variables, skipDeploys)
}

func (r *RailwayApplier) DeleteVariable(ctx context.Context, service, key string) error {
	serviceID, err := r.resolveServiceID(ctx, service)
	if err != nil {
		return err
	}
	return railway.DeleteVariable(ctx, r.Client, r.ProjectID, r.EnvironmentID, serviceID, key)
}

func (r *RailwayApplier) UpdateServiceSettings(ctx context.Context, service string, deploy *config.DesiredDeploy) error {
	if deploy == nil {
		return nil
	}
	serviceID, err := r.resolveServiceID(ctx, service)
	if err != nil {
		return err
	}
	input, err := ToServiceInstanceUpdateInput(deploy)
	if err != nil {
		return err
	}
	return railway.UpdateServiceSettings(ctx, r.Client, serviceID, input)
}

func (r *RailwayApplier) UpdateServiceResources(ctx context.Context, service string, res *config.DesiredResources) error {
	if res == nil {
		return nil
	}
	serviceID, err := r.resolveServiceID(ctx, service)
	if err != nil {
		return err
	}
	return railway.UpdateServiceLimits(ctx, r.Client, r.EnvironmentID, serviceID, res.VCPUs, res.MemoryGB)
}

// --- Service CRUD ---

func (r *RailwayApplier) CreateService(ctx context.Context, name string) (string, error) {
	return railway.CreateService(ctx, r.Client, r.ProjectID, name)
}

func (r *RailwayApplier) DeleteService(ctx context.Context, serviceID string) error {
	return railway.DeleteService(ctx, r.Client, serviceID)
}

// --- Domains ---

func (r *RailwayApplier) CreateCustomDomain(ctx context.Context, serviceID, domain string, port int) error {
	_, err := railway.CreateCustomDomain(ctx, r.Client, r.ProjectID, r.EnvironmentID, serviceID, domain, port)
	return err
}

func (r *RailwayApplier) DeleteCustomDomain(ctx context.Context, domainID string) error {
	return railway.DeleteCustomDomain(ctx, r.Client, domainID)
}

func (r *RailwayApplier) CreateServiceDomain(ctx context.Context, serviceID string, port int) error {
	_, err := railway.CreateServiceDomain(ctx, r.Client, r.EnvironmentID, serviceID, port)
	return err
}

func (r *RailwayApplier) DeleteServiceDomain(ctx context.Context, domainID string) error {
	return railway.DeleteServiceDomain(ctx, r.Client, domainID)
}

// --- Volumes ---

func (r *RailwayApplier) CreateVolume(ctx context.Context, serviceID, mountPath string) error {
	_, err := railway.CreateVolume(ctx, r.Client, r.ProjectID, r.EnvironmentID, serviceID, mountPath)
	return err
}

func (r *RailwayApplier) DeleteVolume(ctx context.Context, volumeID string) error {
	return railway.DeleteVolume(ctx, r.Client, volumeID)
}

// --- TCP Proxies ---

func (r *RailwayApplier) CreateTCPProxy(ctx context.Context, serviceID string, port int) error {
	_, err := railway.CreateTCPProxy(ctx, r.Client, r.EnvironmentID, serviceID, port)
	return err
}

func (r *RailwayApplier) DeleteTCPProxy(ctx context.Context, proxyID string) error {
	return railway.DeleteTCPProxy(ctx, r.Client, proxyID)
}

// --- Private Network ---

func (r *RailwayApplier) EnablePrivateNetwork(ctx context.Context, serviceID string) error {
	_, err := railway.EnablePrivateNetwork(ctx, r.Client, r.EnvironmentID, serviceID)
	return err
}

func (r *RailwayApplier) DisablePrivateNetwork(ctx context.Context, endpointID string) error {
	return railway.DisablePrivateNetworkEndpoint(ctx, r.Client, endpointID)
}

// --- Egress ---

func (r *RailwayApplier) SetEgressGateways(ctx context.Context, serviceID string, regions []string) error {
	if err := railway.ClearEgressGateways(ctx, r.Client, r.EnvironmentID, serviceID); err != nil {
		return err
	}
	for _, region := range regions {
		if _, err := railway.CreateEgressGateway(ctx, r.Client, r.EnvironmentID, serviceID, region); err != nil {
			return err
		}
	}
	return nil
}

// --- Triggers ---

func (r *RailwayApplier) CreateDeploymentTrigger(ctx context.Context, serviceID, repo, branch string) error {
	_, err := railway.CreateDeploymentTrigger(ctx, r.Client, r.EnvironmentID, r.ProjectID, serviceID, repo, branch)
	return err
}

func (r *RailwayApplier) DeleteDeploymentTrigger(ctx context.Context, triggerID string) error {
	return railway.DeleteDeploymentTrigger(ctx, r.Client, triggerID)
}

// --- Deploy ---

func (r *RailwayApplier) TriggerDeploy(ctx context.Context, serviceID string) error {
	_, err := railway.DeployService(ctx, r.Client, r.EnvironmentID, serviceID, nil)
	return err
}
