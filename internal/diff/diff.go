package diff

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hamishmorgan/fat-controller/internal/config"
)

// Action represents the type of change.
type Action int

const (
	ActionCreate Action = iota
	ActionUpdate
	ActionDelete
)

// String returns a human-readable label.
func (a Action) String() string {
	switch a {
	case ActionCreate:
		return "create"
	case ActionUpdate:
		return "update"
	case ActionDelete:
		return "delete"
	default:
		return "unknown"
	}
}

// Change represents a single variable or setting change.
type Change struct {
	Key          string
	Action       Action
	LiveValue    string // current value in Railway (empty for Create)
	DesiredValue string // desired value from config (empty for Delete)
	// Explicit marks a delete that was explicitly requested in the config
	// (empty-string sentinel), as opposed to inferred from absence. Explicit
	// deletes are applied regardless of the --delete flag.
	Explicit bool
}

// SubResourceChange represents a create or delete for a sub-resource
// (domain, volume, TCP proxy, network, trigger, egress).
type SubResourceChange struct {
	Type     string // "domain", "volume", "tcp_proxy", "network", "trigger", "egress"
	Action   Action
	Key      string   // human-readable key (domain name, mount path, port, etc.)
	LiveID   string   // ID for deletes
	Port     int      // for domains/TCP proxies
	Mount    string   // for volumes
	Repo     string   // for triggers
	Branch   string   // for triggers
	Regions  []string // for egress (set semantics)
	IsCustom bool     // for domains: true = custom, false = service domain
}

// SectionDiff holds diffs for one scope (shared or a service).
type SectionDiff struct {
	Variables    []Change
	Settings     []Change
	SubResources []SubResourceChange
}

// Result holds the complete diff between desired config and live state.
type Result struct {
	Shared   *SectionDiff
	Services map[string]*SectionDiff // keyed by service name
}

// IsEmpty returns true if there are no changes.
func (r *Result) IsEmpty() bool {
	if r.Shared != nil && (len(r.Shared.Variables) > 0 || len(r.Shared.Settings) > 0) {
		return false
	}
	for _, svc := range r.Services {
		if len(svc.Variables) > 0 || len(svc.Settings) > 0 || len(svc.SubResources) > 0 {
			return false
		}
	}
	return true
}

// Options controls which change types are included in the diff.
type Options struct {
	Create bool // include creates (default true)
	Update bool // include updates (default true)
	Delete bool // include deletes (default false)
}

// DefaultOptions returns the default diff options.
func DefaultOptions() Options {
	return Options{Create: true, Update: true, Delete: false}
}

// Compute calculates the diff between desired and live config.
// It uses DefaultOptions (create+update, no delete).
func Compute(desired *config.DesiredConfig, live *config.LiveConfig) *Result {
	return ComputeWithOptions(desired, live, DefaultOptions())
}

// ComputeWithOptions calculates the diff between desired and live config,
// filtering changes by the provided options.
func ComputeWithOptions(desired *config.DesiredConfig, live *config.LiveConfig, opts Options) *Result {
	result := &Result{
		Services: make(map[string]*SectionDiff),
	}
	if desired == nil {
		return result
	}

	// Diff shared variables. Run even when desired.Variables is empty so that
	// live-only variables can be emitted as deletes when opts.Delete is true.
	var liveShared map[string]string
	if live != nil {
		liveShared = live.Variables
	}
	if liveShared == nil {
		liveShared = map[string]string{}
	}
	if len(desired.Variables) > 0 || len(liveShared) > 0 {
		changes := diffVariables(map[string]string(desired.Variables), liveShared)
		changes = filterChanges(changes, opts)
		if len(changes) > 0 {
			result.Shared = &SectionDiff{Variables: changes}
		}
	}

	// Diff per-service.
	for _, desiredSvc := range desired.Services {
		svcName := desiredSvc.Name
		liveSvc := findLiveService(live, svcName, desiredSvc.ID)
		sectionDiff := diffService(desiredSvc, liveSvc)
		sectionDiff.Variables = filterChanges(sectionDiff.Variables, opts)
		sectionDiff.Settings = filterChanges(sectionDiff.Settings, opts)
		sectionDiff.SubResources = filterSubResourceChanges(sectionDiff.SubResources, opts)
		if len(sectionDiff.Variables) > 0 || len(sectionDiff.Settings) > 0 || len(sectionDiff.SubResources) > 0 {
			result.Services[svcName] = sectionDiff
		}
	}

	return result
}

