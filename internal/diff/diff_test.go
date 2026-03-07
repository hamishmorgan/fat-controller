package diff_test

import (
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/config"
	"github.com/hamishmorgan/fat-controller/internal/diff"
)

func TestComputeDiff_CreateVariable(t *testing.T) {
	desired := &config.DesiredConfig{
		Services: []*config.DesiredService{
			{Name: "api", Variables: config.Variables{"NEW_VAR": "value"}},
		},
	}
	live := &config.LiveConfig{
		Services: map[string]*config.ServiceConfig{
			"api": {Name: "api", Variables: map[string]string{}},
		},
	}
	result := diff.Compute(desired, live)
	if len(result.Services) != 1 {
		t.Fatalf("expected 1 service diff, got %d", len(result.Services))
	}
	svc := result.Services["api"]
	if len(svc.Variables) != 1 {
		t.Fatalf("expected 1 var change, got %d", len(svc.Variables))
	}
	ch := svc.Variables[0]
	if ch.Action != diff.ActionCreate {
		t.Errorf("action = %v, want Create", ch.Action)
	}
	if ch.Key != "NEW_VAR" || ch.DesiredValue != "value" {
		t.Errorf("unexpected change: %+v", ch)
	}
}

func TestComputeDiff_UpdateVariable(t *testing.T) {
	desired := &config.DesiredConfig{
		Services: []*config.DesiredService{
			{Name: "api", Variables: config.Variables{"PORT": "9090"}},
		},
	}
	live := &config.LiveConfig{
		Services: map[string]*config.ServiceConfig{
			"api": {Name: "api", Variables: map[string]string{"PORT": "8080"}},
		},
	}
	result := diff.Compute(desired, live)
	ch := result.Services["api"].Variables[0]
	if ch.Action != diff.ActionUpdate {
		t.Errorf("action = %v, want Update", ch.Action)
	}
	if ch.LiveValue != "8080" || ch.DesiredValue != "9090" {
		t.Errorf("unexpected values: live=%q desired=%q", ch.LiveValue, ch.DesiredValue)
	}
}

func TestComputeDiff_DeleteVariable(t *testing.T) {
	desired := &config.DesiredConfig{
		Services: []*config.DesiredService{
			{Name: "api", Variables: config.Variables{"OLD": ""}},
		},
	}
	live := &config.LiveConfig{
		Services: map[string]*config.ServiceConfig{
			"api": {Name: "api", Variables: map[string]string{"OLD": "value"}},
		},
	}
	result := diff.Compute(desired, live)
	ch := result.Services["api"].Variables[0]
	if ch.Action != diff.ActionDelete {
		t.Errorf("action = %v, want Delete", ch.Action)
	}
}

func TestComputeDiff_NoOp(t *testing.T) {
	desired := &config.DesiredConfig{
		Services: []*config.DesiredService{
			{Name: "api", Variables: config.Variables{"PORT": "8080"}},
		},
	}
	live := &config.LiveConfig{
		Services: map[string]*config.ServiceConfig{
			"api": {Name: "api", Variables: map[string]string{"PORT": "8080"}},
		},
	}
	result := diff.Compute(desired, live)
	if svc, ok := result.Services["api"]; ok {
		t.Errorf("expected api to be omitted from results (no changes), got %d variable changes",
			len(svc.Variables))
	}
}

func TestComputeDiff_IgnoresUnmentioned(t *testing.T) {
	desired := &config.DesiredConfig{
		Services: []*config.DesiredService{
			{Name: "api", Variables: config.Variables{"PORT": "8080"}},
		},
	}
	live := &config.LiveConfig{
		Services: map[string]*config.ServiceConfig{
			"api": {Name: "api", Variables: map[string]string{
				"PORT":   "8080",
				"SECRET": "hidden",
			}},
		},
	}
	result := diff.Compute(desired, live)
	if svc, ok := result.Services["api"]; ok {
		t.Errorf("expected api to be omitted (PORT matches, SECRET unmentioned), got %d changes",
			len(svc.Variables))
	}
}

func TestComputeDiff_SharedVariables(t *testing.T) {
	desired := &config.DesiredConfig{
		Variables: config.Variables{
			"SHARED_NEW": "value",
			"SHARED_UPD": "new",
		},
	}
	live := &config.LiveConfig{
		Variables: map[string]string{
			"SHARED_UPD":  "old",
			"SHARED_KEEP": "keep",
		},
	}
	result := diff.Compute(desired, live)
	if len(result.Shared.Variables) != 2 {
		t.Fatalf("expected 2 shared changes, got %d", len(result.Shared.Variables))
	}
	// Find create and update.
	var foundCreate, foundUpdate bool
	for _, ch := range result.Shared.Variables {
		if ch.Key == "SHARED_NEW" && ch.Action == diff.ActionCreate {
			foundCreate = true
		}
		if ch.Key == "SHARED_UPD" && ch.Action == diff.ActionUpdate {
			foundUpdate = true
		}
	}
	if !foundCreate {
		t.Error("expected Create for SHARED_NEW")
	}
	if !foundUpdate {
		t.Error("expected Update for SHARED_UPD")
	}
}

