package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/config"
)

func TestParseFile_SharedVariables(t *testing.T) {
	content := `
[shared.variables]
SHARED_KEY = "shared_value"
OTHER = "other"
`
	path := writeTempTOML(t, content)
	cfg, err := config.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile() error: %v", err)
	}
	if cfg.Shared == nil {
		t.Fatal("expected Shared to be non-nil")
	}
	if cfg.Shared.Vars["SHARED_KEY"] != "shared_value" {
		t.Errorf("SHARED_KEY = %q, want %q", cfg.Shared.Vars["SHARED_KEY"], "shared_value")
	}
	if cfg.Shared.Vars["OTHER"] != "other" {
		t.Errorf("OTHER = %q, want %q", cfg.Shared.Vars["OTHER"], "other")
	}
}

func TestParseFile_ServiceVariables(t *testing.T) {
	content := `
[api.variables]
PORT = "8080"
APP_ENV = "production"
`
	path := writeTempTOML(t, content)
	cfg, err := config.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile() error: %v", err)
	}
	svc, ok := cfg.Services["api"]
	if !ok {
		t.Fatal("expected api service")
	}
	if svc.Variables["PORT"] != "8080" {
		t.Errorf("PORT = %q, want %q", svc.Variables["PORT"], "8080")
	}
	if svc.Variables["APP_ENV"] != "production" {
		t.Errorf("APP_ENV = %q, want %q", svc.Variables["APP_ENV"], "production")
	}
}

func TestParseFile_EmptyStringIsDelete(t *testing.T) {
	content := `
[api.variables]
OLD_VAR = ""
`
	path := writeTempTOML(t, content)
	cfg, err := config.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile() error: %v", err)
	}
	val, ok := cfg.Services["api"].Variables["OLD_VAR"]
	if !ok {
		t.Fatal("expected OLD_VAR to be present")
	}
	if val != "" {
		t.Errorf("OLD_VAR = %q, want empty string", val)
	}
}

func TestParseFile_ServiceResources(t *testing.T) {
	content := `
[api.resources]
vcpus = 2
memory_gb = 4.0
`
	path := writeTempTOML(t, content)
	cfg, err := config.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile() error: %v", err)
	}
	svc := cfg.Services["api"]
	if svc.Resources == nil {
		t.Fatal("expected Resources to be non-nil")
	}
	if svc.Resources.VCPUs == nil || *svc.Resources.VCPUs != 2 {
		t.Errorf("VCPUs = %v, want 2", svc.Resources.VCPUs)
	}
	if svc.Resources.MemoryGB == nil || *svc.Resources.MemoryGB != 4.0 {
		t.Errorf("MemoryGB = %v, want 4.0", svc.Resources.MemoryGB)
	}
}

func TestParseFile_ServiceDeploy(t *testing.T) {
	content := `
[api.deploy]
builder = "NIXPACKS"
start_command = "npm start"
`
	path := writeTempTOML(t, content)
	cfg, err := config.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile() error: %v", err)
	}
	svc := cfg.Services["api"]
	if svc.Deploy == nil {
		t.Fatal("expected Deploy to be non-nil")
	}
	if svc.Deploy.Builder == nil || *svc.Deploy.Builder != "NIXPACKS" {
		t.Errorf("Builder = %v, want NIXPACKS", svc.Deploy.Builder)
	}
	if svc.Deploy.StartCommand == nil || *svc.Deploy.StartCommand != "npm start" {
		t.Errorf("StartCommand = %v, want 'npm start'", svc.Deploy.StartCommand)
	}
	// Unspecified fields should be nil
	if svc.Deploy.DockerfilePath != nil {
		t.Error("DockerfilePath should be nil when not specified")
	}
}

func TestParseFile_MultipleServices(t *testing.T) {
	content := `
[shared.variables]
COMMON = "yes"

[api.variables]
PORT = "8080"

[worker.variables]
QUEUE = "default"

[worker.resources]
vcpus = 1
memory_gb = 2
`
	path := writeTempTOML(t, content)
	cfg, err := config.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile() error: %v", err)
	}
	if cfg.Shared == nil || cfg.Shared.Vars["COMMON"] != "yes" {
		t.Error("expected shared COMMON=yes")
	}
	if len(cfg.Services) != 2 {
		t.Errorf("expected 2 services, got %d", len(cfg.Services))
	}
	if cfg.Services["api"].Variables["PORT"] != "8080" {
		t.Error("expected api PORT=8080")
	}
	if cfg.Services["worker"].Variables["QUEUE"] != "default" {
		t.Error("expected worker QUEUE=default")
	}
	if cfg.Services["worker"].Resources == nil {
		t.Error("expected worker resources")
	}
}

func TestParseFile_EmptyFile(t *testing.T) {
	path := writeTempTOML(t, "")
	cfg, err := config.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile() error: %v", err)
	}
	if cfg.Shared != nil {
		t.Error("expected nil Shared for empty config")
	}
	if len(cfg.Services) != 0 {
		t.Errorf("expected 0 services, got %d", len(cfg.Services))
	}
}

func TestParseFile_NonexistentFile(t *testing.T) {
	_, err := config.ParseFile("/tmp/does-not-exist-fc-test.toml")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestParseFile_NonStringVariableValue(t *testing.T) {
	// TOML allows integers without quotes — we should coerce to string.
	content := `
[api.variables]
PORT = 8080
ENABLED = true
`
	path := writeTempTOML(t, content)
	cfg, err := config.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile() error: %v", err)
	}
	if cfg.Services["api"].Variables["PORT"] != "8080" {
		t.Errorf("PORT = %q, want %q", cfg.Services["api"].Variables["PORT"], "8080")
	}
	if cfg.Services["api"].Variables["ENABLED"] != "true" {
		t.Errorf("ENABLED = %q, want %q", cfg.Services["api"].Variables["ENABLED"], "true")
	}
}