// filterChanges removes changes not permitted by the options.
// Explicit deletes (empty-string sentinel in desired config) are always
// included regardless of opts.Delete.
func filterChanges(changes []Change, opts Options) []Change {
	filtered := changes[:0]
	for _, c := range changes {
		switch c.Action {
		case ActionCreate:
			if opts.Create {
				filtered = append(filtered, c)
			}
		case ActionUpdate:
			if opts.Update {
				filtered = append(filtered, c)
			}
		case ActionDelete:
			// Explicit deletes (empty-string sentinel) are always applied.
			// Implicit deletes (absent from desired) require opts.Delete.
			if opts.Delete || c.Explicit {
				filtered = append(filtered, c)
			}
		}
	}
	return filtered
}

func findLiveService(live *config.LiveConfig, name, id string) *config.ServiceConfig {
	if live == nil {
		return nil
	}
	// Try ID-based match first.
	if id != "" {
		for _, svc := range live.Services {
			if svc.ID == id {
				return svc
			}
		}
	}
	// Fall back to name match.
	return live.Services[name]
}

// diffVariables computes variable diffs between desired and live state.
// Keys present in desired with empty value are treated as explicit deletes.
// Keys present in live but absent from desired are also emitted as deletes
// (filtered later by opts.Delete in filterChanges).
func diffVariables(desired, live map[string]string) []Change {
	var changes []Change
	// Sort keys for deterministic output.
	keys := make([]string, 0, len(desired))
	for k := range desired {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		desiredVal := desired[key]
		liveVal, existsInLive := live[key]

		if desiredVal == "" {
			// Empty string = explicit delete sentinel.
			if existsInLive {
				changes = append(changes, Change{
					Key:       key,
					Action:    ActionDelete,
					LiveValue: liveVal,
					Explicit:  true,
				})
			}
			// If not in live, nothing to delete — skip.
			continue
		}

		if !existsInLive {
			changes = append(changes, Change{
				Key:          key,
				Action:       ActionCreate,
				DesiredValue: desiredVal,
			})
		} else if liveVal != desiredVal {
			changes = append(changes, Change{
				Key:          key,
				Action:       ActionUpdate,
				LiveValue:    liveVal,
				DesiredValue: desiredVal,
			})
		}
		// If same value: no-op.
	}

	// Keys present in live but absent from desired → delete.
	// Collect and sort for deterministic output.
	var liveOnlyKeys []string
	for k := range live {
		if _, inDesired := desired[k]; !inDesired {
			liveOnlyKeys = append(liveOnlyKeys, k)
		}
	}
	sort.Strings(liveOnlyKeys)
	for _, key := range liveOnlyKeys {
		changes = append(changes, Change{
			Key:       key,
			Action:    ActionDelete,
			LiveValue: live[key],
		})
	}

	return changes
}

// diffService computes diffs for a single service's variables and settings.
func diffService(desired *config.DesiredService, live *config.ServiceConfig) *SectionDiff {
	sd := &SectionDiff{}

	// Variables.
	// Icon (project-level property, not per-environment).
	// Only diff if the desired value is non-empty — empty means "unspecified".
	if desired.Icon != "" {
		liveIcon := ""
		if live != nil {
			liveIcon = live.Icon
		}
		if desired.Icon != liveIcon {
			action := ActionCreate
			if liveIcon != "" {
				action = ActionUpdate
			}
			sd.Settings = append(sd.Settings, Change{
				Key:          config.KeyIcon,
				Action:       action,
				LiveValue:    liveIcon,
				DesiredValue: desired.Icon,
			})
		}
	}

	if desired.Variables != nil {
		liveVars := map[string]string{}
		if live != nil {
			liveVars = live.Variables
		}
		sd.Variables = diffVariables(desired.Variables, liveVars)
	}

	// Deploy settings.
	if desired.Deploy != nil {
		var liveDeploy config.Deploy
		if live != nil {
			liveDeploy = live.Deploy
		}
		sd.Settings = append(sd.Settings, diffDeploy(desired.Deploy, liveDeploy)...)
	}

	// Resources.
	if desired.Resources != nil {
		sd.Settings = append(sd.Settings, diffResources(desired.Resources, live)...)
	}

	// Sub-resources.
	sd.SubResources = diffSubResources(desired, live)

	return sd
}

