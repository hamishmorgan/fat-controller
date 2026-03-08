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
	// Deletes are only included when opts.Delete is true.
	opts := diff.Options{Create: true, Update: true, Delete: true}
	result := diff.ComputeWithOptions(desired, live, opts)
	ch := result.Services["api"].Variables[0]
	if ch.Action != diff.ActionDelete {
		t.Errorf("action = %v, want Delete", ch.Action)
	}
}

func TestComputeDiff_DeleteVariable_SentinelAlwaysApplies(t *testing.T) {
	// Empty-string sentinel is an explicit user intent — it always applies
	// regardless of whether --delete is set.
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
	// Default options (no --delete flag).
	result := diff.Compute(desired, live)
	svc, ok := result.Services["api"]
	if !ok || len(svc.Variables) != 1 {
		t.Fatalf("expected explicit sentinel delete to be present with default options, got %v", svc)
	}
	if svc.Variables[0].Action != diff.ActionDelete {
		t.Errorf("action = %v, want Delete", svc.Variables[0].Action)
	}
	if !svc.Variables[0].Explicit {
		t.Error("expected Explicit = true for sentinel delete")
	}
}

func TestComputeDiff_DeleteVariable_ImplicitFilteredByDefault(t *testing.T) {
	// Variables absent from desired but present in live are implicit deletes.
	// They are filtered out unless --delete is set.
	desired := &config.DesiredConfig{
		Services: []*config.DesiredService{
			{Name: "api", Variables: config.Variables{"KEEP": "value"}},
		},
	}
	live := &config.LiveConfig{
		Services: map[string]*config.ServiceConfig{
			"api": {Name: "api", Variables: map[string]string{"KEEP": "value", "STALE": "old"}},
		},
	}
	// Default options (no --delete flag) — implicit delete should be filtered.
	result := diff.Compute(desired, live)
	if _, ok := result.Services["api"]; ok {
		t.Error("expected implicit delete to be filtered out with default options")
	}
}

func TestComputeDiff_DeleteVariable_AbsentFromDesired(t *testing.T) {
	// Variable exists in live but is simply not mentioned in desired.
	// With --delete this should produce an ActionDelete change.
	desired := &config.DesiredConfig{
		Services: []*config.DesiredService{
			{Name: "api", Variables: config.Variables{"KEEP": "value"}},
		},
	}
	live := &config.LiveConfig{
		Services: map[string]*config.ServiceConfig{
			"api": {Name: "api", Variables: map[string]string{"KEEP": "value", "STALE": "old"}},
		},
	}
	opts := diff.Options{Create: true, Update: true, Delete: true}
	result := diff.ComputeWithOptions(desired, live, opts)
	svc := result.Services["api"]
	if svc == nil || len(svc.Variables) != 1 {
		t.Fatalf("expected 1 variable change, got %v", svc)
	}
	ch := svc.Variables[0]
	if ch.Action != diff.ActionDelete {
		t.Errorf("action = %v, want Delete", ch.Action)
	}
	if ch.Key != "STALE" {
		t.Errorf("key = %q, want STALE", ch.Key)
	}
	if ch.LiveValue != "old" {
		t.Errorf("live value = %q, want old", ch.LiveValue)
	}
}

func TestComputeDiff_DeleteVariable_AbsentFromDesired_FilteredByDefault(t *testing.T) {
	// Without --delete, absent-from-desired variables should not appear.
	desired := &config.DesiredConfig{
		Services: []*config.DesiredService{
			{Name: "api", Variables: config.Variables{"KEEP": "value"}},
		},
	}
	live := &config.LiveConfig{
		Services: map[string]*config.ServiceConfig{
			"api": {Name: "api", Variables: map[string]string{"KEEP": "value", "STALE": "old"}},
		},
	}
	result := diff.Compute(desired, live)
	if _, ok := result.Services["api"]; ok {
		t.Error("expected no changes with default options when only live-only key differs")
	}
}

func TestComputeDiff_DeleteSharedVariable_AbsentFromDesired(t *testing.T) {
	// Shared variable exists in live but is absent from desired config.
	desired := &config.DesiredConfig{
		Variables: config.Variables{"KEEP": "value"},
	}
	live := &config.LiveConfig{
		Variables: map[string]string{"KEEP": "value", "STALE": "old"},
	}
	opts := diff.Options{Create: true, Update: true, Delete: true}
	result := diff.ComputeWithOptions(desired, live, opts)
	if result.Shared == nil || len(result.Shared.Variables) != 1 {
		t.Fatalf("expected 1 shared variable change, got %v", result.Shared)
	}
	ch := result.Shared.Variables[0]
	if ch.Action != diff.ActionDelete || ch.Key != "STALE" {
		t.Errorf("unexpected change: %+v", ch)
	}
}

