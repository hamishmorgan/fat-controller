package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/config"
)

func writeTempTOML(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "fat-controller.toml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestParse_EnvironmentIdentity(t *testing.T) {
	path := writeTempTOML(t, `
name = "production"
id = "env_abc123"
`)
	cfg, err := config.ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Name != "production" {
		t.Errorf("Name = %q, want %q", cfg.Name, "production")
	}
	if cfg.ID != "env_abc123" {
		t.Errorf("ID = %q, want %q", cfg.ID, "env_abc123")
	}
}

func TestParse_SharedVariables(t *testing.T) {
	path := writeTempTOML(t, `
variables = { NODE_ENV = "production", LOG_LEVEL = "info" }
`)
	cfg, err := config.ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Variables) != 2 {
		t.Fatalf("Variables count = %d, want 2", len(cfg.Variables))
	}
	if cfg.Variables["NODE_ENV"] != "production" {
		t.Errorf("NODE_ENV = %q, want %q", cfg.Variables["NODE_ENV"], "production")
	}
}

func TestParse_WorkspaceAndProject(t *testing.T) {
	path := writeTempTOML(t, `
[workspace]
name = "Acme"
id = "ws_123"

[project]
name = "my-app"
id = "proj_456"
`)
	cfg, err := config.ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Workspace == nil {
		t.Fatal("Workspace is nil")
	}
	if cfg.Workspace.Name != "Acme" {
		t.Errorf("Workspace.Name = %q, want %q", cfg.Workspace.Name, "Acme")
	}
	if cfg.Workspace.ID != "ws_123" {
		t.Errorf("Workspace.ID = %q, want %q", cfg.Workspace.ID, "ws_123")
	}
	if cfg.Project == nil {
		t.Fatal("Project is nil")
	}
	if cfg.Project.Name != "my-app" {
		t.Errorf("Project.Name = %q, want %q", cfg.Project.Name, "my-app")
	}
}

func TestParse_ToolSettings(t *testing.T) {
	path := writeTempTOML(t, `
[tool]
api_timeout = "60s"
log_level = "debug"
prompt = "none"
allow_create = true
allow_delete = false
sensitive_keywords = ["SECRET", "TOKEN"]
suppress_warnings = ["W012"]
`)
	cfg, err := config.ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Tool == nil {
		t.Fatal("Tool is nil")
	}
	if cfg.Tool.APITimeout != "60s" {
		t.Errorf("APITimeout = %q, want %q", cfg.Tool.APITimeout, "60s")
	}
	if cfg.Tool.Prompt != "none" {
		t.Errorf("Prompt = %q, want %q", cfg.Tool.Prompt, "none")
	}
	if cfg.Tool.AllowCreate == nil || *cfg.Tool.AllowCreate != true {
		t.Errorf("AllowCreate = %v, want true", cfg.Tool.AllowCreate)
	}
	if cfg.Tool.AllowDelete == nil || *cfg.Tool.AllowDelete != false {
		t.Errorf("AllowDelete = %v, want false", cfg.Tool.AllowDelete)
	}
	if len(cfg.Tool.SensitiveKeywords) != 2 {
		t.Errorf("SensitiveKeywords = %v, want 2 items", cfg.Tool.SensitiveKeywords)
	}
}

func TestParse_ServiceBasic(t *testing.T) {
	path := writeTempTOML(t, `
[[service]]
name = "api"
id = "srv_abc"
icon = "server"
variables = { PORT = "8080" }
`)
	cfg, err := config.ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Services) != 1 {
		t.Fatalf("Services count = %d, want 1", len(cfg.Services))
	}
	svc := cfg.Services[0]
	if svc.Name != "api" {
		t.Errorf("Name = %q, want %q", svc.Name, "api")
	}
	if svc.ID != "srv_abc" {
		t.Errorf("ID = %q, want %q", svc.ID, "srv_abc")
	}
	if svc.Icon != "server" {
		t.Errorf("Icon = %q, want %q", svc.Icon, "server")
	}
	if svc.Variables["PORT"] != "8080" {
		t.Errorf("PORT = %q, want %q", svc.Variables["PORT"], "8080")
	}
}

