package apply_test

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/apply"
	"github.com/hamishmorgan/fat-controller/internal/config"
)

// recordingApplier records calls in order, optionally failing on a specified op.
type recordingApplier struct {
	calls  []string
	failOn string // "var", "del", "settings", "resources"
}

var errTest = errors.New("test error")

func (r *recordingApplier) UpsertVariable(_ context.Context, service, key, value string, _ bool) error {
	r.calls = append(r.calls, "var:+:"+service+":"+key+"="+value)
	if r.failOn == "var" {
		return errTest
	}
	return nil
}

func (r *recordingApplier) UpsertVariables(_ context.Context, service string, variables map[string]string, _ bool) error {
	// Build a deterministic representation: sorted key=value pairs.
	pairs := make([]string, 0, len(variables))
	for k, v := range variables {
		pairs = append(pairs, k+"="+v)
	}
	sort.Strings(pairs)
	r.calls = append(r.calls, "var:batch:"+service+":"+strings.Join(pairs, ","))
	if r.failOn == "var" {
		return errTest
	}
	return nil
}

func (r *recordingApplier) DeleteVariable(_ context.Context, service, key string) error {
	r.calls = append(r.calls, "var:-:"+service+":"+key)
	if r.failOn == "del" {
		return errTest
	}
	return nil
}

func (r *recordingApplier) UpdateServiceSettings(_ context.Context, service string, _ *config.DesiredDeploy) error {
	r.calls = append(r.calls, "settings:"+service)
	if r.failOn == "settings" {
		return errTest
	}
	return nil
}

func (r *recordingApplier) UpdateServiceResources(_ context.Context, service string, _ *config.DesiredResources) error {
	r.calls = append(r.calls, "resources:"+service)
	if r.failOn == "resources" {
		return errTest
	}
	return nil
}

func TestApply_Order_SettingsThenSharedThenServiceVars(t *testing.T) {
	builder := "NIXPACKS"
	vcpus := 1.0
	desired := &config.DesiredConfig{
		Shared: &config.DesiredVariables{Vars: map[string]string{"SHARED": "1"}},
		Services: map[string]*config.DesiredService{
			"api": {
				Deploy:    &config.DesiredDeploy{Builder: &builder},
				Resources: &config.DesiredResources{VCPUs: &vcpus},
				Variables: map[string]string{"PORT": "8080"},
			},
		},
	}
	live := &config.LiveConfig{
		Services: map[string]*config.ServiceConfig{
			"api": {Name: "api", Variables: map[string]string{}},
		},
	}

	rec := &recordingApplier{}
	result, err := apply.Apply(context.Background(), desired, live, rec, apply.Options{})
	if err != nil {
		t.Fatalf("Apply() error: %v", err)
	}
	if result.Applied != 4 {
		t.Errorf("Applied = %d, want 4", result.Applied)
	}

	// Expected order: settings, resources, shared vars (batched), service vars (batched).
	want := []string{
		"settings:api",
		"resources:api",
		"var:batch::SHARED=1",
		"var:batch:api:PORT=8080",
	}
	if len(rec.calls) != len(want) {
		t.Fatalf("calls = %v, want %v", rec.calls, want)
	}
	for i := range want {
		if rec.calls[i] != want[i] {
			t.Errorf("call[%d] = %q, want %q", i, rec.calls[i], want[i])
		}
	}
}

func TestApply_FailFastStops(t *testing.T) {
	desired := &config.DesiredConfig{
		Services: map[string]*config.DesiredService{
			"api": {Variables: map[string]string{"A": "1"}},
			"web": {Variables: map[string]string{"B": "2"}},
		},
	}
	live := &config.LiveConfig{
		Services: map[string]*config.ServiceConfig{
			"api": {Name: "api", Variables: map[string]string{}},
			"web": {Name: "web", Variables: map[string]string{}},
		},
	}

	rec := &recordingApplier{failOn: "var"}
	result, err := apply.Apply(context.Background(), desired, live, rec, apply.Options{FailFast: true})
	if err == nil {
		t.Fatal("expected error with fail-fast")
	}
	// First batch (api) fails, web batch is never attempted.
	if len(rec.calls) != 1 {
		t.Errorf("expected 1 call with fail-fast, got %d: %v", len(rec.calls), rec.calls)
	}
	// The batch contained 1 variable, so Failed = 1.
	if result.Failed != 1 {
		t.Errorf("Failed = %d, want 1", result.Failed)
	}
}

