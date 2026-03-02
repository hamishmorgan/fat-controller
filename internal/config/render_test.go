package config_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/config"
)

func sampleConfig() config.LiveConfig {
	startCmd := "npm start"
	return config.LiveConfig{
		ProjectID:     "proj-1",
		EnvironmentID: "env-1",
		Shared:        map[string]string{"SHARED": "1"},
		Services: map[string]*config.ServiceConfig{
			"api": {
				ID:        "svc-1",
				Name:      "api",
				Variables: map[string]string{"PORT": "8080"},
				Deploy: config.Deploy{
					Builder:      "NIXPACKS",
					StartCommand: &startCmd,
				},
			},
		},
	}
}

func TestRender_TextIncludesServiceAndKey(t *testing.T) {
	got, err := config.Render(sampleConfig(), "text", false)
	if err != nil {
		t.Fatalf("Render() error: %v", err)
	}
	if !strings.Contains(got, "[shared_variables]") {
		t.Fatalf("expected shared header, got: %s", got)
	}
	if !strings.Contains(got, "[api.variables]") {
		t.Fatalf("expected service variables header, got: %s", got)
	}
	if !strings.Contains(got, "PORT = 8080") {
		t.Fatalf("expected PORT value, got: %s", got)
	}
}

func TestRender_TextFullIncludesIDsAndDeploy(t *testing.T) {
	got, err := config.Render(sampleConfig(), "text", true)
	if err != nil {
		t.Fatalf("Render() error: %v", err)
	}
	if !strings.Contains(got, "project_id:") {
		t.Errorf("expected project_id in full output, got:\n%s", got)
	}
	if !strings.Contains(got, "svc-1") {
		t.Errorf("expected service ID in full output, got:\n%s", got)
	}
	if !strings.Contains(got, "[api.deploy]") {
		t.Errorf("expected deploy section in full output, got:\n%s", got)
	}
	if !strings.Contains(got, "NIXPACKS") {
		t.Errorf("expected builder in full output, got:\n%s", got)
	}
}

func TestRender_TextFullOmitsDeployWhenEmpty(t *testing.T) {
	cfg := config.LiveConfig{
		Services: map[string]*config.ServiceConfig{
			"api": {Name: "api", Variables: map[string]string{"X": "1"}},
		},
	}
	got, err := config.Render(cfg, "text", true)
	if err != nil {
		t.Fatalf("Render() error: %v", err)
	}
	if strings.Contains(got, "[api.deploy]") {
		t.Errorf("deploy section should be omitted when empty, got:\n%s", got)
	}
}

func TestRender_JSONIncludesVariables(t *testing.T) {
	got, err := config.Render(sampleConfig(), "json", false)
	if err != nil {
		t.Fatalf("Render() error: %v", err)
	}
	if !strings.Contains(got, `"PORT"`) {
		t.Errorf("expected PORT in JSON, got:\n%s", got)
	}
	// Non-full mode should not include IDs.
	if strings.Contains(got, `"project_id"`) {
		t.Errorf("project_id should not be in non-full JSON, got:\n%s", got)
	}
}

func TestRender_JSONFullIncludesIDsAndDeploy(t *testing.T) {
	got, err := config.Render(sampleConfig(), "json", true)
	if err != nil {
		t.Fatalf("Render() error: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(got), &m); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if _, ok := m["project_id"]; !ok {
		t.Error("expected project_id in full JSON")
	}
	api, ok := m["api"].(map[string]any)
	if !ok {
		t.Fatal("expected api object in JSON")
	}
	if _, ok := api["id"]; !ok {
		t.Error("expected service id in full JSON")
	}
	if _, ok := api["deploy"]; !ok {
		t.Error("expected deploy in full JSON")
	}
}

func TestRender_TOMLQuotesValues(t *testing.T) {
	cfg := config.LiveConfig{
		Shared: map[string]string{"DSN": `host="db" port=5432`},
	}
	got, err := config.Render(cfg, "toml", false)
	if err != nil {
		t.Fatalf("Render() error: %v", err)
	}
	// TOML output should escape inner quotes.
	if !strings.Contains(got, `\"db\"`) {
		t.Errorf("expected escaped quotes in TOML, got:\n%s", got)
	}
}

func TestRender_TOMLFullIncludesIDs(t *testing.T) {
	got, err := config.Render(sampleConfig(), "toml", true)
	if err != nil {
		t.Fatalf("Render() error: %v", err)
	}
	if !strings.Contains(got, `project_id = "proj-1"`) {
		t.Errorf("expected project_id in full TOML, got:\n%s", got)
	}
	if !strings.Contains(got, `[api.deploy]`) {
		t.Errorf("expected deploy section in full TOML, got:\n%s", got)
	}
}

func TestRender_UnsupportedFormat(t *testing.T) {
	_, err := config.Render(config.LiveConfig{}, "xml", false)
	if err == nil {
		t.Fatal("expected error for unsupported format")
	}
}

func TestRender_EmptyConfig(t *testing.T) {
	got, err := config.Render(config.LiveConfig{}, "text", false)
	if err != nil {
		t.Fatalf("Render() error: %v", err)
	}
	if strings.TrimSpace(got) != "" {
		t.Errorf("expected empty output for empty config, got: %q", got)
	}
}