// diffSubResources computes create/delete changes for domains, volumes,
// TCP proxies, network, triggers, and egress.
func diffSubResources(desired *config.DesiredService, live *config.ServiceConfig) []SubResourceChange {
	var changes []SubResourceChange
	changes = append(changes, diffDomains(desired.Domains, live)...)
	changes = append(changes, diffVolumes(desired.Volumes, live)...)
	changes = append(changes, diffTCPProxies(desired.TCPProxies, live)...)
	changes = append(changes, diffNetwork(desired.Network, live)...)
	changes = append(changes, diffTriggers(desired.Triggers, live)...)
	changes = append(changes, diffEgress(desired.Egress, live)...)
	return changes
}

// diffDomains computes domain create/delete changes.
// Desired domains are keyed by domain name (or "service_domain" for auto-generated).
// Delete flag in DomainConfig marks for removal.
func diffDomains(desired map[string]config.DomainConfig, live *config.ServiceConfig) []SubResourceChange {
	if len(desired) == 0 {
		return nil
	}
	var changes []SubResourceChange

	// Build lookup of live domains by domain name.
	liveByDomain := map[string]config.LiveDomain{}
	if live != nil {
		for _, d := range live.Domains {
			liveByDomain[d.Domain] = d
		}
	}

	for domainName, dc := range desired {
		if dc.Delete {
			// Find the live domain to get its ID for deletion.
			if ld, ok := liveByDomain[domainName]; ok {
				changes = append(changes, SubResourceChange{
					Type:     "domain",
					Action:   ActionDelete,
					Key:      domainName,
					LiveID:   ld.ID,
					IsCustom: !ld.IsService,
				})
			}
			continue
		}
		// Create if not in live.
		if _, ok := liveByDomain[domainName]; !ok {
			port := 0
			if dc.Port != nil {
				port = *dc.Port
			}
			// Determine if it's a service domain or custom.
			isCustom := domainName != "service_domain"
			changes = append(changes, SubResourceChange{
				Type:     "domain",
				Action:   ActionCreate,
				Key:      domainName,
				Port:     port,
				IsCustom: isCustom,
			})
		}
	}

	return changes
}

// diffVolumes computes volume create/delete changes.
func diffVolumes(desired map[string]config.VolumeConfig, live *config.ServiceConfig) []SubResourceChange {
	if len(desired) == 0 {
		return nil
	}
	var changes []SubResourceChange

	// Build lookup of live volumes by name.
	liveByName := map[string]config.LiveVolume{}
	if live != nil {
		for _, v := range live.Volumes {
			liveByName[v.Name] = v
		}
	}

	for volName, vc := range desired {
		if vc.Delete {
			if lv, ok := liveByName[volName]; ok {
				changes = append(changes, SubResourceChange{
					Type:   "volume",
					Action: ActionDelete,
					Key:    volName,
					LiveID: lv.VolumeID,
				})
			}
			continue
		}
		// Create if not in live.
		if _, ok := liveByName[volName]; !ok {
			changes = append(changes, SubResourceChange{
				Type:   "volume",
				Action: ActionCreate,
				Key:    volName,
				Mount:  vc.Mount,
			})
		}
	}

	return changes
}

