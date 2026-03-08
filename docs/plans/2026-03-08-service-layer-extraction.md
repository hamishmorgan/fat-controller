# Service Layer Extraction Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Extract application logic from `internal/cli/` into a new `internal/app/` service layer, creating a clean boundary between interface adapters (CLI, prompt) and domain orchestration (load, resolve, diff, apply).

**Architecture:** The CLI layer currently owns business logic that doesn't belong to it. Functions like `loadAndFetch`, `adoptMerge`, `resolveServiceTargets`, `lookupKey`, and `scopeDesiredByPath` depend only on `config`, `diff`, and abstract interfaces — not Kong, prompt, or terminal concerns. We extract them into `internal/app/`, which becomes the middle layer: CLI calls app, app calls ports (railway via interfaces, config via direct import). The `apply` and `diff` packages stay where they are — they're already correctly positioned as domain logic that `app` orchestrates.

**Tech Stack:** Go, existing `config`/`diff`/`apply` packages. No new dependencies.

---

## Design Decisions

### Package name: `app`

Not `service` (conflicts with Railway's service concept), not `core` (vague), not `engine` (overloaded). `app` is short, clear, and the conventional Go name for application-level orchestration.

### What moves to `app`

Only functions that are pure domain orchestration with no CLI/terminal dependency:

1. **Config pipeline** — `loadAndFetch` (depends only on `config` + `ConfigFetcher` interface + `slog`), `scopeDesiredByPath`, `splitDotPath`
2. **Adopt merge logic** — `adoptMerge`, `scopeLiveByPath`, `desiredServiceToLive`, `desiredDeployToLive`, `findDesiredService`, `joinServiceNames`
3. **Service target resolution** — `resolveServiceTargets`, `serviceTarget` (currently couples to `railway.Client`; will take `ConfigFetcher` instead)
4. **Show/get helpers** — `lookupKey`, `filterSection`
5. **Shared init helpers** — `renderEnvFile`, `ensureGitignoreHasLine` (used by both `config_init.go` and `adopt.go`)
6. **Types** — `configPair` (renamed `ConfigPair`), `configFetcher` interface (renamed `ConfigFetcher`)

### What stays in `cli`

1. **Kong command structs** and `.Run()` methods
2. **Flag mixin structs** (`ApiFlags`, `MergeFlags`, `PromptFlags`, `ConfigFileFlags`)
3. **Adapter glue** — `newClient`, `interactivePicker`, `defaultConfigFetcher`
4. **All `Run*` functions** — `RunConfigGet`, `RunConfigDiff`, `RunConfigApply`, `RunDeploy`, `RunStatus`, etc. These interleave orchestration with output formatting (`if isStructuredOutput { ... } else { ... }`) and are not cleanly separable without returning structured result types. Untangling them is a follow-up.
5. **Output formatting** — structured output types, `writeStructured`, `isStructuredOutput`, `renderDiffStructured`
6. **User interaction** — `emitWarnings` (decides to suppress based on `quiet int`), confirmation prompts, spinners, init flow pickers
7. **Help/completion** — `help.go`, `completion.go`
8. **Single-caller pure utilities** — `parseTimeArg`/`parseRelativeDuration` (only `logs.go`), `databaseImage` (only `new.go`). Not worth extracting.

### What does NOT move

- **`Run*` functions.** Every one of them references `Globals` (for output format) and calls formatting helpers. Separating them would require introducing result types for each command. That's a valid follow-up but too much churn for this plan.
- **`emitWarnings`.** It takes `quiet int` (from `globals.Quiet`) and decides whether to log to `slog.Warn`. The validation computation is trivially one call (`config.ValidateWithOptions`); extracting just that into app gains nothing. CLI can call `config.ValidateWithOptions` directly on the `ConfigPair` it gets back from `app.LoadAndFetch`.
- **`initResolver` interface.** It has a fundamentally different shape from `ConfigFetcher` — step-by-step methods returning `[]prompt.Item` for interactive wizard flows. They should not share an interface.

### Dependency graph after extraction

```text
cmd/fat-controller
  └── cli (Kong structs, .Run() methods, formatting, prompts)
        ├── app (orchestration: load, resolve, scope, merge)
        │     ├── config (schema, parsing, merge, interpolation, validation)
        │     ├── diff (change computation)
        │     └── apply (mutation execution, via Applier interface)
        ├── prompt (interactive UI: pickers, confirmations, spinners)
        ├── railway (GraphQL API adapter)
        └── auth (token management)
```

`app` imports: `config`, `diff`, `context`, stdlib (`fmt`, `log/slog`, `sort`, `strings`, `os`, `path/filepath`).
`app` does NOT import: `cli`, `prompt`, `railway`, `auth`, `apply`, Kong.

Note: `app` does not import `apply`. The `apply.Apply(...)` call stays in CLI's `runConfigApplyWithPairAndOpts` — it's a single clean call that takes domain types already prepared by `app.LoadAndFetch` + `diff.ComputeWithOptions`. Moving it to `app` would require `app` to accept an `apply.Applier`, which adds coupling for no testability gain.

---

## Tasks

Each task is independently committable. Tasks 1-2 are the foundation. Tasks 3-6 are independent moves that depend only on Task 1. Task 7 is cleanup. Task 8 is docs.

### Task 1: Create `internal/app/` with core types and config pipeline

**Files:**

- Create: `internal/app/app.go`
- Delete: `internal/cli/config_common.go` (contents move to app)
- Modify: `internal/cli/config_get.go` (remove `configFetcher` interface definition, import from app)
- Modify: `internal/cli/config_diff.go` (use `*app.ConfigPair`)
- Modify: `internal/cli/config_apply.go` (use `*app.ConfigPair`)
- Modify: `internal/cli/apply_cmd.go` (call `app.LoadAndFetch` directly)

**What:** Create the package with `ConfigFetcher`, `ConfigPair`, `LoadAndFetch`, `ScopeDesiredByPath`, and `splitDotPath`.

`configPair` has only 3 consuming sites (`config_common.go`, `config_apply.go:40,44`), so delete the local type and use `*app.ConfigPair` everywhere. This is cleaner than a wrapper.

`configFetcher` is defined in `config_get.go:16-19` and used by `config_get.go`, `config_common.go`, `config_diff.go`, `config_apply.go`. Replace all uses with `app.ConfigFetcher`. The `defaultConfigFetcher` struct stays in CLI — it's the adapter that implements `app.ConfigFetcher` using `railway` + `interactivePicker`.

`emitWarnings` stays in CLI. It currently takes `*configPair` — update it to take `*app.ConfigPair`. Its signature is `func emitWarnings(pair *app.ConfigPair, quiet int, configDir string)`.

The `loadAndFetch` body moves verbatim to `app.LoadAndFetch`. The CLI file `config_common.go` shrinks to just `emitWarnings` (or `emitWarnings` moves to wherever it's called from — it's used by `config_diff.go:42`, `config_apply.go:34`, and `apply_cmd.go:45`).

**Current `ApplyCmd.Run()` calls `loadAndFetch` directly** (not through `RunConfigApply`), then passes the pair to `runConfigApplyWithPairAndOpts`. After this change, it calls `app.LoadAndFetch` directly.

**Verification:**

- `go build ./...`
- `go test -race -count=1 ./...`

**Commit:** `extract app package with config pipeline and core types`

---

### Task 2: Move adopt merge logic to `app`

**Files:**

- Create: `internal/app/adopt.go`
- Create: `internal/app/adopt_test.go`
- Modify: `internal/cli/adopt.go` (call `app.AdoptMerge`, `app.ScopeLiveByPath`, etc.)
- Delete: `internal/cli/adopt_test.go` (tests move to app)

**What:** Move these pure functions from `cli/adopt.go` to `app/adopt.go`:

- `adoptMerge` → `app.AdoptMerge` (lines 244-351)
- `scopeLiveByPath` → `app.ScopeLiveByPath` (lines 204-234)
- `desiredServiceToLive` → `app.DesiredServiceToLive` (lines 355-389)
- `desiredDeployToLive` → `app.DesiredDeployToLive` (lines 392-418)
- `findDesiredService` → `app.FindDesiredService` (lines 421-428)
- `joinServiceNames` → `app.JoinServiceNames` (lines 431-438)

All are pure functions on `config` types. They import only `config`, `sort`, `strings` (stdlib).

`adopt_test.go` is `package cli` (internal tests calling unexported functions). The tests move to `internal/app/adopt_test.go` as `package app_test`, calling the now-exported functions. All 11 tests: `TestAdoptMerge_*` (7), `TestScopeLiveByPath` (1), `TestDesiredServiceToLive` (2), `TestDesiredServiceToLive_NilLive` (1).

`AdoptCmd.Run()` stays in CLI — it handles prompting, file I/O, and output.

**Verification:**

- `go build ./...`
- `go test -race -count=1 ./...`

**Commit:** `move adopt merge logic and tests to app package`

---

### Task 3: Move service target resolution to `app`

**Files:**

- Create: `internal/app/targets.go`
- Modify: `internal/cli/deploy.go` (call `app.ResolveServiceTargets`)
- Modify: `internal/cli/deployment_actions.go` (same)
- Modify: `internal/cli/logs.go` (same)
- Modify: `internal/cli/status.go` (same)

**What:** Move `serviceTarget` and `resolveServiceTargets` to `app/targets.go`.

`resolveServiceTargets` currently takes `*railway.Client` and calls `railway.FetchLiveConfig` directly (line 110 of `deploy.go`). Change it to take `app.ConfigFetcher` instead:

```go
// ServiceTarget holds a resolved service name and ID.
type ServiceTarget struct {
    Name string
    ID   string
}

// ResolveServiceTargets maps service name arguments to name+ID pairs.
// If no names are given, returns all services in the project.
func ResolveServiceTargets(ctx context.Context, fetcher ConfigFetcher, projectID, envID string, serviceNames []string) ([]ServiceTarget, error) {
    live, err := fetcher.Fetch(ctx, projectID, envID, nil)
    if err != nil {
        return nil, fmt.Errorf("fetching services: %w", err)
    }
    // ... (rest of body unchanged, but uses ServiceTarget instead of serviceTarget)
}
```

There are 4 callers, all in CLI: `deploy.go:36`, `deployment_actions.go:99`, `logs.go:58`, `status.go:34`. Each currently constructs a `*railway.Client`, so they need to wrap it in a `defaultConfigFetcher` (or pass one they already have). Check each callsite:

- `deploy.go` (`DeployCmd.Run`): has `client` — wrap in `&defaultConfigFetcher{client: client}`
- `deployment_actions.go` (`runDeploymentAction`): has `client` — same
- `logs.go` (`LogsCmd.Run`): has `client` — same
- `status.go` (`StatusCmd.Run`): has `client` — same

**Verification:**

- `go build ./...`
- `go test -race -count=1 ./...`

**Commit:** `move service target resolution to app package`

---

### Task 4: Move show/get helpers to `app`

**Files:**

- Create: `internal/app/show.go`
- Modify: `internal/cli/config_get.go`

**What:** Move `lookupKey` and `filterSection` to `app/show.go`. Both are pure functions on `config.LiveConfig` and `config.Path`. No CLI imports.

```go
// LookupKey retrieves a single value from the config for a fully-qualified path.
func LookupKey(cfg config.LiveConfig, p config.Path) (string, bool) { ... }

// FilterSection returns a copy of cfg containing only the requested section.
func FilterSection(cfg config.LiveConfig, p config.Path) config.LiveConfig { ... }
```

`RunConfigGet` stays in CLI — it handles masking, output formatting, and the `globals.Output` check.

**Verification:**

- `go build ./...`
- `go test -race -count=1 ./...`

**Commit:** `move show/get helpers to app package`

---

### Task 5: Move shared init/adopt helpers to `app`

**Files:**

- Create: `internal/app/inithelpers.go`
- Modify: `internal/cli/config_init.go` (call app functions)
- Modify: `internal/cli/adopt.go` (call app functions)

**What:** Move `renderEnvFile` and `ensureGitignoreHasLine` to app. Both are used by two callers (`config_init.go` and `adopt.go`), so they're genuinely shared, not single-caller utilities.

- `renderEnvFile` depends on `config.CollectSecrets` + stdlib (`sort`, `strings`, `fmt`). → `app.RenderEnvFile`
- `ensureGitignoreHasLine` depends only on stdlib (`os`, `path/filepath`, `strings`). → `app.EnsureGitignoreHasLine`

**Verification:**

- `go build ./...`
- `go test -race -count=1 ./...`

**Commit:** `move shared init/adopt helpers to app package`

---

### Task 6: Add `LoadAndFetch` unit tests

**Files:**

- Create: `internal/app/app_test.go`

**What:** `loadAndFetch` currently has zero direct unit tests — it's only exercised indirectly through `RunConfigDiff` and `RunConfigApply` tests. Now that it lives in `app` as an exported function, add direct tests:

1. Test basic load + resolve + fetch pipeline with a fake `ConfigFetcher`
2. Test that config-file project/environment names are used as fallback for resolution
3. Test that `--service` filter narrows the desired config
4. Test error propagation from `LoadCascade`, `Interpolate`, `Resolve`, `Fetch`

These tests create temp directories with TOML config files and use a `fakeConfigFetcher` that returns canned responses. The existing test helpers in `cli/*_test.go` (like `fakeConfigFetcher` and `capturingFetcher`) show the pattern.

Also add tests for `ScopeDesiredByPath` (currently untested directly).

**Verification:**

- `go test -race -count=1 ./internal/app/`

**Commit:** `add unit tests for app.LoadAndFetch and ScopeDesiredByPath`

---

### Task 7: Clean up `cli` package

**Files:**

- Modify: `internal/cli/config_common.go` (may be empty or just `emitWarnings`)
- Modify: `internal/cli/config_get.go` (remove dead interface/type definitions)
- Audit: all `internal/cli/*.go` for stale imports of moved symbols

**What:** After Tasks 1-6, review `config_common.go`. If it only contains `emitWarnings`, consider whether it should stay there or move inline to callers (it's called from 3 places: `config_diff.go:42`, `config_apply.go:34`, `apply_cmd.go:45`). If kept, the file name `config_common.go` is misleading since the "common" pipeline logic moved — rename to `warnings.go` or move `emitWarnings` to wherever makes sense.

Remove any orphaned imports. Run `go vet ./...` to catch issues.

**Verification:**

- `go build ./...`
- `go vet ./...`
- `go test -race -count=1 ./...`

**Commit:** `clean up cli package after app extraction`

---

### Task 8: Document package layout in ARCHITECTURE.md

**Files:**

- Modify: `docs/ARCHITECTURE.md`

**What:** Add a "Package Layout" section near the top describing the layer hierarchy and dependency rules:

```text
internal/
  app/        — application orchestration (load, resolve, scope, merge, target resolution)
  apply/      — mutation execution (Applier interface + Railway adapter)
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

Dependency rules:

- `app` depends on `config` and `diff`. Does not import `cli`, `prompt`, `railway`, `auth`, or `apply`.
- `cli` depends on everything (it's the outermost layer).
- `railway` depends on `config` and `auth`. Does not import `cli` or `prompt`.
- `apply` depends on `config` and `diff`. Does not import `cli`, `railway`, or `prompt`. Railway coupling is via the `Applier` interface.

**Commit:** `document package layout and dependency rules in ARCHITECTURE.md`

---

## What This Plan Does NOT Do

1. **Extract `Run*` functions from CLI.** Every `Run*` function (`RunConfigGet`, `RunConfigDiff`, `RunConfigApply`, `RunDeploy`, `RunStatus`, etc.) interleaves orchestration with `if isStructuredOutput { json } else { text }` format branching. Separating them requires returning structured result types from app — a valid follow-up, but mechanical and high-churn.

2. **Create interfaces for `config.LoadCascade` or `diff.ComputeWithOptions`.** These are stable domain functions, not swappable adapters. `app` calls them directly.

3. **Move `apply` or `diff` packages.** Already correctly layered.

4. **Unify `initResolver` and `ConfigFetcher`.** They have fundamentally different shapes: `ConfigFetcher.Resolve` is a single combined call; `initResolver` has step-by-step methods (`FetchWorkspaces`, `FetchProjects`, `FetchEnvironments`) returning `[]prompt.Item` for interactive wizard flows. At most `FetchLiveState`/`Fetch` overlap, but unifying them isn't worth the abstraction.

5. **Move single-caller pure utilities.** `parseTimeArg`/`parseRelativeDuration` (only `logs.go`), `databaseImage` (only `new.go`) stay in CLI. Not worth the package hop.
