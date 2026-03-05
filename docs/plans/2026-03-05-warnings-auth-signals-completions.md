# Warnings, Auth Fixes, Signal Handling & Shell Completions

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Fix all P1 and P2 issues (warning system gaps, validation wiring, live resource limits, auth timeout/output, signal handling) and add shell completions.

**Architecture:** The warning system in `validate.go` needs 8 new warning codes (W010 demoted from error is out of scope — see decision below). Validation must be wired into `config diff` and `config apply` via the shared `loadAndFetch` pipeline. Live resource limits require a new GraphQL query, genqlient binding, model expansion, and diff fix. Auth commands need `TimeoutContext` and `io.Writer` injection. Signal handling adds `signal.NotifyContext` in `main.go`. Shell completions use `jotaen/kong-completion`.

**Tech Stack:** Go 1.23+, Kong v1.14, genqlient, `jotaen/kong-completion`

---

## Decisions

- **W010 (unresolved local interpolation):** Currently a hard error in `Interpolate()`. Demoting to a warning would change error semantics and risk silent deployment of broken configs. **Keep as error, remove from WARNINGS.md.** The docs will note that unresolved `${VAR}` is a fatal error, not a warning.
- **W001 (unknown top-level key):** Already enforced as a parse error at `parse.go:109`. Non-table top-level keys that aren't known settings produce `"unrecognised config key"`. **Remove from WARNINGS.md** — this is intentionally a hard error.
- **W002 (unknown key in service block):** Service blocks accept `variables`, `resources`, `deploy`. Unknown sub-keys are silently ignored by the current parser. **Implement as a warning** — iterate raw map keys in `parseService` and warn on unrecognized ones.
- **`ServiceInstanceLimit` scalar:** Railway returns this as an opaque JSON blob. We'll add a genqlient binding as `map[string]interface{}` and extract `vCPUs` and `memoryGB` float fields from it.
- **`serviceInstanceLimits` vs `serviceInstanceLimitOverride`:** Use `serviceInstanceLimits` (merged, includes plan defaults) because that's what the user sees in the Railway dashboard and what the diff should compare against.
- **Auth `io.Writer` refactor:** The `auth` package's `Login()` function prints progress to stdout. Rather than threading `io.Writer` through the entire auth package API surface, we'll refactor only the CLI-layer auth commands to use `io.Writer` and add an `io.Writer` parameter to `auth.Login()` for progress messages.

---

### Task 1: Implement missing warning codes (W002, W021, W031, W050, W051, W060)

**Files:**

- Modify: `internal/config/validate.go`
- Modify: `internal/config/validate_test.go`
- Modify: `internal/config/parse.go` (W002 — pass unknown keys info through)
- Modify: `internal/config/desired.go` (W021 — track per-file origins)
- Modify: `internal/config/merge.go` (W021 — track overrides)
- Modify: `internal/config/load.go` (W051 — gitignore check)

#### W031: Invalid variable name characters

Add to `checkVarName()` in `validate.go` after the W030 check. A variable name is invalid if it contains spaces, or characters outside `[A-Za-z0-9_]`.

```go
// validate.go — add regex at package level
var validVarNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// In checkVarName, after W030:
if !validVarNameRe.MatchString(name) {
    warnings = append(warnings, Warning{
        Code:    "W031",
        Message: fmt.Sprintf("variable name %q contains invalid characters (expected [A-Za-z0-9_])", name),
        Path:    path,
    })
}
```

Test: config with variable `"MY VAR"` and `"PORT@8080"` → W031.

#### W060: Reference to unknown service

Scan all variable values for `${{service.VAR}}` patterns and check whether `service` is a known service name (from `cfg.Services` keys).

