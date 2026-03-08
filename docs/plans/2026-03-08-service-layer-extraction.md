# Service Layer Extraction Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Extract application logic from `internal/cli/` into a new `internal/app/` service layer, creating a clean boundary between interface adapters (CLI, prompt) and domain orchestration (load → resolve → diff → apply).

**Architecture:** The CLI layer currently owns business logic that doesn't belong to it — `loadAndFetch`, `RunConfigApply`, `RunConfigDiff`, adopt merge logic, service target resolution, etc. These "testable core" functions import only `config`, `diff`, `apply`, and abstract interfaces — not Kong, prompt, or terminal concerns. We extract them into `internal/app/`, which becomes the middle layer: CLI calls app, app calls ports (railway via interfaces, config via direct import). The CLI layer shrinks to command structs, `.Run()` methods, flag parsing, and output formatting. The `apply` and `diff` packages stay where they are — they're already correctly layered as domain logic that `app` orchestrates.

**Tech Stack:** Go, existing `config`/`diff`/`apply` packages. No new dependencies.

---

## Design Decisions

### Package name: `app`

Not `service` (conflicts with Railway service concept), not `core` (vague), not `engine` (overloaded). `app` is the Go convention for application-level orchestration (see stdlib `net/http` patterns, Go project layout guides). Short, clear, already the term used in hexagonal architecture.

### What moves to `app`

Functions that orchestrate business logic without depending on CLI/terminal concerns:

1. **Config pipeline** — `loadAndFetch`, `configPair`, `configFetcher` interface, `scopeDesiredByPath`, `splitDotPath`
2. **Diff orchestration** — the compute-and-return-result logic from `RunConfigDiff` (not the formatting)
3. **Apply orchestration** — the load-diff-apply logic from `RunConfigApply` (not the confirmation prompt or formatting)
4. **Validate orchestration** — the load-and-validate logic from `RunConfigValidate` (not the output formatting)
5. **Show/get orchestration** — `lookupKey`, `filterSection`, the resolve-fetch-lookup logic
6. **Adopt merge logic** — `adoptMerge`, `scopeLiveByPath`, conversion helpers
7. **Service target resolution** — `resolveServiceTargets`, `serviceTarget`
8. **Deploy/action orchestration** — the iterate-services-call-action pattern
9. **Pure utilities** — `parseTimeArg`, `parseRelativeDuration`, `databaseImage`, `ensureGitignoreHasLine`, `renderEnvFile`

### What stays in `cli`

1. **Kong command structs** and their `.Run()` methods
2. **Flag mixin structs** (`ApiFlags`, `MergeFlags`, `PromptFlags`, `ConfigFileFlags`, etc.)
3. **`newClient`** — creates railway.Client from flags + auth store
4. **`interactivePicker`** — bridges railway.Picker to prompt.PickItem
5. **`defaultConfigFetcher`** — the adapter that implements `app.ConfigFetcher` using railway + interactivePicker
6. **Output formatting** — all the structured output types (`DiffOutput`, `StatusOutput`, etc.), `writeStructured`, `isStructuredOutput`, `renderDiffStructured`, the `switch globals.Output` blocks
7. **Help rendering** — `help.go`, `completion.go`
8. **User interaction** — confirmation prompts, spinners, init flow pickers

### The key boundary

`app` functions accept `io.Writer` for output and callback functions for interaction (confirmation, picking). They return domain types (`*diff.Result`, `*apply.Result`, `*config.LiveConfig`). The CLI layer wraps these with formatting and interaction adapters.

However: we do this **incrementally**. Many `Run*` functions currently interleave orchestration with format branching (`if isStructuredOutput { ... } else { ... }`). Rather than untangling every one in this plan, we focus on extracting the pieces that have clear boundaries today, and leave the `Run*` functions as thin wrappers that call `app.*` then format the result.

### Dependency graph after extraction

```text
cmd/fat-controller
  └── cli (Kong structs, .Run() methods, formatting, prompts)
        ├── app (orchestration: load, resolve, diff, apply, validate, adopt)
        │     ├── config (schema, parsing, merge, interpolation, validation)
        │     ├── diff (change computation)
        │     └── apply (mutation execution, via Applier interface)
        ├── prompt (interactive UI: pickers, confirmations, spinners)
        ├── railway (GraphQL API adapter, via app interfaces + direct calls)
        └── auth (token management)
```

`app` imports: `config`, `diff`, `apply`, `context`, `io`, stdlib.
`app` does NOT import: `cli`, `prompt`, `railway`, `auth`, `Kong`.

The `railway` dependency is injected via the `ConfigFetcher` interface (already exists as `configFetcher` in `cli`).

---

## Task Dependency Order

Tasks 1-4 are independent setup. Tasks 5-10 each move a logical group. Tasks 11-12 are cleanup.