func TestParseFile_InterpolationSyntaxPreserved(t *testing.T) {
	content := `
[api.variables]
DATABASE_URL = "${{postgres.DATABASE_URL}}"
STRIPE_KEY = "${STRIPE_KEY}"
`
	path := writeTempTOML(t, content)
	cfg, err := config.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile() error: %v", err)
	}
	vars := cfg.Services["api"].Variables
	if vars["DATABASE_URL"] != "${{postgres.DATABASE_URL}}" {
		t.Errorf("Railway ref not preserved: %q", vars["DATABASE_URL"])
	}
	if vars["STRIPE_KEY"] != "${STRIPE_KEY}" {
		t.Errorf("Local env ref not preserved: %q", vars["STRIPE_KEY"])
	}
}

func TestParseFile_ProjectAndEnvironment(t *testing.T) {
	content := `
project = "my-app"
environment = "production"

[api.variables]
PORT = "8080"
`
	path := writeTempTOML(t, content)
	cfg, err := config.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile() error: %v", err)
	}
	if cfg.Project != "my-app" {
		t.Errorf("Project = %q, want %q", cfg.Project, "my-app")
	}
	if cfg.Environment != "production" {
		t.Errorf("Environment = %q, want %q", cfg.Environment, "production")
	}
	// Verify they're not treated as service names.
	if _, ok := cfg.Services["project"]; ok {
		t.Error("'project' should not be a service")
	}
	if _, ok := cfg.Services["environment"]; ok {
		t.Error("'environment' should not be a service")
	}
}

func TestParseFile_ProjectAndEnvironmentOptional(t *testing.T) {
	content := `
[api.variables]
PORT = "8080"
`
	path := writeTempTOML(t, content)
	cfg, err := config.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile() error: %v", err)
	}
	if cfg.Project != "" {
		t.Errorf("Project should be empty, got %q", cfg.Project)
	}
	if cfg.Environment != "" {
		t.Errorf("Environment should be empty, got %q", cfg.Environment)
	}
}

func TestParse_RejectsNonStringProject(t *testing.T) {
	_, err := config.Parse([]byte(`project = 123`))
	if err == nil {
		t.Fatal("expected error for non-string project")
	}
	if !strings.Contains(err.Error(), "project") {
		t.Errorf("error should mention project: %v", err)
	}
}

func TestParse_RejectsNonStringEnvironment(t *testing.T) {
	_, err := config.Parse([]byte(`environment = true`))
	if err == nil {
		t.Fatal("expected error for non-string environment")
	}
	if !strings.Contains(err.Error(), "environment") {
		t.Errorf("error should mention environment: %v", err)
	}
}

func TestParse_RejectsUnknownScalarTopLevelKey(t *testing.T) {
	_, err := config.Parse([]byte(`projct = "typo"`))
	if err == nil {
		t.Fatal("expected error for unrecognised top-level key")
	}
	if !strings.Contains(err.Error(), "projct") {
		t.Errorf("error should mention the key: %v", err)
	}
}

func TestParse_AcceptsUnknownTableTopLevelKey(t *testing.T) {
	cfg, err := config.Parse([]byte("[my_service]\n[my_service.variables]\nFOO = \"bar\""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := cfg.Services["my_service"]; !ok {
		t.Error("expected my_service in services")
	}
}

func TestParse_ExtractsSensitiveKeywords(t *testing.T) {
	cfg, err := config.Parse([]byte(`
sensitive_keywords = ["SECRET", "TOKEN"]
sensitive_allowlist = ["TOKEN_URL"]
suppress_warnings = ["W012", "W030"]
`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.SensitiveKeywords) != 2 || cfg.SensitiveKeywords[0] != "SECRET" {
		t.Errorf("unexpected keywords: %v", cfg.SensitiveKeywords)
	}
	if len(cfg.SensitiveAllowlist) != 1 || cfg.SensitiveAllowlist[0] != "TOKEN_URL" {
		t.Errorf("unexpected allowlist: %v", cfg.SensitiveAllowlist)
	}
	if len(cfg.SuppressWarnings) != 2 || cfg.SuppressWarnings[0] != "W012" {
		t.Errorf("unexpected suppress_warnings: %v", cfg.SuppressWarnings)
	}
}

func TestParse_Workspace(t *testing.T) {
	cfg, err := config.Parse([]byte(`workspace = "my-team"`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Workspace != "my-team" {
		t.Errorf("expected workspace 'my-team', got %q", cfg.Workspace)
	}
}

func TestParse_RejectsNonStringWorkspace(t *testing.T) {
	_, err := config.Parse([]byte(`workspace = 42`))
	if err == nil {
		t.Fatal("expected error for non-string workspace")
	}
	if !strings.Contains(err.Error(), "workspace") {
		t.Errorf("error should mention workspace: %v", err)
	}
}

func TestParse_RejectsInvalidSensitiveKeywords(t *testing.T) {
	_, err := config.Parse([]byte(`sensitive_keywords = "not-an-array"`))
	if err == nil {
		t.Fatal("expected error for non-array sensitive_keywords")
	}
}

// writeTempTOML writes content to a temp .toml file and returns its path.
func writeTempTOML(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "fat-controller.toml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	return path
}
