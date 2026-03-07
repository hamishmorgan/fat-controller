package apply

import (
	"context"
	"log/slog"
	"sort"

	"github.com/hamishmorgan/fat-controller/internal/config"
	"github.com/hamishmorgan/fat-controller/internal/diff"
)

// Options controls apply behavior.
type Options struct {
	FailFast    bool
	SkipDeploys bool
}

// Applier executes changes against Railway.
// The service parameter is the service name (empty string = shared scope).
type Applier interface {
	// Variables
	UpsertVariable(ctx context.Context, service, key, value string, skipDeploys bool) error
	UpsertVariables(ctx context.Context, service string, variables map[string]string, skipDeploys bool) error
	DeleteVariable(ctx context.Context, service, key string) error

	// Settings
	UpdateServiceSettings(ctx context.Context, service string, deploy *config.DesiredDeploy) error
	UpdateServiceResources(ctx context.Context, service string, res *config.DesiredResources) error

	// Service CRUD
	CreateService(ctx context.Context, name string) (string, error)
	DeleteService(ctx context.Context, serviceID string) error

	// Domains
	CreateCustomDomain(ctx context.Context, serviceID, domain string, port int) error
	DeleteCustomDomain(ctx context.Context, domainID string) error
	CreateServiceDomain(ctx context.Context, serviceID string, port int) error
	DeleteServiceDomain(ctx context.Context, domainID string) error

	// Volumes
	CreateVolume(ctx context.Context, serviceID, mountPath string) error
	DeleteVolume(ctx context.Context, volumeID string) error

	// TCP Proxies
	CreateTCPProxy(ctx context.Context, serviceID string, port int) error
	DeleteTCPProxy(ctx context.Context, proxyID string) error

	// Private Network
	EnablePrivateNetwork(ctx context.Context, serviceID string) error
	DisablePrivateNetwork(ctx context.Context, endpointID string) error

	// Egress
	SetEgressGateways(ctx context.Context, serviceID string, regions []string) error

	// Triggers
	CreateDeploymentTrigger(ctx context.Context, serviceID, repo, branch string) error
	DeleteDeploymentTrigger(ctx context.Context, triggerID string) error

	// Deploy
	TriggerDeploy(ctx context.Context, serviceID string) error
}

// Apply computes diffs and executes them in the required order:
//  1. Service settings (deploy + resources), services sorted alphabetically
//  2. Shared variables
//  3. Per-service variables, services sorted alphabetically
//
// Returns a Result with counts. In best-effort mode (FailFast=false),
// all operations are attempted and the error return is nil even if some
// fail. In fail-fast mode, the first error stops execution and is returned.
func Apply(ctx context.Context, desired *config.DesiredConfig, live *config.LiveConfig, applier Applier, opts Options) (*Result, error) {
	result := &Result{}
	if desired == nil {
		return result, nil
	}
	slog.Debug("starting apply", "services", len(desired.Services))
	changes := diff.Compute(desired, live)

	if err := ctx.Err(); err != nil {
		return result, err
	}

	// Phase 1: Settings first (services sorted).
	serviceNames := sortedServiceNames(desired.Services)
	for _, name := range serviceNames {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		sd := changes.Services[name]
		if sd == nil {
			continue
		}
		desiredSvc := findServiceByName(desired.Services, name)
		if hasDeployChanges(sd.Settings) {
			slog.Debug("updating service settings", "service", name)
			if err := applier.UpdateServiceSettings(ctx, name, desiredSvc.Deploy); err != nil {
				result.Failed++
				if opts.FailFast {
					return result, err
				}
			} else {
				result.Applied++
			}
		}
		if hasResourceChanges(sd.Settings) {
			slog.Debug("updating service resources", "service", name)
			if err := applier.UpdateServiceResources(ctx, name, desiredSvc.Resources); err != nil {
				result.Failed++
				if opts.FailFast {
					return result, err
				}
			} else {
				result.Applied++
			}
		}
	}

	// Phase 2: Shared variables.
	if changes.Shared != nil {
		if err := applyVariables(ctx, applier, "", changes.Shared.Variables, opts, result); err != nil {
			return result, err
		}
	}

	// Phase 3: Per-service variables (services sorted).
	for _, name := range serviceNames {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		sd := changes.Services[name]
		if sd == nil {
			continue
		}
		if err := applyVariables(ctx, applier, name, sd.Variables, opts, result); err != nil {
			return result, err
		}
	}

	slog.Debug("apply complete", "applied", result.Applied, "failed", result.Failed)
	return result, nil
}

// applyVariables batches create/update changes into a single UpsertVariables
// call per scope, then processes deletes individually. The result counters
// are updated in place. Returns a non-nil error only in fail-fast mode.
func applyVariables(ctx context.Context, applier Applier, service string, changes []diff.Change, opts Options, result *Result) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	// Collect upserts into a batch.
	batch := make(map[string]string)
	var deletes []diff.Change
	for _, ch := range changes {
		switch ch.Action {
		case diff.ActionCreate, diff.ActionUpdate:
			batch[ch.Key] = ch.DesiredValue
		case diff.ActionDelete:
			deletes = append(deletes, ch)
		}
	}

	// Batch upsert.
	if len(batch) > 0 {
		scope := service
		if scope == "" {
			scope = "shared"
		}
		slog.Debug("upserting variables", "scope", scope, "count", len(batch))
		if err := applier.UpsertVariables(ctx, service, batch, opts.SkipDeploys); err != nil {
			result.Failed += len(batch)
			if opts.FailFast {
				return err
			}
		} else {
			result.Applied += len(batch)
		}
	}

	// Deletes still happen individually.
	for _, ch := range deletes {
		if err := ctx.Err(); err != nil {
			return err
		}
		scope := service
		if scope == "" {
			scope = "shared"
		}
		slog.Debug("deleting variable", "scope", scope, "key", ch.Key)
		if err := applier.DeleteVariable(ctx, service, ch.Key); err != nil {
			result.Failed++
			if opts.FailFast {
				return err
			}
		} else {
			result.Applied++
		}
	}

	return nil
}

func hasDeployChanges(changes []diff.Change) bool {
	for _, ch := range changes {
		switch ch.Key {
		case "builder", "dockerfile_path", "root_directory", "start_command", "healthcheck_path":
			return true
		}
	}
	return false
}

func hasResourceChanges(changes []diff.Change) bool {
	for _, ch := range changes {
		if ch.Key == "vcpus" || ch.Key == "memory_gb" {
			return true
		}
	}
	return false
}

func sortedServiceNames(services []*config.DesiredService) []string {
	names := make([]string, 0, len(services))
	for _, svc := range services {
		names = append(names, svc.Name)
	}
	sort.Strings(names)
	return names
}

func findServiceByName(services []*config.DesiredService, name string) *config.DesiredService {
	for _, svc := range services {
		if svc.Name == name {
			return svc
		}
	}
	return nil
}
