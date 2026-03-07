# Config Schema Migration

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Migrate the TOML config schema from top-level service tables (`[api]`) to `[[service]]` array-of-tables with structured `[workspace]`/`[project]` tables, `[tool]` settings, and all entity fields from the architecture doc.

**Architecture:** The config file IS the environment. Top-level `name`/`id` identify the environment. `[workspace]` and `[project]` are parent context with `name`/`id` fields. `[[service]]` entries use `name` (required) and `id` (optional) for identity matching. `[tool]` holds all tool settings. Sub-resources (domains, volumes, TCP proxies, etc.) are inline tables under each service. The desired config model uses pointer fields so nil means "not specified, don't touch."

**Tech Stack:** Go 1.26, BurntSushi/toml v1.6.0 (TOML v1.1 with multiline inline tables), standard `testing` library (no testify), external test packages (`package config_test`).

**Reference:** `docs/ARCHITECTURE.md` (the complete end-state design).

---

## Context for the implementer

### Current state

The existing TOML schema uses top-level tables keyed by service name:

```toml
project = "my-app"
environment = "production"
workspace = "Acme"

[shared.variables]
KEY = "value"

[api.variables]
PORT = "8080"

[api.resources]
vcpus = 2
memory_gb = 4

[api.deploy]
builder = "NIXPACKS"
```

Services are parsed by iterating raw TOML keys and treating any unknown
top-level key as a service name (`internal/config/parse.go`).

### Target state

```toml
name = "production"
id = "env_abc123"
variables = { KEY = "value" }

[tool]
api_timeout = "60s"
env_file = ".env"

[workspace]
name = "Acme"
id = "ws_abc123"

[project]
name = "my-app"
id = "proj_abc123"

[[service]]
name = "api"
id = "srv_abc123"
icon = "server"
variables = { PORT = "8080" }
deploy = { builder = "NIXPACKS" }
resources = { vcpus = 2, memory_gb = 4 }
```

### Key files

| File | Role |
|------|------|
| `internal/config/desired.go` | Desired config types (what TOML declares) |
| `internal/config/model.go` | Live config types (what Railway has) |
| `internal/config/parse.go` | TOML → DesiredConfig |
| `internal/config/merge.go` | Multi-config merge |
| `internal/config/interpolate.go` | `${VAR}` resolution |
| `internal/config/validate.go` | Validation warnings |
| `internal/config/mask.go` | Secret masking |
| `internal/config/render.go` | Output formatting + init TOML |
| `internal/config/path.go` | Dot-path parsing |
| `internal/config/keys.go` | Field name constants |
| `internal/config/load.go` | File discovery + loading |
| `fat-controller.toml` | Project's own config (must migrate) |
| `docs/fat-controller.example.toml` | Example config (must migrate) |

### Testing conventions

- External test package: `package config_test`
- No testify — use `t.Fatal`, `t.Errorf`, `t.Helper`
- Helper: `writeTempTOML(t, content)` writes to temp file, returns path
- Naming: `TestFuncName_Scenario`
- All data inline (no fixture files)
- Run tests: `go test ./internal/config/ -run TestName -v`
- Run all tests: `go test ./...`
- Full check: `mise run check`

### Commit convention

Commit after each task completes. Message format: `feat:`, `refactor:`, `test:`, `fix:`. Keep commits small and focused.

---

## Task 1: Restructure desired config types

This is the foundation — all other tasks build on these types.

**Files:**

- Modify: `internal/config/desired.go`
- Test: `internal/config/desired_test.go` (create)

### Step 1: Write the types test

Create `internal/config/desired_test.go`. This verifies the new types
compile and have the expected zero values. It also documents the
struct shapes for anyone reading the code.

```go
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
```

### Step 2: Run test to verify it fails

Run: `go test ./internal/config/ -run TestDesiredConfig_ZeroValue -v`

Expected: FAIL — types don't exist yet.

### Step 3: Write the new types

Replace the contents of `internal/config/desired.go`. The old types
(`DesiredConfig`, `DesiredService`, `DesiredVariables`, `DesiredResources`,
`DesiredDeploy`, `Override`) are replaced by the new ones below.

**Do NOT delete the old types yet** — other files depend on them. Instead,
add the new types alongside. We'll migrate consumers in later tasks.