func TestComputeDiff_DeleteAllSharedVariables(t *testing.T) {
	// All shared variables removed from desired — live-only vars should be deleted.
	desired := &config.DesiredConfig{}
	live := &config.LiveConfig{
		Variables: map[string]string{"A": "1", "B": "2"},
	}
	opts := diff.Options{Create: true, Update: true, Delete: true}
	result := diff.ComputeWithOptions(desired, live, opts)
	if result.Shared == nil || len(result.Shared.Variables) != 2 {
		t.Fatalf("expected 2 shared variable deletes, got %v", result.Shared)
	}
	for _, ch := range result.Shared.Variables {
		if ch.Action != diff.ActionDelete {
			t.Errorf("expected delete action, got %v for key %q", ch.Action, ch.Key)
		}
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

func TestComputeDiff_IconUpdate(t *testing.T) {
	desired := &config.DesiredConfig{
		Services: []*config.DesiredService{
			{Name: "api", Icon: "server"},
		},
	}
	live := &config.LiveConfig{
		Services: map[string]*config.ServiceConfig{
			"api": {Name: "api", Icon: "database"},
		},
	}
	result := diff.Compute(desired, live)
	svc := result.Services["api"]
	if len(svc.Settings) != 1 {
		t.Fatalf("expected 1 setting change, got %d: %v", len(svc.Settings), svc.Settings)
	}
	ch := svc.Settings[0]
	if ch.Key != config.KeyIcon {
		t.Errorf("key = %q, want %q", ch.Key, config.KeyIcon)
	}
	if ch.Action != diff.ActionUpdate {
		t.Errorf("action = %v, want Update", ch.Action)
	}
	if ch.LiveValue != "database" || ch.DesiredValue != "server" {
		t.Errorf("values: live=%q desired=%q", ch.LiveValue, ch.DesiredValue)
	}
}

func TestComputeDiff_IconCreate(t *testing.T) {
	desired := &config.DesiredConfig{
		Services: []*config.DesiredService{
			{Name: "api", Icon: "server"},
		},
	}
	live := &config.LiveConfig{
		Services: map[string]*config.ServiceConfig{
			"api": {Name: "api", Icon: ""},
		},
	}
	result := diff.Compute(desired, live)
	svc := result.Services["api"]
	if len(svc.Settings) != 1 {
		t.Fatalf("expected 1 setting change, got %d", len(svc.Settings))
	}
	if svc.Settings[0].Action != diff.ActionCreate {
		t.Errorf("action = %v, want Create", svc.Settings[0].Action)
	}
}

func TestComputeDiff_IconNoChange(t *testing.T) {
	desired := &config.DesiredConfig{
		Services: []*config.DesiredService{
			{Name: "api", Icon: "server"},
		},
	}
	live := &config.LiveConfig{
		Services: map[string]*config.ServiceConfig{
			"api": {Name: "api", Icon: "server"},
		},
	}
	result := diff.Compute(desired, live)
	if svc, ok := result.Services["api"]; ok {
		t.Errorf("expected no settings changes, got %v", svc.Settings)
	}
}

func TestComputeDiff_IconNotSetInDesired_NoChange(t *testing.T) {
	// If icon is omitted from desired (empty string), don't diff it -
	// the user hasn't expressed intent.
	desired := &config.DesiredConfig{
		Services: []*config.DesiredService{
			{Name: "api", Icon: ""},
		},
	}
	live := &config.LiveConfig{
		Services: map[string]*config.ServiceConfig{
			"api": {Name: "api", Icon: "database"},
		},
	}
	result := diff.Compute(desired, live)
	if svc, ok := result.Services["api"]; ok {
		t.Errorf("expected no settings changes when desired icon is empty, got %v", svc.Settings)
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

// Sub-resource diff tests

func TestDiff_DomainCreate(t *testing.T) {
	port := 8080
	desired := &config.DesiredConfig{
		Services: []*config.DesiredService{{
			Name: "api",
			Domains: map[string]config.DomainConfig{
				"api.example.com": {Port: &port},
			},
		}},
	}
	live := &config.LiveConfig{
		Services: map[string]*config.ServiceConfig{
			"api": {Name: "api"},
		},
	}
	result := diff.Compute(desired, live)
	svc := result.Services["api"]
	if svc == nil {
		t.Fatal("expected api in diff")
	}
	if len(svc.SubResources) != 1 {
		t.Fatalf("expected 1 sub-resource change, got %d", len(svc.SubResources))
	}
	ch := svc.SubResources[0]
	if ch.Type != "domain" || ch.Action != diff.ActionCreate {
		t.Errorf("expected domain create, got type=%s action=%v", ch.Type, ch.Action)
	}
	if ch.Key != "api.example.com" || ch.Port != 8080 {
		t.Errorf("unexpected values: key=%q port=%d", ch.Key, ch.Port)
	}
}

func TestDiff_DomainDelete(t *testing.T) {
	desired := &config.DesiredConfig{
		Services: []*config.DesiredService{{
			Name: "api",
			Domains: map[string]config.DomainConfig{
				"old.example.com": {Delete: true},
			},
		}},
	}
	live := &config.LiveConfig{
		Services: map[string]*config.ServiceConfig{
			"api": {
				Name: "api",
				Domains: []config.LiveDomain{
					{ID: "dom-1", Domain: "old.example.com"},
				},
			},
		},
	}
	opts := diff.Options{Create: true, Update: true, Delete: true}
	result := diff.ComputeWithOptions(desired, live, opts)
	svc := result.Services["api"]
	if svc == nil {
		t.Fatal("expected api in diff")
	}
	if len(svc.SubResources) != 1 {
		t.Fatalf("expected 1 sub-resource change, got %d", len(svc.SubResources))
	}
	ch := svc.SubResources[0]
	if ch.Type != "domain" || ch.Action != diff.ActionDelete || ch.LiveID != "dom-1" {
		t.Errorf("unexpected: type=%s action=%v liveID=%s", ch.Type, ch.Action, ch.LiveID)
	}
}

func TestDiff_DomainNoChange(t *testing.T) {
	port := 8080
	desired := &config.DesiredConfig{
		Services: []*config.DesiredService{{
			Name: "api",
			Domains: map[string]config.DomainConfig{
				"api.example.com": {Port: &port},
			},
		}},
	}
	live := &config.LiveConfig{
		Services: map[string]*config.ServiceConfig{
			"api": {
				Name: "api",
				Domains: []config.LiveDomain{
					{ID: "dom-1", Domain: "api.example.com"},
				},
			},
		},
	}
	result := diff.Compute(desired, live)
	if svc, ok := result.Services["api"]; ok && len(svc.SubResources) > 0 {
		t.Errorf("expected no domain changes, got %d", len(svc.SubResources))
	}
}

func TestDiff_VolumeCreate(t *testing.T) {
	desired := &config.DesiredConfig{
		Services: []*config.DesiredService{{
			Name: "api",
			Volumes: map[string]config.VolumeConfig{
				"data": {Mount: "/data"},
			},
		}},
	}
	live := &config.LiveConfig{
		Services: map[string]*config.ServiceConfig{
			"api": {Name: "api"},
		},
	}
	result := diff.Compute(desired, live)
	svc := result.Services["api"]
	if len(svc.SubResources) != 1 {
		t.Fatalf("expected 1 sub-resource change, got %d", len(svc.SubResources))
	}
	ch := svc.SubResources[0]
	if ch.Type != "volume" || ch.Action != diff.ActionCreate || ch.Mount != "/data" {
		t.Errorf("unexpected: type=%s action=%v mount=%s", ch.Type, ch.Action, ch.Mount)
	}
}

func TestDiff_TCPProxyCreateAndDelete(t *testing.T) {
	desired := &config.DesiredConfig{
		Services: []*config.DesiredService{{
			Name:       "api",
			TCPProxies: []int{8080, 9090},
		}},
	}
	live := &config.LiveConfig{
		Services: map[string]*config.ServiceConfig{
			"api": {
				Name: "api",
				TCPProxies: []config.LiveTCPProxy{
					{ID: "tcp-1", ApplicationPort: 8080, ProxyPort: 12345},
					{ID: "tcp-2", ApplicationPort: 3000, ProxyPort: 12346},
				},
			},
		},
	}
	opts := diff.Options{Create: true, Update: true, Delete: true}
	result := diff.ComputeWithOptions(desired, live, opts)
	svc := result.Services["api"]
	if svc == nil {
		t.Fatal("expected api in diff")
	}
	// Should create 9090, delete 3000. 8080 exists in both — no change.
	var creates, deletes int
	for _, ch := range svc.SubResources {
		if ch.Type != "tcp_proxy" {
			continue
		}
		switch ch.Action {
		case diff.ActionCreate:
			creates++
			if ch.Port != 9090 {
				t.Errorf("expected create for port 9090, got %d", ch.Port)
			}
		case diff.ActionDelete:
			deletes++
			if ch.LiveID != "tcp-2" {
				t.Errorf("expected delete for tcp-2, got %s", ch.LiveID)
			}
		}
	}
	if creates != 1 || deletes != 1 {
		t.Errorf("expected 1 create + 1 delete, got %d creates + %d deletes", creates, deletes)
	}
}

func TestDiff_NetworkEnable(t *testing.T) {
	enabled := true
	desired := &config.DesiredConfig{
		Services: []*config.DesiredService{{
			Name:    "api",
			Network: &enabled,
		}},
	}
	live := &config.LiveConfig{
		Services: map[string]*config.ServiceConfig{
			"api": {Name: "api"},
		},
	}
	result := diff.Compute(desired, live)
	svc := result.Services["api"]
	if len(svc.SubResources) != 1 {
		t.Fatalf("expected 1 sub-resource change, got %d", len(svc.SubResources))
	}
	if svc.SubResources[0].Type != "network" || svc.SubResources[0].Action != diff.ActionCreate {
		t.Errorf("expected network create, got %+v", svc.SubResources[0])
	}
}

func TestDiff_TriggerCreate(t *testing.T) {
	desired := &config.DesiredConfig{
		Services: []*config.DesiredService{{
			Name: "api",
			Triggers: []config.TriggerConfig{
				{Repository: "org/repo", Branch: "main"},
			},
		}},
	}
	live := &config.LiveConfig{
		Services: map[string]*config.ServiceConfig{
			"api": {Name: "api"},
		},
	}
	result := diff.Compute(desired, live)
	svc := result.Services["api"]
	if len(svc.SubResources) != 1 {
		t.Fatalf("expected 1 sub-resource change, got %d", len(svc.SubResources))
	}
	ch := svc.SubResources[0]
	if ch.Type != "trigger" || ch.Action != diff.ActionCreate {
		t.Errorf("expected trigger create, got type=%s action=%v", ch.Type, ch.Action)
	}
	if ch.Repo != "org/repo" || ch.Branch != "main" {
		t.Errorf("unexpected trigger: repo=%q branch=%q", ch.Repo, ch.Branch)
	}
}

func TestDiff_EgressUpdate(t *testing.T) {
	desired := &config.DesiredConfig{
		Services: []*config.DesiredService{{
			Name:   "api",
			Egress: []string{"us-west-2", "eu-west-1"},
		}},
	}
	live := &config.LiveConfig{
		Services: map[string]*config.ServiceConfig{
			"api": {
				Name: "api",
				Egress: []config.LiveEgressGateway{
					{Region: "us-west-2", IPv4: "1.2.3.4"},
				},
			},
		},
	}
	result := diff.Compute(desired, live)
	svc := result.Services["api"]
	if len(svc.SubResources) != 1 {
		t.Fatalf("expected 1 sub-resource change, got %d", len(svc.SubResources))
	}
	ch := svc.SubResources[0]
	if ch.Type != "egress" || ch.Action != diff.ActionUpdate {
		t.Errorf("expected egress update, got type=%s action=%v", ch.Type, ch.Action)
	}
	if len(ch.Regions) != 2 {
		t.Errorf("expected 2 regions, got %d", len(ch.Regions))
	}
}

func TestDiff_EgressNoChange(t *testing.T) {
	desired := &config.DesiredConfig{
		Services: []*config.DesiredService{{
			Name:   "api",
			Egress: []string{"us-west-2"},
		}},
	}
	live := &config.LiveConfig{
		Services: map[string]*config.ServiceConfig{
			"api": {
				Name: "api",
				Egress: []config.LiveEgressGateway{
					{Region: "us-west-2", IPv4: "1.2.3.4"},
				},
			},
		},
	}
	result := diff.Compute(desired, live)
	if svc, ok := result.Services["api"]; ok && len(svc.SubResources) > 0 {
		t.Errorf("expected no egress changes, got %d", len(svc.SubResources))
	}
}

func TestDiff_SubResourcesFilteredByOptions(t *testing.T) {
	port := 8080
	desired := &config.DesiredConfig{
		Services: []*config.DesiredService{{
			Name: "api",
			Domains: map[string]config.DomainConfig{
				"new.example.com": {Port: &port},
				"old.example.com": {Delete: true},
			},
		}},
	}
	live := &config.LiveConfig{
		Services: map[string]*config.ServiceConfig{
			"api": {
				Name: "api",
				Domains: []config.LiveDomain{
					{ID: "dom-1", Domain: "old.example.com"},
				},
			},
		},
	}

	// With creates only — should see create but not delete.
	opts := diff.Options{Create: true, Update: false, Delete: false}
	result := diff.ComputeWithOptions(desired, live, opts)
	svc := result.Services["api"]
	if svc == nil {
		t.Fatal("expected api in diff")
	}
	for _, ch := range svc.SubResources {
		if ch.Action == diff.ActionDelete {
			t.Error("delete should be filtered out with Create-only options")
		}
	}

	// With deletes only — should see delete but not create.
	opts = diff.Options{Create: false, Update: false, Delete: true}
	result = diff.ComputeWithOptions(desired, live, opts)
	svc = result.Services["api"]
	if svc == nil {
		t.Fatal("expected api in diff")
	}
	for _, ch := range svc.SubResources {
		if ch.Action == diff.ActionCreate {
			t.Error("create should be filtered out with Delete-only options")
		}
	}
}
