package apply_test

import (
	"context"
	"errors"
	"fmt"
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

func (r *recordingApplier) UpdateServiceIcon(_ context.Context, service, icon string) error {
	r.calls = append(r.calls, "icon:"+service+":"+icon)
	return nil
}

func (r *recordingApplier) UpdateServiceResources(_ context.Context, service string, _ *config.DesiredResources) error {
	r.calls = append(r.calls, "resources:"+service)
	if r.failOn == "resources" {
		return errTest
	}
	return nil
}

func (r *recordingApplier) CreateService(_ context.Context, name string) (string, error) {
	r.calls = append(r.calls, "create-service:"+name)
	return "svc-new-" + name, nil
}

func (r *recordingApplier) DeleteService(_ context.Context, id string) error {
	r.calls = append(r.calls, "delete-service:"+id)
	return nil
}

func (r *recordingApplier) CreateCustomDomain(_ context.Context, svcID, domain string, port int) error {
	r.calls = append(r.calls, fmt.Sprintf("create-custom-domain:%s:%s:%d", svcID, domain, port))
	return nil
}

func (r *recordingApplier) DeleteCustomDomain(_ context.Context, id string) error {
	r.calls = append(r.calls, "delete-custom-domain:"+id)
	return nil
}

func (r *recordingApplier) CreateServiceDomain(_ context.Context, svcID string, port int) error {
	r.calls = append(r.calls, fmt.Sprintf("create-service-domain:%s:%d", svcID, port))
	return nil
}

func (r *recordingApplier) DeleteServiceDomain(_ context.Context, id string) error {
	r.calls = append(r.calls, "delete-service-domain:"+id)
	return nil
}

func (r *recordingApplier) CreateVolume(_ context.Context, svcID, mount, region string) error {
	r.calls = append(r.calls, fmt.Sprintf("create-volume:%s:%s:%s", svcID, mount, region))
	return nil
}

func (r *recordingApplier) DeleteVolume(_ context.Context, id string) error {
	r.calls = append(r.calls, "delete-volume:"+id)
	return nil
}

func (r *recordingApplier) CreateTCPProxy(_ context.Context, svcID string, port int) error {
	r.calls = append(r.calls, fmt.Sprintf("create-tcp:%s:%d", svcID, port))
	return nil
}

func (r *recordingApplier) DeleteTCPProxy(_ context.Context, id string) error {
	r.calls = append(r.calls, "delete-tcp:"+id)
	return nil
}

func (r *recordingApplier) EnablePrivateNetwork(_ context.Context, svcID string) error {
	r.calls = append(r.calls, "enable-network:"+svcID)
	return nil
}

func (r *recordingApplier) DisablePrivateNetwork(_ context.Context, id string) error {
	r.calls = append(r.calls, "disable-network:"+id)
	return nil
}

func (r *recordingApplier) SetEgressGateways(_ context.Context, svcID string, regions []string) error {
	sort.Strings(regions)
	r.calls = append(r.calls, "set-egress:"+svcID+":"+strings.Join(regions, ","))
	return nil
}

func (r *recordingApplier) CreateDeploymentTrigger(_ context.Context, svcID, repo, branch, provider string) error {
	r.calls = append(r.calls, fmt.Sprintf("create-trigger:%s:%s:%s:%s", svcID, repo, branch, provider))
	return nil
}

func (r *recordingApplier) DeleteDeploymentTrigger(_ context.Context, id string) error {
	r.calls = append(r.calls, "delete-trigger:"+id)
	return nil
}

func (r *recordingApplier) TriggerDeploy(_ context.Context, svcID string) error {
	r.calls = append(r.calls, "deploy:"+svcID)
	return nil
}

func TestApply_Order_SettingsThenSharedThenServiceVars(t *testing.T) {
	builder := "NIXPACKS"
	vcpus := 1.0
	desired := &config.DesiredConfig{
		Variables: config.Variables{"SHARED": "1"},
		Services: []*config.DesiredService{
			{
				Name:      "api",
				Deploy:    &config.DesiredDeploy{Builder: &builder},
				Resources: &config.DesiredResources{VCPUs: &vcpus},
				Variables: config.Variables{"PORT": "8080"},
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
		Services: []*config.DesiredService{
			{Name: "api", Variables: config.Variables{"A": "1"}},
			{Name: "web", Variables: config.Variables{"B": "2"}},
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
		Services: []*config.DesiredService{
			{Name: "api", Variables: config.Variables{"A": "1"}},
			{Name: "web", Variables: config.Variables{"B": "2"}},
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
		Services: []*config.DesiredService{
			{Name: "api", Variables: config.Variables{"PORT": "8080"}},
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
		Services: []*config.DesiredService{
			{Name: "api", Variables: config.Variables{"FOO": "bar"}},
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

func TestApply_CreateService(t *testing.T) {
	desired := &config.DesiredConfig{
		Services: []*config.DesiredService{
			{Name: "new-svc", Variables: config.Variables{"X": "1"}},
		},
	}
	live := &config.LiveConfig{
		Services: map[string]*config.ServiceConfig{},
	}

	rec := &recordingApplier{}
	result, err := apply.Apply(context.Background(), desired, live, rec, apply.Options{})
	if err != nil {
		t.Fatalf("Apply() error: %v", err)
	}
	// Should create the service + create var.
	if result.Applied < 2 {
		t.Errorf("Applied = %d, want >= 2", result.Applied)
	}
	foundCreate := false
	for _, c := range rec.calls {
		if c == "create-service:new-svc" {
			foundCreate = true
		}
	}
	if !foundCreate {
		t.Errorf("expected create-service call, got: %v", rec.calls)
	}
}

func TestApply_DeleteService(t *testing.T) {
	desired := &config.DesiredConfig{
		Services: []*config.DesiredService{
			{Name: "old-svc", ID: "svc-1", Delete: true},
		},
	}
	live := &config.LiveConfig{
		Services: map[string]*config.ServiceConfig{
			"old-svc": {ID: "svc-1", Name: "old-svc"},
		},
	}

	rec := &recordingApplier{}
	result, err := apply.Apply(context.Background(), desired, live, rec, apply.Options{AllowDelete: true})
	if err != nil {
		t.Fatalf("Apply() error: %v", err)
	}
	if result.Applied != 1 {
		t.Errorf("Applied = %d, want 1", result.Applied)
	}
	if len(rec.calls) != 1 || rec.calls[0] != "delete-service:svc-1" {
		t.Errorf("unexpected calls: %v", rec.calls)
	}
}

func TestApply_IconChange(t *testing.T) {
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
	rec := &recordingApplier{}
	result, err := apply.Apply(context.Background(), desired, live, rec, apply.Options{})
	if err != nil {
		t.Fatalf("Apply() error: %v", err)
	}
	if result.Applied != 1 {
		t.Errorf("Applied = %d, want 1", result.Applied)
	}
	if len(rec.calls) != 1 || rec.calls[0] != "icon:api:server" {
		t.Errorf("calls = %v, want [icon:api:server]", rec.calls)
	}
}

func TestApply_SubResources_Domain(t *testing.T) {
	port := 8080
	desired := &config.DesiredConfig{
		Services: []*config.DesiredService{
			{
				Name: "api",
				Domains: map[string]config.DomainConfig{
					"api.example.com": {Port: &port},
				},
			},
		},
	}
	live := &config.LiveConfig{
		Services: map[string]*config.ServiceConfig{
			"api": {ID: "svc-1", Name: "api"},
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
	found := false
	for _, c := range rec.calls {
		if c == "create-custom-domain:svc-1:api.example.com:8080" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected create-custom-domain call, got: %v", rec.calls)
	}
}

func TestApply_SubResources_TCPProxy(t *testing.T) {
	desired := &config.DesiredConfig{
		Services: []*config.DesiredService{
			{
				Name:       "api",
				TCPProxies: []int{8080},
			},
		},
	}
	live := &config.LiveConfig{
		Services: map[string]*config.ServiceConfig{
			"api": {ID: "svc-1", Name: "api"},
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
	found := false
	for _, c := range rec.calls {
		if c == "create-tcp:svc-1:8080" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected create-tcp call, got: %v", rec.calls)
	}
}

func TestApply_SubResources_Network(t *testing.T) {
	enabled := true
	desired := &config.DesiredConfig{
		Services: []*config.DesiredService{
			{
				Name:    "api",
				Network: &enabled,
			},
		},
	}
	live := &config.LiveConfig{
		Services: map[string]*config.ServiceConfig{
			"api": {ID: "svc-1", Name: "api"},
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
	found := false
	for _, c := range rec.calls {
		if c == "enable-network:svc-1" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected enable-network call, got: %v", rec.calls)
	}
}

func TestApply_SubResources_Egress(t *testing.T) {
	desired := &config.DesiredConfig{
		Services: []*config.DesiredService{
			{
				Name:   "api",
				Egress: []string{"us-west-2", "eu-west-1"},
			},
		},
	}
	live := &config.LiveConfig{
		Services: map[string]*config.ServiceConfig{
			"api": {ID: "svc-1", Name: "api"},
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
	found := false
	for _, c := range rec.calls {
		if c == "set-egress:svc-1:eu-west-1,us-west-2" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected set-egress call, got: %v", rec.calls)
	}
}

func TestApply_SubResources_VolumePassesRegion(t *testing.T) {
	desired := &config.DesiredConfig{
		Services: []*config.DesiredService{
			{
				Name: "api",
				Volumes: map[string]config.VolumeConfig{
					"data": {Mount: "/data", Region: "us-west1"},
				},
			},
		},
	}
	live := &config.LiveConfig{
		Services: map[string]*config.ServiceConfig{
			"api": {ID: "svc-1", Name: "api"},
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
	found := false
	for _, c := range rec.calls {
		if c == "create-volume:svc-1:/data:us-west1" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected region-aware create-volume call, got: %v", rec.calls)
	}
}

func TestApply_SubResources_Trigger(t *testing.T) {
	desired := &config.DesiredConfig{
		Services: []*config.DesiredService{
			{
				Name: "api",
				Triggers: []config.TriggerConfig{
					{Repository: "org/repo", Branch: "main", Provider: "gitlab"},
				},
			},
		},
	}
	live := &config.LiveConfig{
		Services: map[string]*config.ServiceConfig{
			"api": {ID: "svc-1", Name: "api"},
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
	found := false
	for _, c := range rec.calls {
		if c == "create-trigger:svc-1:org/repo:main:gitlab" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected create-trigger call, got: %v", rec.calls)
	}
}

func TestApply_DeleteVariable(t *testing.T) {
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

	rec := &recordingApplier{}
	// Deletes require AllowDelete to be set.
	result, err := apply.Apply(context.Background(), desired, live, rec, apply.Options{AllowDelete: true})
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
