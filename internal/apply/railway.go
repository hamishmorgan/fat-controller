package apply

import (
	"context"
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
	defer r.mu.Unlock()

	if r.serviceIDs == nil {
		r.serviceIDs = make(map[string]string)
	}
	if id, ok := r.serviceIDs[name]; ok {
		return id, nil
	}

	id, err := railway.ResolveServiceID(ctx, r.Client, r.ProjectID, name)
	if err != nil {
		return "", err
	}
	r.serviceIDs[name] = id
	return id, nil
}

func (r *RailwayApplier) UpsertVariable(ctx context.Context, service, key, value string, skipDeploys bool) error {
	serviceID, err := r.resolveServiceID(ctx, service)
	if err != nil {
		return err
	}
	return railway.UpsertVariable(ctx, r.Client, r.ProjectID, r.EnvironmentID, serviceID, key, value, skipDeploys)
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
