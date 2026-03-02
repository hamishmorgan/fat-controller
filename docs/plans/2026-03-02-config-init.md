# Config Init and Project/Environment in Config

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make config files self-describing by adding optional `project` and `environment` fields, then implement `config init` to interactively bootstrap a `fat-controller.toml` from live Railway state.

**Architecture:** Two phases. Phase 1 adds `project`/`environment` as top-level TOML keys in the config format — parsed, merged, and used as defaults when CLI flags/env vars are not set. Phase 2 adds `config init` which authenticates, prompts for workspace/project/environment, fetches live state, and writes a new `fat-controller.toml` with the header and service variables. The existing `renderTOML` function (which operates on `LiveConfig`) is reused to generate the service sections; a new function prepends the project/environment header.

**Tech Stack:** Go, BurntSushi/toml, charmbracelet/huh (interactive prompts), kong, stdlib testing

---

## Context for the implementor

### How config loading works today

1. `config.LoadConfigs(dir, extraFiles)` loads `fat-controller.toml` (required) + `fat-controller.local.toml` (auto-discovered) + `--config` files, then merges via `config.Merge()`.
2. `config.Parse(data)` unmarshals TOML into `map[string]any`. It skips keys in `knownTopLevelKeys` (`shared`, `sensitive_keywords`, `sensitive_allowlist`, `suppress_warnings`) and treats everything else as a service name. Non-table top-level keys are silently skipped.
3. `DesiredConfig` has two fields: `Shared *DesiredVariables` and `Services map[string]*DesiredService`. There is no place for project/environment metadata.
4. `config.Merge()` combines `DesiredConfig` values — later wins at the field level.

### How project/environment resolution works today

The resolution chain in `RunConfigDiff`/`RunConfigApply`:

1. `globals.Project`/`globals.Environment` come from `--project`/`--environment` flags or `FAT_CONTROLLER_PROJECT`/`FAT_CONTROLLER_ENVIRONMENT` env vars.
2. These are passed to `fetcher.Resolve()` → `railway.ResolveProjectEnvironment()`.
3. If empty, the resolve function prompts interactively (if TTY) or errors with a list of available options.
4. For project tokens (`RAILWAY_TOKEN`), project/environment are extracted from the token itself — flags are ignored.

### What changes

After this plan, resolution becomes:

1. CLI flag / env var (highest priority, unchanged)
2. **Config file value** (new — from merged `DesiredConfig.Project`/`DesiredConfig.Environment`)
3. Interactive prompt / error (lowest priority, unchanged)

### File inventory

| File | Action |
|------|--------|
| `internal/config/desired.go` | **Modify** — add `Project`, `Environment` fields to `DesiredConfig` |
| `internal/config/parse.go` | **Modify** — extract `project`/`environment` from TOML, add to `knownTopLevelKeys` |
| `internal/config/parse_test.go` | **Modify** — add tests for new fields |
| `internal/config/merge.go` | **Modify** — merge `Project`/`Environment` (later wins, non-empty overrides) |
| `internal/config/merge_test.go` | **Modify** — add merge test |
| `internal/config/render.go` | **Modify** — add `RenderInitTOML` for writing config files with header |
| `internal/config/render_test.go` | **Modify** — add render test |
| `internal/cli/config_diff.go` | **Modify** — use config file project/environment as fallback |
| `internal/cli/config_apply.go` | **Modify** — same fallback |
| `internal/cli/config_diff_test.go` | **Modify** — add test for config-file-based resolution |
| `internal/cli/cli.go` | **Modify** — add `ConfigInitCmd` to command tree |
| `internal/cli/config_init.go` | **Create** — `config init` command |
| `internal/cli/config_init_test.go` | **Create** — init tests |

### Hazards

- `knownTopLevelKeys` is the gatekeeper — `project` and `environment` must be added there or they'll be treated as service names (a service called "project" with no subsections would be silently skipped as a non-table, but it's still wrong).
- The merge of `Project`/`Environment` must use "last non-empty wins" not "last wins", so that a base file with `project = "my-app"` isn't wiped by a `.local.toml` that doesn't mention project.
- `config init` must refuse to overwrite an existing `fat-controller.toml`. This is a safety invariant.
- `renderTOML` operates on `LiveConfig` and masks secrets. For `init`, we want secrets masked (the user can fill them in via `.local.toml` / `${VAR}` interpolation). This is the right default.
- The config file should store project/environment **names**, not UUIDs. Names are human-readable and portable across Railway accounts. The resolve chain already handles name→ID mapping.