Actually — the simplest approach is to replace in-place. Every consumer
will break and we'll fix them task by task. But since there are many
consumers, a better approach: rename the old types with an `Old` prefix
temporarily, add the new types, then migrate consumers one at a time.

**Simplest approach for this plan:** Replace `desired.go` entirely with
the new types. The remaining tasks in this plan fix all consumers.

```go
package config

// ContextBlock identifies a workspace or project by name and optional ID.
type ContextBlock struct {
	Name string `toml:"name"`
	ID   string `toml:"id,omitempty"`
}

// ToolSettings holds tool behavior configuration (how fat-controller
// behaves, not what it manages).
type ToolSettings struct {
	APITimeout        string   `toml:"api_timeout,omitempty"`
	LogLevel          string   `toml:"log_level,omitempty"`
	OutputFormat      string   `toml:"output_format,omitempty"`
	OutputColor       string   `toml:"output_color,omitempty"`
	Prompt            string   `toml:"prompt,omitempty"`
	Deploy            string   `toml:"deploy,omitempty"`
	ShowSecrets       *bool    `toml:"show_secrets,omitempty"`
	FailFast          *bool    `toml:"fail_fast,omitempty"`
	AllowCreate       *bool    `toml:"allow_create,omitempty"`
	AllowUpdate       *bool    `toml:"allow_update,omitempty"`
	AllowDelete       *bool    `toml:"allow_delete,omitempty"`
	EnvFile           any      `toml:"env_file,omitempty"` // string or []string
	SensitiveKeywords []string `toml:"sensitive_keywords,omitempty"`
	SensitiveAllowlist []string `toml:"sensitive_allowlist,omitempty"`
	SuppressWarnings  []string `toml:"suppress_warnings,omitempty"`
}

// DesiredDeploy holds deploy settings for a service.
// Pointer fields: nil = "not specified, don't touch".
type DesiredDeploy struct {
	// Source
	Repo                *string              `toml:"repo,omitempty"`
	Image               *string              `toml:"image,omitempty"`
	Branch              *string              `toml:"branch,omitempty"`
	RegistryCredentials *RegistryCredentials `toml:"registry_credentials,omitempty"`

	// Build
	Builder        *string  `toml:"builder,omitempty"`
	BuildCommand   *string  `toml:"build_command,omitempty"`
	DockerfilePath *string  `toml:"dockerfile_path,omitempty"`
	RootDirectory  *string  `toml:"root_directory,omitempty"`
	NixpacksPlan   any      `toml:"nixpacks_plan,omitempty"` // inline table, passed as-is
	WatchPatterns  []string `toml:"watch_patterns,omitempty"`

	// Run
	StartCommand    *string `toml:"start_command,omitempty"`
	PreDeployCommand any    `toml:"pre_deploy_command,omitempty"` // string or []string
	CronSchedule    *string `toml:"cron_schedule,omitempty"`

	// Health
	HealthcheckPath       *string `toml:"healthcheck_path,omitempty"`
	HealthcheckTimeout    *int    `toml:"healthcheck_timeout,omitempty"`
	RestartPolicy         *string `toml:"restart_policy,omitempty"`
	RestartPolicyMaxRetries *int  `toml:"restart_policy_max_retries,omitempty"`

	// Deploy strategy
	DrainingSeconds *int  `toml:"draining_seconds,omitempty"`
	OverlapSeconds  *int  `toml:"overlap_seconds,omitempty"`
	SleepApplication *bool `toml:"sleep_application,omitempty"`

	// Placement (single-region shorthand)
	NumReplicas *int    `toml:"num_replicas,omitempty"`
	Region      *string `toml:"region,omitempty"`

	// Networking
	IPv6Egress *bool `toml:"ipv6_egress,omitempty"`
}

// RegistryCredentials holds Docker registry auth.
type RegistryCredentials struct {
	Username string `toml:"username"`
	Password string `toml:"password"`
}

// DesiredResources holds resource limits for a service.
type DesiredResources struct {
	VCPUs    *float64 `toml:"vcpus,omitempty"`
	MemoryGB *float64 `toml:"memory_gb,omitempty"`
}

// DomainConfig holds settings for a custom or service domain.
type DomainConfig struct {
	Port   *int  `toml:"port,omitempty"`
	Delete bool  `toml:"delete,omitempty"`
}

// VolumeConfig holds settings for an attached or unattached volume.
type VolumeConfig struct {
	Mount  string `toml:"mount"`
	Region string `toml:"region,omitempty"`
	Delete bool   `toml:"delete,omitempty"`
}

// TriggerConfig holds deployment trigger settings.
type TriggerConfig struct {
	Branch      string `toml:"branch"`
	Repository  string `toml:"repository"`
	Provider    string `toml:"provider,omitempty"`
	CheckSuites *bool  `toml:"check_suites,omitempty"`
	RootDirectory string `toml:"root_directory,omitempty"`
}

// DesiredService declares a service within the environment.
type DesiredService struct {
	Name      string            `toml:"name"`
	ID        string            `toml:"id,omitempty"`
	Icon      string            `toml:"icon,omitempty"`
	Delete    bool              `toml:"delete,omitempty"`
	Variables map[string]string `toml:"variables,omitempty"`
	Deploy    *DesiredDeploy    `toml:"deploy,omitempty"`
	Resources *DesiredResources `toml:"resources,omitempty"`
	Scale     map[string]int    `toml:"scale,omitempty"`
	Domains   map[string]DomainConfig  `toml:"domains,omitempty"`
	Volumes   map[string]VolumeConfig  `toml:"volumes,omitempty"`
	TCPProxies []int            `toml:"tcp_proxies,omitempty"`
	Network   *bool             `toml:"network,omitempty"`
	Triggers  []TriggerConfig   `toml:"triggers,omitempty"`
	Egress    []string          `toml:"egress,omitempty"`
}

// DesiredConfig is the top-level config — one file, one environment.
type DesiredConfig struct {
	Name      string                  `toml:"name,omitempty"`
	ID        string                  `toml:"id,omitempty"`
	Variables map[string]string       `toml:"variables,omitempty"`
	Volumes   map[string]VolumeConfig `toml:"volumes,omitempty"`
	Buckets   []string                `toml:"buckets,omitempty"`
	Tool      *ToolSettings           `toml:"tool,omitempty"`
	Workspace *ContextBlock           `toml:"workspace,omitempty"`
	Project   *ContextBlock           `toml:"project,omitempty"`
	Services  []*DesiredService       `toml:"service,omitempty"`
}
```