// diffTCPProxies computes TCP proxy create/delete changes.
// Desired is a list of application ports.
func diffTCPProxies(desired []int, live *config.ServiceConfig) []SubResourceChange {
	if len(desired) == 0 {
		return nil
	}
	var changes []SubResourceChange

	// Build set of live application ports.
	liveByPort := map[int]config.LiveTCPProxy{}
	if live != nil {
		for _, p := range live.TCPProxies {
			liveByPort[p.ApplicationPort] = p
		}
	}

	// Desired set.
	desiredSet := map[int]bool{}
	for _, port := range desired {
		desiredSet[port] = true
		if _, ok := liveByPort[port]; !ok {
			changes = append(changes, SubResourceChange{
				Type:   "tcp_proxy",
				Action: ActionCreate,
				Key:    fmt.Sprintf("%d", port),
				Port:   port,
			})
		}
	}

	// Check for live TCP proxies not in desired (deletes).
	if live != nil {
		for _, p := range live.TCPProxies {
			if !desiredSet[p.ApplicationPort] {
				changes = append(changes, SubResourceChange{
					Type:   "tcp_proxy",
					Action: ActionDelete,
					Key:    fmt.Sprintf("%d", p.ApplicationPort),
					LiveID: p.ID,
				})
			}
		}
	}

	return changes
}

// diffNetwork computes private network enable/disable changes.
func diffNetwork(desired *bool, live *config.ServiceConfig) []SubResourceChange {
	if desired == nil {
		return nil
	}
	var changes []SubResourceChange
	liveEnabled := live != nil && live.Network != nil

	if *desired && !liveEnabled {
		changes = append(changes, SubResourceChange{
			Type:   "network",
			Action: ActionCreate,
			Key:    "private_network",
		})
	} else if !*desired && liveEnabled {
		changes = append(changes, SubResourceChange{
			Type:   "network",
			Action: ActionDelete,
			Key:    "private_network",
			LiveID: live.Network.ID,
		})
	}

	return changes
}

// diffTriggers computes deployment trigger create/delete changes.
func diffTriggers(desired []config.TriggerConfig, live *config.ServiceConfig) []SubResourceChange {
	if len(desired) == 0 {
		return nil
	}
	var changes []SubResourceChange

	// Build lookup of live triggers by repo+branch.
	type triggerKey struct{ repo, branch string }
	liveByKey := map[triggerKey]config.LiveTrigger{}
	if live != nil {
		for _, t := range live.Triggers {
			liveByKey[triggerKey{t.Repository, t.Branch}] = t
		}
	}

	desiredKeys := map[triggerKey]bool{}
	for _, tc := range desired {
		key := triggerKey{tc.Repository, tc.Branch}
		desiredKeys[key] = true
		if _, ok := liveByKey[key]; !ok {
			changes = append(changes, SubResourceChange{
				Type:   "trigger",
				Action: ActionCreate,
				Key:    tc.Repository + ":" + tc.Branch,
				Repo:   tc.Repository,
				Branch: tc.Branch,
			})
		}
	}

	// Check for live triggers not in desired (deletes).
	if live != nil {
		for _, t := range live.Triggers {
			key := triggerKey{t.Repository, t.Branch}
			if !desiredKeys[key] {
				changes = append(changes, SubResourceChange{
					Type:   "trigger",
					Action: ActionDelete,
					Key:    t.Repository + ":" + t.Branch,
					LiveID: t.ID,
				})
			}
		}
	}

	return changes
}

// diffEgress computes egress gateway changes.
// Egress uses set semantics — if desired regions differ from live, produce a
// single update change with the full desired set.
func diffEgress(desired []string, live *config.ServiceConfig) []SubResourceChange {
	if len(desired) == 0 {
		return nil
	}

	// Build live region set.
	liveRegions := map[string]bool{}
	if live != nil {
		for _, g := range live.Egress {
			liveRegions[g.Region] = true
		}
	}

	// Build desired region set.
	desiredRegions := map[string]bool{}
	for _, r := range desired {
		desiredRegions[r] = true
	}

	// Compare sets.
	if len(liveRegions) == len(desiredRegions) {
		same := true
		for r := range desiredRegions {
			if !liveRegions[r] {
				same = false
				break
			}
		}
		if same {
			return nil // no change
		}
	}

	// Produce a single update (or create if no live egress).
	action := ActionUpdate
	if len(liveRegions) == 0 {
		action = ActionCreate
	}
	return []SubResourceChange{{
		Type:    "egress",
		Action:  action,
		Key:     "egress_gateways",
		Regions: desired,
	}}
}

