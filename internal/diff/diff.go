package diff

import (
	"fmt"
	"sort"

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
}

// SectionDiff holds diffs for one scope (shared or a service).
type SectionDiff struct {
	Variables []Change
	Settings  []Change
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
		if len(svc.Variables) > 0 || len(svc.Settings) > 0 {
			return false
		}
	}
	return true
}

// Compute calculates the additive-only diff between desired and live config.
func Compute(desired *config.DesiredConfig, live *config.LiveConfig) *Result {
	result := &Result{
		Services: make(map[string]*SectionDiff),
	}
	if desired == nil {
		return result
	}

	// Diff shared variables.
	if desired.Shared != nil {
		var liveShared map[string]string
		if live != nil {
			liveShared = live.Shared
		}
		if liveShared == nil {
			liveShared = map[string]string{}
		}
		changes := diffVariables(desired.Shared.Vars, liveShared)
		if len(changes) > 0 {
			result.Shared = &SectionDiff{Variables: changes}
		}
	}

	// Diff per-service.
	for svcName, desiredSvc := range desired.Services {
		liveSvc := findLiveService(live, svcName)
		sectionDiff := diffService(desiredSvc, liveSvc)
		if len(sectionDiff.Variables) > 0 || len(sectionDiff.Settings) > 0 {
			result.Services[svcName] = sectionDiff
		}
	}

	return result
}

func findLiveService(live *config.LiveConfig, name string) *config.ServiceConfig {
	if live == nil {
		return nil
	}
	return live.Services[name]
}

// diffVariables computes additive-only variable diffs.
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
			// Empty string = delete.
			if existsInLive {
				changes = append(changes, Change{
					Key:       key,
					Action:    ActionDelete,
					LiveValue: liveVal,
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
	return changes
}

// diffService computes diffs for a single service's variables and settings.
func diffService(desired *config.DesiredService, live *config.ServiceConfig) *SectionDiff {
	sd := &SectionDiff{}

	// Variables.
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
		sd.Settings = append(sd.Settings, diffResources(desired.Resources)...)
	}

	return sd
}

// diffDeploy compares desired deploy settings against live.
func diffDeploy(desired *config.DesiredDeploy, live config.Deploy) []Change {
	var changes []Change

	if desired.Builder != nil {
		changes = appendSettingDiff(changes, "builder", live.Builder, *desired.Builder)
	}
	if desired.DockerfilePath != nil {
		liveVal := ""
		if live.DockerfilePath != nil {
			liveVal = *live.DockerfilePath
		}
		changes = appendSettingDiff(changes, "dockerfile_path", liveVal, *desired.DockerfilePath)
	}
	if desired.RootDirectory != nil {
		liveVal := ""
		if live.RootDirectory != nil {
			liveVal = *live.RootDirectory
		}
		changes = appendSettingDiff(changes, "root_directory", liveVal, *desired.RootDirectory)
	}
	if desired.StartCommand != nil {
		liveVal := ""
		if live.StartCommand != nil {
			liveVal = *live.StartCommand
		}
		changes = appendSettingDiff(changes, "start_command", liveVal, *desired.StartCommand)
	}
	if desired.HealthcheckPath != nil {
		liveVal := ""
		if live.HealthcheckPath != nil {
			liveVal = *live.HealthcheckPath
		}
		changes = appendSettingDiff(changes, "healthcheck_path", liveVal, *desired.HealthcheckPath)
	}

	return changes
}

// diffResources compares desired resource limits. Live resource limits aren't
// currently in the LiveConfig model (they require a separate query), so we
// treat any specified desired value as a change. This is conservative — the
// apply step will set the value regardless.
func diffResources(desired *config.DesiredResources) []Change {
	var changes []Change
	if desired.VCPUs != nil {
		changes = append(changes, Change{
			Key:          "vcpus",
			Action:       ActionUpdate,
			DesiredValue: fmt.Sprintf("%.1f", *desired.VCPUs),
		})
	}
	if desired.MemoryGB != nil {
		changes = append(changes, Change{
			Key:          "memory_gb",
			Action:       ActionUpdate,
			DesiredValue: fmt.Sprintf("%.1f", *desired.MemoryGB),
		})
	}
	return changes
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