func TestParse_ServiceDeploy(t *testing.T) {
	path := writeTempTOML(t, `
[[service]]
name = "api"
deploy = {
    repo = "org/api",
    branch = "main",
    builder = "NIXPACKS",
    build_command = "npm run build",
    start_command = "node server.js",
    healthcheck_path = "/health",
    healthcheck_timeout = 30,
    restart_policy = "ON_FAILURE",
    restart_policy_max_retries = 5,
    draining_seconds = 30,
    overlap_seconds = 5,
    sleep_application = false,
    watch_patterns = ["apps/api/**"],
    ipv6_egress = false,
}
`)
	cfg, err := config.ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	svc := cfg.Services[0]
	if svc.Deploy == nil {
		t.Fatal("Deploy is nil")
	}
	if svc.Deploy.Repo == nil || *svc.Deploy.Repo != "org/api" {
		t.Errorf("Repo = %v, want %q", svc.Deploy.Repo, "org/api")
	}
	if svc.Deploy.Builder == nil || *svc.Deploy.Builder != "NIXPACKS" {
		t.Errorf("Builder = %v, want %q", svc.Deploy.Builder, "NIXPACKS")
	}
	if svc.Deploy.HealthcheckTimeout == nil || *svc.Deploy.HealthcheckTimeout != 30 {
		t.Errorf("HealthcheckTimeout = %v, want 30", svc.Deploy.HealthcheckTimeout)
	}
	if len(svc.Deploy.WatchPatterns) != 1 {
		t.Errorf("WatchPatterns = %v, want 1 item", svc.Deploy.WatchPatterns)
	}
}

func TestParse_ServiceResources(t *testing.T) {
	path := writeTempTOML(t, `
[[service]]
name = "api"
resources = { vcpus = 2, memory_gb = 4 }
`)
	cfg, err := config.ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	svc := cfg.Services[0]
	if svc.Resources == nil {
		t.Fatal("Resources is nil")
	}
	if svc.Resources.VCPUs == nil || *svc.Resources.VCPUs != 2 {
		t.Errorf("VCPUs = %v, want 2", svc.Resources.VCPUs)
	}
	if svc.Resources.MemoryGB == nil || *svc.Resources.MemoryGB != 4 {
		t.Errorf("MemoryGB = %v, want 4", svc.Resources.MemoryGB)
	}
}

func TestParse_ServiceSubResources(t *testing.T) {
	path := writeTempTOML(t, `
[[service]]
name = "api"
scale = { "us-west1" = 3, "europe-west4" = 2 }
domains = {
    "api.example.com" = { port = 8080 },
    service_domain = { port = 8080 },
}
volumes = {
    data = { mount = "/data" },
    cache = { mount = "/cache", region = "us-west1" },
}
tcp_proxies = [5432]
network = true
triggers = [
    { branch = "main", repository = "org/api", check_suites = true },
]
egress = ["us-west1"]
`)
	cfg, err := config.ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	svc := cfg.Services[0]

	// Scale
	if len(svc.Scale) != 2 {
		t.Errorf("Scale count = %d, want 2", len(svc.Scale))
	}
	if svc.Scale["us-west1"] != 3 {
		t.Errorf("Scale[us-west1] = %d, want 3", svc.Scale["us-west1"])
	}

	// Domains
	if len(svc.Domains) != 2 {
		t.Errorf("Domains count = %d, want 2", len(svc.Domains))
	}
	if d, ok := svc.Domains["api.example.com"]; !ok || d.Port == nil || *d.Port != 8080 {
		t.Errorf("Domains[api.example.com] = %v", svc.Domains["api.example.com"])
	}

	// Volumes
	if len(svc.Volumes) != 2 {
		t.Errorf("Volumes count = %d, want 2", len(svc.Volumes))
	}
	if v := svc.Volumes["cache"]; v.Mount != "/cache" || v.Region != "us-west1" {
		t.Errorf("Volumes[cache] = %+v", v)
	}

	// TCP Proxies
	if len(svc.TCPProxies) != 1 || svc.TCPProxies[0] != 5432 {
		t.Errorf("TCPProxies = %v, want [5432]", svc.TCPProxies)
	}

	// Network
	if svc.Network == nil || *svc.Network != true {
		t.Errorf("Network = %v, want true", svc.Network)
	}

	// Triggers
	if len(svc.Triggers) != 1 {
		t.Errorf("Triggers count = %d, want 1", len(svc.Triggers))
	}
	if svc.Triggers[0].Branch != "main" {
		t.Errorf("Triggers[0].Branch = %q, want %q", svc.Triggers[0].Branch, "main")
	}

	// Egress
	if len(svc.Egress) != 1 || svc.Egress[0] != "us-west1" {
		t.Errorf("Egress = %v, want [us-west1]", svc.Egress)
	}
}