func TestApply_ContinueOnFailure(t *testing.T) {
	desired := &config.DesiredConfig{
		Services: map[string]*config.DesiredService{
			"api": {Variables: map[string]string{"A": "1"}},
			"web": {Variables: map[string]string{"B": "2"}},
		},
	}
	live := &config.LiveConfig{
		Services: map[string]*config.ServiceConfig{
			"api": {Name: "api", Variables: map[string]string{}},
			"web": {Name: "web", Variables: map[string]string{}},
		},
	}

	rec := &recordingApplier{failOn: "var"}
	result, err := apply.Apply(context.Background(), desired, live, rec, apply.Options{FailFast: false})
	if err != nil {
		t.Fatalf("Apply() should not return error in best-effort mode: %v", err)
	}
	// Both services should have been attempted (sorted: api, web), one batch each.
	if len(rec.calls) != 2 {
		t.Errorf("expected 2 calls in best-effort mode, got %d: %v", len(rec.calls), rec.calls)
	}
	// Each batch had 1 variable, so Failed = 2.
	if result.Failed != 2 {
		t.Errorf("Failed = %d, want 2", result.Failed)
	}
}

func TestApply_NoChanges(t *testing.T) {
	desired := &config.DesiredConfig{
		Services: map[string]*config.DesiredService{
			"api": {Variables: map[string]string{"PORT": "8080"}},
		},
	}
	live := &config.LiveConfig{
		Services: map[string]*config.ServiceConfig{
			"api": {Name: "api", Variables: map[string]string{"PORT": "8080"}},
		},
	}

	rec := &recordingApplier{}
	result, err := apply.Apply(context.Background(), desired, live, rec, apply.Options{})
	if err != nil {
		t.Fatalf("Apply() error: %v", err)
	}
	if result.Applied != 0 || result.Failed != 0 {
		t.Errorf("expected no changes, got applied=%d failed=%d", result.Applied, result.Failed)
	}
	if len(rec.calls) != 0 {
		t.Errorf("expected no calls, got %v", rec.calls)
	}
}

func TestApply_StopsOnContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	desired := &config.DesiredConfig{
		Services: map[string]*config.DesiredService{
			"api": {Variables: map[string]string{"FOO": "bar"}},
		},
	}
	live := &config.LiveConfig{
		Services: map[string]*config.ServiceConfig{
			"api": {Name: "api", Variables: map[string]string{}},
		},
	}
	applier := &recordingApplier{}
	_, err := apply.Apply(ctx, desired, live, applier, apply.Options{})
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
	if len(applier.calls) > 0 {
		t.Errorf("expected no calls on cancelled context, got %d", len(applier.calls))
	}
}

func TestApply_DeleteVariable(t *testing.T) {
	desired := &config.DesiredConfig{
		Services: map[string]*config.DesiredService{
			"api": {Variables: map[string]string{"OLD": ""}},
		},
	}
	live := &config.LiveConfig{
		Services: map[string]*config.ServiceConfig{
			"api": {Name: "api", Variables: map[string]string{"OLD": "value"}},
		},
	}

	rec := &recordingApplier{}
	result, err := apply.Apply(context.Background(), desired, live, rec, apply.Options{})
	if err != nil {
		t.Fatalf("Apply() error: %v", err)
	}
	if result.Applied != 1 {
		t.Errorf("Applied = %d, want 1", result.Applied)
	}
	if len(rec.calls) != 1 || rec.calls[0] != "var:-:api:OLD" {
		t.Errorf("unexpected calls: %v", rec.calls)
	}
}
