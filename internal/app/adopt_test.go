package app_test

import (
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/app"
	"github.com/hamishmorgan/fat-controller/internal/config"
)

func TestAdoptMerge_CreateNewService(t *testing.T) {
	desired := &config.DesiredConfig{
		Name: "production",
		Services: []*config.DesiredService{
			{Name: "api", Variables: config.Variables{"PORT": "3000"}},
		},
	}
	live := &config.LiveConfig{
		Variables: map[string]string{"GLOBAL": "val"},
		Services: map[string]*config.ServiceConfig{
			"api":    {Name: "api", Variables: map[string]string{"PORT": "3000"}},
			"worker": {Name: "worker", Variables: map[string]string{"QUEUE": "default"}},
		},
	}

	result := app.AdoptMerge(desired, live, true, true, false, "")
	if _, ok := result.Services["worker"]; !ok {
		t.Error("expected worker service to be created")
	}
	if _, ok := result.Services["api"]; !ok {
		t.Error("expected api service to be preserved")
	}
}

func TestAdoptMerge_NoCreate(t *testing.T) {
	desired := &config.DesiredConfig{
		Name: "production",
		Services: []*config.DesiredService{
			{Name: "api"},
		},
	}
	live := &config.LiveConfig{
		Variables: map[string]string{},
		Services: map[string]*config.ServiceConfig{
			"api":    {Name: "api"},
			"worker": {Name: "worker"},
		},
	}

	result := app.AdoptMerge(desired, live, false, true, false, "")
	if _, ok := result.Services["worker"]; ok {
		t.Error("expected worker service NOT to be created when create=false")
	}
	if _, ok := result.Services["api"]; !ok {
		t.Error("expected api service to be preserved")
	}
}

func TestAdoptMerge_Delete(t *testing.T) {
	desired := &config.DesiredConfig{
		Name:      "production",
		Variables: config.Variables{"STALE": "old"},
		Services: []*config.DesiredService{
			{Name: "api"},
			{Name: "old-service"},
		},
	}
	live := &config.LiveConfig{
		Variables: map[string]string{"FRESH": "new"},
		Services: map[string]*config.ServiceConfig{
			"api": {Name: "api"},
		},
	}

	result := app.AdoptMerge(desired, live, true, true, true, "")
	if _, ok := result.Services["old-service"]; ok {
		t.Error("expected old-service to be deleted")
	}
	if v, ok := result.Variables["STALE"]; ok {
		t.Errorf("expected STALE variable to be deleted, got %q", v)
	}
	if _, ok := result.Variables["FRESH"]; !ok {
		t.Error("expected FRESH variable to be created")
	}
}

func TestAdoptMerge_NoDelete(t *testing.T) {
	desired := &config.DesiredConfig{
		Name:      "production",
		Variables: config.Variables{"STALE": "old"},
		Services: []*config.DesiredService{
			{Name: "old-service"},
		},
	}
	live := &config.LiveConfig{
		Variables: map[string]string{},
		Services:  map[string]*config.ServiceConfig{},
	}

	result := app.AdoptMerge(desired, live, true, true, false, "")
	if _, ok := result.Services["old-service"]; !ok {
		t.Error("expected old-service to be preserved when delete=false")
	}
	if _, ok := result.Variables["STALE"]; !ok {
		t.Error("expected STALE variable to be preserved when delete=false")
	}
}

func TestAdoptMerge_UpdateExisting(t *testing.T) {
	desired := &config.DesiredConfig{
		Name:      "production",
		Variables: config.Variables{"PORT": "3000"},
		Services: []*config.DesiredService{
			{Name: "api", Variables: config.Variables{"PORT": "3000"}},
		},
	}
	live := &config.LiveConfig{
		Variables: map[string]string{"PORT": "8080"},
		Services: map[string]*config.ServiceConfig{
			"api": {Name: "api", Variables: map[string]string{"PORT": "8080"}},
		},
	}

	result := app.AdoptMerge(desired, live, true, true, false, "")
	if result.Variables["PORT"] != "8080" {
		t.Errorf("expected PORT to be updated to 8080, got %q", result.Variables["PORT"])
	}
	if result.Services["api"].Variables["PORT"] != "8080" {
		t.Errorf("expected api PORT to be updated to 8080, got %q", result.Services["api"].Variables["PORT"])
	}
}

func TestAdoptMerge_NoUpdate(t *testing.T) {
	desired := &config.DesiredConfig{
		Name:      "production",
		Variables: config.Variables{"PORT": "3000"},
		Services: []*config.DesiredService{
			{Name: "api", Variables: config.Variables{"PORT": "3000"}},
		},
	}
	live := &config.LiveConfig{
		Variables: map[string]string{"PORT": "8080"},
		Services: map[string]*config.ServiceConfig{
			"api": {Name: "api", Variables: map[string]string{"PORT": "8080"}},
		},
	}

	result := app.AdoptMerge(desired, live, true, false, false, "")
	if result.Variables["PORT"] != "3000" {
		t.Errorf("expected PORT to be preserved as 3000, got %q", result.Variables["PORT"])
	}
}