// diffDeploy compares desired deploy settings against live.
func diffDeploy(desired *config.DesiredDeploy, live config.Deploy) []Change {
	var changes []Change

	// Source
	if desired.Builder != nil {
		changes = appendSettingDiff(changes, config.KeyBuilder, live.Builder, *desired.Builder)
	}
	changes = appendPtrDiff(changes, config.KeyRepo, live.Repo, desired.Repo)
	changes = appendPtrDiff(changes, config.KeyImage, live.Image, desired.Image)

	// Build
	changes = appendPtrDiff(changes, config.KeyBuildCommand, live.BuildCommand, desired.BuildCommand)
	changes = appendPtrDiff(changes, config.KeyDockerfilePath, live.DockerfilePath, desired.DockerfilePath)
	changes = appendPtrDiff(changes, config.KeyRootDirectory, live.RootDirectory, desired.RootDirectory)
	changes = appendStringSliceDiff(changes, config.KeyWatchPatterns, live.WatchPatterns, desired.WatchPatterns)
	changes = appendPreDeployDiff(changes, config.KeyPreDeployCommand, live.PreDeployCommand, desired.PreDeployCommand)

	// Run
	changes = appendPtrDiff(changes, config.KeyStartCommand, live.StartCommand, desired.StartCommand)
	changes = appendPtrDiff(changes, config.KeyCronSchedule, live.CronSchedule, desired.CronSchedule)

	// Health
	changes = appendPtrDiff(changes, config.KeyHealthcheckPath, live.HealthcheckPath, desired.HealthcheckPath)
	changes = appendIntPtrDiff(changes, config.KeyHealthcheckTimeout, live.HealthcheckTimeout, desired.HealthcheckTimeout)
	if desired.RestartPolicy != nil {
		changes = appendSettingDiff(changes, config.KeyRestartPolicy, live.RestartPolicy, *desired.RestartPolicy)
	}
	changes = appendIntPtrDiff(changes, config.KeyRestartPolicyMaxRetries, live.RestartPolicyMaxRetries, desired.RestartPolicyMaxRetries)

	// Deploy strategy
	changes = appendIntPtrDiff(changes, config.KeyDrainingSeconds, live.DrainingSeconds, desired.DrainingSeconds)
	changes = appendIntPtrDiff(changes, config.KeyOverlapSeconds, live.OverlapSeconds, desired.OverlapSeconds)
	changes = appendBoolPtrDiff(changes, config.KeySleepApplication, live.SleepApplication, desired.SleepApplication)

	// Placement
	changes = appendIntPtrDiff(changes, config.KeyNumReplicas, live.NumReplicas, desired.NumReplicas)
	changes = appendPtrDiff(changes, config.KeyRegion, live.Region, desired.Region)

	// Networking
	changes = appendBoolPtrDiff(changes, config.KeyIPv6Egress, live.IPv6Egress, desired.IPv6Egress)

	return changes
}

// appendPtrDiff appends a setting diff for *string fields. No-op if desired is nil.
func appendPtrDiff(changes []Change, key string, live, desired *string) []Change {
	if desired == nil {
		return changes
	}
	liveVal := ""
	if live != nil {
		liveVal = *live
	}
	return appendSettingDiff(changes, key, liveVal, *desired)
}

// appendIntPtrDiff appends a setting diff for *int fields. No-op if desired is nil.
func appendIntPtrDiff(changes []Change, key string, live, desired *int) []Change {
	if desired == nil {
		return changes
	}
	liveVal := ""
	if live != nil {
		liveVal = fmt.Sprintf("%d", *live)
	}
	desiredVal := fmt.Sprintf("%d", *desired)
	return appendSettingDiff(changes, key, liveVal, desiredVal)
}

// appendBoolPtrDiff appends a setting diff for *bool fields. No-op if desired is nil.
func appendBoolPtrDiff(changes []Change, key string, live, desired *bool) []Change {
	if desired == nil {
		return changes
	}
	liveVal := ""
	if live != nil {
		liveVal = fmt.Sprintf("%t", *live)
	}
	desiredVal := fmt.Sprintf("%t", *desired)
	return appendSettingDiff(changes, key, liveVal, desiredVal)
}

