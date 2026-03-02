package apply

import (
	"context"
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
	UpsertVariable(ctx context.Context, service, key, value string, skipDeploys bool) error
	DeleteVariable(ctx context.Context, service, key string) error
	UpdateServiceSettings(ctx context.Context, service string, deploy *config.DesiredDeploy) error
	UpdateServiceResources(ctx context.Context, service string, res *config.DesiredResources) error
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
	changes := diff.Compute(desired, live)

	// Phase 1: Settings first (services sorted).
	serviceNames := sortedServiceNames(desired.Services)
	for _, name := range serviceNames {
		sd := changes.Services[name]
		if sd == nil {
			continue
		}
		if hasDeployChanges(sd.Settings) {
			if err := applier.UpdateServiceSettings(ctx, name, desired.Services[name].Deploy); err != nil {
				result.Failed++
				if opts.FailFast {
					return result, err
				}
			} else {
				result.Applied++
			}
		}
		if hasResourceChanges(sd.Settings) {
			if err := applier.UpdateServiceResources(ctx, name, desired.Services[name].Resources); err != nil {
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
		for _, ch := range changes.Shared.Variables {
			if err := applyVariable(ctx, applier, "", ch, opts.SkipDeploys); err != nil {
				result.Failed++
				if opts.FailFast {
					return result, err
				}
			} else {
				result.Applied++
			}
		}
	}

	// Phase 3: Per-service variables (services sorted).
	for _, name := range serviceNames {
		sd := changes.Services[name]
		if sd == nil {
			continue
		}
		for _, ch := range sd.Variables {
			if err := applyVariable(ctx, applier, name, ch, opts.SkipDeploys); err != nil {
				result.Failed++
				if opts.FailFast {
					return result, err
				}
			} else {
				result.Applied++
			}
		}
	}

	return result, nil
}

func applyVariable(ctx context.Context, applier Applier, service string, ch diff.Change, skipDeploys bool) error {
	switch ch.Action {
	case diff.ActionCreate, diff.ActionUpdate:
		return applier.UpsertVariable(ctx, service, ch.Key, ch.DesiredValue, skipDeploys)
	case diff.ActionDelete:
		return applier.DeleteVariable(ctx, service, ch.Key)
	default:
		return nil
	}
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

func sortedServiceNames(services map[string]*config.DesiredService) []string {
	names := make([]string, 0, len(services))
	for name := range services {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
