package config_test

import (
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/config"
)

func TestDesiredConfig_ZeroValue(t *testing.T) {
	var cfg config.DesiredConfig
	if cfg.Name != "" {
		t.Errorf("Name = %q, want empty", cfg.Name)
	}
	if cfg.ID != "" {
		t.Errorf("ID = %q, want empty", cfg.ID)
	}
	if cfg.Variables != nil {
		t.Errorf("Variables = %v, want nil", cfg.Variables)
	}
	if cfg.Services != nil {
		t.Errorf("Services = %v, want nil", cfg.Services)
	}
	if cfg.Tool != nil {
		t.Errorf("Tool = %v, want nil", cfg.Tool)
	}
	if cfg.Workspace != nil {
		t.Errorf("Workspace = %v, want nil", cfg.Workspace)
	}
	if cfg.Project != nil {
		t.Errorf("Project = %v, want nil", cfg.Project)
	}
}

func TestDesiredService_ZeroValue(t *testing.T) {
	var svc config.DesiredService
	if svc.Name != "" {
		t.Errorf("Name = %q, want empty", svc.Name)
	}
	if svc.ID != "" {
		t.Errorf("ID = %q, want empty", svc.ID)
	}
	if svc.Variables != nil {
		t.Errorf("Variables = %v, want nil", svc.Variables)
	}
	if svc.Deploy != nil {
		t.Errorf("Deploy = %v, want nil", svc.Deploy)
	}
	if svc.Resources != nil {
		t.Errorf("Resources = %v, want nil", svc.Resources)
	}
	if svc.Domains != nil {
		t.Errorf("Domains = %v, want nil", svc.Domains)
	}
	if svc.Volumes != nil {
		t.Errorf("Volumes = %v, want nil", svc.Volumes)
	}
}

func TestContextBlock_ZeroValue(t *testing.T) {
	var ctx config.ContextBlock
	if ctx.Name != "" {
		t.Errorf("Name = %q, want empty", ctx.Name)
	}
	if ctx.ID != "" {
		t.Errorf("ID = %q, want empty", ctx.ID)
	}
}

func TestToolSettings_ZeroValue(t *testing.T) {
	var tool config.ToolSettings
	if tool.APITimeout != "" {
		t.Errorf("APITimeout = %q, want empty", tool.APITimeout)
	}
	if tool.Prompt != "" {
		t.Errorf("Prompt = %q, want empty", tool.Prompt)
	}
	if tool.AllowCreate != nil {
		t.Errorf("AllowCreate = %v, want nil", tool.AllowCreate)
	}
}