// diffResources compares desired resource limits against live values.
func diffResources(desired *config.DesiredResources, live *config.ServiceConfig) []Change {
	var changes []Change
	if desired.VCPUs != nil {
		liveVal := ""
		if live != nil && live.VCPUs != nil {
			liveVal = fmt.Sprintf("%.1f", *live.VCPUs)
		}
		desiredVal := fmt.Sprintf("%.1f", *desired.VCPUs)
		if liveVal != desiredVal {
			action := ActionUpdate
			if liveVal == "" {
				action = ActionCreate
			}
			changes = append(changes, Change{
				Key:          config.KeyVCPUs,
				Action:       action,
				LiveValue:    liveVal,
				DesiredValue: desiredVal,
			})
		}
	}
	if desired.MemoryGB != nil {
		liveVal := ""
		if live != nil && live.MemoryGB != nil {
			liveVal = fmt.Sprintf("%.1f", *live.MemoryGB)
		}
		desiredVal := fmt.Sprintf("%.1f", *desired.MemoryGB)
		if liveVal != desiredVal {
			action := ActionUpdate
			if liveVal == "" {
				action = ActionCreate
			}
			changes = append(changes, Change{
				Key:          config.KeyMemoryGB,
				Action:       action,
				LiveValue:    liveVal,
				DesiredValue: desiredVal,
			})
		}
	}
	return changes
}

// filterSubResourceChanges removes sub-resource changes not permitted by the options.
func filterSubResourceChanges(changes []SubResourceChange, opts Options) []SubResourceChange {
	if opts.Create && opts.Update && opts.Delete {
		return changes
	}
	filtered := changes[:0]
	for _, c := range changes {
		switch c.Action {
		case ActionCreate:
			if opts.Create {
				filtered = append(filtered, c)
			}
		case ActionUpdate:
			if opts.Update {
				filtered = append(filtered, c)
			}
		case ActionDelete:
			if opts.Delete {
				filtered = append(filtered, c)
			}
		}
	}
	return filtered
}

// appendStringSliceDiff compares []string fields (e.g. watch_patterns).
// Only diffs when desired is non-nil.
func appendStringSliceDiff(changes []Change, key string, live, desired []string) []Change {
	if desired == nil {
		return changes
	}
	liveVal := strings.Join(live, ", ")
	desiredVal := strings.Join(desired, ", ")
	if liveVal == desiredVal {
		return changes
	}
	action := ActionUpdate
	if liveVal == "" {
		action = ActionCreate
	}
	return append(changes, Change{
		Key:          key,
		Action:       action,
		LiveValue:    liveVal,
		DesiredValue: desiredVal,
	})
}

// appendPreDeployDiff compares pre_deploy_command fields.
// Desired is any (string or []string); live is []string.
func appendPreDeployDiff(changes []Change, key string, live []string, desired any) []Change {
	if desired == nil {
		return changes
	}
	var desiredSlice []string
	switch v := desired.(type) {
	case string:
		desiredSlice = []string{v}
	case []string:
		desiredSlice = v
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok {
				desiredSlice = append(desiredSlice, s)
			}
		}
	default:
		return changes
	}
	liveVal := strings.Join(live, ", ")
	desiredVal := strings.Join(desiredSlice, ", ")
	if liveVal == desiredVal {
		return changes
	}
	action := ActionUpdate
	if liveVal == "" {
		action = ActionCreate
	}
	return append(changes, Change{
		Key:          key,
		Action:       action,
		LiveValue:    liveVal,
		DesiredValue: desiredVal,
	})
}

func appendSettingDiff(changes []Change, key, liveVal, desiredVal string) []Change {
	if liveVal == desiredVal {
		return changes
	}
	action := ActionUpdate
	if liveVal == "" {
		action = ActionCreate
	}
	return append(changes, Change{
		Key:          key,
		Action:       action,
		LiveValue:    liveVal,
		DesiredValue: desiredVal,
	})
}
