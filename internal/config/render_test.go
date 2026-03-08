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
		Variables:     map[string]string{"SHARED": "1"},
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
	got, err := config.Render(sampleConfig(), config.RenderOptions{Format: "text"})
	if err != nil {
		t.Fatalf("Render() error: %v", err)
	}
	if !strings.Contains(got, "[variables]") {
		t.Fatalf("expected shared header, got: %s", got)
	}
	if !strings.Contains(got, "[[service]]") {
		t.Fatalf("expected service header, got: %s", got)
	}
	if !strings.Contains(got, "[service.variables]") {
		t.Fatalf("expected service variables header, got: %s", got)
	}
	if !strings.Contains(got, "PORT = 8080") {
		t.Fatalf("expected PORT value, got: %s", got)
	}
}

func TestRender_TextFullIncludesIDsAndDeploy(t *testing.T) {
	got, err := config.Render(sampleConfig(), config.RenderOptions{Format: "text", Full: true})
	if err != nil {
		t.Fatalf("Render() error: %v", err)
	}
	if !strings.Contains(got, "project_id:") {
		t.Errorf("expected project_id in full output, got:\n%s", got)
	}
	if !strings.Contains(got, "svc-1") {
		t.Errorf("expected service ID in full output, got:\n%s", got)
	}
	if !strings.Contains(got, "[service.deploy]") {
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
	got, err := config.Render(cfg, config.RenderOptions{Format: "text", Full: true})
	if err != nil {
		t.Fatalf("Render() error: %v", err)
	}
	if strings.Contains(got, "[service.deploy]") {
		t.Errorf("deploy section should be omitted when empty, got:\n%s", got)
	}
}

func TestRender_JSONIncludesVariables(t *testing.T) {
	got, err := config.Render(sampleConfig(), config.RenderOptions{Format: "json"})
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
	got, err := config.Render(sampleConfig(), config.RenderOptions{Format: "json", Full: true})
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
		Variables: map[string]string{"CONNECTION_INFO": `host="db" port=5432`},
	}
	got, err := config.Render(cfg, config.RenderOptions{Format: "toml"})
	if err != nil {
		t.Fatalf("Render() error: %v", err)
	}
	// TOML output should escape inner quotes.
	if !strings.Contains(got, `\"db\"`) {
		t.Errorf("expected escaped quotes in TOML, got:\n%s", got)
	}
}

func TestRender_TOMLFullIncludesIDs(t *testing.T) {
	got, err := config.Render(sampleConfig(), config.RenderOptions{Format: "toml", Full: true})
	if err != nil {
		t.Fatalf("Render() error: %v", err)
	}
	if !strings.Contains(got, `project_id = "proj-1"`) {
		t.Errorf("expected project_id in full TOML, got:\n%s", got)
	}
	if !strings.Contains(got, `[service.deploy]`) {
		t.Errorf("expected deploy section in full TOML, got:\n%s", got)
	}
}

func TestRender_UnsupportedFormat(t *testing.T) {
	_, err := config.Render(config.LiveConfig{}, config.RenderOptions{Format: "xml"})
	if err == nil {
		t.Fatal("expected error for unsupported format")
	}
}

func TestRender_EmptyConfig(t *testing.T) {
	got, err := config.Render(config.LiveConfig{}, config.RenderOptions{Format: "text"})
	if err != nil {
		t.Fatalf("Render() error: %v", err)
	}
	if strings.TrimSpace(got) != "" {
		t.Errorf("expected empty output for empty config, got: %q", got)
	}
}

func TestRender_MasksSecretsByDefault(t *testing.T) {
	cfg := config.LiveConfig{
		Variables: map[string]string{"DATABASE_PASSWORD": "hunter2"},
		Services: map[string]*config.ServiceConfig{
			"api": {Name: "api", Variables: map[string]string{
				"AUTH_TOKEN": "secret123",
				"APP_ENV":    "production",
			}},
		},
	}
	got, err := config.Render(cfg, config.RenderOptions{Format: "text"})
	if err != nil {
		t.Fatalf("Render() error: %v", err)
	}
	if !strings.Contains(got, "********") {
		t.Errorf("expected masked values, got:\n%s", got)
	}
	if !strings.Contains(got, "production") {
		t.Errorf("expected non-secret shown, got:\n%s", got)
	}
	if strings.Contains(got, "hunter2") {
		t.Errorf("password should be masked, got:\n%s", got)
	}
	if strings.Contains(got, "secret123") {
		t.Errorf("token should be masked, got:\n%s", got)
	}
}

func TestRender_ShowSecretsOverridesMasking(t *testing.T) {
	cfg := config.LiveConfig{
		Variables: map[string]string{"DATABASE_PASSWORD": "hunter2"},
	}
	got, err := config.Render(cfg, config.RenderOptions{
		Format:      "text",
		ShowSecrets: true,
	})
	if err != nil {
		t.Fatalf("Render() error: %v", err)
	}
	if !strings.Contains(got, "hunter2") {
		t.Errorf("--show-secrets should show password, got:\n%s", got)
	}
}