```go
// validate.go — add regex at package level
var serviceRefRe = regexp.MustCompile(`\$\{\{([a-zA-Z_][a-zA-Z0-9_-]*)\.[a-zA-Z_][a-zA-Z0-9_]*\}\}`)

// In Validate(), after existing checks, extract all service refs:
knownServices := make(map[string]bool, len(cfg.Services))
for name := range cfg.Services {
    knownServices[name] = true
}
// Check shared vars
if cfg.Shared != nil {
    for varName, value := range cfg.Shared.Vars {
        for _, ref := range serviceRefRe.FindAllStringSubmatch(value, -1) {
            if !knownServices[ref[1]] {
                warnings = append(warnings, Warning{
                    Code:    "W060",
                    Message: fmt.Sprintf("reference ${{%s...}} refers to service %q not defined in config", ref[1], ref[1]),
                    Path:    "shared.variables." + varName,
                })
            }
        }
    }
}
// Same for service vars
for svcName, svc := range cfg.Services {
    for varName, value := range svc.Variables {
        for _, ref := range serviceRefRe.FindAllStringSubmatch(value, -1) {
            if !knownServices[ref[1]] {
                warnings = append(warnings, Warning{
                    Code:    "W060",
                    Message: fmt.Sprintf("reference ${{%s...}} refers to service %q not defined in config", ref[1], ref[1]),
                    Path:    svcName + ".variables." + varName,
                })
            }
        }
    }
}
```

Test: `DB_URL = "${{postgres.DATABASE_URL}}"` where `postgres` is not in `cfg.Services` → W060.

#### W050: Hardcoded secret in config

Reuse the existing `Masker` from `mask.go`. A value is a hardcoded secret if `masker.MaskValue(name, value)` returns `MaskedValue` AND the value doesn't contain `${` (which would mean it's interpolated at runtime).

```go
// In Validate(), add masker parameter or create one internally:
masker := NewMasker(cfg.SensitiveKeywords, cfg.SensitiveAllowlist)

// For each variable (shared and per-service), after existing checks:
if value != "" && !strings.Contains(value, "${") && masker.MaskValue(name, value) == MaskedValue {
    warnings = append(warnings, Warning{
        Code:    "W050",
        Message: fmt.Sprintf("variable %q appears to contain a hardcoded secret — consider using ${ENV_VAR} interpolation", name),
        Path:    path,
    })
}
```

Test: `API_KEY = "hardcoded_value_that_is_long_enough_to_trigger"` (keyword match + high entropy) → W050. But `API_KEY = "${API_KEY}"` → no W050.

#### W051: Local override file not gitignored

Check whether `fat-controller.local.toml` exists in the config directory and whether a `.gitignore` file contains it. This runs during validation, so add a new parameter or a separate function.

Approach: Add a `ValidateFiles(dir string)` function that checks filesystem conditions. Call it from `Validate()` wouldn't work since `Validate` doesn't know about files. Instead, call it from the CLI layer (`RunConfigValidate`, and the new validation wiring in `loadAndFetch`).

```go
// validate.go — new function
func ValidateFiles(dir string) []Warning {
    var warnings []Warning
    localPath := filepath.Join(dir, LocalConfigFile)
    if _, err := os.Stat(localPath); err != nil {
        return warnings // no local file, nothing to check
    }
    gitignorePath := filepath.Join(dir, ".gitignore")
    data, err := os.ReadFile(gitignorePath)
    if err != nil {
        // No .gitignore — warn
        warnings = append(warnings, Warning{
            Code:    "W051",
            Message: fmt.Sprintf("%s exists but is not in .gitignore — secrets may be committed", LocalConfigFile),
            Path:    LocalConfigFile,
        })
        return warnings
    }
    for _, line := range strings.Split(string(data), "\n") {
        line = strings.TrimSpace(line)
        if line == LocalConfigFile || line == "**/"+LocalConfigFile {
            return warnings // found, no warning
        }
    }
    warnings = append(warnings, Warning{
        Code:    "W051",
        Message: fmt.Sprintf("%s exists but is not in .gitignore — secrets may be committed", LocalConfigFile),
        Path:    LocalConfigFile,
    })
    return warnings
}
```

Test: create temp dir with `fat-controller.local.toml` but no `.gitignore` → W051. Add `.gitignore` with the filename → no W051.

#### W002: Unknown key in service block

The parser at `parse.go:125` (`parseService`) processes `raw map[string]any`. Valid sub-keys are `"variables"`, `"resources"`, `"deploy"`. Unknown keys are silently ignored. To produce W002 warnings, we need to surface unknown keys.

Approach: Add a `UnknownServiceKeys` field to `DesiredService` that `parseService` populates, then check in `Validate`.