### Task 1: Create `internal/app/` package with core types

**Files:**

- Create: `internal/app/app.go`

**What:** Define the package and the shared types that multiple functions will use. These are extracted from `internal/cli/config_common.go` and `internal/cli/config_get.go`.

```go
// Package app provides application-level orchestration for fat-controller.
// It sits between interface adapters (CLI, API) and domain logic (config,
// diff, apply), owning the load → resolve → diff → apply pipeline.
package app

import (
    "context"

    "github.com/hamishmorgan/fat-controller/internal/config"
)

// ConfigFetcher abstracts project/environment resolution and live state fetching.
// The CLI layer provides an implementation that delegates to the railway package
// with interactive picking support.
type ConfigFetcher interface {
    Resolve(ctx context.Context, workspace, project, environment string) (string, string, error)
    Fetch(ctx context.Context, projectID, environmentID string, services []string) (*config.LiveConfig, error)
}

// ConfigPair bundles the desired and live config together with resolved IDs,
// produced by the shared load → interpolate → resolve → fetch → filter pipeline.
type ConfigPair struct {
    Desired       *config.DesiredConfig
    Live          *config.LiveConfig
    ProjectID     string
    EnvironmentID string
}
```

**Verification:**

- `go build ./internal/app/`

**Commit:** `add internal/app package with core ConfigFetcher and ConfigPair types`

---

### Task 2: Move config pipeline to `app`

**Files:**

- Modify: `internal/app/app.go` (add functions)
- Modify: `internal/cli/config_common.go` (delegate to app)

**What:** Move `loadAndFetch`, `scopeDesiredByPath`, `splitDotPath` to `app`. The CLI's `loadAndFetch` becomes a thin wrapper (or direct call). `configPair` → `app.ConfigPair`.

The functions move almost verbatim. The only change is exporting names and changing the package.

In `internal/app/app.go`, add:

```go
// LoadAndFetch runs the shared pipeline:
//  1. Load config files (cascade or single --config-file)
//  2. Interpolate ${VAR} references (env files → process env)
//  3. Fall back to config-file project/environment when flags are empty
//  4. Resolve project and environment IDs
//  5. Fetch live state
//  6. Filter desired config by service if set
func LoadAndFetch(ctx context.Context, workspace, project, environment, configDir, configFile, service string, fetcher ConfigFetcher) (*ConfigPair, error) {
    // ... (body from cli/config_common.go loadAndFetch, unchanged)
}

// ScopeDesiredByPath narrows a DesiredConfig to only include the service or
// section specified by path.
func ScopeDesiredByPath(cfg *config.DesiredConfig, path string) *config.DesiredConfig {
    // ... (body from cli/config_common.go scopeDesiredByPath, unchanged)
}
```

In `internal/cli/config_common.go`, replace the function bodies with calls to `app`:

```go
func loadAndFetch(...) (*configPair, error) {
    pair, err := app.LoadAndFetch(ctx, ...)
    if err != nil {
        return nil, err
    }
    return &configPair{
        Desired:       pair.Desired,
        Live:          pair.Live,
        ProjectID:     pair.ProjectID,
        EnvironmentID: pair.EnvironmentID,
    }, nil
}
```

Or better: change callers of `configPair` to use `*app.ConfigPair` directly and delete the local type. This is cleaner but touches more files. Decide based on how many callers there are.

**Important:** `emitWarnings` stays in CLI — it logs to slog (stderr), which is a presentation concern.

**Verification:**

- `go build ./...`
- `go test -race -count=1 ./...`

**Commit:** `move config pipeline (loadAndFetch, scopeDesiredByPath) to app package`

---

### Task 3: Move adopt merge logic to `app`

**Files:**

- Create: `internal/app/adopt.go`
- Modify: `internal/cli/adopt.go` (call app functions)
- Move: `internal/cli/adopt_test.go` tests that test pure merge logic

**What:** Move `adoptMerge`, `scopeLiveByPath`, `desiredServiceToLive`, `desiredDeployToLive`, `findDesiredService` to `app/adopt.go`. These are pure functions on config types — no CLI dependency.

The `adopt_test.go` tests for these functions are internal tests (`package cli`). They need to become `package app_test` or `package app` tests in the new location.

`AdoptCmd.Run()` stays in CLI — it handles prompting, file writing, and output. It calls `app.AdoptMerge(...)` instead of the local function.

**Verification:**

- `go build ./...`
- `go test -race -count=1 ./...`

**Commit:** `move adopt merge logic to app package`

---

### Task 4: Move service target resolution to `app`

**Files:**

- Create: `internal/app/targets.go`
- Modify: `internal/cli/deploy.go` (call app function)