func TestRender_MaskingWorksInJSON(t *testing.T) {
	cfg := config.LiveConfig{
		Variables: map[string]string{"API_KEY": "fakekeyfakekeyfakekey"},
	}
	got, err := config.Render(cfg, config.RenderOptions{Format: "json"})
	if err != nil {
		t.Fatalf("Render() error: %v", err)
	}
	if strings.Contains(got, "fakekeyfakekeyfakekey") {
		t.Errorf("API key should be masked in JSON, got:\n%s", got)
	}
}

func TestRender_MaskingWorksInTOML(t *testing.T) {
	cfg := config.LiveConfig{
		Variables: map[string]string{"API_KEY": "fakekeyfakekeyfakekey"},
	}
	got, err := config.Render(cfg, config.RenderOptions{Format: "toml"})
	if err != nil {
		t.Fatalf("Render() error: %v", err)
	}
	if strings.Contains(got, "fakekeyfakekeyfakekey") {
		t.Errorf("API key should be masked in TOML, got:\n%s", got)
	}
}

func TestRender_ReferenceTemplateNotMasked(t *testing.T) {
	cfg := config.LiveConfig{
		Variables: map[string]string{
			"DATABASE_URL": "${{postgres.DATABASE_URL}}",
		},
	}
	got, err := config.Render(cfg, config.RenderOptions{Format: "text"})
	if err != nil {
		t.Fatalf("Render() error: %v", err)
	}
	if !strings.Contains(got, "${{postgres.DATABASE_URL}}") {
		t.Errorf("reference template should not be masked, got:\n%s", got)
	}
}

func TestRenderInitTOML_Header(t *testing.T) {
	cfg := config.LiveConfig{
		Services: map[string]*config.ServiceConfig{
			"api": {
				Name:      "api",
				Variables: map[string]string{"PORT": "8080"},
			},
		},
	}
	got := config.RenderInitTOML("", "my-app", "production", cfg)
	if !strings.Contains(got, "[project]") || !strings.Contains(got, `name = "my-app"`) {
		t.Errorf("expected project header:\n%s", got)
	}
	if !strings.Contains(got, `name = "production"`) {
		t.Errorf("expected environment name:\n%s", got)
	}
	if strings.Contains(got, `[workspace]`) {
		t.Errorf("did not expect workspace header when not provided:\n%s", got)
	}
	if !strings.Contains(got, "[service.variables]") {
		t.Errorf("expected service variables section:\n%s", got)
	}
	if !strings.Contains(got, `PORT = "8080"`) {
		t.Errorf("expected PORT variable:\n%s", got)
	}
}

func TestRenderInitTOML_MasksSecrets(t *testing.T) {
	cfg := config.LiveConfig{
		Services: map[string]*config.ServiceConfig{
			"api": {
				Name: "api",
				Variables: map[string]string{
					"PORT":              "8080",
					"DATABASE_PASSWORD": "hunter2",
				},
			},
		},
	}
	got := config.RenderInitTOML("", "proj", "env", cfg)
	// Secret value should not appear in output.
	if strings.Contains(got, "hunter2") {
		t.Errorf("secret value should not appear in output:\n%s", got)
	}
	// Secret should be rendered as ${VAR} reference, not "********".
	if !strings.Contains(got, `"${DATABASE_PASSWORD}"`) {
		t.Errorf("expected ${DATABASE_PASSWORD} env reference:\n%s", got)
	}
	if strings.Contains(got, "********") {
		t.Errorf("should not contain masked placeholder:\n%s", got)
	}
	// Non-secret should be literal.
	if !strings.Contains(got, `PORT = "8080"`) {
		t.Errorf("expected literal PORT value:\n%s", got)
	}
}

func TestRenderInitTOML_PreservesRailwayRefs(t *testing.T) {
	cfg := config.LiveConfig{
		Services: map[string]*config.ServiceConfig{
			"api": {
				Name: "api",
				Variables: map[string]string{
					"DATABASE_URL": "postgresql://${{postgres.PGUSER}}:${{postgres.PGPASSWORD}}@host:5432/db",
				},
			},
		},
	}
	got := config.RenderInitTOML("", "proj", "env", cfg)
	// Railway references should be preserved as-is, not turned into ${VAR}.
	if !strings.Contains(got, "${{postgres.PGUSER}}") {
		t.Errorf("expected Railway reference preserved:\n%s", got)
	}
	if strings.Contains(got, "${DATABASE_URL}") {
		t.Errorf("Railway ref variable should not become env ref:\n%s", got)
	}
}