```go
// desired.go — add field:
type DesiredService struct {
    Variables          map[string]string
    Resources          *DesiredResources
    Deploy             *DesiredDeploy
    UnknownKeys        []string `toml:"-"` // populated by parser, used by validator
}

// parse.go — in parseService(), after processing known keys:
var knownServiceKeys = map[string]bool{
    "variables": true,
    "resources": true,
    "deploy":    true,
}

// In parseService, before return:
for key := range raw {
    if !knownServiceKeys[key] {
        svc.UnknownKeys = append(svc.UnknownKeys, key)
    }
}

// validate.go — in the service loop, before W003:
for _, key := range svc.UnknownKeys {
    warnings = append(warnings, Warning{
        Code:    "W002",
        Message: fmt.Sprintf("unknown key %q in service %q (expected: variables, resources, deploy)", key, svcName),
        Path:    svcName + "." + key,
    })
}
```

Test: config with `[api]\nfoo = "bar"` (unknown key `foo` in service `api`) → W002.

#### W021: Variable overridden by local file

This requires tracking which variables came from which file. During merge, record when a variable in a later config overrides one from an earlier config.

Approach: Add an `Overrides` field to `DesiredConfig` that merge populates.

```go
// desired.go — add:
type Override struct {
    Path    string // dot-path e.g. "api.variables.PORT"
    Source  string // "local" or the file that overrode it
}

type DesiredConfig struct {
    // ... existing fields ...
    Overrides []Override `toml:"-"` // populated by Merge, checked by Validate
}
```

Actually, this is complex because `Merge` doesn't know file names. Simpler approach: `LoadConfigs` knows the file order (base, local, extras). After merge, compare the base config's variables against the merged result. If a variable exists in both base and local (or extra), that's an override.

Simpler still: compare configs pairwise. In `LoadConfigs`, after parsing each overlay, check which variables it overrides from the accumulated result so far.

```go
// load.go — after merging, track overrides:
// After base is parsed (line 43), track its vars.
// After local is parsed (line 52), check for overlaps.
// Store overrides on the merged result.
```

This changes `LoadConfigs` to return overrides alongside the merged config. Best approach: add `Overrides []Override` to `DesiredConfig` and populate during `LoadConfigs`.

```go
// load.go — new helper
func findOverrides(base, overlay *DesiredConfig, sourceName string) []Override {
    var overrides []Override
    if base.Shared != nil && overlay.Shared != nil {
        for k := range overlay.Shared.Vars {
            if _, ok := base.Shared.Vars[k]; ok {
                overrides = append(overrides, Override{
                    Path: "shared.variables." + k, Source: sourceName,
                })
            }
        }
    }
    for svcName, overlaySvc := range overlay.Services {
        baseSvc, ok := base.Services[svcName]
        if !ok || baseSvc == nil {
            continue
        }
        for k := range overlaySvc.Variables {
            if _, ok := baseSvc.Variables[k]; ok {
                overrides = append(overrides, Override{
                    Path: svcName + ".variables." + k, Source: sourceName,
                })
            }
        }
    }
    return overrides
}
```

In `LoadConfigs`, after merging local:

```go
if local != nil {
    result.Overrides = append(result.Overrides, findOverrides(base, local, "local override")...)
}
```

In `Validate`:

```go
for _, ov := range cfg.Overrides {
    warnings = append(warnings, Warning{
        Code:    "W021",
        Message: fmt.Sprintf("%s is overridden by %s", ov.Path, ov.Source),
        Path:    ov.Path,
    })
}
```

Test: base config with `PORT = "3000"`, local config with `PORT = "4000"` → W021.

**Step 1:** Write tests for all 6 new warning codes.

**Step 2:** Run tests, verify they fail.

**Step 3:** Implement W031, W060, W050 in `validate.go` (pure validation, no model changes needed).

**Step 4:** Implement W002 in `parse.go` + `desired.go` + `validate.go`.

**Step 5:** Implement W021 in `load.go` + `desired.go` + `validate.go`.

**Step 6:** Implement W051 in `validate.go` (new `ValidateFiles` function).

**Step 7:** Run tests, verify they pass.

**Step 8:** Run `go test ./internal/config/...` to check nothing is broken.

**Step 9:** Commit: `feat: implement warning codes W002, W021, W031, W050, W051, W060`

---

### Task 2: Update WARNINGS.md to match reality

**Files:**

- Modify: `docs/WARNINGS.md`

**Step 1:** Remove W001 row (now a parse error, not a warning). Add a note: "Unknown non-table top-level keys are rejected as parse errors."