Key changes from old types:

- `Project string` → `Project *ContextBlock` (table with name + id)
- `Environment string` → `Name string` + `ID string` (top-level)
- `Workspace string` → `Workspace *ContextBlock`
- `Services map[string]*DesiredService` → `Services []*DesiredService` (array of tables)
- `DesiredService` gains `Name`, `ID`, `Icon`, `Delete`, plus all sub-resource fields
- `Shared *DesiredVariables` → `Variables map[string]string` (top-level)
- `SensitiveKeywords` etc. move into `ToolSettings`
- New: `DesiredDeploy` has all fields from architecture
- New: `DomainConfig`, `VolumeConfig`, `TriggerConfig`, `RegistryCredentials`

**Important:** `Services` uses TOML tag `toml:"service"` (singular) because
TOML arrays of tables use `[[service]]` — BurntSushi/toml maps the tag
name to the TOML key.

### Step 4: Run tests to verify they pass

Run: `go test ./internal/config/ -run "TestDesiredConfig_ZeroValue|TestDesiredService_ZeroValue|TestContextBlock_ZeroValue|TestToolSettings_ZeroValue" -v`

Expected: PASS (all zero-value checks pass).

Note: other tests in the package WILL fail because the old types are gone.
That's expected — we fix them in subsequent tasks.

### Step 5: Commit

```bash
git add internal/config/desired.go internal/config/desired_test.go
git commit -m "feat: replace desired config types with architecture-aligned schema

New types: ContextBlock, ToolSettings, DesiredDeploy (expanded),
DesiredService (with name/id/sub-resources), DesiredConfig (with
[[service]] array, [tool], [workspace], [project] tables).

Breaking change: all consumers of old types will need updating."
```

---

## Task 2: Update the TOML parser

The parser must handle the new schema. Currently it manually walks
`map[string]any` from `toml.Unmarshal`. The new schema can use
struct-based unmarshalling for most fields, with the parser handling
validation and coercion.

**Files:**

- Modify: `internal/config/parse.go`
- Test: `internal/config/parse_test.go` (rewrite tests)

### Step 1: Rewrite parse tests for the new schema