func TestComputeDiff_DeleteEmptyStringNotInLive(t *testing.T) {
	// If config says KEY="" but KEY doesn't exist in live, it's a no-op
	// (can't delete what doesn't exist).
	desired := &config.DesiredConfig{
		Services: []*config.DesiredService{
			{Name: "api", Variables: config.Variables{"GONE": ""}},
		},
	}
	live := &config.LiveConfig{
		Services: map[string]*config.ServiceConfig{
			"api": {Name: "api", Variables: map[string]string{}},
		},
	}
	result := diff.Compute(desired, live)
	if svc, ok := result.Services["api"]; ok {
		t.Errorf("expected api to be omitted (can't delete non-existent var), got %d changes",
			len(svc.Variables))
	}
}

func TestComputeDiff_NewServiceNotInLive(t *testing.T) {
	desired := &config.DesiredConfig{
		Services: []*config.DesiredService{
			{Name: "new-svc", Variables: config.Variables{"X": "1"}},
		},
	}
	live := &config.LiveConfig{
		Services: map[string]*config.ServiceConfig{},
	}
	result := diff.Compute(desired, live)
	svc, ok := result.Services["new-svc"]
	if !ok {
		t.Fatal("expected new-svc in diff result")
	}
	if len(svc.Variables) != 1 || svc.Variables[0].Action != diff.ActionCreate {
		t.Error("expected Create for new service variable")
	}
}

func TestComputeDiff_DeploySettingsChange(t *testing.T) {
	builder := "RAILPACK"
	startCmd := "npm start"
	desired := &config.DesiredConfig{
		Services: []*config.DesiredService{
			{
				Name: "api",
				Deploy: &config.DesiredDeploy{
					Builder:      &builder,
					StartCommand: &startCmd,
				},
			},
		},
	}
	live := &config.LiveConfig{
		Services: map[string]*config.ServiceConfig{
			"api": {
				Name: "api",
				Deploy: config.Deploy{
					Builder:      "NIXPACKS",
					StartCommand: &startCmd, // same
				},
			},
		},
	}
	result := diff.Compute(desired, live)
	svc := result.Services["api"]
	if len(svc.Settings) != 1 {
		t.Fatalf("expected 1 setting change, got %d", len(svc.Settings))
	}
	ch := svc.Settings[0]
	if ch.Key != "builder" || ch.Action != diff.ActionUpdate {
		t.Errorf("unexpected setting change: %+v", ch)
	}
	if ch.LiveValue != "NIXPACKS" || ch.DesiredValue != "RAILPACK" {
		t.Errorf("unexpected values: live=%q desired=%q", ch.LiveValue, ch.DesiredValue)
	}
}

func TestComputeDiff_ResourcesChange_NoLive(t *testing.T) {
	vcpus := 4.0
	desired := &config.DesiredConfig{
		Services: []*config.DesiredService{
			{
				Name:      "api",
				Resources: &config.DesiredResources{VCPUs: &vcpus},
			},
		},
	}
	// Live service has no resource data — diff should show create.
	live := &config.LiveConfig{
		Services: map[string]*config.ServiceConfig{
			"api": {Name: "api"},
		},
	}
	result := diff.Compute(desired, live)
	svc := result.Services["api"]
	if len(svc.Settings) != 1 {
		t.Fatalf("expected 1 setting change, got %d", len(svc.Settings))
	}
	if svc.Settings[0].Key != "vcpus" {
		t.Errorf("expected vcpus change, got %s", svc.Settings[0].Key)
	}
	if svc.Settings[0].Action != diff.ActionCreate {
		t.Errorf("expected Create action, got %v", svc.Settings[0].Action)
	}
}

func TestComputeDiff_ResourcesNoChange(t *testing.T) {
	vcpus := 2.0
	liveVcpus := 2.0
	desired := &config.DesiredConfig{
		Services: []*config.DesiredService{
			{
				Name:      "api",
				Resources: &config.DesiredResources{VCPUs: &vcpus},
			},
		},
	}
	live := &config.LiveConfig{
		Services: map[string]*config.ServiceConfig{
			"api": {Name: "api", VCPUs: &liveVcpus},
		},
	}
	result := diff.Compute(desired, live)
	if svc, ok := result.Services["api"]; ok && len(svc.Settings) > 0 {
		t.Errorf("expected no resource diff when values match, got %d settings", len(svc.Settings))
	}
}

func TestComputeDiff_ResourcesUpdate(t *testing.T) {
	vcpus := 4.0
	liveVcpus := 2.0
	desired := &config.DesiredConfig{
		Services: []*config.DesiredService{
			{
				Name:      "api",
				Resources: &config.DesiredResources{VCPUs: &vcpus},
			},
		},
	}
	live := &config.LiveConfig{
		Services: map[string]*config.ServiceConfig{
			"api": {Name: "api", VCPUs: &liveVcpus},
		},
	}
	result := diff.Compute(desired, live)
	svc := result.Services["api"]
	if len(svc.Settings) != 1 {
		t.Fatalf("expected 1 setting change, got %d", len(svc.Settings))
	}
	ch := svc.Settings[0]
	if ch.Action != diff.ActionUpdate {
		t.Errorf("expected Update action, got %v", ch.Action)
	}
	if ch.LiveValue != "2.0" || ch.DesiredValue != "4.0" {
		t.Errorf("unexpected values: live=%q desired=%q", ch.LiveValue, ch.DesiredValue)
	}
}

func TestComputeDiff_IsEmpty(t *testing.T) {
	result := &diff.Result{}
	if !result.IsEmpty() {
		t.Error("empty result should report IsEmpty")
	}
	result.Shared = &diff.SectionDiff{
		Variables: []diff.Change{{Action: diff.ActionCreate}},
	}
	if result.IsEmpty() {
		t.Error("result with shared change should not be empty")
	}
}