**Step 2:** Remove W010 row (unresolved `${VAR}` is a fatal error). Add a note: "Unresolved local environment variables (`${VAR}` where `VAR` is not set) are treated as errors, not warnings."

**Step 3:** Verify all remaining rows (W002, W003, W011, W012, W020, W021, W030, W031, W040, W041, W050, W051, W060) match the implementation.

**Step 4:** Commit: `docs: align WARNINGS.md with implemented warning codes`

---

### Task 3: Wire validation into `config diff` and `config apply`

**Files:**

- Modify: `internal/cli/config_common.go`
- Modify: `internal/cli/config_diff.go`
- Modify: `internal/cli/config_apply.go`
- Modify: `internal/cli/config_validate.go` (call `ValidateFiles` too)
- Add tests: `internal/cli/config_diff_test.go` and `internal/cli/config_apply_test.go` (check warnings appear in stderr)

After `loadAndFetch()` returns, extract live service names and run validation. Emit warnings to stderr (not stdout, which is for data output). Respect `--quiet` to suppress warnings. `config validate` ignores `--quiet` (per docs).

```go
// config_common.go — new function
func emitWarnings(pair *configPair, globals *Globals, configDir string) {
    if globals.Quiet {
        return
    }
    // Extract live service names for W040.
    var liveNames []string
    for name := range pair.Live.Services {
        liveNames = append(liveNames, name)
    }

    warnings := config.Validate(pair.Desired, liveNames)
    warnings = append(warnings, config.ValidateFiles(configDir)...)

    // Filter suppressed warnings (already done inside Validate, but ValidateFiles
    // warnings need filtering too).
    suppressed := make(map[string]bool, len(pair.Desired.SuppressWarnings))
    for _, code := range pair.Desired.SuppressWarnings {
        suppressed[code] = true
    }

    for _, w := range warnings {
        if suppressed[w.Code] {
            continue
        }
        path := ""
        if w.Path != "" {
            path = " (" + w.Path + ")"
        }
        slog.Warn(fmt.Sprintf("[%s]%s %s", w.Code, path, w.Message))
    }
}
```

Call `emitWarnings(pair, globals, wd)` in both `RunConfigDiff` and `RunConfigApply` (after `loadAndFetch`, before `diff.Compute`). In `config_diff.go:Run()` and `config_apply.go:Run()`, pass `wd` to the testable core or call `emitWarnings` directly.

Also update `RunConfigValidate` to call `ValidateFiles(configDir)` and append those warnings.

**Step 1:** Write test: `config diff` with an unknown service name → stderr contains W040 warning.

**Step 2:** Run test, verify it fails.

**Step 3:** Add `emitWarnings` to `config_common.go`. Wire into `RunConfigDiff` and `runConfigApplyWithPair` (or the `Run` methods). Update `RunConfigValidate` to call `ValidateFiles`.

**Step 4:** Run tests, verify they pass.

**Step 5:** Run `go test ./internal/cli/...`.

**Step 6:** Commit: `feat: emit validation warnings during config diff and config apply`

---

### Task 4: Fetch live resource limits and fix diff

**Files:**

- Modify: `internal/railway/operations.graphql` (add `ServiceInstanceLimits` query)
- Modify: `.config/genqlient.yaml` (add `ServiceInstanceLimit` scalar binding)
- Regenerate: `internal/railway/generated.go` (run genqlient)
- Modify: `internal/config/model.go` (add `Resources` to `ServiceConfig`)
- Modify: `internal/railway/state.go` (fetch limits per service)
- Modify: `internal/diff/diff.go` (fix `diffResources` to compare against live)
- Modify: `internal/diff/diff_test.go` (update resource diff tests)

#### Step 1: Add GraphQL query

```graphql
# operations.graphql — add:
query ServiceInstanceLimits($environmentId: String!, $serviceId: String!) {
  serviceInstanceLimits(environmentId: $environmentId, serviceId: $serviceId)
}
```

#### Step 2: Add genqlient binding

```yaml
# .config/genqlient.yaml — add under bindings:
  ServiceInstanceLimit:
    type: map[string]interface{}
```

#### Step 3: Regenerate

```bash
mise run generate  # or: go generate ./internal/railway/...
```

#### Step 4: Add resource fields to live config model