func TestAdoptMerge_PathScoping(t *testing.T) {
	desired := &config.DesiredConfig{
		Name:      "production",
		Variables: config.Variables{"GLOBAL": "val"},
		Services: []*config.DesiredService{
			{Name: "api", Variables: config.Variables{"PORT": "3000"}},
			{Name: "worker", Variables: config.Variables{"QUEUE": "default"}},
		},
	}
	live := &config.LiveConfig{
		Variables: map[string]string{"GLOBAL": "val"},
		Services: map[string]*config.ServiceConfig{
			"api": {Name: "api", Variables: map[string]string{"PORT": "8080"}},
		},
	}

	// Scope to "api" — only api should be affected by merge flags.
	scopedLive := app.ScopeLiveByPath(live, "api")
	result := app.AdoptMerge(desired, scopedLive, true, true, true, "api")

	// api should be updated from live.
	if _, ok := result.Services["api"]; !ok {
		t.Error("expected api service in result")
	}
	// worker is out of scope — should be preserved even with delete=true.
	if _, ok := result.Services["worker"]; !ok {
		t.Error("expected worker service to be preserved (out of scope)")
	}
	// Variables should be preserved (out of scope for service path).
	if result.Variables["GLOBAL"] != "val" {
		t.Errorf("expected GLOBAL variable to be preserved, got %q", result.Variables["GLOBAL"])
	}
}

func TestAdoptMerge_VariablesPathScope(t *testing.T) {
	desired := &config.DesiredConfig{
		Name:      "production",
		Variables: config.Variables{"OLD": "val"},
		Services: []*config.DesiredService{
			{Name: "api"},
		},
	}
	live := &config.LiveConfig{
		Variables: map[string]string{"NEW": "live"},
		Services: map[string]*config.ServiceConfig{
			"api": {Name: "api"},
		},
	}

	scopedLive := app.ScopeLiveByPath(live, "variables")
	result := app.AdoptMerge(desired, scopedLive, true, true, true, "variables")

	// Variables in scope: OLD should be deleted, NEW should be created.
	if _, ok := result.Variables["OLD"]; ok {
		t.Error("expected OLD variable to be deleted (in scope, delete=true)")
	}
	if result.Variables["NEW"] != "live" {
		t.Errorf("expected NEW variable, got %q", result.Variables["NEW"])
	}
	// Services out of scope — should be preserved.
	if _, ok := result.Services["api"]; !ok {
		t.Error("expected api service to be preserved (out of scope)")
	}
}

func TestScopeLiveByPath(t *testing.T) {
	live := &config.LiveConfig{
		Variables: map[string]string{"A": "1"},
		Services: map[string]*config.ServiceConfig{
			"api":    {Name: "api"},
			"worker": {Name: "worker"},
		},
	}

	t.Run("empty path returns full config", func(t *testing.T) {
		result := app.ScopeLiveByPath(live, "")
		if len(result.Services) != 2 {
			t.Errorf("expected 2 services, got %d", len(result.Services))
		}
	})

	t.Run("variables path returns only variables", func(t *testing.T) {
		result := app.ScopeLiveByPath(live, "variables")
		if len(result.Services) != 0 {
			t.Errorf("expected 0 services, got %d", len(result.Services))
		}
		if result.Variables["A"] != "1" {
			t.Error("expected variables to be preserved")
		}
	})

	t.Run("service name returns only that service", func(t *testing.T) {
		result := app.ScopeLiveByPath(live, "api")
		if len(result.Services) != 1 {
			t.Errorf("expected 1 service, got %d", len(result.Services))
		}
		if _, ok := result.Services["api"]; !ok {
			t.Error("expected api service")
		}
		if result.Variables != nil {
			t.Error("expected nil variables when scoped to service")
		}
	})

	t.Run("unknown path returns empty", func(t *testing.T) {
		result := app.ScopeLiveByPath(live, "nonexistent")
		if len(result.Services) != 0 {
			t.Errorf("expected 0 services, got %d", len(result.Services))
		}
	})
}

func TestDesiredServiceToLive(t *testing.T) {
	ds := &config.DesiredService{
		Name:      "api",
		ID:        "svc-123",
		Variables: config.Variables{"PORT": "3000"},
		Deploy: &config.DesiredDeploy{
			Image: strPtr("node:18"),
		},
	}
	liveSvc := &config.ServiceConfig{
		ID:   "svc-123",
		Name: "api",
		Domains: []config.LiveDomain{
			{ID: "dom-1", Domain: "api.example.com"},
		},
	}

	result := app.DesiredServiceToLive(ds, liveSvc)
	if result.Name != "api" {
		t.Errorf("expected name api, got %s", result.Name)
	}
	if result.ID != "svc-123" {
		t.Errorf("expected ID svc-123, got %s", result.ID)
	}
	if result.Variables["PORT"] != "3000" {
		t.Errorf("expected PORT 3000, got %s", result.Variables["PORT"])
	}
	if result.Deploy.Image == nil || *result.Deploy.Image != "node:18" {
		t.Error("expected deploy image node:18")
	}
	if len(result.Domains) != 1 {
		t.Errorf("expected 1 domain from live, got %d", len(result.Domains))
	}
}

func TestDesiredServiceToLive_NilLive(t *testing.T) {
	ds := &config.DesiredService{
		Name:      "api",
		ID:        "svc-123",
		Variables: config.Variables{"PORT": "3000"},
	}

	result := app.DesiredServiceToLive(ds, nil)
	if result.ID != "svc-123" {
		t.Errorf("expected ID svc-123, got %s", result.ID)
	}
	if len(result.Domains) != 0 {
		t.Errorf("expected 0 domains, got %d", len(result.Domains))
	}
}

func strPtr(s string) *string { return &s }