Replace the tests in `internal/config/parse_test.go` to test the new
TOML format. Keep the same helper (`writeTempTOML`) and test structure.

```go
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
	// env_file can be string or []string — parser normalizes to []string
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
```

### Step 2: Run tests to verify they fail

Run: `go test ./internal/config/ -run "TestParse_" -v`

Expected: FAIL — parser returns old types.

### Step 3: Rewrite the parser

Replace `internal/config/parse.go`. The new parser uses struct-based
TOML unmarshalling for most fields but still needs manual handling for:

- Variable value coercion (non-string → string)
- `env_file` polymorphism (string or []string)
- Service `name` required validation
- `pre_deploy_command` polymorphism (string or []string)

```go
package config

import (
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
)

// ParseFile reads a TOML file and returns the desired config.
func ParseFile(path string) (*DesiredConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}
	return Parse(data)
}

// Parse decodes TOML bytes into a DesiredConfig.
func Parse(data []byte) (*DesiredConfig, error) {
	var cfg DesiredConfig
	if _, err := toml.Decode(string(data), &cfg); err != nil {
		return nil, fmt.Errorf("parsing TOML: %w", err)
	}

	// Validate: every service must have a name.
	for i, svc := range cfg.Services {
		if svc.Name == "" {
			return nil, fmt.Errorf("service at index %d has no name", i)
		}
	}

	// Coerce non-string variable values to strings.
	coerceVariables(cfg.Variables)
	for _, svc := range cfg.Services {
		coerceVariables(svc.Variables)
	}

	return &cfg, nil
}

// coerceVariables converts non-string values to their string representation.
// BurntSushi/toml may decode integers as int64 and booleans as bool
// when the target is map[string]string with interface values, but since
// our target IS map[string]string, TOML will handle most coercion.
// This function is a safety net for edge cases.
func coerceVariables(vars map[string]string) {
	// With map[string]string as the target type, BurntSushi/toml
	// already coerces scalar values. This is a no-op placeholder
	// in case we need custom coercion later.
}
```

**Wait** — BurntSushi/toml won't automatically coerce `PORT = 8080`
(integer) into a `map[string]string` target. It will error. We need
to use `map[string]any` for variables and coerce manually, OR use a
custom `UnmarshalTOML` method.

Let me revise. The cleanest approach: define a custom `Variables` type
that implements `toml.Unmarshaler`.

Add to `internal/config/desired.go`:

```go
// Variables is a map[string]string that accepts non-string TOML values
// by coercing them to their string representation.
type Variables map[string]string

// UnmarshalTOML implements toml.Unmarshaler for Variables.
func (v *Variables) UnmarshalTOML(data any) error {
	m, ok := data.(map[string]any)
	if !ok {
		return fmt.Errorf("expected table, got %T", data)
	}
	*v = make(Variables, len(m))
	for k, val := range m {
		(*v)[k] = fmt.Sprint(val)
	}
	return nil
}
```

Then change the `Variables` field types from `map[string]string` to
`Variables` in `DesiredConfig` and `DesiredService`.

Similarly, `EnvFile` needs a custom type or the `EnvFiles()` helper method
on `ToolSettings`.

Add to `internal/config/desired.go`:

```go
// EnvFiles returns the env_file setting normalized to a string slice.
// Handles both string and []string TOML values.
func (t *ToolSettings) EnvFiles() []string {
	if t == nil {
		return nil
	}
	switch v := t.EnvFile.(type) {
	case string:
		if v == "" {
			return nil
		}
		return []string{v}
	case []any:
		result := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	default:
		return nil
	}
}
```

### Step 4: Run tests to verify they pass

Run: `go test ./internal/config/ -run "TestParse_" -v`

Expected: PASS.

### Step 5: Commit

```bash
git add internal/config/parse.go internal/config/parse_test.go internal/config/desired.go
git commit -m "feat: rewrite TOML parser for [[service]] array-of-tables schema

Struct-based unmarshalling with custom Variables type for non-string
coercion. Validates service names are present. Supports all new
fields: tool settings, context blocks, sub-resources."
```

---

## Task 3: Update the merge logic

The merge must handle `[]*DesiredService` (matched by ID then name)
instead of `map[string]*DesiredService`.

**Files:**

- Modify: `internal/config/merge.go`
- Rewrite: `internal/config/merge_test.go`