```go
// model.go — add to ServiceConfig:
type ServiceConfig struct {
    ID        string
    Name      string
    Variables map[string]string
    Deploy    Deploy
    VCPUs     *float64 // live resource limit
    MemoryGB  *float64 // live resource limit
}
```

#### Step 5: Fetch limits in `FetchLiveConfig`

```go
// state.go — after fetching deploy settings (line 64), add:
limits, err := ServiceInstanceLimits(ctx, client.GQL(), environmentID, edge.Node.Id)
if err != nil {
    slog.Debug("could not fetch resource limits", "service", edge.Node.Name, "error", err)
    // Non-fatal: resource limits may not be available for all services
} else if limits.ServiceInstanceLimits != nil {
    if v, ok := limits.ServiceInstanceLimits["vCPUs"]; ok {
        if f, ok := toFloat64(v); ok {
            svc.VCPUs = &f
        }
    }
    if v, ok := limits.ServiceInstanceLimits["memoryGB"]; ok {
        if f, ok := toFloat64(v); ok {
            svc.MemoryGB = &f
        }
    }
}
```

Need a `toFloat64` helper in `state.go` (or import from config — but better to keep it local to avoid circular imports):

```go
func toFloat64(v interface{}) (float64, bool) {
    switch n := v.(type) {
    case float64:
        return n, true
    case int64:
        return float64(n), true
    case json.Number:
        f, err := n.Float64()
        return f, err == nil
    default:
        return 0, false
    }
}
```

#### Step 6: Fix `diffResources` to compare against live

```go
// diff.go — change signature and implementation:
func diffResources(desired *config.DesiredResources, live *config.ServiceConfig) []Change {
    var changes []Change
    if desired.VCPUs != nil {
        liveVal := ""
        if live != nil && live.VCPUs != nil {
            liveVal = fmt.Sprintf("%.1f", *live.VCPUs)
        }
        desiredVal := fmt.Sprintf("%.1f", *desired.VCPUs)
        if liveVal != desiredVal {
            action := ActionUpdate
            if liveVal == "" {
                action = ActionCreate
            }
            changes = append(changes, Change{
                Key:          config.KeyVCPUs,
                Action:       action,
                LiveValue:    liveVal,
                DesiredValue: desiredVal,
            })
        }
    }
    if desired.MemoryGB != nil {
        liveVal := ""
        if live != nil && live.MemoryGB != nil {
            liveVal = fmt.Sprintf("%.1f", *live.MemoryGB)
        }
        desiredVal := fmt.Sprintf("%.1f", *desired.MemoryGB)
        if liveVal != desiredVal {
            action := ActionUpdate
            if liveVal == "" {
                action = ActionCreate
            }
            changes = append(changes, Change{
                Key:          config.KeyMemoryGB,
                Action:       action,
                LiveValue:    liveVal,
                DesiredValue: desiredVal,
            })
        }
    }
    return changes
}
```

Update the call site in `diffService` to pass the live service:

```go
// diff.go line 178-179 — change:
if desired.Resources != nil {
    sd.Settings = append(sd.Settings, diffResources(desired.Resources, live)...)
}
```

#### Step 7: Update diff tests

Add test: desired `vcpus = 2.0` with live `VCPUs = 2.0` → no diff. Desired `vcpus = 4.0` with live `VCPUs = 2.0` → update diff.

#### Step 8: Run `go test ./internal/diff/... ./internal/railway/...`

#### Step 9: Commit: `feat: fetch live resource limits and fix resource diffing`

---

### Task 5: Fix auth timeout context

**Files:**

- Modify: `internal/cli/auth.go`
- Modify: `internal/cli/client.go`

Replace `context.Background()` with `globals.TimeoutContext(context.Background())` in all auth commands and `newClient`.

