package app

import (
	"context"
	"fmt"
	"sort"
)

// ServiceTarget holds a resolved service name and ID.
//
// It is intentionally small and shared between CLI commands so they can accept
// pre-resolved targets without depending on the railway client directly.
type ServiceTarget struct {
	Name string
	ID   string
}

// ResolveServiceTargets resolves service arguments to name+ID pairs.
// If no service names are given, it returns all services in the environment.
func ResolveServiceTargets(ctx context.Context, fetcher ConfigFetcher, projectID, environmentID string, serviceNames []string) ([]ServiceTarget, error) {
	live, err := fetcher.Fetch(ctx, projectID, environmentID, nil)
	if err != nil {
		return nil, fmt.Errorf("fetching services: %w", err)
	}
	if live == nil {
		return nil, fmt.Errorf("fetching services: got nil live config")
	}

	if len(serviceNames) == 0 {
		// All services.
		targets := make([]ServiceTarget, 0, len(live.Services))
		for _, svc := range live.Services {
			targets = append(targets, ServiceTarget{Name: svc.Name, ID: svc.ID})
		}
		sort.Slice(targets, func(i, j int) bool { return targets[i].Name < targets[j].Name })
		return targets, nil
	}

	// Specific services.
	targets := make([]ServiceTarget, 0, len(serviceNames))
	for _, name := range serviceNames {
		svc, ok := live.Services[name]
		if !ok {
			return nil, fmt.Errorf("service %q not found", name)
		}
		targets = append(targets, ServiceTarget{Name: svc.Name, ID: svc.ID})
	}
	return targets, nil
}