**What:** Move `serviceTarget` and `resolveServiceTargets` to `app`. This function depends on `railway.FetchLiveConfig` — but it's called through the `configFetcher.Fetch` interface indirectly (it uses `railway.FetchLiveConfig` directly today).

To avoid `app` importing `railway`, change `resolveServiceTargets` to accept a `Fetcher`-like function:

```go
// ServiceTarget holds a resolved service name and ID.
type ServiceTarget struct {
    Name string
    ID   string
}

// ResolveServiceTargets resolves service arguments to name+ID pairs.
// If no service names are given, returns all services.
func ResolveServiceTargets(ctx context.Context, fetcher ConfigFetcher, projectID, envID string, serviceNames []string) ([]ServiceTarget, error) {
    live, err := fetcher.Fetch(ctx, projectID, envID, nil)
    // ...
}
```

CLI callers update from `resolveServiceTargets(ctx, client, ...)` to `app.ResolveServiceTargets(ctx, fetcher, ...)`.

**Verification:**

- `go build ./...`
- `go test -race -count=1 ./...`

**Commit:** `move service target resolution to app package`

---

### Task 5: Move show/get logic to `app`

**Files:**

- Create: `internal/app/show.go`
- Modify: `internal/cli/config_get.go` (thin down)

**What:** Move `lookupKey`, `filterSection` to `app/show.go`. These are pure functions on config types. The `RunConfigGet` function stays in CLI (it handles output formatting).

**Verification:**

- `go build ./...`
- `go test -race -count=1 ./...`

**Commit:** `move show/get lookup logic to app package`

---

### Task 6: Move validate orchestration to `app`

**Files:**

- Create: `internal/app/validate.go`
- Modify: `internal/cli/config_validate.go` (thin down)

**What:** Extract the load-and-validate core from `RunConfigValidate` into `app.Validate`:

```go
// Validate loads config and returns validation warnings.
func Validate(configDir, configFile string) (*config.DesiredConfig, []config.Warning, error) {
    result, err := config.LoadCascade(config.LoadOptions{
        WorkDir:    configDir,
        ConfigFile: configFile,
    })
    if err != nil {
        return nil, nil, err
    }
    warnings := config.ValidateWithOptions(result.Config, config.ValidateOptions{EnvFileVars: result.EnvVars})
    warnings = append(warnings, config.ValidateFiles(configDir)...)
    return result.Config, warnings, nil
}
```

CLI's `RunConfigValidate` becomes: call `app.Validate`, then format the warnings.

**Verification:**

- `go build ./...`
- `go test -race -count=1 ./...`

**Commit:** `move validate orchestration to app package`

---

### Task 7: Move diff orchestration to `app`

**Files:**

- Create: `internal/app/diff.go`
- Modify: `internal/cli/config_diff.go` (thin down)

**What:** Extract the load-scope-compute core from `RunConfigDiffWithOpts`:

```go
// Diff loads config, fetches live state, and computes the diff.
func Diff(ctx context.Context, workspace, project, environment, configDir, configFile, service, path string, diffOpts diff.Options, fetcher ConfigFetcher) (*ConfigPair, *diff.Result, error) {
    pair, err := LoadAndFetch(ctx, workspace, project, environment, configDir, configFile, service, fetcher)
    if err != nil {
        return nil, nil, err
    }
    desired := pair.Desired
    if path != "" {
        desired = ScopeDesiredByPath(desired, path)
    }
    result := diff.ComputeWithOptions(desired, pair.Live, diffOpts)
    return pair, result, nil
}
```

CLI's `RunConfigDiffWithOpts` becomes: call `app.Diff`, emit warnings, format result.

**Verification:**

- `go build ./...`
- `go test -race -count=1 ./...`

**Commit:** `move diff orchestration to app package`

---

### Task 8: Move apply orchestration to `app`

**Files:**

- Create: `internal/app/apply.go` (NOT the same as `internal/apply/` — this is orchestration, not the Applier interface)
- Modify: `internal/cli/config_apply.go` (thin down)

**What:** Extract the load-diff-apply core. The tricky part: `runConfigApplyWithPairAndOpts` currently calls `prompt.Confirm` for the confirmation step. That interaction stays in CLI. The app function computes the diff and returns it; the CLI decides whether to confirm and then calls `apply.Apply`.

```go
// ComputeApplyPlan loads config, fetches live state, and computes
// what apply would do. Returns the pair, diff result, and the desired
// config (possibly scoped by path). The caller decides whether to
// confirm and execute.
func ComputeApplyPlan(ctx context.Context, workspace, project, environment, configDir, configFile, service, path string, diffOpts diff.Options, fetcher ConfigFetcher) (*ConfigPair, *diff.Result, *config.DesiredConfig, error) {
    pair, err := LoadAndFetch(ctx, workspace, project, environment, configDir, configFile, service, fetcher)
    if err != nil {
        return nil, nil, nil, err
    }
    desired := pair.Desired
    if path != "" {
        desired = ScopeDesiredByPath(desired, path)
    }
    result := diff.ComputeWithOptions(desired, pair.Live, diffOpts)
    return pair, result, desired, nil
}
```