```go
// auth.go — AuthLoginCmd.Run:
func (c *AuthLoginCmd) Run(globals *Globals) error {
    slog.Debug("starting auth login")
    ctx, cancel := globals.TimeoutContext(context.Background())
    defer cancel()
    oauth := auth.NewOAuthClient()
    store := auth.NewTokenStore(
        auth.WithFallbackPath(platform.AuthFilePath()),
    )
    return auth.Login(ctx, oauth, store, auth.OpenBrowser)
}

// auth.go — AuthLogoutCmd.Run: no context needed (local operation only)

// auth.go — AuthStatusCmd.Run:
func (c *AuthStatusCmd) Run(globals *Globals) error {
    slog.Debug("checking auth status")
    ctx, cancel := globals.TimeoutContext(context.Background())
    defer cancel()
    store := auth.NewTokenStore(
        auth.WithFallbackPath(platform.AuthFilePath()),
    )
    resolved, err := auth.ResolveAuth(ctx, globals.Token, store)
    // ... rest uses ctx instead of context.Background()
    info, err := oauth.FetchUserInfo(ctx)
    // ...
}

// client.go:
func newClient(globals *Globals) (*railway.Client, error) {
    slog.Debug("creating Railway client")
    ctx, cancel := globals.TimeoutContext(context.Background())
    defer cancel()
    store := auth.NewTokenStore(auth.WithFallbackPath(platform.AuthFilePath()))
    resolved, err := auth.ResolveAuth(ctx, globals.Token, store)
    // ...
}
```

**Step 1:** Make the changes in `auth.go` and `client.go`.

**Step 2:** Run `go test ./internal/cli/...` — existing tests should still pass.

**Step 3:** Commit: `fix: use timeout context in auth commands and client creation`

---

### Task 6: Make auth output testable via `io.Writer`

**Files:**

- Modify: `internal/cli/auth.go` (accept `io.Writer`, write to it)
- Modify: `internal/cli/cli.go` (add `output` field to auth command structs)
- Modify: `internal/auth/login.go` (accept `io.Writer` for progress messages)
- Add/modify tests: `internal/cli/auth_test.go`

#### CLI auth commands

Refactor auth commands to have testable cores like other commands:

```go
// auth.go — RunAuthLogin testable core:
func RunAuthLogin(ctx context.Context, globals *Globals, out io.Writer) error {
    slog.Debug("starting auth login")
    oauth := auth.NewOAuthClient()
    store := auth.NewTokenStore(
        auth.WithFallbackPath(platform.AuthFilePath()),
    )
    return auth.Login(ctx, oauth, store, auth.OpenBrowser, out)
}

func (c *AuthLoginCmd) Run(globals *Globals) error {
    ctx, cancel := globals.TimeoutContext(context.Background())
    defer cancel()
    return RunAuthLogin(ctx, globals, os.Stdout)
}

// Similarly for RunAuthLogout and RunAuthStatus
```

#### auth.Login — add `io.Writer`

```go
// login.go — change signature:
func Login(ctx context.Context, oauth *OAuthClient, store *TokenStore, openBrowser BrowserOpener, out io.Writer) error

func loginAttempt(ctx context.Context, oauth *OAuthClient, store *TokenStore, openBrowser BrowserOpener, forceNewClient bool, out io.Writer) error
```

Replace all `fmt.Println(...)` with `fmt.Fprintln(out, ...)` and `fmt.Printf(...)` with `fmt.Fprintf(out, ...)`.

**Step 1:** Update `auth.Login` and `loginAttempt` signatures, replace `fmt.Println` → `fmt.Fprintln(out, ...)`.

**Step 2:** Update `cli/auth.go` — add `RunAuthLogin`, `RunAuthLogout`, `RunAuthStatus` testable cores with `io.Writer`.

**Step 3:** Add basic tests in `cli/auth_test.go` verifying output contains expected strings (e.g., "Logged out successfully").

**Step 4:** Run `go test ./internal/cli/... ./internal/auth/...`.

**Step 5:** Commit: `refactor: make auth commands testable with injectable io.Writer`

---

### Task 7: Add signal handling (SIGINT/SIGTERM)

**Files:**

- Modify: `cmd/fat-controller/main.go`

Add `signal.NotifyContext` to create a cancellable context that gets cancelled on SIGINT/SIGTERM. Thread this context through to commands.

The challenge: Kong's `ctx.Run()` doesn't accept a `context.Context`. Commands create their own contexts via `globals.TimeoutContext(context.Background())`. We need the signal context to be the parent.

Approach: Store a base context on `Globals` that commands use as the parent for `TimeoutContext`.

```go
// cli.go — add field and method:
type Globals struct {
    // ... existing fields ...
    BaseCtx context.Context `kong:"-"` // set by main, cancelled on signal
}

func (g *Globals) TimeoutContext() (context.Context, context.CancelFunc) {
    ctx := g.BaseCtx
    if ctx == nil {
        ctx = context.Background()
    }
    if g.Timeout > 0 {
        return context.WithTimeout(ctx, g.Timeout)
    }
    return ctx, func() {}
}
```