func TestParse_MultipleServices(t *testing.T) {
	path := writeTempTOML(t, `
[[service]]
name = "api"
variables = { PORT = "8080" }

[[service]]
name = "worker"
variables = { CONCURRENCY = "5" }
`)
	cfg, err := config.ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Services) != 2 {
		t.Fatalf("Services count = %d, want 2", len(cfg.Services))
	}
	if cfg.Services[0].Name != "api" {
		t.Errorf("Services[0].Name = %q, want %q", cfg.Services[0].Name, "api")
	}
	if cfg.Services[1].Name != "worker" {
		t.Errorf("Services[1].Name = %q, want %q", cfg.Services[1].Name, "worker")
	}
}

func TestParse_DeleteMarker(t *testing.T) {
	path := writeTempTOML(t, `
[[service]]
name = "old-service"
delete = true
`)
	cfg, err := config.ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Services[0].Delete {
		t.Errorf("Delete = false, want true")
	}
}

func TestParse_RegistryCredentials(t *testing.T) {
	path := writeTempTOML(t, `
[[service]]
name = "api"
deploy = {
    image = "registry.example.com/app:latest",
    registry_credentials = {
        username = "deploy",
        password = "${REGISTRY_PASSWORD}",
    },
}
`)
	cfg, err := config.ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	svc := cfg.Services[0]
	if svc.Deploy.RegistryCredentials == nil {
		t.Fatal("RegistryCredentials is nil")
	}
	if svc.Deploy.RegistryCredentials.Username != "deploy" {
		t.Errorf("Username = %q, want %q", svc.Deploy.RegistryCredentials.Username, "deploy")
	}
}

func TestParse_EmptyFile(t *testing.T) {
	path := writeTempTOML(t, "")
	cfg, err := config.ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Name != "" {
		t.Errorf("Name = %q, want empty", cfg.Name)
	}
	if len(cfg.Services) != 0 {
		t.Errorf("Services = %v, want empty", cfg.Services)
	}
}

func TestParse_NonexistentFile(t *testing.T) {
	_, err := config.ParseFile("/nonexistent/path.toml")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestParse_VariableCoercion(t *testing.T) {
	// Non-string variable values should be coerced to strings.
	path := writeTempTOML(t, `
[[service]]
name = "api"
variables = { PORT = 8080, DEBUG = true }
`)
	cfg, err := config.ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	svc := cfg.Services[0]
	if svc.Variables["PORT"] != "8080" {
		t.Errorf("PORT = %q, want %q", svc.Variables["PORT"], "8080")
	}
	if svc.Variables["DEBUG"] != "true" {
		t.Errorf("DEBUG = %q, want %q", svc.Variables["DEBUG"], "true")
	}
}

func TestParse_TopLevelVolumesAndBuckets(t *testing.T) {
	path := writeTempTOML(t, `
volumes = { data = { mount = "/data", region = "us-west1" } }
buckets = ["my-bucket"]
`)
	cfg, err := config.ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Volumes) != 1 {
		t.Fatalf("Volumes count = %d, want 1", len(cfg.Volumes))
	}
	if cfg.Volumes["data"].Mount != "/data" {
		t.Errorf("Volumes[data].Mount = %q, want %q", cfg.Volumes["data"].Mount, "/data")
	}
	if len(cfg.Buckets) != 1 || cfg.Buckets[0] != "my-bucket" {
		t.Errorf("Buckets = %v, want [my-bucket]", cfg.Buckets)
	}
}

func TestParse_EnvFileString(t *testing.T) {
	path := writeTempTOML(t, `
[tool]
env_file = ".env"
`)
	cfg, err := config.ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	files := cfg.Tool.EnvFiles()
	if len(files) != 1 || files[0] != ".env" {
		t.Errorf("EnvFiles() = %v, want [.env]", files)
	}
}

func TestParse_EnvFileList(t *testing.T) {
	path := writeTempTOML(t, `
[tool]
env_file = [".env", ".env.production"]
`)
	cfg, err := config.ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	files := cfg.Tool.EnvFiles()
	if len(files) != 2 {
		t.Fatalf("EnvFiles() count = %d, want 2", len(files))
	}
	if files[0] != ".env" || files[1] != ".env.production" {
		t.Errorf("EnvFiles() = %v", files)
	}
}

func TestParse_ServiceMissingName(t *testing.T) {
	path := writeTempTOML(t, `
[[service]]
variables = { PORT = "8080" }
`)
	_, err := config.ParseFile(path)
	if err == nil {
		t.Fatal("expected error for service without name")
	}
}