---

## Task 1: Add `Project`/`Environment` fields to `DesiredConfig`

**Files:**

- Modify: `internal/config/desired.go`
- Modify: `internal/config/parse.go`
- Modify: `internal/config/parse_test.go`

**Step 1: Write the failing tests**

Append to `internal/config/parse_test.go`:

```go
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
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/config/... -run TestParseFile_ProjectAndEnvironment -v`
Expected: FAIL — `cfg.Project` undefined.

**Step 3: Write the implementation**

In `internal/config/desired.go`, add fields to `DesiredConfig`:

```go
type DesiredConfig struct {
    Project     string                     // from top-level `project` key (optional)
    Environment string                     // from top-level `environment` key (optional)
    Shared      *DesiredVariables          // nil means no shared section in config
    Services    map[string]*DesiredService // keyed by service name
}
```

In `internal/config/parse.go`, add `"project"` and `"environment"` to `knownTopLevelKeys`:

```go
var knownTopLevelKeys = map[string]bool{
    "project":             true,
    "environment":         true,
    "shared":              true,
    "sensitive_keywords":  true,
    "sensitive_allowlist": true,
    "suppress_warnings":   true,
}
```

In the `Parse` function, extract the new fields after unmarshaling:

```go
func Parse(data []byte) (*DesiredConfig, error) {
    var raw map[string]any
    if err := toml.Unmarshal(data, &raw); err != nil {
        return nil, fmt.Errorf("parsing config: %w", err)
    }

    cfg := &DesiredConfig{}

    // Extract project/environment metadata.
    if v, ok := raw["project"].(string); ok {
        cfg.Project = v
    }
    if v, ok := raw["environment"].(string); ok {
        cfg.Environment = v
    }

    // Extract shared section (unchanged)...
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/config/... -v`
Expected: all PASS (including existing tests — nothing should break).

**Step 5: Commit**

```bash
git add internal/config/desired.go internal/config/parse.go internal/config/parse_test.go
git commit -m "feat(config): parse project and environment from config file"
```

---

## Task 2: Merge `Project`/`Environment` across config files

**Files:**

- Modify: `internal/config/merge.go`
- Modify: `internal/config/merge_test.go`

**Step 1: Write the failing test**

Append to `internal/config/merge_test.go`:

```go
func TestMerge_ProjectEnvironment(t *testing.T) {
    base := &config.DesiredConfig{
        Project:     "my-app",
        Environment: "production",
        Services:    map[string]*config.DesiredService{},
    }
    // Local override sets environment but not project.
    local := &config.DesiredConfig{
        Environment: "staging",
        Services:    map[string]*config.DesiredService{},
    }
    result := config.Merge(base, local)
    if result.Project != "my-app" {
        t.Errorf("Project = %q, want %q (preserved from base)", result.Project, "my-app")
    }
    if result.Environment != "staging" {
        t.Errorf("Environment = %q, want %q (overridden by local)", result.Environment, "staging")
    }
}

func TestMerge_ProjectEnvironment_EmptyDoesNotOverride(t *testing.T) {
    base := &config.DesiredConfig{
        Project:     "my-app",
        Environment: "production",
        Services:    map[string]*config.DesiredService{},
    }
    // Overlay with empty project/environment should not wipe base values.
    overlay := &config.DesiredConfig{
        Services: map[string]*config.DesiredService{
            "api": {Variables: map[string]string{"PORT": "9090"}},
        },
    }
    result := config.Merge(base, overlay)
    if result.Project != "my-app" {
        t.Errorf("Project = %q, want %q (empty should not override)", result.Project, "my-app")
    }
    if result.Environment != "production" {
        t.Errorf("Environment = %q, want %q (empty should not override)", result.Environment, "production")
    }
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/config/... -run TestMerge_ProjectEnvironment -v`
Expected: FAIL — `Project` and `Environment` are not merged.

**Step 3: Write the implementation**

In `internal/config/merge.go`, add merging inside the `for _, cfg` loop in `Merge()`, after the nil check:

```go
func Merge(configs ...*DesiredConfig) *DesiredConfig {
    result := &DesiredConfig{
        Services: make(map[string]*DesiredService),
    }
    for _, cfg := range configs {
        if cfg == nil {
            continue
        }
        // Merge project/environment: non-empty overrides.
        if cfg.Project != "" {
            result.Project = cfg.Project
        }
        if cfg.Environment != "" {
            result.Environment = cfg.Environment
        }
        result.Shared = mergeVariables(result.Shared, cfg.Shared)
        // ... rest unchanged
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/config/... -v`
Expected: all PASS.

**Step 5: Commit**

```bash
git add internal/config/merge.go internal/config/merge_test.go
git commit -m "feat(config): merge project/environment across config files (non-empty wins)"
```

---

## Task 3: Use config-file project/environment as fallback in diff and apply

**Files:**

- Modify: `internal/cli/config_diff.go`
- Modify: `internal/cli/config_apply.go`
- Modify: `internal/cli/config_diff_test.go`

The pattern: after loading/merging the config, if `globals.Project` is empty, fall back to `desired.Project`. Same for `Environment`. CLI flags and env vars still take priority because kong populates `globals` from those sources before `Run()` is called.

**Step 1: Write the failing test**

Append to `internal/cli/config_diff_test.go`:

```go
func TestRunConfigDiff_UsesConfigFileProject(t *testing.T) {
    dir := t.TempDir()
    writeTOMLFile(t, dir, "fat-controller.toml", `
project = "my-app"
environment = "production"

[api.variables]
PORT = "9090"
`)
    // fakeFetcher doesn't validate project/environment args, but we
    // verify via a capturing fetcher that the config-file values are used.
    captureFetcher := &capturingFetcher{
        cfg: &config.LiveConfig{
            ProjectID: "proj-1", EnvironmentID: "env-1",
            Services: map[string]*config.ServiceConfig{
                "api": {Name: "api", Variables: map[string]string{"PORT": "8080"}},
            },
        },
    }
    var buf bytes.Buffer
    // Globals with empty Project/Environment — should fall back to config file.
    globals := &cli.Globals{Output: "text"}
    err := cli.RunConfigDiff(context.Background(), globals, dir, nil, captureFetcher, &buf)
    if err != nil {
        t.Fatalf("RunConfigDiff() error: %v", err)
    }
    if captureFetcher.project != "my-app" {
        t.Errorf("project passed to Resolve = %q, want %q", captureFetcher.project, "my-app")
    }
    if captureFetcher.environment != "production" {
        t.Errorf("environment passed to Resolve = %q, want %q", captureFetcher.environment, "production")
    }
}

func TestRunConfigDiff_FlagOverridesConfigFile(t *testing.T) {
    dir := t.TempDir()
    writeTOMLFile(t, dir, "fat-controller.toml", `
project = "my-app"
environment = "production"

[api.variables]
PORT = "9090"
`)
    captureFetcher := &capturingFetcher{
        cfg: &config.LiveConfig{
            ProjectID: "proj-1", EnvironmentID: "env-1",
            Services: map[string]*config.ServiceConfig{
                "api": {Name: "api", Variables: map[string]string{"PORT": "8080"}},
            },
        },
    }
    var buf bytes.Buffer
    // Flag values should override config file.
    globals := &cli.Globals{Project: "other-project", Environment: "staging", Output: "text"}
    err := cli.RunConfigDiff(context.Background(), globals, dir, nil, captureFetcher, &buf)
    if err != nil {
        t.Fatalf("RunConfigDiff() error: %v", err)
    }
    if captureFetcher.project != "other-project" {
        t.Errorf("project = %q, want %q (flag should override)", captureFetcher.project, "other-project")
    }
    if captureFetcher.environment != "staging" {
        t.Errorf("environment = %q, want %q (flag should override)", captureFetcher.environment, "staging")
    }
}
```

You'll also need to add a `capturingFetcher` to the test file (unless one already exists — check `config_get_test.go` for `serviceCaptureFetcher`). Add to `config_diff_test.go`:

```go
// capturingFetcher records the project/environment passed to Resolve.
type capturingFetcher struct {
    cfg         *config.LiveConfig
    project     string
    environment string
}

func (f *capturingFetcher) Resolve(_ context.Context, _, project, environment string) (string, string, error) {
    f.project = project
    f.environment = environment
    return "proj-1", "env-1", nil
}

func (f *capturingFetcher) Fetch(_ context.Context, _, _, _ string) (*config.LiveConfig, error) {
    return f.cfg, nil
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/cli/... -run TestRunConfigDiff_UsesConfigFile -v`
Expected: FAIL — config-file values not passed to Resolve.