Wait — changing `TimeoutContext` signature from `(context.Context)` to `()` would break all callers. Better: set `BaseCtx` but keep `TimeoutContext(ctx)` — callers pass `globals.BaseCtx` instead of `context.Background()`.

Actually simplest: keep `TimeoutContext(parent)` but have all callers use `globals.BaseCtx` as the parent. Set `BaseCtx` in main.

```go
// main.go:
import "os/signal"

func main() {
    applyColorMode()

    ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
    defer stop()

    var c cli.CLI
    kongCtx := kong.Parse(&c, ...)
    slog.SetDefault(c.Logger())

    c.Globals.BaseCtx = ctx

    if err := kongCtx.Run(&c.Globals); err != nil {
        // Don't print error for context cancellation (user pressed Ctrl+C)
        if !errors.Is(err, context.Canceled) {
            fmt.Fprintln(os.Stderr, "error:", err)
        }
        os.Exit(1)
    }
}
```

Then update all `globals.TimeoutContext(context.Background())` calls to `globals.TimeoutContext(globals.BaseCtx)`.

There are ~7 callers: `config_apply.go`, `config_diff.go`, `config_get.go`, `config_set.go`, `config_delete.go`, `config_init.go`, and the auth commands (after Task 5). Each needs `context.Background()` replaced with `globals.BaseCtx`.

Also need a nil guard in `TimeoutContext`:

```go
func (g *Globals) TimeoutContext(parent context.Context) (context.Context, context.CancelFunc) {
    if parent == nil {
        parent = context.Background()
    }
    if g.Timeout > 0 {
        return context.WithTimeout(parent, g.Timeout)
    }
    return parent, func() {}
}
```

**Step 1:** Add `BaseCtx context.Context` field to `Globals` in `cli.go`. Add nil guard to `TimeoutContext`.

**Step 2:** Update `main.go` — add `signal.NotifyContext`, set `c.Globals.BaseCtx = ctx`, suppress `context.Canceled` errors.

**Step 3:** Replace `context.Background()` with `globals.BaseCtx` in all command `Run` methods that call `globals.TimeoutContext(...)`.

**Step 4:** Run `go test ./...` — existing tests pass (BaseCtx is nil → falls back to `context.Background()`).

**Step 5:** Commit: `feat: add SIGINT/SIGTERM signal handling with graceful cancellation`

---

### Task 8: Add shell completions

**Files:**

- Modify: `go.mod` (add `github.com/jotaen/kong-completion`)
- Modify: `internal/cli/cli.go` (add `Completion` field to `CLI`)
- Modify: `cmd/fat-controller/main.go` (split `kong.Parse` into `kong.Must` + `Register` + `Parse`)

#### Step 1: Add dependency

```bash
go get github.com/jotaen/kong-completion
```

#### Step 2: Add Completion command to CLI struct

```go
// cli.go:
import kongcompletion "github.com/jotaen/kong-completion"

type CLI struct {
    Globals `kong:"embed"`
    Version    kong.VersionFlag         `help:"Print version." short:"V"`
    Completion kongcompletion.Completion `cmd:"" help:"Output shell completion code." hidden:""`

    Auth        AuthCmd        `cmd:"" help:"Manage authentication."`
    Config      ConfigCmd      `cmd:"" name:"config" help:"Declarative configuration management."`
    Project     ProjectCmd     `cmd:"" help:"Manage projects."`
    Environment EnvironmentCmd `cmd:"" help:"Manage environments."`
    Workspace   WorkspaceCmd   `cmd:"" help:"Manage workspaces."`
}
```

#### Step 3: Split kong.Parse in main.go

```go
func main() {
    applyColorMode()

    ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
    defer stop()

    var c cli.CLI
    parser, err := kong.New(&c,
        kong.Name("fat-controller"),
        kong.Description("CLI for managing Railway projects. Pull live config, diff against desired state, apply the difference."),
        kong.Vars{"version": version.String()},
        kong.UsageOnError(),
        kong.Help(cli.ColorHelpPrinter),
    )
    if err != nil {
        fmt.Fprintln(os.Stderr, "error:", err)
        os.Exit(1)
    }

    kongcompletion.Register(parser)

    kongCtx, err := parser.Parse(os.Args[1:])
    parser.FatalIfErrorf(err)

    slog.SetDefault(c.Logger())
    c.Globals.BaseCtx = ctx

    if err := kongCtx.Run(&c.Globals); err != nil {
        if !errors.Is(err, context.Canceled) {
            fmt.Fprintln(os.Stderr, "error:", err)
        }
        os.Exit(1)
    }
}
```