### Step 1: Write merge tests for the new schema

```go
package config_test

import (
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/config"
)

func TestMerge_Empty(t *testing.T) {
	result := config.Merge()
	if result == nil {
		t.Fatal("Merge() returned nil")
	}
}

func TestMerge_Single(t *testing.T) {
	cfg := &config.DesiredConfig{
		Name: "production",
		Services: []*config.DesiredService{
			{Name: "api", Variables: config.Variables{"PORT": "8080"}},
		},
	}
	result := config.Merge(cfg)
	if result.Name != "production" {
		t.Errorf("Name = %q, want %q", result.Name, "production")
	}
	if len(result.Services) != 1 {
		t.Fatalf("Services count = %d, want 1", len(result.Services))
	}
}

func TestMerge_ScalarsOverride(t *testing.T) {
	base := &config.DesiredConfig{
		Name: "staging",
		Workspace: &config.ContextBlock{Name: "Acme", ID: "ws_1"},
	}
	overlay := &config.DesiredConfig{
		Name: "production",
	}
	result := config.Merge(base, overlay)
	if result.Name != "production" {
		t.Errorf("Name = %q, want %q", result.Name, "production")
	}
	// Workspace should be preserved from base.
	if result.Workspace == nil || result.Workspace.Name != "Acme" {
		t.Errorf("Workspace = %v, want Acme", result.Workspace)
	}
}

func TestMerge_VariablesDeepMerge(t *testing.T) {
	base := &config.DesiredConfig{
		Variables: config.Variables{"A": "1", "B": "2"},
	}
	overlay := &config.DesiredConfig{
		Variables: config.Variables{"B": "3", "C": "4"},
	}
	result := config.Merge(base, overlay)
	if result.Variables["A"] != "1" {
		t.Errorf("A = %q, want %q", result.Variables["A"], "1")
	}
	if result.Variables["B"] != "3" {
		t.Errorf("B = %q, want %q", result.Variables["B"], "3")
	}
	if result.Variables["C"] != "4" {
		t.Errorf("C = %q, want %q", result.Variables["C"], "4")
	}
}

func TestMerge_ServiceMatchByName(t *testing.T) {
	base := &config.DesiredConfig{
		Services: []*config.DesiredService{
			{Name: "api", Variables: config.Variables{"PORT": "8080"}},
		},
	}
	overlay := &config.DesiredConfig{
		Services: []*config.DesiredService{
			{Name: "api", Variables: config.Variables{"HOST": "0.0.0.0"}},
		},
	}
	result := config.Merge(base, overlay)
	if len(result.Services) != 1 {
		t.Fatalf("Services count = %d, want 1", len(result.Services))
	}
	svc := result.Services[0]
	if svc.Variables["PORT"] != "8080" {
		t.Errorf("PORT = %q, want %q", svc.Variables["PORT"], "8080")
	}
	if svc.Variables["HOST"] != "0.0.0.0" {
		t.Errorf("HOST = %q, want %q", svc.Variables["HOST"], "0.0.0.0")
	}
}

func TestMerge_ServiceMatchByID(t *testing.T) {
	base := &config.DesiredConfig{
		Services: []*config.DesiredService{
			{Name: "api", ID: "srv_1", Variables: config.Variables{"PORT": "8080"}},
		},
	}
	overlay := &config.DesiredConfig{
		Services: []*config.DesiredService{
			// Name differs but ID matches.
			{Name: "api-v2", ID: "srv_1", Variables: config.Variables{"HOST": "0.0.0.0"}},
		},
	}
	result := config.Merge(base, overlay)
	if len(result.Services) != 1 {
		t.Fatalf("Services count = %d, want 1", len(result.Services))
	}
	svc := result.Services[0]
	// Name comes from overlay (later wins).
	if svc.Name != "api-v2" {
		t.Errorf("Name = %q, want %q", svc.Name, "api-v2")
	}
	if svc.Variables["PORT"] != "8080" {
		t.Errorf("PORT = %q, want %q", svc.Variables["PORT"], "8080")
	}
}

func TestMerge_ServiceNewInOverlay(t *testing.T) {
	base := &config.DesiredConfig{
		Services: []*config.DesiredService{
			{Name: "api"},
		},
	}
	overlay := &config.DesiredConfig{
		Services: []*config.DesiredService{
			{Name: "worker"},
		},
	}
	result := config.Merge(base, overlay)
	if len(result.Services) != 2 {
		t.Fatalf("Services count = %d, want 2", len(result.Services))
	}
}

func TestMerge_ToolSettingsDeepMerge(t *testing.T) {
	base := &config.DesiredConfig{
		Tool: &config.ToolSettings{APITimeout: "30s", LogLevel: "info"},
	}
	overlay := &config.DesiredConfig{
		Tool: &config.ToolSettings{LogLevel: "debug"},
	}
	result := config.Merge(base, overlay)
	if result.Tool.APITimeout != "30s" {
		t.Errorf("APITimeout = %q, want %q", result.Tool.APITimeout, "30s")
	}
	if result.Tool.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want %q", result.Tool.LogLevel, "debug")
	}
}

func TestMerge_ServiceDeployFieldLevel(t *testing.T) {
	builder := "NIXPACKS"
	startCmd := "node server.js"
	healthPath := "/health"
	base := &config.DesiredConfig{
		Services: []*config.DesiredService{
			{Name: "api", Deploy: &config.DesiredDeploy{
				Builder:      &builder,
				StartCommand: &startCmd,
			}},
		},
	}
	overlay := &config.DesiredConfig{
		Services: []*config.DesiredService{
			{Name: "api", Deploy: &config.DesiredDeploy{
				HealthcheckPath: &healthPath,
			}},
		},
	}
	result := config.Merge(base, overlay)
	svc := result.Services[0]
	if svc.Deploy.Builder == nil || *svc.Deploy.Builder != "NIXPACKS" {
		t.Errorf("Builder = %v, want NIXPACKS", svc.Deploy.Builder)
	}
	if svc.Deploy.StartCommand == nil || *svc.Deploy.StartCommand != "node server.js" {
		t.Errorf("StartCommand = %v", svc.Deploy.StartCommand)
	}
	if svc.Deploy.HealthcheckPath == nil || *svc.Deploy.HealthcheckPath != "/health" {
		t.Errorf("HealthcheckPath = %v", svc.Deploy.HealthcheckPath)
	}
}

func TestMerge_EmptyStringDoesNotOverride(t *testing.T) {
	base := &config.DesiredConfig{Name: "production"}
	overlay := &config.DesiredConfig{Name: ""}
	result := config.Merge(base, overlay)
	if result.Name != "production" {
		t.Errorf("Name = %q, want %q", result.Name, "production")
	}
}
```

