package config_test

import (
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/config"
)

func TestMerge_EmptySlice(t *testing.T) {
	result := config.Merge()
	if result.Shared != nil {
		t.Error("expected nil shared from empty merge")
	}
	if len(result.Services) != 0 {
		t.Error("expected no services from empty merge")
	}
}

func TestMerge_Single(t *testing.T) {
	cfg := &config.DesiredConfig{
		Shared: &config.DesiredVariables{Vars: map[string]string{"A": "1"}},
		Services: map[string]*config.DesiredService{
			"api": {Variables: map[string]string{"PORT": "8080"}},
		},
	}
	result := config.Merge(cfg)
	if result.Shared == nil || result.Shared.Vars["A"] != "1" {
		t.Error("expected shared A=1")
	}
	if result.Services["api"].Variables["PORT"] != "8080" {
		t.Error("expected api PORT=8080")
	}
}

func TestMerge_LaterOverridesEarlier(t *testing.T) {
	base := &config.DesiredConfig{
		Shared: &config.DesiredVariables{Vars: map[string]string{
			"KEEP":     "base",
			"OVERRIDE": "base",
		}},
		Services: map[string]*config.DesiredService{
			"api": {Variables: map[string]string{
				"PORT":    "8080",
				"APP_ENV": "staging",
			}},
		},
	}
	local := &config.DesiredConfig{
		Shared: &config.DesiredVariables{Vars: map[string]string{
			"OVERRIDE": "local",
			"NEW":      "local",
		}},
		Services: map[string]*config.DesiredService{
			"api": {Variables: map[string]string{
				"APP_ENV": "production",
			}},
			"worker": {Variables: map[string]string{
				"QUEUE": "default",
			}},
		},
	}
	result := config.Merge(base, local)

	// Shared: KEEP preserved, OVERRIDE overridden, NEW added
	if result.Shared.Vars["KEEP"] != "base" {
		t.Errorf("KEEP = %q, want base", result.Shared.Vars["KEEP"])
	}
	if result.Shared.Vars["OVERRIDE"] != "local" {
		t.Errorf("OVERRIDE = %q, want local", result.Shared.Vars["OVERRIDE"])
	}
	if result.Shared.Vars["NEW"] != "local" {
		t.Errorf("NEW = %q, want local", result.Shared.Vars["NEW"])
	}

	// Service api: PORT preserved from base, APP_ENV overridden
	if result.Services["api"].Variables["PORT"] != "8080" {
		t.Errorf("api PORT = %q, want 8080", result.Services["api"].Variables["PORT"])
	}
	if result.Services["api"].Variables["APP_ENV"] != "production" {
		t.Errorf("api APP_ENV = %q, want production", result.Services["api"].Variables["APP_ENV"])
	}

	// Service worker: added from local
	if result.Services["worker"] == nil {
		t.Fatal("expected worker service from local")
	}
	if result.Services["worker"].Variables["QUEUE"] != "default" {
		t.Error("expected worker QUEUE=default")
	}
}

func TestMerge_ResourcesAndDeployOverride(t *testing.T) {
	vcpus2 := 2.0
	vcpus4 := 4.0
	mem4 := 4.0
	builder := "NIXPACKS"

	base := &config.DesiredConfig{
		Services: map[string]*config.DesiredService{
			"api": {
				Resources: &config.DesiredResources{VCPUs: &vcpus2, MemoryGB: &mem4},
				Deploy:    &config.DesiredDeploy{Builder: &builder},
			},
		},
	}
	override := &config.DesiredConfig{
		Services: map[string]*config.DesiredService{
			"api": {
				Resources: &config.DesiredResources{VCPUs: &vcpus4},
			},
		},
	}
	result := config.Merge(base, override)
	svc := result.Services["api"]

	if svc.Resources == nil || svc.Resources.VCPUs == nil || *svc.Resources.VCPUs != 4.0 {
		t.Error("expected VCPUs overridden to 4")
	}
	if svc.Resources.MemoryGB == nil || *svc.Resources.MemoryGB != 4.0 {
		t.Error("expected MemoryGB preserved from base")
	}
	if svc.Deploy == nil || svc.Deploy.Builder == nil || *svc.Deploy.Builder != "NIXPACKS" {
		t.Error("expected Deploy.Builder preserved from base")
	}
}

func TestMerge_SharedNilInBaseNonNilInOverride(t *testing.T) {
	base := &config.DesiredConfig{Services: map[string]*config.DesiredService{}}
	local := &config.DesiredConfig{
		Shared:   &config.DesiredVariables{Vars: map[string]string{"X": "1"}},
		Services: map[string]*config.DesiredService{},
	}
	result := config.Merge(base, local)
	if result.Shared == nil || result.Shared.Vars["X"] != "1" {
		t.Error("expected shared X=1 from override")
	}
}