#### Step 4: Regenerate CLI docs

```bash
go run internal/tools/docgen/main.go
```

The `completion` command has `hidden:""` tag so it won't appear in the main help output, but will still be usable. If we want it visible, remove `hidden:""`.

#### Step 5: Test manually

```bash
go build -o fat-controller ./cmd/fat-controller
./fat-controller completion bash
./fat-controller completion zsh
./fat-controller completion fish
```

#### Step 6: Run `go test ./...`

#### Step 7: Commit: `feat: add shell completion support (bash, zsh, fish)`

---

### Task 9: Update TODO.md and regenerate CLI docs

**Files:**

- Modify: `docs/TODO.md`

**Step 1:** Mark completed items:

- `[x] Align docs/WARNINGS.md warning codes with internal/config/validate.go`
- `[x] Decide whether warnings should run during config diff/config apply`
- `[x] Fetch/include live serviceInstanceLimits`
- `[x] Add shell completions`
- `[x] Tie auth callback server goroutine lifecycle to context/cancellation` (partially — auth now respects `--timeout` and signal cancellation)

**Step 2:** Regenerate CLI docs if any help text changed:

```bash
go run internal/tools/docgen/main.go
```

**Step 3:** Commit: `docs: update TODO.md and regenerate CLI docs`

---

### Task 10: Final verification

**Step 1:** Run full checks:

```bash
mise run check
```

**Step 2:** Verify all tests pass with race detection.

**Step 3:** Verify `go vet ./...` and `staticcheck ./...` are clean.

**Step 4:** Fix any issues that arise.

**Step 5:** Final commit if any fixes needed.

---

## Summary of changes by file

| File | Tasks | What changes |
|------|-------|--------------|
| `internal/config/validate.go` | 1 | Add W002, W021, W031, W050, W051, W060 |
| `internal/config/validate_test.go` | 1 | Tests for all new warning codes |
| `internal/config/desired.go` | 1 | Add `UnknownKeys`, `Overrides`, `Override` |
| `internal/config/parse.go` | 1 | Populate `UnknownKeys` in `parseService` |
| `internal/config/load.go` | 1 | Populate `Overrides` during merge |
| `internal/config/merge.go` | — | No changes (overrides tracked in `load.go`) |
| `docs/WARNINGS.md` | 2 | Remove W001, W010; verify rest |
| `internal/cli/config_common.go` | 3 | Add `emitWarnings` function |
| `internal/cli/config_diff.go` | 3 | Call `emitWarnings` |
| `internal/cli/config_apply.go` | 3 | Call `emitWarnings` |
| `internal/cli/config_validate.go` | 3 | Call `ValidateFiles` |
| `internal/railway/operations.graphql` | 4 | Add `ServiceInstanceLimits` query |
| `.config/genqlient.yaml` | 4 | Add `ServiceInstanceLimit` binding |
| `internal/railway/generated.go` | 4 | Regenerated |
| `internal/config/model.go` | 4 | Add `VCPUs`, `MemoryGB` to `ServiceConfig` |
| `internal/railway/state.go` | 4 | Fetch resource limits |
| `internal/diff/diff.go` | 4 | Fix `diffResources` to compare live values |
| `internal/diff/diff_test.go` | 4 | Resource diff tests |
| `internal/cli/auth.go` | 5, 6 | Timeout context + `io.Writer` |
| `internal/cli/client.go` | 5 | Timeout context |
| `internal/auth/login.go` | 6 | Add `io.Writer` parameter |
| `internal/cli/cli.go` | 7, 8 | `BaseCtx` field; `Completion` field |
| `cmd/fat-controller/main.go` | 7, 8 | Signal handling; kong-completion |
| `go.mod` / `go.sum` | 8 | Add `jotaen/kong-completion` |
| `docs/TODO.md` | 9 | Mark items done |
| `docs/cli/` | 9 | Regenerated |