### Step 2: Run tests to verify they fail

Run: `go test ./internal/config/ -run "TestMerge_" -v`

Expected: FAIL — Merge signature/behavior doesn't match.

### Step 3: Rewrite merge.go

The key change: services are matched by ID (when present) then name.
Implement `Merge(configs ...*DesiredConfig) *DesiredConfig`.

The implementation should:

1. Start with an empty `DesiredConfig`
2. For each config in order, merge scalars (non-empty overwrites), deep-merge maps, match and merge services
3. Service matching: build a lookup of existing services by ID and by name. For each overlay service, find the match and deep-merge. If no match, append.
4. Tool, Workspace, Project: deep-merge (non-empty fields overwrite)
5. Deploy: field-level merge (non-nil pointer overwrites)
6. Resources: field-level merge (non-nil pointer overwrites)

### Step 4: Run tests

Run: `go test ./internal/config/ -run "TestMerge_" -v`

Expected: PASS.

### Step 5: Commit

```bash
git add internal/config/merge.go internal/config/merge_test.go
git commit -m "feat: rewrite merge for [[service]] array matching by ID then name"
```

---

## Task 4: Update interpolation

Minimal change — just update field paths since variables moved.

**Files:**

- Modify: `internal/config/interpolate.go`
- Rewrite: `internal/config/interpolate_test.go`

### Step 1: Write interpolation tests for the new schema

Tests should cover:

- Top-level `Variables` interpolation
- Per-service `Variables` interpolation
- `${{railway.REF}}` left untouched
- Mixed `${LOCAL}` + `${{railway.REF}}`
- Missing env var → error
- Registry credentials interpolation (`deploy.registry_credentials.password`)

### Step 2: Run tests — expect fail

### Step 3: Update interpolate.go

Change from `cfg.Shared.Vars` → `cfg.Variables` and from
`cfg.Services[name].Variables` → iterate `cfg.Services` slice.
Also interpolate `RegistryCredentials.Password` if present.