func TestCollectSecrets(t *testing.T) {
	cfg := config.LiveConfig{
		Variables: map[string]string{
			"SHARED_KEY": "shared-secret",
			"APP_MODE":   "production",
		},
		Services: map[string]*config.ServiceConfig{
			"api": {
				Name: "api",
				Variables: map[string]string{
					"PORT":              "8080",
					"DATABASE_PASSWORD": "hunter2",
					"DATABASE_URL":      "postgresql://${{postgres.PGUSER}}:${{postgres.PGPASSWORD}}@host/db",
				},
			},
		},
	}
	secrets := config.CollectSecrets(cfg)

	// DATABASE_PASSWORD should be collected (sensitive name, literal value).
	if secrets["DATABASE_PASSWORD"] != "hunter2" {
		t.Errorf("DATABASE_PASSWORD = %q, want %q", secrets["DATABASE_PASSWORD"], "hunter2")
	}
	// SHARED_KEY should be collected (sensitive name).
	if secrets["SHARED_KEY"] != "shared-secret" {
		t.Errorf("SHARED_KEY = %q, want %q", secrets["SHARED_KEY"], "shared-secret")
	}
	// PORT should not be collected (not sensitive).
	if _, ok := secrets["PORT"]; ok {
		t.Error("PORT should not be in secrets")
	}
	// DATABASE_URL with Railway refs should not be collected.
	if _, ok := secrets["DATABASE_URL"]; ok {
		t.Error("DATABASE_URL with Railway refs should not be in secrets")
	}
	// APP_MODE should not be collected.
	if _, ok := secrets["APP_MODE"]; ok {
		t.Error("APP_MODE should not be in secrets")
	}
}

func TestRenderTOML_QuotesSpecialKeys(t *testing.T) {
	cfg := config.LiveConfig{
		Services: map[string]*config.ServiceConfig{
			"api": {
				Name: "api",
				Variables: map[string]string{
					"my.dotted.key":   "value1",
					"key with spaces": "value2",
					"NORMAL_KEY":      "value3",
				},
			},
		},
	}
	output, err := config.Render(cfg, config.RenderOptions{Format: "toml", ShowSecrets: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, `"my.dotted.key"`) {
		t.Errorf("dotted key should be quoted:\n%s", output)
	}
	if !strings.Contains(output, `"key with spaces"`) {
		t.Errorf("key with spaces should be quoted:\n%s", output)
	}
	if strings.Contains(output, `"NORMAL_KEY"`) {
		t.Errorf("normal key should not be quoted:\n%s", output)
	}
}

func TestRenderInitTOML_SharedVariables(t *testing.T) {
	cfg := config.LiveConfig{
		Variables: map[string]string{"GLOBAL": "value"},
		Services:  map[string]*config.ServiceConfig{},
	}
	got := config.RenderInitTOML("ws", "proj", "env", cfg)
	if !strings.Contains(got, `[workspace]`) || !strings.Contains(got, `name = "ws"`) {
		t.Errorf("expected workspace header:\n%s", got)
	}
	if !strings.Contains(got, "[variables]") {
		t.Errorf("expected variables section:\n%s", got)
	}
}

func TestRenderInitTOML_IncludesEnvFileWhenSecretsPresent(t *testing.T) {
	cfg := config.LiveConfig{
		Services: map[string]*config.ServiceConfig{
			"api": {
				Name: "api",
				Variables: map[string]string{
					"PORT":              "8080",
					"DATABASE_PASSWORD": "hunter2",
				},
			},
		},
	}
	got := config.RenderInitTOML("", "proj", "env", cfg)
	if !strings.Contains(got, "[tool]") {
		t.Errorf("expected [tool] section when secrets present:\n%s", got)
	}
	if !strings.Contains(got, `env_file = ".secrets.fat-controller"`) {
		t.Errorf("expected env_file setting:\n%s", got)
	}
}

func TestRenderInitTOML_OmitsToolWhenNoSecrets(t *testing.T) {
	cfg := config.LiveConfig{
		Services: map[string]*config.ServiceConfig{
			"api": {
				Name:      "api",
				Variables: map[string]string{"PORT": "8080", "NODE_ENV": "production"},
			},
		},
	}
	got := config.RenderInitTOML("", "proj", "env", cfg)
	if strings.Contains(got, "[tool]") {
		t.Errorf("should not include [tool] section when no secrets:\n%s", got)
	}
}

func TestRenderInitTOML_IncludesIDs(t *testing.T) {
	cfg := config.LiveConfig{
		ProjectID:     "proj-abc123",
		EnvironmentID: "env-xyz789",
		Services: map[string]*config.ServiceConfig{
			"api": {
				ID:        "svc-111",
				Name:      "api",
				Variables: map[string]string{"PORT": "8080"},
			},
			"worker": {
				ID:        "svc-222",
				Name:      "worker",
				Variables: map[string]string{"QUEUE": "default"},
			},
		},
	}
	got := config.RenderInitTOML("ws", "proj", "production", cfg)

	// Environment ID at top level.
	if !strings.Contains(got, `id = "env-xyz789"`) {
		t.Errorf("expected environment ID:\n%s", got)
	}
	// Project ID in [project] block.
	if !strings.Contains(got, `id = "proj-abc123"`) {
		t.Errorf("expected project ID:\n%s", got)
	}
	// Service IDs in [[service]] blocks.
	if !strings.Contains(got, `id = "svc-111"`) {
		t.Errorf("expected api service ID:\n%s", got)
	}
	if !strings.Contains(got, `id = "svc-222"`) {
		t.Errorf("expected worker service ID:\n%s", got)
	}
}