**Step 3: Write the implementation**

In `internal/cli/config_diff.go`, after loading and interpolating the config (step 2), add the fallback:

```go
    // 2. Interpolate local env vars.
    if err := config.Interpolate(desired); err != nil {
        return err
    }

    // 2b. Use config-file project/environment as fallback for resolution.
    project := globals.Project
    if project == "" {
        project = desired.Project
    }
    environment := globals.Environment
    if environment == "" {
        environment = desired.Environment
    }

    // 3. Fetch live state.
    projID, envID, err := fetcher.Resolve(ctx, globals.Workspace, project, environment)
```

Apply the identical change to `internal/cli/config_apply.go` in the `RunConfigApply` function (same location — after interpolation, before resolve).

**Step 4: Run tests to verify they pass**

Run:

```bash
go test ./internal/cli/... -v
```

Expected: all PASS.

**Step 5: Commit**

```bash
git add internal/cli/config_diff.go internal/cli/config_apply.go internal/cli/config_diff_test.go
git commit -m "feat(cli): use config-file project/environment as resolution fallback"
```

---

## Task 4: Add `RenderInitTOML` for writing config files with header

**Files:**

- Modify: `internal/config/render.go`
- Modify: `internal/config/render_test.go`

The `init` command needs to write a complete `fat-controller.toml` with `project`/`environment` at the top, followed by the service sections. The existing `renderTOML` operates on `LiveConfig` and handles the service sections. We add a new `RenderInitTOML` function that prepends the metadata header and delegates to the existing rendering logic.

**Step 1: Write the failing test**