The actual `apply.Apply(ctx, desired, live, applier, opts)` call remains in CLI — it's already a clean domain call. The app layer just prepares the inputs.

**Verification:**

- `go build ./...`
- `go test -race -count=1 ./...`

**Commit:** `move apply plan computation to app package`

---

### Task 9: Move pure utilities to `app`

**Files:**

- Create: `internal/app/time.go` (or just add to `app.go`)
- Modify: CLI callers

**What:** Move `parseTimeArg`, `parseRelativeDuration`, `databaseImage`, `renderEnvFile`, `ensureGitignoreHasLine` to app. These are pure functions with no CLI dependency.

Note: `databaseImage` and `ensureGitignoreHasLine` might be better staying in CLI since they're only used by init/new commands and are trivially small. Use judgment — if they'd be the only callers, leave them. Don't move things just for the principle.

**Verification:**

- `go build ./...`
- `go test -race -count=1 ./...`

**Commit:** `move pure utilities to app package`

---

### Task 10: Move warning emission to `app`

**Files:**

- Modify: `internal/app/app.go`
- Modify: `internal/cli/config_common.go`

**What:** `emitWarnings` currently runs validation and logs to slog. The validation part (computing warnings) belongs in app. The slog emission is borderline — slog is a stdlib logger, not a CLI concern per se. But the `quiet` flag check is a presentation concern.

Option A: Move `emitWarnings` as-is to app (it uses slog, which is fine for a service layer).
Option B: Have app return warnings, CLI decides whether to log them.

Go with Option B — it's cleaner. `app.Diff` and `app.ComputeApplyPlan` can optionally return warnings alongside the result. CLI decides to log or suppress.

This may already be handled by Tasks 7-8 if those functions return the `ConfigPair` (which has the Desired config for validation). CLI can call `config.ValidateWithOptions` itself on the returned pair.

**Verdict:** This task may be unnecessary if Tasks 7-8 are designed correctly. Evaluate after implementing those.

---

### Task 11: Move tests

**Files:**

- Create: `internal/app/*_test.go`
- Modify: `internal/cli/*_test.go` (remove moved tests)

**What:** Tests that test pure app logic (adopt merge, scope functions, time parsing, etc.) move to `internal/app/`. Tests that test CLI output formatting stay in `internal/cli/`.

The e2e tests stay in CLI — they test the full stack through `Run*` functions.

**Verification:**

- `go test -race -count=1 ./...`

**Commit:** `move app-logic tests to app package`

---

### Task 12: Update ARCHITECTURE.md

**Files:**

- Modify: `docs/ARCHITECTURE.md`

**What:** Add a "Package Layout" section describing the layer hierarchy:

```text
internal/
  app/        — application orchestration (load, resolve, diff, apply pipelines)
  apply/      — mutation execution (Applier interface + Railway implementation)
  auth/       — token management, OAuth flow
  cli/        — CLI wiring (Kong commands, output formatting, prompts)
  config/     — config schema, parsing, merge, interpolation, validation
  diff/       — change computation
  logger/     — structured logging setup
  platform/   — OS-specific paths
  prompt/     — interactive UI (pickers, confirmations)
  railway/    — Railway GraphQL API adapter
  version/    — build version info
```

Describe the dependency rules: `app` imports `config`, `diff`, `apply` but not `cli`, `prompt`, `railway`. `cli` imports everything. `railway` imports `config` and `auth` but not `cli` or `prompt`.

**Commit:** `document package layout and layer boundaries in ARCHITECTURE.md`

---

## What This Plan Does NOT Do

1. **Fully separate output formatting from orchestration in every `Run*` function.** Many `Run*` functions have `if isStructuredOutput { json } else { text }` blocks interleaved with logic. Splitting those requires returning structured result types from app and building formatters in CLI — a worthwhile follow-up but too much churn for this plan.

2. **Create interfaces for everything.** `app` calls `config.LoadCascade` and `diff.ComputeWithOptions` directly — these are stable domain functions, not swappable adapters. Only the `ConfigFetcher` interface exists at the boundary where we actually need injection (railway vs. test mock).

3. **Move the `apply` or `diff` packages.** They're already correctly layered. `apply.Apply` is called by CLI today; after this plan it'll still be called by CLI (with inputs prepared by `app`). The `apply` package is domain logic, not orchestration.

4. **Move `prompt`-dependent code to `app`.** Anything that imports `prompt` (confirmation dialogs, pickers, spinners) stays in CLI. The `app` layer is non-interactive.
