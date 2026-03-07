package config_test

import (
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/config"
)

func TestMerge_EmptySlice(t *testing.T) {
	result := config.Merge()
	if result.Variables != nil {
		t.Error("expected nil variables from empty merge")
	}
	if len(result.Services) != 0 {
		t.Error("expected no services from empty merge")
	}
}

func TestMerge_Single(t *testing.T) {
	cfg := &config.DesiredConfig{
		Variables: config.Variables{"A": "1"},
		Services: []*config.DesiredService{
			{Name: "api", Variables: config.Variables{"PORT": "8080"}},
		},
	}
	result := config.Merge(cfg)
	if result.Variables == nil || result.Variables["A"] != "1" {
		t.Error("expected variables A=1")
	}
	if result.Services[0].Variables["PORT"] != "8080" {
		t.Error("expected api PORT=8080")
	}
}

func TestMerge_LaterOverridesEarlier(t *testing.T) {
	base := &config.DesiredConfig{
		Variables: config.Variables{
			"KEEP":     "base",
			"OVERRIDE": "base",
		},
		Services: []*config.DesiredService{
			{Name: "api", Variables: config.Variables{
				"PORT":    "8080",
				"APP_ENV": "staging",
			}},
		},
	}
	local := &config.DesiredConfig{
		Variables: config.Variables{
			"OVERRIDE": "local",
			"NEW":      "local",
		},
		Services: []*config.DesiredService{
			{Name: "api", Variables: config.Variables{
				"APP_ENV": "production",
			}},
			{Name: "worker", Variables: config.Variables{
				"QUEUE": "default",
			}},
		},
	}
	result := config.Merge(base, local)

	// Variables: KEEP preserved, OVERRIDE overridden, NEW added
	if result.Variables["KEEP"] != "base" {
		t.Errorf("KEEP = %q, want base", result.Variables["KEEP"])
	}
	if result.Variables["OVERRIDE"] != "local" {
		t.Errorf("OVERRIDE = %q, want local", result.Variables["OVERRIDE"])
	}
	if result.Variables["NEW"] != "local" {
		t.Errorf("NEW = %q, want local", result.Variables["NEW"])
	}

	// Service api: PORT preserved from base, APP_ENV overridden
	if result.Services[0].Variables["PORT"] != "8080" {
		t.Errorf("api PORT = %q, want 8080", result.Services[0].Variables["PORT"])
	}
	if result.Services[0].Variables["APP_ENV"] != "production" {
		t.Errorf("api APP_ENV = %q, want production", result.Services[0].Variables["APP_ENV"])
	}

	// Service worker: added from local
	var worker *config.DesiredService
	for _, svc := range result.Services {
		if svc.Name == "worker" {
			worker = svc
		}
	}
	if worker == nil {
		t.Fatal("expected worker service from local")
	}
	if worker.Variables["QUEUE"] != "default" {
		t.Error("expected worker QUEUE=default")
	}
}

func TestMerge_ResourcesAndDeployOverride(t *testing.T) {
	vcpus2 := 2.0
	vcpus4 := 4.0
	mem4 := 4.0
	builder := "NIXPACKS"

	base := &config.DesiredConfig{
		Services: []*config.DesiredService{
			{
				Name:      "api",
				Resources: &config.DesiredResources{VCPUs: &vcpus2, MemoryGB: &mem4},
				Deploy:    &config.DesiredDeploy{Builder: &builder},
			},
		},
	}
	override := &config.DesiredConfig{
		Services: []*config.DesiredService{
			{
				Name:      "api",
				Resources: &config.DesiredResources{VCPUs: &vcpus4},
			},
		},
	}
	result := config.Merge(base, override)
	svc := result.Services[0]

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

func TestMerge_VariablesNilInBaseNonNilInOverride(t *testing.T) {
	base := &config.DesiredConfig{Services: []*config.DesiredService{}}
	local := &config.DesiredConfig{
		Variables: config.Variables{"X": "1"},
		Services:  []*config.DesiredService{},
	}
	result := config.Merge(base, local)
	if result.Variables == nil || result.Variables["X"] != "1" {
		t.Error("expected variables X=1 from override")
	}
}

func TestMerge_NameOverride(t *testing.T) {
	base := &config.DesiredConfig{
		Name:     "production",
		Services: []*config.DesiredService{},
	}
	// Local override sets name.
	local := &config.DesiredConfig{
		Name:     "staging",
		Services: []*config.DesiredService{},
	}
	result := config.Merge(base, local)
	if result.Name != "staging" {
		t.Errorf("Name = %q, want %q (overridden by local)", result.Name, "staging")
	}
}

func TestMerge_ToolSettings(t *testing.T) {
	base := &config.DesiredConfig{Tool: &config.ToolSettings{SensitiveKeywords: []string{"SECRET"}}}
	overlay := &config.DesiredConfig{Tool: &config.ToolSettings{SensitiveKeywords: []string{"TOKEN", "KEY"}}}
	result := config.Merge(base, overlay)
	if result.Tool == nil || len(result.Tool.SensitiveKeywords) != 2 || result.Tool.SensitiveKeywords[0] != "TOKEN" {
		t.Errorf("expected overlay tool settings to win")
	}
}

func TestMerge_NameEmpty_DoesNotOverride(t *testing.T) {
	base := &config.DesiredConfig{
		Name:     "production",
		Services: []*config.DesiredService{},
	}
	// Overlay with empty name should not wipe base values.
	overlay := &config.DesiredConfig{
		Services: []*config.DesiredService{
			{Name: "api", Variables: config.Variables{"PORT": "9090"}},
		},
	}
	result := config.Merge(base, overlay)
	if result.Name != "production" {
		t.Errorf("Name = %q, want %q (empty should not override)", result.Name, "production")
	}
}