Append to `internal/config/render_test.go` (or create it if it doesn't exist — check first):

```go
func TestRenderInitTOML_Header(t *testing.T) {
    cfg := config.LiveConfig{
        Services: map[string]*config.ServiceConfig{
            "api": {
                Name:      "api",
                Variables: map[string]string{"PORT": "8080"},
            },
        },
    }
    got := config.RenderInitTOML("my-app", "production", cfg)
    if !strings.Contains(got, `project = "my-app"`) {
        t.Errorf("expected project header:\n%s", got)
    }
    if !strings.Contains(got, `environment = "production"`) {
        t.Errorf("expected environment header:\n%s", got)
    }
    if !strings.Contains(got, "[api.variables]") {
        t.Errorf("expected service section:\n%s", got)
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
                    "PORT":             "8080",
                    "DATABASE_PASSWORD": "hunter2",
                },
            },
        },
    }
    got := config.RenderInitTOML("proj", "env", cfg)
    if strings.Contains(got, "hunter2") {
        t.Errorf("secret should be masked:\n%s", got)
    }
    if !strings.Contains(got, "PORT") {
        t.Errorf("expected PORT variable:\n%s", got)
    }
}

func TestRenderInitTOML_SharedVariables(t *testing.T) {
    cfg := config.LiveConfig{
        Shared: map[string]string{"GLOBAL": "value"},
        Services: map[string]*config.ServiceConfig{},
    }
    got := config.RenderInitTOML("proj", "env", cfg)
    if !strings.Contains(got, "[shared.variables]") {
        t.Errorf("expected shared section:\n%s", got)
    }
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/config/... -run TestRenderInitTOML -v`
Expected: FAIL — `RenderInitTOML` is undefined.

**Step 3: Write the implementation**

Append to `internal/config/render.go`:

```go
// RenderInitTOML generates a fat-controller.toml for the init command.
// It includes a project/environment header, masks secrets, and excludes
// deploy settings and IDs (those are operational, not config).
func RenderInitTOML(project, environment string, cfg LiveConfig) string {
    masker := NewMasker(nil, nil)
    masked := maskConfig(cfg, masker)

    var out strings.Builder
    out.WriteString("project = " + tomlQuote(project) + "\n")
    out.WriteString("environment = " + tomlQuote(environment) + "\n")

    // Render service sections using the existing TOML renderer (without
    // IDs or deploy settings — those are fetched live, not managed in config).
    body := renderTOML(masked, false)
    if body != "" {
        out.WriteString("\n")
        out.WriteString(body)
    }

    return out.String()
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/config/... -v`
Expected: all PASS.

**Step 5: Commit**

```bash
git add internal/config/render.go internal/config/render_test.go
git commit -m "feat(config): add RenderInitTOML for bootstrapping config files"
```

---

## Task 5: Add `config init` command

**Files:**

- Create: `internal/cli/config_init.go`
- Create: `internal/cli/config_init_test.go`
- Modify: `internal/cli/cli.go` — add `ConfigInitCmd` to command tree

**Step 1: Write the failing tests**

Create `internal/cli/config_init_test.go`:

```go
package cli_test

import (
    "bytes"
    "context"
    "os"
    "path/filepath"
    "strings"
    "testing"

    "github.com/hamishmorgan/fat-controller/internal/cli"
    "github.com/hamishmorgan/fat-controller/internal/config"
)

// initFetcher provides controlled responses for init tests.
type initFetcher struct {
    cfg         *config.LiveConfig
    resolveErr  error
    fetchErr    error
    project     string
    environment string
}

func (f *initFetcher) Resolve(_ context.Context, _, project, environment string) (string, string, error) {
    f.project = project
    f.environment = environment
    if f.resolveErr != nil {
        return "", "", f.resolveErr
    }
    return "proj-1", "env-1", nil
}

func (f *initFetcher) Fetch(_ context.Context, _, _, _ string) (*config.LiveConfig, error) {
    if f.fetchErr != nil {
        return nil, f.fetchErr
    }
    return f.cfg, nil
}

func TestRunConfigInit_WritesConfigFile(t *testing.T) {
    dir := t.TempDir()
    fetcher := &initFetcher{
        cfg: &config.LiveConfig{
            ProjectID:     "proj-1",
            EnvironmentID: "env-1",
            Services: map[string]*config.ServiceConfig{
                "api": {
                    Name:      "api",
                    Variables: map[string]string{"PORT": "8080", "APP_ENV": "production"},
                },
            },
        },
    }
    var buf bytes.Buffer
    err := cli.RunConfigInit(context.Background(), dir, "my-app", "production", fetcher, &buf)
    if err != nil {
        t.Fatalf("RunConfigInit() error: %v", err)
    }

    // Verify the file was written.
    content, err := os.ReadFile(filepath.Join(dir, "fat-controller.toml"))
    if err != nil {
        t.Fatalf("reading config file: %v", err)
    }
    got := string(content)
    if !strings.Contains(got, `project = "my-app"`) {
        t.Errorf("expected project header in file:\n%s", got)
    }
    if !strings.Contains(got, `environment = "production"`) {
        t.Errorf("expected environment header in file:\n%s", got)
    }
    if !strings.Contains(got, "[api.variables]") {
        t.Errorf("expected service section in file:\n%s", got)
    }
    if !strings.Contains(got, "PORT") {
        t.Errorf("expected PORT in file:\n%s", got)
    }
}

func TestRunConfigInit_RefusesToOverwrite(t *testing.T) {
    dir := t.TempDir()
    // Create an existing config file.
    existing := filepath.Join(dir, "fat-controller.toml")
    if err := os.WriteFile(existing, []byte("existing"), 0o644); err != nil {
        t.Fatalf("write existing: %v", err)
    }

    fetcher := &initFetcher{
        cfg: &config.LiveConfig{Services: map[string]*config.ServiceConfig{}},
    }
    var buf bytes.Buffer
    err := cli.RunConfigInit(context.Background(), dir, "proj", "env", fetcher, &buf)
    if err == nil {
        t.Fatal("expected error when config file already exists")
    }
    if !strings.Contains(err.Error(), "already exists") {
        t.Errorf("error should mention 'already exists': %v", err)
    }
}

func TestRunConfigInit_CreatesLocalTOMLStub(t *testing.T) {
    dir := t.TempDir()
    fetcher := &initFetcher{
        cfg: &config.LiveConfig{
            Services: map[string]*config.ServiceConfig{
                "api": {Name: "api", Variables: map[string]string{"PORT": "8080"}},
            },
        },
    }
    var buf bytes.Buffer
    err := cli.RunConfigInit(context.Background(), dir, "proj", "env", fetcher, &buf)
    if err != nil {
        t.Fatalf("RunConfigInit() error: %v", err)
    }

    // Verify .local.toml stub was created.
    localPath := filepath.Join(dir, "fat-controller.local.toml")
    content, err := os.ReadFile(localPath)
    if err != nil {
        t.Fatalf("reading local config: %v", err)
    }
    if !strings.Contains(string(content), "local overrides") {
        t.Errorf("expected comment in local stub:\n%s", string(content))
    }
}

func TestRunConfigInit_PrintsSummary(t *testing.T) {
    dir := t.TempDir()
    fetcher := &initFetcher{
        cfg: &config.LiveConfig{
            Services: map[string]*config.ServiceConfig{
                "api": {Name: "api", Variables: map[string]string{"PORT": "8080"}},
            },
        },
    }
    var buf bytes.Buffer
    err := cli.RunConfigInit(context.Background(), dir, "proj", "env", fetcher, &buf)
    if err != nil {
        t.Fatalf("RunConfigInit() error: %v", err)
    }
    got := buf.String()
    if !strings.Contains(got, "fat-controller.toml") {
        t.Errorf("expected filename in output:\n%s", got)
    }
}

func TestRunConfigInit_ResolveError(t *testing.T) {
    dir := t.TempDir()
    fetcher := &initFetcher{resolveErr: errForTest("no project")}
    var buf bytes.Buffer
    err := cli.RunConfigInit(context.Background(), dir, "proj", "env", fetcher, &buf)
    if err == nil {
        t.Fatal("expected error from resolve failure")
    }
}

func errForTest(msg string) error {
    return errors.New(msg)
}
```

Add the missing `errors` import at the top of the test file:

```go
import (
    "bytes"
    "context"
    "errors"
    "os"
    "path/filepath"
    "strings"
    "testing"

    "github.com/hamishmorgan/fat-controller/internal/cli"
    "github.com/hamishmorgan/fat-controller/internal/config"
)
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/cli/... -run TestRunConfigInit -v`
Expected: FAIL — `RunConfigInit` is undefined.

**Step 3: Write the implementation**

Create `internal/cli/config_init.go`:

```go
package cli

import (
    "context"
    "fmt"
    "io"
    "os"
    "path/filepath"

    "github.com/hamishmorgan/fat-controller/internal/auth"
    "github.com/hamishmorgan/fat-controller/internal/config"
    "github.com/hamishmorgan/fat-controller/internal/platform"
    "github.com/hamishmorgan/fat-controller/internal/railway"
)

const localConfigStub = `# Local overrides (gitignored). Use for secrets and per-developer settings.
# Example:
#   [api.variables]
#   STRIPE_KEY = "${STRIPE_KEY}"
`

// Run implements `config init`.
func (c *ConfigInitCmd) Run(globals *Globals) error {
    store := auth.NewTokenStore(auth.WithFallbackPath(platform.AuthFilePath()))
    resolved, err := auth.ResolveAuth(globals.Token, store)
    if err != nil {
        return err
    }
    client := railway.NewClient(railway.Endpoint, resolved, store, auth.NewOAuthClient())
    fetcher := &defaultConfigFetcher{client: client}

    wd, err := os.Getwd()
    if err != nil {
        return fmt.Errorf("getting working directory: %w", err)
    }

    return RunConfigInit(context.Background(), wd, globals.Project, globals.Environment, fetcher, os.Stdout)
}

// RunConfigInit is the testable core of `config init`.
func RunConfigInit(ctx context.Context, dir, project, environment string, fetcher configFetcher, out io.Writer) error {
    if out == nil {
        out = os.Stdout
    }

    // 1. Refuse to overwrite existing config.
    configPath := filepath.Join(dir, config.BaseConfigFile)
    if _, err := os.Stat(configPath); err == nil {
        return fmt.Errorf("%s already exists — refusing to overwrite", config.BaseConfigFile)
    }

    // 2. Resolve project/environment (may prompt interactively).
    projID, envID, err := fetcher.Resolve(ctx, "", project, environment)
    if err != nil {
        return err
    }

    // 3. Fetch live state.
    live, err := fetcher.Fetch(ctx, projID, envID, "")
    if err != nil {
        return err
    }

    // 4. Resolve human-readable names for the header.
    // If the user provided names, use them. If they provided IDs or
    // were prompted, we still have the names in the fetched data or
    // from the original args. For simplicity, use what was provided.
    projName := project
    envName := environment

    // 5. Render and write the config file.
    content := config.RenderInitTOML(projName, envName, *live)
    if err := os.WriteFile(configPath, []byte(content+"\n"), 0o644); err != nil {
        return fmt.Errorf("writing %s: %w", config.BaseConfigFile, err)
    }
    fmt.Fprintf(out, "wrote %s (%d services)\n", config.BaseConfigFile, len(live.Services))

    // 6. Create .local.toml stub if it doesn't exist.
    localPath := filepath.Join(dir, config.LocalConfigFile)
    if _, err := os.Stat(localPath); os.IsNotExist(err) {
        if err := os.WriteFile(localPath, []byte(localConfigStub), 0o644); err != nil {
            return fmt.Errorf("writing %s: %w", config.LocalConfigFile, err)
        }
        fmt.Fprintf(out, "wrote %s (local overrides, gitignored)\n", config.LocalConfigFile)
    }

    return nil
}
```

**Step 4: Add `ConfigInitCmd` to the command tree**

In `internal/cli/cli.go`, add `Init` to `ConfigCmd`:

```go
type ConfigCmd struct {
    Init     ConfigInitCmd     `cmd:"" help:"Bootstrap a fat-controller.toml from live Railway state."`
    Get      ConfigGetCmd      `cmd:"" help:"Fetch live config from Railway."`
    Set      ConfigSetCmd      `cmd:"" help:"Set a single value by dot-path."`
    Delete   ConfigDeleteCmd   `cmd:"" help:"Delete a single value by dot-path."`
    Diff     ConfigDiffCmd     `cmd:"" help:"Compare local config against live state."`
    Apply    ConfigApplyCmd    `cmd:"" help:"Push configuration changes to Railway."`
    Validate ConfigValidateCmd `cmd:"" help:"Check config file for warnings (no API calls)."`
}
```

Add the type definition near the other config command types:

```go
type ConfigInitCmd struct{}
```

Update the Run methods comment:

```go
// Run methods:
// - ConfigInitCmd.Run   → config_init.go
// - ConfigGetCmd.Run    → config_get.go
// ...
```

**Step 5: Run tests to verify they pass**

Run:

```bash
go test ./internal/cli/... -v
go test ./internal/config/... -v
```

Expected: all PASS.

**Step 6: Commit**

```bash
git add internal/cli/config_init.go internal/cli/config_init_test.go internal/cli/cli.go
git commit -m "feat(cli): add config init command to bootstrap fat-controller.toml"
```

---

## Task 6: Final verification

**Step 1: Run all tests**

Run:

```bash
go test ./internal/config/... -v
go test ./internal/cli/... -v
```

Expected: all PASS.

**Step 2: Run the full check suite**

Run:

```bash
mise run check
```

Expected: all linters pass, all tests pass, build succeeds.

**Step 3: Smoke test help output**

Run:

```bash
.build/fat-controller config init --help
```

Expected: help text for the init command.

Run:

```bash
.build/fat-controller config diff --help
```

Expected: should still show `--project` and `--environment` flags.

**Step 4: Commit any lint fixes**

If `mise run check` required any fixes:

```bash
git add -A
git commit -m "chore: fix lint issues from config init implementation"
```

---

## Post-implementation notes

### Multi-environment workflow examples

**One file per environment:**

```bash
# fat-controller.toml (base, committed)
project = "my-app"

[api.variables]
APP_ENV = "production"

# fat-controller.staging.toml (committed)
environment = "staging"

[api.variables]
APP_ENV = "staging"
```

```bash
# CI for production:
fat-controller config apply --config fat-controller.production.toml --confirm

# CI for staging:
fat-controller config apply --config fat-controller.staging.toml --confirm
```

**Single file with flag override:**

```bash
# fat-controller.toml
project = "my-app"
environment = "production"

[api.variables]
PORT = "8080"
```

```bash
# Apply to staging (flag overrides config file):
fat-controller config diff --environment staging
```

### What this plan does NOT include (deferred)

- **`workspace` in config file** — workspace is rarely needed (most users have one). Can be added later if needed.
- **`config init` for environment-specific files** — `init` only creates the base `fat-controller.toml`. Environment overlays are created manually.
- **Name resolution for IDs in init** — if the user provides a UUID for `--project`, the config file will contain the UUID, not the human-readable name. A future enhancement could resolve IDs back to names.
- **Prompting for services to include** — `init` includes all services. A future enhancement could let users select which services to include.
- **`.gitignore` management** — `init` does not modify `.gitignore`. The existing `*.local.*` pattern already covers `fat-controller.local.toml`.