### Step 4: Run tests — expect pass

### Step 5: Commit

```bash
git add internal/config/interpolate.go internal/config/interpolate_test.go
git commit -m "feat: update interpolation for new config schema"
```

---

## Task 5: Update validation

Add new warnings, remove obsolete ones, update paths.

**Files:**

- Modify: `internal/config/validate.go`
- Rewrite: `internal/config/validate_test.go`

### Step 1: Write validation tests

New/updated warnings to test:

- **W002**: Unknown keys in service — now via TOML strict mode or removed (struct-based parsing catches unknown keys)
- **W003**: Empty service block (no variables, no deploy, no resources, no sub-resources)
- **W011**: Suspicious `${word.word}` in variable values
- **W012**: Empty string variable value (= delete)
- **W020**: Variable in both top-level and service
- **W030**: Lowercase variable name
- **W031**: Invalid variable name characters
- **W040**: Unknown service name (not in live)
- **W041**: Nothing actionable
- **W050**: Hardcoded secret
- **W060**: Broken `${{service.VAR}}` reference
- **NEW W070**: Duplicate service name in config
- **NEW W071**: Mutually exclusive fields (`repo` + `image`)
- **NEW W072**: `scale` + `deploy.region` both set

### Step 2–5: Standard TDD cycle + commit

---

## Task 6: Update masking

Minimal change — masking logic itself doesn't change, but
`CollectSecrets` and any code that walks services needs updating.

**Files:**

- Modify: `internal/config/mask.go` (likely no changes needed)
- Modify: `internal/config/render.go` (update `CollectSecrets`, `Render`, `RenderInitTOML`)
- Rewrite: `internal/config/render_test.go`

### Step 1: Write render tests for the new schema

Test `Render` with the new `LiveConfig` (if unchanged) and new
`RenderInitTOML` that outputs `[[service]]` format.

### Step 2–5: Standard TDD cycle + commit

---

## Task 7: Update path parsing

The path model changes slightly: `shared.variables.KEY` → `variables.KEY`
for top-level. Service paths remain `service.section.key`.

**Files:**

- Modify: `internal/config/path.go`
- Rewrite: `internal/config/path_test.go`

### Step 1: Write path tests

Update test cases:

- `"variables.PORT"` → top-level variable
- `"api"` → service by name
- `"api.variables.PORT"` → service variable
- `"workspace"` → workspace context
- `"project"` → project context

### Step 2–5: Standard TDD cycle + commit

---

## Task 8: Update the load pipeline

`LoadConfigs` currently requires a base file and supports extra files.
Update to work with the new types. (The full file cascade with upward
walk is Plan 2 — this task just updates the existing load pipeline.)

**Files:**

- Modify: `internal/config/load.go`
- Rewrite: `internal/config/load_test.go`

### Step 1: Write load tests for the new schema

### Step 2–5: Standard TDD cycle + commit

---

## Task 9: Update the live config model

Expand `LiveConfig` and `ServiceConfig` to include all fields that
the architecture manages. This is needed for the diff and apply
engines to work with the new schema.

**Files:**

- Modify: `internal/config/model.go`
- Test: `internal/config/model_test.go` (create, zero-value tests)

### Step 1: Write tests for expanded live types

```go
package config_test

import (
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/config"
)

func TestLiveConfig_ZeroValue(t *testing.T) {
	var cfg config.LiveConfig
	if cfg.ProjectID != "" {
		t.Errorf("ProjectID = %q, want empty", cfg.ProjectID)
	}
	if cfg.Services != nil {
		t.Errorf("Services = %v, want nil", cfg.Services)
	}
}

func TestServiceConfig_ZeroValue(t *testing.T) {
	var svc config.ServiceConfig
	if svc.Domains != nil {
		t.Errorf("Domains = %v, want nil", svc.Domains)
	}
	if svc.Volumes != nil {
		t.Errorf("Volumes = %v, want nil", svc.Volumes)
	}
}
```

### Step 2: Run tests — expect fail

### Step 3: Expand live config types

Add to `LiveConfig`:

- `Name`, `ID` (environment identity)
- `Variables map[string]string` (replaces `Shared`)
- `Volumes`, `Buckets`

Add to `ServiceConfig`:

