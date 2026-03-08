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
	AllowCreate bool // include create changes (default true when zero value via Apply)
	AllowUpdate bool // include update changes (default true when zero value via Apply)
	AllowDelete bool // include delete changes
}

// Applier executes changes against Railway.
// The service parameter is the service name (empty string = shared scope).
type Applier interface {
	// Variables
	UpsertVariable(ctx context.Context, service, key, value string, skipDeploys bool) error
	UpsertVariables(ctx context.Context, service string, variables map[string]string, skipDeploys bool) error
	DeleteVariable(ctx context.Context, service, key string) error

	// Settings
	UpdateServiceIcon(ctx context.Context, service, icon string) error
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
//  0. Service CRUD (create new services, delete marked ones)
//  1. Service settings (deploy + resources), services sorted alphabetically
//  2. Shared variables
//  3. Per-service variables, services sorted alphabetically
//  4. Sub-resources (domains, volumes, TCP proxies, network, triggers, egress)
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
	// Map apply options to diff options. Zero-value Options means
	// AllowCreate=false and AllowUpdate=false, but the conventional default
	// is create+update on. Callers that care set them explicitly.
	diffOpts := diff.Options{
		Create: opts.AllowCreate,
		Update: opts.AllowUpdate,
		Delete: opts.AllowDelete,
	}
	// For backward compat: if nothing is enabled, default to create+update.
	if !diffOpts.Create && !diffOpts.Update && !diffOpts.Delete {
		diffOpts.Create = true
		diffOpts.Update = true
	}

	// Phase 0: Service CRUD — create new services, delete marked ones.
	// This must happen before diff computation so that newly created
	// services exist when we try to apply settings/variables.
	if err := applyServiceCRUD(ctx, desired, live, applier, opts, result); err != nil {
		return result, err
	}

	changes := diff.ComputeWithOptions(desired, live, diffOpts)

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
		if hasIconChange(sd.Settings) {
			slog.Debug("updating service icon", "service", name)
			if err := applier.UpdateServiceIcon(ctx, name, desiredSvc.Icon); err != nil {
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

	// Phase 4: Sub-resources (domains, volumes, TCP proxies, network, triggers, egress).
	for _, name := range serviceNames {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		sd := changes.Services[name]
		if sd == nil {
			continue
		}
		desiredSvc := findServiceByName(desired.Services, name)
		liveSvc := findLiveServiceConfig(live, name, desiredSvc.ID)
		serviceID := ""
		if liveSvc != nil {
			serviceID = liveSvc.ID
		}
		if err := applySubResources(ctx, applier, serviceID, sd.SubResources, opts, result); err != nil {
			return result, err
		}
	}

	slog.Debug("apply complete", "applied", result.Applied, "failed", result.Failed)
	return result, nil
}

// applyServiceCRUD handles creating new services and deleting marked ones.
func applyServiceCRUD(ctx context.Context, desired *config.DesiredConfig, live *config.LiveConfig, applier Applier, opts Options, result *Result) error {
	for _, desiredSvc := range desired.Services {
		if err := ctx.Err(); err != nil {
			return err
		}
		liveSvc := findLiveServiceConfig(live, desiredSvc.Name, desiredSvc.ID)

		// Delete marked services.
		if desiredSvc.Delete && liveSvc != nil && opts.AllowDelete {
			slog.Debug("deleting service", "service", desiredSvc.Name)
			if err := applier.DeleteService(ctx, liveSvc.ID); err != nil {
				result.Failed++
				if opts.FailFast {
					return err
				}
			} else {
				result.Applied++
			}
			continue
		}

		// Create new services.
		if liveSvc == nil && !desiredSvc.Delete && (opts.AllowCreate || (!opts.AllowCreate && !opts.AllowUpdate && !opts.AllowDelete)) {
			slog.Debug("creating service", "service", desiredSvc.Name)
			newID, err := applier.CreateService(ctx, desiredSvc.Name)
			if err != nil {
				result.Failed++
				if opts.FailFast {
					return err
				}
			} else {
				result.Applied++
				// Add the new service to live config so subsequent phases can find it.
				if live != nil {
					live.Services[desiredSvc.Name] = &config.ServiceConfig{
						ID:        newID,
						Name:      desiredSvc.Name,
						Variables: map[string]string{},
					}
				}
			}
		}
	}
	return nil
}

// applySubResources executes sub-resource changes (domains, volumes, TCP proxies,
// network, triggers, egress) for a single service.
func applySubResources(ctx context.Context, applier Applier, serviceID string, changes []diff.SubResourceChange, opts Options, result *Result) error {
	for _, ch := range changes {
		if err := ctx.Err(); err != nil {
			return err
		}
		var err error
		switch ch.Type {
		case "domain":
			err = applyDomainChange(ctx, applier, serviceID, ch)
		case "volume":
			err = applyVolumeChange(ctx, applier, serviceID, ch)
		case "tcp_proxy":
			err = applyTCPProxyChange(ctx, applier, serviceID, ch)
		case "network":
			err = applyNetworkChange(ctx, applier, serviceID, ch)
		case "trigger":
			err = applyTriggerChange(ctx, applier, serviceID, ch)
		case "egress":
			err = applyEgressChange(ctx, applier, serviceID, ch)
		default:
			slog.Warn("unknown sub-resource type", "type", ch.Type)
			continue
		}

		if err != nil {
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

func applyDomainChange(ctx context.Context, applier Applier, serviceID string, ch diff.SubResourceChange) error {
	switch ch.Action {
	case diff.ActionCreate:
		if ch.IsCustom {
			slog.Debug("creating custom domain", "service", serviceID, "domain", ch.Key)
			return applier.CreateCustomDomain(ctx, serviceID, ch.Key, ch.Port)
		}
		slog.Debug("creating service domain", "service", serviceID)
		return applier.CreateServiceDomain(ctx, serviceID, ch.Port)
	case diff.ActionDelete:
		if ch.IsCustom {
			slog.Debug("deleting custom domain", "domain", ch.Key)
			return applier.DeleteCustomDomain(ctx, ch.LiveID)
		}
		slog.Debug("deleting service domain", "domain", ch.Key)
		return applier.DeleteServiceDomain(ctx, ch.LiveID)
	}
	return nil
}

func applyVolumeChange(ctx context.Context, applier Applier, serviceID string, ch diff.SubResourceChange) error {
	switch ch.Action {
	case diff.ActionCreate:
		slog.Debug("creating volume", "service", serviceID, "mount", ch.Mount)
		return applier.CreateVolume(ctx, serviceID, ch.Mount)
	case diff.ActionDelete:
		slog.Debug("deleting volume", "volume", ch.Key)
		return applier.DeleteVolume(ctx, ch.LiveID)
	}
	return nil
}

func applyTCPProxyChange(ctx context.Context, applier Applier, serviceID string, ch diff.SubResourceChange) error {
	switch ch.Action {
	case diff.ActionCreate:
		slog.Debug("creating TCP proxy", "service", serviceID, "port", ch.Port)
		return applier.CreateTCPProxy(ctx, serviceID, ch.Port)
	case diff.ActionDelete:
		slog.Debug("deleting TCP proxy", "port", ch.Key)
		return applier.DeleteTCPProxy(ctx, ch.LiveID)
	}
	return nil
}

func applyNetworkChange(ctx context.Context, applier Applier, serviceID string, ch diff.SubResourceChange) error {
	switch ch.Action {
	case diff.ActionCreate:
		slog.Debug("enabling private network", "service", serviceID)
		return applier.EnablePrivateNetwork(ctx, serviceID)
	case diff.ActionDelete:
		slog.Debug("disabling private network", "service", serviceID)
		return applier.DisablePrivateNetwork(ctx, ch.LiveID)
	}
	return nil
}

func applyTriggerChange(ctx context.Context, applier Applier, serviceID string, ch diff.SubResourceChange) error {
	switch ch.Action {
	case diff.ActionCreate:
		slog.Debug("creating trigger", "service", serviceID, "repo", ch.Repo, "branch", ch.Branch)
		return applier.CreateDeploymentTrigger(ctx, serviceID, ch.Repo, ch.Branch)
	case diff.ActionDelete:
		slog.Debug("deleting trigger", "trigger", ch.Key)
		return applier.DeleteDeploymentTrigger(ctx, ch.LiveID)
	}
	return nil
}

func applyEgressChange(ctx context.Context, applier Applier, serviceID string, ch diff.SubResourceChange) error {
	slog.Debug("setting egress gateways", "service", serviceID, "regions", ch.Regions)
	return applier.SetEgressGateways(ctx, serviceID, ch.Regions)
}

// findLiveServiceConfig looks up a service in the live config by ID then name.
func findLiveServiceConfig(live *config.LiveConfig, name, id string) *config.ServiceConfig {
	if live == nil {
		return nil
	}
	if id != "" {
		for _, svc := range live.Services {
			if svc.ID == id {
				return svc
			}
		}
	}
	return live.Services[name]
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
		case config.KeyBuilder, config.KeyRepo, config.KeyImage,
			config.KeyBuildCommand, config.KeyDockerfilePath, config.KeyRootDirectory,
			config.KeyWatchPatterns, config.KeyStartCommand, config.KeyCronSchedule,
			config.KeyHealthcheckPath, config.KeyHealthcheckTimeout,
			config.KeyRestartPolicy, config.KeyRestartPolicyMaxRetries,
			config.KeyDrainingSeconds, config.KeyOverlapSeconds,
			config.KeySleepApplication, config.KeyNumReplicas, config.KeyRegion,
			config.KeyIPv6Egress:
			return true
		}
	}
	return false
}

func hasIconChange(changes []diff.Change) bool {
	for _, ch := range changes {
		if ch.Key == config.KeyIcon {
			return true
		}
	}
	return false
}

func hasResourceChanges(changes []diff.Change) bool {
	for _, ch := range changes {
		if ch.Key == config.KeyVCPUs || ch.Key == config.KeyMemoryGB {
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