- `Icon`
- All deploy fields (expanded `Deploy` struct)
- `Resources` (VCPUs, MemoryGB as before)
- `Scale map[string]int`
- `Domains` (custom + service domain info)
- `Volumes` (attached)
- `TCPProxies`
- `Network` (private network endpoint)
- `Triggers`
- `Egress`

### Step 4: Run tests — expect pass

### Step 5: Commit

---

## Task 10: Update diff engine

The diff engine compares desired vs live. With new types on both sides,
update `Compute` to handle the new fields.

**Files:**

- Modify: `internal/diff/diff.go`
- Modify: `internal/diff/format.go`
- Rewrite: `internal/diff/diff_test.go`
- Rewrite: `internal/diff/format_test.go`

### Step 1: Write diff tests for the new schema

Focus on: services matched by array position/name (not map key),
new deploy fields, resource fields, sub-resource diffs.

### Step 2–5: Standard TDD cycle + commit

---

## Task 11: Update apply engine

The apply engine pushes desired state to Railway. Update to work with
the new service list and expanded deploy fields.

**Files:**

- Modify: `internal/apply/apply.go`
- Modify: `internal/apply/convert.go`
- Rewrite: `internal/apply/apply_test.go`
- Rewrite: `internal/apply/convert_test.go`

### Step 1: Write apply tests for the new schema

### Step 2–5: Standard TDD cycle + commit

---

## Task 12: Update CLI consumers

All CLI commands that use config types need updating.

**Files:**

- Modify: `internal/cli/config_common.go`
- Modify: `internal/cli/config_init.go`
- Modify: `internal/cli/config_get.go`
- Modify: `internal/cli/config_set.go`
- Modify: `internal/cli/config_delete.go`
- Modify: `internal/cli/config_diff.go`
- Modify: `internal/cli/config_apply.go`
- Modify: `internal/cli/config_validate.go`
- Modify: `internal/railway/state.go`
- Rewrite affected test files

### Step 1: Update `config_common.go`

The `loadAndFetch` function reads `desired.Project`, `desired.Environment`,
`desired.Workspace` — these are now `desired.Project.Name`,
`desired.Name`, `desired.Workspace.Name`.

### Step 2: Update each CLI file, fixing compile errors

### Step 3: Update E2E tests

### Step 4: Run full test suite

Run: `go test ./...`

### Step 5: Commit

---

## Task 13: Migrate project config files

Update the project's own TOML files to the new format.

**Files:**

- Modify: `fat-controller.toml`
- Modify: `docs/fat-controller.example.toml`
- Modify: `docs/fat-controller.schema.json` (if it exists)

### Step 1: Migrate `fat-controller.toml`

Convert from:

```toml
workspace = "Hamish Morgan's Projects"
project = "Life"
environment = "production"
[shared.variables]
...
[api.variables]
...
```

To:

```toml
name = "production"
variables = { MEILI_MASTER_KEY = "${MEILI_MASTER_KEY}" }

[workspace]
name = "Hamish Morgan's Projects"

[project]
name = "Life"

[[service]]
name = "embeddings"
variables = { ... }

[[service]]
name = "hanko"
variables = { ... }
# ... etc for all services
```

### Step 2: Migrate `docs/fat-controller.example.toml`

### Step 3: Run `mise run check`

Expected: all checks pass.

### Step 4: Commit

```bash
git add fat-controller.toml docs/fat-controller.example.toml
git commit -m "feat: migrate project config files to [[service]] schema"
```

---

## Task 14: Delete dead code and keys.go cleanup

Remove any leftover references to old types, old `knownTopLevelKeys`,
etc. Update `keys.go` with new field constants if needed.

**Files:**

- Modify: `internal/config/keys.go`
- Possibly modify various files

### Step 1: Search for references to old type names

Run: `grep -r "DesiredVariables\|knownTopLevelKeys\|Override\b" internal/`

### Step 2: Remove dead code

### Step 3: Run `mise run check`

### Step 4: Commit

---

## Task 15: Final verification

### Step 1: Run full check suite

Run: `mise run check`

Expected: all tests pass, all linters pass, no secrets detected.

### Step 2: Run tests with race detector

Run: `go test -race ./...`

Expected: no race conditions.

### Step 3: Verify the binary builds and runs

Run: `go build -o /dev/null ./cmd/fat-controller`

Expected: clean build.
