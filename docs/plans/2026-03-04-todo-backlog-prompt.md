# Implementation Prompt: TODO Backlog

This document contains per-task prompts for implementing the plan in
`docs/plans/2026-03-04-todo-backlog.md`. Each prompt is self-contained and can
be given to a fresh Claude Code session.

## How to use

Tasks are grouped into **batches** by dependency order. Within a batch, tasks
are independent and can be run in parallel (separate sessions). Between batches,
earlier batches must be committed before later ones start.

**Execution pattern per task:**

1. Start a fresh Claude Code session
2. Copy the **shared preamble** (Section 1) + the **task prompt** into the
   session
3. The agent implements the task following TDD: failing test → implement → verify
   → commit
4. Review the commit before moving to the next task

**Batch order:**

| Batch | Tasks | Theme |
|-------|-------|-------|
| A | 1, 2, 4, 5, 11, 15, 16, 17 | Independent fixes (no deps) |
| B | 3, 6, 7, 12 | Depend on nothing, but benefit from batch A being done |
| C | 8 | Auth boilerplate extraction (benefits from all Run methods being stable) |
| D | 9, 10, 13, 18 | Depend on Task 8 or are independent refactors |
| E | 14, 19, 20, 21 | Depend on Tasks 3, 8, 13 |
| F | 22, 23a, 23b, 23c, 23d | Final features and cleanup |

---

## 1. Shared preamble

Copy this into every session before the task-specific prompt.

````
You are implementing a task from the TODO backlog plan for `fat-controller`, a
Go CLI tool for declarative Railway PaaS project management.

## Codebase conventions

- **Go 1.25**, modules at `github.com/hamishmorgan/fat-controller`
- **CLI framework**: Kong (`github.com/alecthomas/kong`)
- **TOML**: `github.com/BurntSushi/toml`
- **GraphQL**: genqlient (`github.com/Khan/genqlient`)
- **Styled output**: lipgloss (`github.com/charmbracelet/lipgloss`)
- **Testing**: stdlib `testing` package, `net/http/httptest`, external test
  packages (`package cli_test`, `package config_test`, etc.)

### Code patterns

- Every CLI command has a thin `Run(globals *Globals) error` method (wires real
  deps) and a testable `RunXxx(...)` function (accepts interfaces + `io.Writer`).
- External test packages: tests live in `*_test.go` files with `package xxx_test`.
- Test helpers in `internal/cli/helpers_test.go`: `writeTOML`, `fakeFetcher`,
  `recordingMutator`, `capturingFetcher`.
- Test helpers in `internal/apply/apply_test.go`: `recordingApplier` (records
  calls as strings like `"var:+:service:key=value"`).
- Fakes use obviously-fake values like `"proj-1"`, `"env-1"`, `"fakekeyfakekeyfakekey"`.
- Pointer fields mean "not specified" (nil = omit). All `Desired*` sub-structs
  use this convention.
- Auth boilerplate in every config/list `Run()`:
  ```go
  store := auth.NewTokenStore(auth.WithFallbackPath(platform.AuthFilePath()))
  resolved, err := auth.ResolveAuth(globals.Token, store)
  if err != nil {
      return err
  }
  client := railway.NewClient(railway.Endpoint, resolved, store, auth.NewOAuthClient())
  ```
- Config pipeline: `LoadConfigs` → `Merge` → `Interpolate` → `Fetch` →
  `diff.Compute` → `diff.Format` or `apply.Apply`.
- Merge function uses variable name `result` (not `merged`).
- `browserCommand` in `login.go` is `var browserCommand = exec.Command` — a
  **function variable**, not a string.
- `CallbackServer` already stores `*http.Server` as a field and has `Shutdown()`.
- List command info structs (`ProjectInfo`, `EnvironmentInfo`, `WorkspaceInfo`)
  only have `json` tags — no `toml` tags.
- `variableCollectionUpsert` exists in `schema.graphql` but NOT in
  `operations.graphql` — must be added before use.

### Running tests

```bash
go test -race ./...                           # all tests
go test -race ./internal/config/...           # config package
go test -race ./internal/cli/...              # CLI tests
go test -race -run TestSpecificName ./pkg/... # single test
mise run check                                # full lint + test + build
```

### Hazards

- **gitleaks pre-commit hook**: use fake values like `fakekeyfakekeyfakekey`
- **taplo format**: runs on pre-commit, may reformat TOML fixtures
- **TOML key quoting**: keys with dots/spaces need quoting per TOML spec
- **Pre-commit hooks**: run gitleaks and formatters. If commit fails due to
  formatting, stage the reformatted files and commit again.

### Workflow

1. Read the referenced source files to understand current state
2. Write failing test(s)
3. Run the test to confirm it fails
4. Implement the minimal change
5. Run the specific test to confirm it passes
6. Run full test suite: `go test -race ./...`
7. Commit with the suggested message (or similar)

If a test fails unexpectedly on step 6, fix it before committing. If the same
approach fails twice, stop and report what went wrong.

Do NOT make changes beyond what the task specifies. Do NOT refactor unrelated
code. Do NOT add features not in the task description.
````

---

## 2. Task prompts

### Task 1: Return errors for non-string `project`/`environment` values

```
Implement Task 1 from docs/plans/2026-03-04-todo-backlog.md.

## What to do

Make `config.Parse()` return an error when `project` or `environment` values
are not strings (e.g., `project = 123` or `environment = true`). Currently
these are silently ignored.

## Files to modify

- `internal/config/parse.go` — lines 39-45 (project/environment extraction)
- `internal/config/parse_test.go` — add new tests

## Implementation

In `parse.go`, the current code (lines 39-45) does:
```go
if v, ok := raw["project"].(string); ok {
    cfg.Project = v
}
if v, ok := raw["environment"].(string); ok {
    cfg.Environment = v
}
```

Change to check presence first, then type-assert with error:

```go
if v, ok := raw["project"]; ok {
    s, ok := v.(string)
    if !ok {
        return nil, fmt.Errorf("invalid 'project': expected string, got %T", v)
    }
    cfg.Project = s
}
if v, ok := raw["environment"]; ok {
    s, ok := v.(string)
    if !ok {
        return nil, fmt.Errorf("invalid 'environment': expected string, got %T", v)
    }
    cfg.Environment = s
}
```

## Tests to write

Add to `internal/config/parse_test.go`:

```go
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
```

## Verification

1. `go test -race -run TestParse_RejectsNonString ./internal/config/...` — PASS
2. `go test -race ./...` — all pass

## Commit

```
git add internal/config/parse.go internal/config/parse_test.go
git commit -m "fix: return errors for non-string project/environment config values"
```

```

---

### Task 2: Return errors for unrecognised non-table top-level config keys

```

Implement Task 2 from docs/plans/2026-03-04-todo-backlog.md.

## What to do

Make `config.Parse()` return an error when a top-level key is not in
`knownTopLevelKeys` and is not a TOML table (service). Currently line 69 of
`parse.go` does `continue` for non-table values — typos like `projct = "x"`
are silently ignored.

## Files to modify

- `internal/config/parse.go` — line 69 (the `continue` in the service-parsing loop)
- `internal/config/parse_test.go` — add new tests

## Implementation

In `parse.go`, the loop starting around line 63:

```go
for key, val := range raw {
    if knownTopLevelKeys[key] {
        continue
    }
    svcMap, ok := val.(map[string]any)
    if !ok {
        continue  // ← THIS LINE: change to return error
    }
```

Replace `continue` with:

```go
    if !ok {
        return nil, fmt.Errorf("unrecognised config key %q (not a known setting or service table)", key)
    }
```

## Tests to write

```go
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
```

## Verification

1. `go test -race -run "TestParse_RejectsUnknownScalar|TestParse_AcceptsUnknownTable" ./internal/config/...` — PASS
2. `go test -race ./...` — all pass (check no existing tests rely on silent ignore)

## Commit

```
git add internal/config/parse.go internal/config/parse_test.go
git commit -m "fix: return errors for unrecognised non-table top-level config keys"
```

```

---

### Task 3: Parse and validate `sensitive_keywords`, `sensitive_allowlist`, and `suppress_warnings`

```

Implement Task 3 from docs/plans/2026-03-04-todo-backlog.md.

## What to do

These three config keys are in `knownTopLevelKeys` but never extracted from
parsed TOML. Add fields to `DesiredConfig`, parse them with type validation,
and merge them.

## Files to modify

- `internal/config/desired.go` — add 3 new fields to `DesiredConfig`
- `internal/config/parse.go` — extract the new fields after project/environment
- `internal/config/merge.go` — merge the new fields (non-empty overrides)
- `internal/config/parse_test.go` — add tests for parsing
- `internal/config/merge_test.go` — add tests for merging

## Implementation details

### desired.go

Add to `DesiredConfig` struct (after `Services`):

```go
SensitiveKeywords  []string
SensitiveAllowlist []string
SuppressWarnings   []string
```

### parse.go

Add a `toStringSlice` helper:

```go
func toStringSlice(val any, field string) ([]string, error) {
    arr, ok := val.([]any)
    if !ok {
        return nil, fmt.Errorf("invalid '%s': expected array of strings, got %T", field, val)
    }
    result := make([]string, 0, len(arr))
    for i, item := range arr {
        s, ok := item.(string)
        if !ok {
            return nil, fmt.Errorf("invalid '%s[%d]': expected string, got %T", field, i, item)
        }
        result = append(result, s)
    }
    return result, nil
}
```

After the project/environment extraction, add extraction for all three fields
using `toStringSlice`.

### merge.go

In the `Merge` function, after the project/environment merging (the loop body
uses variable `result`), add:

```go
if len(cfg.SensitiveKeywords) > 0 {
    result.SensitiveKeywords = cfg.SensitiveKeywords
}
if len(cfg.SensitiveAllowlist) > 0 {
    result.SensitiveAllowlist = cfg.SensitiveAllowlist
}
if len(cfg.SuppressWarnings) > 0 {
    result.SuppressWarnings = cfg.SuppressWarnings
}
```

## Tests to write

Parse tests:

```go
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

func TestParse_RejectsInvalidSensitiveKeywords(t *testing.T) {
    _, err := config.Parse([]byte(`sensitive_keywords = "not-an-array"`))
    if err == nil {
        t.Fatal("expected error for non-array sensitive_keywords")
    }
}
```

Merge test:

```go
func TestMerge_SensitiveKeywords(t *testing.T) {
    base := &config.DesiredConfig{SensitiveKeywords: []string{"SECRET"}}
    overlay := &config.DesiredConfig{SensitiveKeywords: []string{"TOKEN", "KEY"}}
    result := config.Merge(base, overlay)
    if len(result.SensitiveKeywords) != 2 || result.SensitiveKeywords[0] != "TOKEN" {
        t.Errorf("expected overlay keywords to win: %v", result.SensitiveKeywords)
    }
}
```

## Verification

1. `go test -race -run "TestParse_ExtractsSensitive|TestParse_RejectsInvalid|TestMerge_Sensitive" ./internal/config/...` — PASS
2. `go test -race ./...` — all pass

## Commit

```
git add internal/config/desired.go internal/config/parse.go internal/config/merge.go internal/config/parse_test.go internal/config/merge_test.go
git commit -m "feat: parse and validate sensitive_keywords, sensitive_allowlist, and suppress_warnings config keys"
```

```

---

### Task 4: Quote TOML keys in rendered output

```

Implement Task 4 from docs/plans/2026-03-04-todo-backlog.md.

## What to do

The `renderTOML` function in `render.go` writes variable keys bare. Keys
containing dots, spaces, or other special chars produce invalid TOML. Add a
`tomlKey` helper that quotes keys when needed.

## Files to modify

- `internal/config/render.go` — add `tomlKey` helper, use it in `renderTOML`
- `internal/config/render_test.go` — add test for special keys

## Implementation

Add helper in `render.go`:

```go
func tomlKey(key string) string {
    for _, r := range key {
        if !((r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-') {
            return tomlQuote(key)
        }
    }
    return key
}
```

In `renderTOML` (around line 124-158), find all places where variable keys are
written bare (pattern: `k + " = " + tomlQuote(...)`) and replace `k` with
`tomlKey(k)`. There are writes for shared variables and per-service variables.
Also check `RenderInitTOML` and `writeTOMLDeploy` for any bare key writes.

The existing `tomlQuote` function (lines 212-241) handles value quoting with
escaping — reuse it for key quoting since TOML uses the same quoting rules.

## Test to write

```go
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
```

## Verification

1. `go test -race -run TestRenderTOML_QuotesSpecialKeys ./internal/config/...` — PASS
2. `go test -race ./...` — all pass

## Commit

```
git add internal/config/render.go internal/config/render_test.go
git commit -m "fix: quote TOML keys containing dots, spaces, or special characters"
```

```

---

### Task 5: Handle `toml` output format in list commands

```

Implement Task 5 from docs/plans/2026-03-04-todo-backlog.md.

## What to do

`project list`, `environment list`, and `workspace list` switch on
`globals.Output` for `"json"` and default (text). The `"toml"` case silently
falls through to text. Add TOML output support.

## Files to modify

- `internal/cli/project_list.go` — add `"toml"` case + `toml` struct tags
- `internal/cli/environment_list.go` — add `"toml"` case + `toml` struct tags
- `internal/cli/workspace_list.go` — add `"toml"` case + `toml` struct tags
- `internal/cli/project_list_test.go` — add TOML output test
- `internal/cli/environment_list_test.go` — add TOML output test
- `internal/cli/workspace_list_test.go` — add TOML output test

## Implementation

### Step 1: Add `toml` struct tags

The info structs only have `json` tags. Add `toml` tags:

```go
// In project_list.go:
type ProjectInfo struct {
    ID   string `json:"id" toml:"id"`
    Name string `json:"name" toml:"name"`
}

// In environment_list.go:
type EnvironmentInfo struct {
    ID   string `json:"id" toml:"id"`
    Name string `json:"name" toml:"name"`
}

// In workspace_list.go:
type WorkspaceInfo struct {
    ID   string `json:"id" toml:"id"`
    Name string `json:"name" toml:"name"`
}
```

### Step 2: Add `"toml"` case to output switch

In each `RunXxxList` function, add a `"toml"` case. You'll need to import
`github.com/BurntSushi/toml`. Use a wrapper struct so the TOML output has a
named array:

```go
case "toml":
    wrapper := struct {
        Projects []ProjectInfo `toml:"projects"`
    }{Projects: projects}
    return toml.NewEncoder(out).Encode(wrapper)
```

Use `Environments` and `Workspaces` as the wrapper field names for the other
two commands.

## Tests to write

Write a TOML output test for each command. Example for project list (adapt
the pattern for the existing test fakes — look at the existing JSON test in
the same file):

```go
func TestRunProjectList_TOMLOutput(t *testing.T) {
    // Use the same fake lister pattern as existing tests
    // Set globals.Output = "toml"
    // Assert output contains project name and ID
    // Assert output is valid TOML (parse it back)
}
```

## Verification

1. `go test -race -run "TOML" ./internal/cli/...` — PASS
2. `go test -race ./...` — all pass

## Commit

```
git add internal/cli/project_list.go internal/cli/environment_list.go internal/cli/workspace_list.go internal/cli/project_list_test.go internal/cli/environment_list_test.go internal/cli/workspace_list_test.go
git commit -m "feat: handle TOML output format in list commands"
```

```

---

### Task 6: `config get` path argument should filter by section/key

```

Implement Task 6 from docs/plans/2026-03-04-todo-backlog.md.

## What to do

Currently `config get api.variables.PORT` returns all of `api`'s config. Make
it filter to just the requested variable (or section).

## Files to modify

- `internal/cli/config_get.go` — modify `RunConfigGet` to filter output
- `internal/cli/config_get_test.go` — add filtering test

## Current state

`RunConfigGet` (lines 52-87 in config_get.go) already parses the path to
extract a service name for the `fetcher.Fetch` call's `serviceFilter` param.
But after fetching, it renders the entire fetched config — it doesn't filter
to just the requested key.

The path parsing uses `config.ParsePath(path)` which returns a struct with
`Service`, `Section`, and `Key` fields.

## Implementation

After fetching `cfg`, if a specific key was requested (e.g., `api.variables.PORT`),
look up that single value and output just the raw value. If a section was
requested (e.g., `api.variables`), filter `cfg` to only that section before
rendering.

For single variable lookup:

```go
if parsed.Key != "" && parsed.Section == "variables" {
    svc, ok := cfg.Services[parsed.Service]
    if !ok {
        return fmt.Errorf("service not found: %s", parsed.Service)
    }
    val, ok := svc.Variables[parsed.Key]
    if !ok {
        return fmt.Errorf("variable not found: %s.variables.%s", parsed.Service, parsed.Key)
    }
    _, err = fmt.Fprintln(out, val)
    return err
}
```

Read `config_get.go` and `internal/config/path.go` (if it exists) to understand
the current path parsing before implementing.

## Test to write

```go
func TestRunConfigGet_FiltersByPathSectionAndKey(t *testing.T) {
    fetcher := &fakeFetcher{
        cfg: &config.LiveConfig{
            Services: map[string]*config.ServiceConfig{
                "api": {
                    Name: "api",
                    Variables: map[string]string{
                        "PORT":  "8080",
                        "DEBUG": "false",
                    },
                },
            },
        },
    }
    var buf bytes.Buffer
    globals := &cli.Globals{Output: "text", ShowSecrets: true}
    err := cli.RunConfigGet(context.Background(), globals, "api.variables.PORT", fetcher, &buf)
    if err != nil {
        t.Fatal(err)
    }
    output := buf.String()
    if !strings.Contains(output, "8080") {
        t.Errorf("expected PORT value: %s", output)
    }
    if strings.Contains(output, "DEBUG") {
        t.Errorf("should not contain other variables: %s", output)
    }
}
```

## Verification

1. `go test -race -run TestRunConfigGet_FiltersByPath ./internal/cli/...` — PASS
2. `go test -race ./...` — all pass

## Commit

```
git add internal/cli/config_get.go internal/cli/config_get_test.go
git commit -m "feat: filter config get output by section/key path"
```

```

---

### Task 7: `config set` and `config delete` interactive confirmation

```

Implement Task 7 from docs/plans/2026-03-04-todo-backlog.md.

## What to do

`config set` and `config delete` use a single `if !globals.Confirm || globals.DryRun`
condition that goes straight to dry-run output. They should match `config apply`'s
three-branch pattern: explicit `--dry-run`, non-TTY dry-run, TTY interactive prompt.

## Files to modify

- `internal/cli/config_set.go` — `RunConfigSet` (around lines 24-40)
- `internal/cli/config_delete.go` — `RunConfigDelete` (around lines 24-40)
- `internal/cli/config_set_test.go` — add/update tests
- `internal/cli/config_delete_test.go` — add/update tests

## Current state

Both commands have this pattern (e.g., config_set.go lines 35-38):

```go
if !globals.Confirm || globals.DryRun {
    fmt.Fprintf(out, "dry run: would set %s = %q (use --confirm to apply)\n", path, value)
    return nil
}
```

## Implementation

Replace with the three-branch pattern from `config_apply.go` (look at lines
~150-170 for the exact pattern used there):

```go
if globals.DryRun {
    _, err := fmt.Fprintf(out, "dry run: would set %s = %q\n", path, value)
    return err
}
if !globals.Confirm {
    if !prompt.StdinIsInteractive() {
        _, err := fmt.Fprintf(out, "dry run: would set %s = %q (use --confirm to apply)\n", path, value)
        return err
    }
    fmt.Fprintf(out, "Will set %s = %q\n\n", path, value)
    confirmed, err := prompt.ConfirmRW(os.Stdin, out, "Are you sure?", false)
    if err != nil {
        return fmt.Errorf("reading confirmation: %w", err)
    }
    if !confirmed {
        _, err := fmt.Fprintln(out, "Cancelled.")
        return err
    }
}
```

You'll need to import `github.com/hamishmorgan/fat-controller/internal/prompt`
and `os` in both files. Read `config_apply.go` to see the exact import paths
and prompt function signatures.

Apply the same pattern to `config delete`, adjusting the message text.

## Tests

Existing tests run in non-TTY mode, so they exercise the non-interactive path.
Verify existing tests still pass. Add a test that verifies `--dry-run` flag
outputs dry-run message and doesn't call the mutator:

```go
func TestRunConfigSet_DryRunFlag(t *testing.T) {
    mut := &recordingMutator{}
    var buf bytes.Buffer
    globals := &cli.Globals{DryRun: true, Confirm: true}
    err := cli.RunConfigSet(context.Background(), globals, "api.variables.PORT", "8080", mut, &buf)
    if err != nil {
        t.Fatal(err)
    }
    if !strings.Contains(buf.String(), "dry run") {
        t.Error("expected dry-run message")
    }
    if mut.called {
        t.Error("should not call setter in dry-run mode")
    }
}
```

## Verification

1. `go test -race ./internal/cli/...` — all pass
2. `go test -race ./...` — all pass

## Commit

```
git add internal/cli/config_set.go internal/cli/config_delete.go internal/cli/config_set_test.go internal/cli/config_delete_test.go
git commit -m "feat: add interactive confirmation to config set and config delete"
```

```

---

### Task 8: Extract shared auth/client boilerplate

```

Implement Task 8 from docs/plans/2026-03-04-todo-backlog.md.

## What to do

Every config/list CLI `Run()` method repeats the same 4-line auth bootstrap.
Extract it into a `newClient` helper in a new file.

## Files to create/modify

- Create: `internal/cli/client.go`
- Modify: `internal/cli/config_get.go`, `config_set.go`, `config_delete.go`,
  `config_diff.go`, `config_apply.go`, `config_init.go`, `project_list.go`,
  `environment_list.go`, `workspace_list.go`
- Do NOT modify: `internal/cli/auth.go` — auth commands have different patterns

## Implementation

### New file: `internal/cli/client.go`

```go
package cli

import (
    "github.com/hamishmorgan/fat-controller/internal/auth"
    "github.com/hamishmorgan/fat-controller/internal/platform"
    "github.com/hamishmorgan/fat-controller/internal/railway"
)

// newClient creates an authenticated Railway client from the globals.
func newClient(globals *Globals) (*railway.Client, error) {
    store := auth.NewTokenStore(auth.WithFallbackPath(platform.AuthFilePath()))
    resolved, err := auth.ResolveAuth(globals.Token, store)
    if err != nil {
        return nil, err
    }
    return railway.NewClient(railway.Endpoint, resolved, store, auth.NewOAuthClient()), nil
}
```

### Replace boilerplate in each Run method

Replace the 4-line pattern:

```go
store := auth.NewTokenStore(auth.WithFallbackPath(platform.AuthFilePath()))
resolved, err := auth.ResolveAuth(globals.Token, store)
if err != nil {
    return err
}
client := railway.NewClient(railway.Endpoint, resolved, store, auth.NewOAuthClient())
```

With:

```go
client, err := newClient(globals)
if err != nil {
    return err
}
```

Remove unused imports (`auth`, `platform`) from each modified file.

**Careful with `environment_list.go`** — it has a slightly different pattern
(calls `railway.ResolveProjectID` instead of `fetcher.Resolve`). The auth
bootstrap part is still the same, just what happens after is different.

## Verification

This is a pure refactor — no behaviour change.

1. `go test -race ./...` — all pass
2. Verify `internal/cli/auth.go` is NOT modified

## Commit

```
git add internal/cli/client.go internal/cli/config_get.go internal/cli/config_set.go internal/cli/config_delete.go internal/cli/config_diff.go internal/cli/config_apply.go internal/cli/config_init.go internal/cli/project_list.go internal/cli/environment_list.go internal/cli/workspace_list.go
git commit -m "refactor: extract shared auth/client bootstrap into newClient helper"
```

```

---

### Task 9: Extract shared config-load/resolve/fetch/filter logic

```

Implement Task 9 from docs/plans/2026-03-04-todo-backlog.md.

## What to do

`config_diff.go` and `config_apply.go` share nearly identical code for loading
configs, interpolating, resolving project/environment, fetching live state, and
filtering by service. Extract this into a shared function.

## Files to create/modify

- Create: `internal/cli/config_common.go`
- Modify: `internal/cli/config_diff.go` — use the shared function
- Modify: `internal/cli/config_apply.go` — use the shared function

## Implementation

Read `config_diff.go` (`RunConfigDiff`, lines 35-90) and `config_apply.go`
(`RunConfigApply`, lines 69-204) to identify the duplicated pipeline. The
shared steps are:

1. `config.LoadConfigs(configDir, extraFiles)`
2. `config.Interpolate(desired)`
3. Fallback project/environment from config when globals are empty
4. `fetcher.Resolve(ctx, workspace, project, environment)`
5. `fetcher.Fetch(ctx, projID, envID, service)`
6. Filter desired config by `--service` if set

Create in `internal/cli/config_common.go`:

```go
type configPair struct {
    Desired       *config.DesiredConfig
    Live          *config.LiveConfig
    ProjectID     string
    EnvironmentID string
}

func loadAndFetch(ctx context.Context, globals *Globals, configDir string, extraFiles []string, fetcher configFetcher) (*configPair, error) {
    // ... implement the 6 steps above
}
```

Then refactor `RunConfigDiff` and `RunConfigApply` to use `loadAndFetch`.

**Important**: `config_apply.go`'s `Run()` method currently does steps 1-5
itself before calling `RunConfigApply`. After this refactor, either move that
into `RunConfigApply` or have `Run()` call `loadAndFetch` and pass the result.
Look at the current code to decide the cleanest approach.

## Verification

Pure refactor — no behaviour change.

1. `go test -race ./internal/cli/...` — all pass
2. `go test -race ./...` — all pass

## Commit

```
git add internal/cli/config_common.go internal/cli/config_diff.go internal/cli/config_apply.go
git commit -m "refactor: extract shared config load/resolve/fetch/filter pipeline"
```

```

---

### Task 10: Define constants for deploy/resource setting keys

```

Implement Task 10 from docs/plans/2026-03-04-todo-backlog.md.

## What to do

Hard-coded string keys like `"builder"`, `"dockerfile_path"`, `"start_command"`
etc. appear in `internal/diff/diff.go`, `internal/apply/convert.go`, and
`internal/config/parse.go`. Define them as constants in one place.

## Files to create/modify

- Create: `internal/config/keys.go`
- Modify: `internal/diff/diff.go` — use constants
- Modify: `internal/apply/convert.go` — use constants
- Modify: `internal/config/parse.go` — use constants

## Implementation

Create `internal/config/keys.go`:

```go
package config

const (
    KeyBuilder         = "builder"
    KeyDockerfilePath  = "dockerfile_path"
    KeyRootDirectory   = "root_directory"
    KeyStartCommand    = "start_command"
    KeyHealthcheckPath = "healthcheck_path"

    KeyVCPUs    = "vcpus"
    KeyMemoryGB = "memory_gb"
)
```

Then search each file for the string literals and replace with the constants.

In `diff/diff.go`, the `diffDeploy` function (around line 186) uses these
strings. Import `config` package and use `config.KeyBuilder`, etc.

In `apply/convert.go`, the `ToServiceInstanceUpdateInput` function uses deploy
field names. It already imports `config`.

In `config/parse.go`, the `parseService` function uses these strings when
parsing deploy and resources sections.

## Verification

Compile-time only — no runtime behaviour change.

1. `go test -race ./...` — all pass

## Commit

```
git add internal/config/keys.go internal/diff/diff.go internal/apply/convert.go internal/config/parse.go
git commit -m "refactor: define constants for deploy/resource setting keys"
```

```

---

### Task 11: Fix `OpenBrowser` zombie process leak

```

Implement Task 11 from docs/plans/2026-03-04-todo-backlog.md.

## What to do

`OpenBrowser` in `auth/login.go` calls `cmd.Start()` without `cmd.Wait()`,
leaking zombie processes. Add a goroutine to wait on the child.

## Files to modify

- `internal/auth/login.go` — `OpenBrowser` function (lines 20-31)

## Current code

```go
func OpenBrowser(url string) error {
    var cmd *exec.Cmd
    switch runtime.GOOS {
    case "darwin":
        cmd = browserCommand("open", url)
    case "windows":
        cmd = browserCommand("rundll32", "url.dll,FileProtocolHandler", url)
    default:
        cmd = browserCommand("xdg-open", url)
    }
    return cmd.Start()
}
```

Note: `browserCommand` is a function variable (`var browserCommand = exec.Command`).

## Implementation

```go
func OpenBrowser(url string) error {
    var cmd *exec.Cmd
    switch runtime.GOOS {
    case "darwin":
        cmd = browserCommand("open", url)
    case "windows":
        cmd = browserCommand("rundll32", "url.dll,FileProtocolHandler", url)
    default:
        cmd = browserCommand("xdg-open", url)
    }
    if err := cmd.Start(); err != nil {
        return err
    }
    go cmd.Wait() //nolint:errcheck
    return nil
}
```

## Verification

1. `go test -race ./internal/auth/...` — all pass
2. `go test -race ./...` — all pass

## Commit

```
git add internal/auth/login.go
git commit -m "fix: prevent zombie processes in OpenBrowser by waiting on child"
```

```

---

### Task 12: Remove mutable `browserCommand` package variable

```

Implement Task 12 from docs/plans/2026-03-04-todo-backlog.md.

## What to do

`auth/login.go` has a mutable package-level `browserCommand` variable and
`SetBrowserCommand`/`BrowserCommand` functions. The `Login` function already
accepts `BrowserOpener` for test injection, making the mutable variable
unnecessary. Remove it.

## Files to modify

- `internal/auth/login.go` — remove `browserCommand`, `SetBrowserCommand`,
  `BrowserCommand`; change `OpenBrowser` to use `exec.Command` directly
- `internal/auth/login_test.go` — update `TestOpenBrowser` (which uses
  `SetBrowserCommand`)

## Implementation

In `login.go`:

1. Remove `var browserCommand = exec.Command` (line 33)
2. Remove `func BrowserCommand()` (line 36)
3. Remove `func SetBrowserCommand()` (line 42)
4. Change `OpenBrowser` to use `exec.Command` directly instead of
   `browserCommand`:

```go
func OpenBrowser(url string) error {
    var cmd *exec.Cmd
    switch runtime.GOOS {
    case "darwin":
        cmd = exec.Command("open", url)
    case "windows":
        cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
    default:
        cmd = exec.Command("xdg-open", url)
    }
    if err := cmd.Start(); err != nil {
        return err
    }
    go cmd.Wait() //nolint:errcheck
    return nil
}
```

In `login_test.go`:

- `TestOpenBrowser` (line 226) uses `auth.SetBrowserCommand` to inject a stub.
  Since `OpenBrowser` now uses `exec.Command` directly, this test cannot stub
  the command anymore. Remove `TestOpenBrowser` — the function is a thin wrapper
  and is tested indirectly via `Login` tests that inject `BrowserOpener`.

## Verification

1. `go test -race ./internal/auth/...` — all pass
2. `go test -race ./...` — all pass

## Commit

```
git add internal/auth/login.go internal/auth/login_test.go
git commit -m "refactor: remove mutable browserCommand variable in auth/login.go"
```

```

---

### Task 13: Add `context.Context` to auth functions

```

Implement Task 13 from docs/plans/2026-03-04-todo-backlog.md.

## What to do

Add `context.Context` as the first parameter to `RegisterClient`, `ExchangeCode`,
and `ResolveAuth`. Also include response body in error messages for
`RegisterClient` and `ExchangeCode` (only `RefreshToken` currently does this).

## Files to modify

- `internal/auth/oauth.go` — `RegisterClient`, `ExchangeCode`
- `internal/auth/resolver.go` — `ResolveAuth`
- `internal/auth/login.go` — update call sites
- `internal/cli/auth.go` — update call sites
- `internal/cli/client.go` — update call site (if Task 8 is done)
- OR all CLI `Run()` methods if Task 8 is not done

## Implementation

### oauth.go

**RegisterClient** — current signature (line 77):

```go
func (c *OAuthClient) RegisterClient(redirectURI string) (*RegistrationResponse, error)
```

New:

```go
func (c *OAuthClient) RegisterClient(ctx context.Context, redirectURI string) (*RegistrationResponse, error)
```

- Replace `c.httpClient().Post(...)` with `http.NewRequestWithContext(ctx, "POST", ...)` + `c.httpClient().Do(req)`
- Add response body to non-OK error: `body, _ := io.ReadAll(resp.Body); return nil, fmt.Errorf("registration failed with status %d: %s", resp.StatusCode, string(body))`

**ExchangeCode** — current signature (line 128):

```go
func (c *OAuthClient) ExchangeCode(clientID, code, redirectURI, codeVerifier string) (*TokenResponse, error)
```

New (preserve parameter order, prepend ctx):

```go
func (c *OAuthClient) ExchangeCode(ctx context.Context, clientID, code, redirectURI, codeVerifier string) (*TokenResponse, error)
```

- Replace `c.httpClient().PostForm(...)` with `http.NewRequestWithContext(ctx, "POST", ...)` + `c.httpClient().Do(req)`
- Add response body to error (same pattern)

### resolver.go

**ResolveAuth** — current signature (line 33):

```go
func ResolveAuth(flagToken string, store *TokenStore) (*ResolvedAuth, error)
```

New:

```go
func ResolveAuth(ctx context.Context, flagToken string, store *TokenStore) (*ResolvedAuth, error)
```

No internal changes needed (no network calls), but accept ctx for consistency.

### Update all call sites

- In `login.go`: `Login` and `loginAttempt` call `RegisterClient` and
  `ExchangeCode` — pass a context (Login should accept ctx too, see Task 14).
  For now, use `context.Background()` if Login doesn't have a ctx yet.
- In `loadOrRegisterClient`: calls `RegisterClient` — pass context.
- In `client.go` (if Task 8 done): calls `ResolveAuth` — pass `context.Background()`.
- In `auth.go`: `AuthStatusCmd.Run` calls `ResolveAuth` — pass `context.Background()`.
- All other `Run()` methods that call `ResolveAuth` directly.

### Update tests

Update test call sites in `oauth_test.go` (if exists), `resolver_test.go`
(if exists), and `login_test.go` to pass `context.Background()`.

## Verification

1. `go test -race ./internal/auth/...` — all pass
2. `go test -race ./...` — all pass

## Commit

```
git add internal/auth/oauth.go internal/auth/resolver.go internal/auth/login.go internal/cli/
git commit -m "feat: add context.Context to RegisterClient, ExchangeCode, and ResolveAuth; include response body in auth errors"
```

```

---

### Task 14: Auth callback server and login safety improvements

```

Implement Task 14 from docs/plans/2026-03-04-todo-backlog.md.

## What to do

Multiple auth safety fixes:

1. `CallbackServer.Shutdown()` uses `context.Background()` — add a 5s timeout
2. `Login` should accept `context.Context` and bound the callback wait
3. Surface token refresh failures from transport (instead of silent 401)
4. Make `ResolvedAuth.Token` thread-safe

## Files to modify

- `internal/auth/callback.go` — `Shutdown()` method (line 81)
- `internal/auth/login.go` — `Login` and `loginAttempt` functions
- `internal/railway/transport.go` — `RoundTrip` method (around line 88)
- `internal/auth/resolver.go` — `ResolvedAuth` struct

## Implementation

### callback.go

Current `Shutdown` (line 81-82):

```go
func (s *CallbackServer) Shutdown() {
    s.server.Shutdown(context.Background())
}
```

Change to:

```go
func (s *CallbackServer) Shutdown() {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    s.server.Shutdown(ctx) //nolint:errcheck
}
```

Add `"time"` to imports.

### login.go

Make `Login` accept context:

```go
func Login(ctx context.Context, oauth *OAuthClient, store *TokenStore, openBrowser BrowserOpener) error
```

In the callback wait (inside `loginAttempt`), use `select` with ctx:

```go
select {
case result := <-cb.Result:
    // handle result (existing code)
case <-ctx.Done():
    cb.Shutdown()
    return ctx.Err()
}
```

Update `loginAttempt` signature similarly.

Update call site in `internal/cli/auth.go` (`AuthLoginCmd.Run`) to pass
`context.Background()`.

### transport.go

In `RoundTrip` (around line 88), when refresh fails, the current code returns
the original 401 response. Change to return an error:

```go
newTokens, refreshErr := t.tryRefresh(req.Context())
if refreshErr != nil {
    resp.Body.Close() //nolint:errcheck
    return nil, fmt.Errorf("authentication failed (token refresh error: %w)", refreshErr)
}
```

Read the current code carefully to find the exact location.

### resolver.go — thread-safe Token

`ResolvedAuth.Token` is mutated under `AuthTransport.mu` but readable externally.
Add a mutex and accessor methods:

```go
type ResolvedAuth struct {
    mu          sync.Mutex
    Token       string  // Deprecated: use GetToken/SetToken
    HeaderName  string
    HeaderValue string
    Source      string
}

func (r *ResolvedAuth) GetToken() string {
    r.mu.Lock()
    defer r.mu.Unlock()
    return r.Token
}

func (r *ResolvedAuth) SetToken(token string) {
    r.mu.Lock()
    defer r.mu.Unlock()
    r.Token = token
}
```

Update `transport.go` to use `SetToken` when updating after refresh.

## Verification

1. `go test -race ./internal/auth/...` — all pass
2. `go test -race ./internal/railway/...` — all pass
3. `go test -race ./...` — all pass

## Commit

```
git add internal/auth/callback.go internal/auth/login.go internal/railway/transport.go internal/auth/resolver.go internal/auth/login_test.go
git commit -m "fix: add timeout to callback shutdown, bound login wait, surface refresh errors, thread-safe token"
```

```

---

### Task 15: Handle marshal errors in config_apply.go

```

Implement Task 15 from docs/plans/2026-03-04-todo-backlog.md.

## What to do

`config_apply.go` discards errors from `json.MarshalIndent` and `toml.Marshal`
with `_`. Fix all four locations to return errors.

## Files to modify

- `internal/cli/config_apply.go`

## Implementation

Read `config_apply.go` and find all `b, _ := json.MarshalIndent(...)` and
`b, _ := toml.Marshal(...)` patterns. There are four total:

- Two in the "no changes" output block (around lines 122-138)
- Two in the apply results output block (around lines 179-189)

Replace each with:

```go
b, err := json.MarshalIndent(...)
if err != nil {
    return fmt.Errorf("marshalling result: %w", err)
}
```

Same for `toml.Marshal`.

## Verification

1. `go test -race ./internal/cli/...` — all pass
2. `go test -race ./...` — all pass

## Commit

```
git add internal/cli/config_apply.go
git commit -m "fix: handle json/toml marshal errors in config apply instead of discarding"
```

```

---

### Task 16: Add `ctx.Err()` check in apply best-effort loops

```

Implement Task 16 from docs/plans/2026-03-04-todo-backlog.md.

## What to do

The apply engine in `internal/apply/apply.go` continues making network calls
when the context is cancelled. Add `ctx.Err()` checks at the top of each loop.

## Files to modify

- `internal/apply/apply.go` — add checks in the Apply function
- `internal/apply/apply_test.go` — add cancellation test

## Implementation

In `apply.go`, the `Apply` function (line 34) has loops for applying settings,
shared variables, and per-service variables. Add at the top of each loop
iteration:

```go
if err := ctx.Err(); err != nil {
    return result, err
}
```

## Test to write

```go
func TestApply_StopsOnContextCancellation(t *testing.T) {
    ctx, cancel := context.WithCancel(context.Background())
    cancel() // Cancel immediately.

    desired := &config.DesiredConfig{
        Services: map[string]*config.DesiredService{
            "api": {Variables: map[string]string{"FOO": "bar"}},
        },
    }
    live := &config.LiveConfig{
        Services: map[string]*config.ServiceConfig{
            "api": {Name: "api", Variables: map[string]string{}},
        },
    }
    applier := &recordingApplier{}
    result, err := apply.Apply(ctx, desired, live, applier, apply.Options{})
    if err == nil {
        t.Fatal("expected error from cancelled context")
    }
    if len(applier.calls) > 0 {
        t.Errorf("expected no calls on cancelled context, got %d", len(applier.calls))
    }
    _ = result
}
```

Use the existing `recordingApplier` from `apply_test.go`.

## Verification

1. `go test -race -run TestApply_StopsOnContext ./internal/apply/...` — PASS
2. `go test -race ./...` — all pass

## Commit

```
git add internal/apply/apply.go internal/apply/apply_test.go
git commit -m "fix: check ctx.Err() in apply loops to avoid wasted calls on cancellation"
```

```

---

### Task 17: Remove `apply.Result.Skipped`

```

Implement Task 17 from docs/plans/2026-03-04-todo-backlog.md.

## What to do

`Result.Skipped` is declared and serialised but never incremented. Remove it.

## Files to modify

- `internal/apply/result.go` — remove `Skipped` field
- `internal/apply/apply.go` — remove any references
- `internal/apply/apply_test.go` — fix any tests referencing `Skipped`

## Implementation

In `result.go` (lines 6-10), remove the `Skipped` field:

```go
type Result struct {
    Applied int `json:"applied" toml:"applied"`
    Failed  int `json:"failed" toml:"failed"`
}
```

Update `Summary()` to remove the skipped count from output.

Search for `Skipped` in all files under `internal/apply/` and `internal/cli/`
to find any references (test assertions, JSON output checks in E2E tests, etc.).

## Verification

1. `go test -race ./internal/apply/...` — all pass
2. `go test -race ./internal/cli/...` — all pass (E2E tests may reference Skipped)
3. `go test -race ./...` — all pass

## Commit

```
git add internal/apply/result.go internal/apply/apply.go internal/apply/apply_test.go
git commit -m "refactor: remove unused Result.Skipped field"
```

```

---

### Task 18: CI and build improvements

```

Implement Task 18 from docs/plans/2026-03-04-todo-backlog.md.

## What to do

Three CI/build improvements:

1. Pin GitHub Actions to commit SHAs
2. Add `concurrency` with `cancel-in-progress` to CI workflows
3. Pin mise tool versions to specific releases

## Files to modify

- `.github/workflows/test.yml`
- `.github/workflows/build.yml`
- `.github/workflows/lint-go.yml`
- `.github/workflows/lint-docs.yml`
- `.github/workflows/release.yml`
- `.github/workflows/secrets.yml`
- `.config/mise/config.toml`

## Implementation

### Step 1: Pin GitHub Actions to commit SHAs

Look up the current latest commit SHAs for:

- `actions/checkout@v4` → find SHA for v4.2.2 (or latest v4.x)
- `actions/upload-artifact@v4` → find SHA for latest v4.x
- `jdx/mise-action@v2` → find SHA for latest v2.x
- `goreleaser/goreleaser-action@v7` → find SHA for latest v7.x (release.yml only)

Use `gh api` or web fetch to look up the exact SHAs. Format:

```yaml
- uses: actions/checkout@<full-sha> # v4.2.2
```

### Step 2: Add concurrency blocks

Add to each PR-triggered workflow (test, build, lint-go, lint-docs, secrets)
at the top level (after `permissions`):

```yaml
concurrency:
  group: ${{ github.workflow }}-${{ github.ref }}
  cancel-in-progress: true
```

Do NOT add concurrency to `release.yml` (tag-triggered, should not cancel).

### Step 3: Pin mise tool versions

Run `mise ls` to find currently installed versions, then pin in
`.config/mise/config.toml`. Replace `"latest"` with specific versions:

```toml
[tools]
go = "1.25"
golangci-lint = "1.64"           # or current version
"npm:markdownlint-cli2" = "0.17" # or current version
"npm:prettier" = "3.5"           # or current version
taplo = "0.9"                    # or current version
actionlint = "1.7"              # or current version
gitleaks = "8.24"                # or current version
apollo-rover = "0.30"            # or current version
```

Run `mise ls` first to get actual current versions.

## Verification

1. Validate YAML syntax: `actionlint .github/workflows/*.yml` (if available)
2. Validate TOML: `taplo check .config/mise/config.toml`

## Commit

```
git add .github/workflows/ .config/mise/config.toml
git commit -m "chore: pin CI actions to SHAs, add concurrency, pin tool versions"
```

```

---

### Task 19: Config validation warnings system and `config validate` command

```

Implement Task 19 from docs/plans/2026-03-04-todo-backlog.md.

**Prerequisite:** Task 3 must be completed first (SensitiveKeywords,
SensitiveAllowlist, SuppressWarnings fields on DesiredConfig).

## What to do

Implement the warning system described in `docs/WARNINGS.md` and wire up
`config validate` as a real command.

## Files to create/modify

- Create: `internal/config/validate.go` — `Warning` type and `Validate` function
- Create: `internal/config/validate_test.go` — tests for each warning code
- Create: `internal/cli/config_validate.go` — CLI command implementation
- Create: `internal/cli/config_validate_test.go` — CLI test
- Modify: `internal/cli/cli.go` — remove validate stub (lines 123-126),
  un-hide command (line 72: change `hidden:""` to remove it)

## Implementation

### Warning type and Validate function

Read `docs/WARNINGS.md` for the full list of warning codes. Implement at least:

- W003: Empty service block
- W011: Suspicious `${word.word}` reference syntax (should be `${{...}}`)
- W012: Empty string = delete
- W020: Variable in both shared and service
- W030: Lowercase variable name
- W040: Unknown service name (requires `liveServiceNames` parameter)
- W041: No services or shared variables defined
- W050: Hardcoded secret (use existing `Masker` from the config package)

The `Validate` function should accept `*DesiredConfig` and `[]string`
(live service names, nil for offline). It should filter out suppressed warnings
using `cfg.SuppressWarnings`.

### CLI command

Replace the stub in `cli.go` (lines 123-126) with a real implementation in
`config_validate.go`. Remove `hidden:""` from the ConfigValidateCmd field tag
in `cli.go` line 72.

The command loads config files, runs `Validate`, and prints warnings.

### Tests

Write tests for each warning code (see the plan for exact test code).
Write a CLI test that creates a temp config file and verifies warning output.

## Verification

1. `go test -race -run TestValidate ./internal/config/...` — PASS
2. `go test -race -run TestRunConfigValidate ./internal/cli/...` — PASS
3. `go test -race ./...` — all pass

## Commit

```
git add internal/config/validate.go internal/config/validate_test.go internal/cli/config_validate.go internal/cli/config_validate_test.go internal/cli/cli.go
git commit -m "feat: implement config validation warning system and config validate command"
```

```

---

### Task 20: Apply `--timeout` flag to derived contexts

```

Implement Task 20 from docs/plans/2026-03-04-todo-backlog.md.

## What to do

The `--timeout` CLI flag is declared on `Globals` but unused. Apply it to
`context.Background()` calls in command `Run()` methods.

## Files to modify

- All CLI `Run()` methods that create contexts
- If Task 8 is done, you may also want to consider whether `newClient` needs ctx

## Implementation

In each `Run()` method that calls `context.Background()`, replace with a
timeout context:

```go
func (c *ConfigGetCmd) Run(globals *Globals) error {
    ctx, cancel := context.WithTimeout(context.Background(), globals.Timeout)
    defer cancel()
    // use ctx...
}
```

Check `Globals` to find the `Timeout` field and its type (likely `time.Duration`).

Apply to: config get, set, delete, diff, apply, init, project list,
environment list, workspace list.

Do NOT apply to auth commands (they may need different timeout handling).

## Verification

1. `go test -race ./internal/cli/...` — all pass
2. `go test -race ./...` — all pass

## Commit

```
git add internal/cli/
git commit -m "feat: apply --timeout flag to command contexts"
```

```

---

### Task 21: Wire up `--verbose` and `--quiet` flags

```

Implement Task 21 from docs/plans/2026-03-04-todo-backlog.md.

## What to do

The `--verbose` and `--quiet` flags are declared on `Globals` but not wired.
Create output helpers and use them in key operations.

## Files to create/modify

- Create: `internal/cli/output.go` — `info` and `debug` helpers
- Modify: CLI commands as needed

## Implementation

### output.go

```go
package cli

import (
    "fmt"
    "os"
)

func info(globals *Globals, format string, args ...any) {
    if globals.Quiet {
        return
    }
    fmt.Fprintf(os.Stderr, format+"\n", args...)
}

func debug(globals *Globals, format string, args ...any) {
    if !globals.Verbose {
        return
    }
    fmt.Fprintf(os.Stderr, "debug: "+format+"\n", args...)
}
```

### Add debug logging

Add `debug` calls to key operations:

- Config loading: `debug(globals, "loading config from %s", configDir)`
- Resolution: `debug(globals, "resolving project=%q environment=%q", project, environment)`
- Fetching: `debug(globals, "fetching live state for project=%s environment=%s", projID, envID)`
- Apply: `debug(globals, "applying %d changes", result.Applied)`

### Suppress info in quiet mode

In `config validate`, use `info` for "No warnings." message.
In `config apply`, suppress the summary text in quiet mode.

Keep it minimal — add debug/info calls only where they provide clear value.

## Verification

1. `go test -race ./internal/cli/...` — all pass
2. `go test -race ./...` — all pass

## Commit

```
git add internal/cli/output.go internal/cli/
git commit -m "feat: wire up --verbose and --quiet flags for output control"
```

```

---

### Task 22: Batch variable updates using `variableCollectionUpsert`

```

Implement Task 22 from docs/plans/2026-03-04-todo-backlog.md.

## What to do

Use `variableCollectionUpsert` for bulk variable updates instead of per-variable
mutations. This reduces API calls and triggers only one redeployment.

## Prerequisites

`variableCollectionUpsert` exists in `schema.graphql` but is NOT in
`operations.graphql`. You must add it and re-run code generation first.

## Files to modify

- `internal/railway/operations.graphql` — add mutation
- `internal/railway/generated.go` — regenerate
- `internal/railway/mutate.go` — add batch function
- `internal/apply/apply.go` — batch variables before calling applier
- `internal/apply/railway.go` — implement batch method
- `internal/apply/apply_test.go` — update tests
- `internal/cli/helpers_test.go` — update test doubles

## Implementation

### Step 1: Add mutation to operations.graphql

First, read `internal/railway/schema.graphql` to find the exact
`VariableCollectionUpsertInput` type definition. Then add to `operations.graphql`:

```graphql
mutation VariableCollectionUpsert($input: VariableCollectionUpsertInput!) {
  variableCollectionUpsert(input: $input)
}
```

### Step 2: Regenerate

```bash
mise run generate
```

Verify `generated.go` has the new types.

### Step 3: Add batch method to Applier interface

```go
type Applier interface {
    UpsertVariable(ctx context.Context, service, key, value string, skipDeploys bool) error
    UpsertVariables(ctx context.Context, service string, variables map[string]string, skipDeploys bool) error
    DeleteVariable(ctx context.Context, service, key string) error
    UpdateServiceSettings(ctx context.Context, service string, deploy *config.DesiredDeploy) error
    UpdateServiceResources(ctx context.Context, service string, res *config.DesiredResources) error
}
```

### Step 4: Update apply engine

In the variable application loops, collect all create/update changes for a
service into a map, then call `UpsertVariables` once. Deletes still happen
individually.

### Step 5: Implement in RailwayApplier

Add `UpsertVariables` method to `RailwayApplier` using the new generated code.

### Step 6: Update ALL test doubles

Every fake/mock implementing `Applier` needs the new `UpsertVariables` method:

- `recordingApplier` in `apply_test.go`
- Any applier fakes in `internal/cli/helpers_test.go`
- Any applier fakes in E2E tests

## Verification

1. `go test -race ./internal/apply/...` — PASS
2. `go test -race ./internal/cli/...` — PASS
3. `go test -race ./...` — all pass

## Commit

```
git add internal/railway/operations.graphql internal/railway/generated.go internal/railway/mutate.go internal/apply/ internal/cli/
git commit -m "feat: batch variable upserts using variableCollectionUpsert"
```

```

---

### Task 23a: `resolveServiceID` mutex across network calls

```

Implement Task 23a from docs/plans/2026-03-04-todo-backlog.md.

## What to do

`RailwayApplier.resolveServiceID` holds a mutex across network calls. Refactor
to cache-aside pattern: check cache under lock, release lock, do network call,
re-acquire lock to store result.

## Files to modify

- `internal/apply/railway.go` — `resolveServiceID` method (line 23)

## Implementation

Current code holds lock for the entire duration. Change to:

```go
func (r *RailwayApplier) resolveServiceID(ctx context.Context, service string) (string, error) {
    r.mu.Lock()
    if id, ok := r.serviceIDs[service]; ok {
        r.mu.Unlock()
        return id, nil
    }
    r.mu.Unlock()

    id, err := railway.ResolveServiceID(ctx, r.Client, r.ProjectID, service)
    if err != nil {
        return "", err
    }

    r.mu.Lock()
    if r.serviceIDs == nil {
        r.serviceIDs = make(map[string]string)
    }
    r.serviceIDs[service] = id
    r.mu.Unlock()
    return id, nil
}
```

Read the current code first to understand the exact structure.

## Verification

1. `go test -race ./internal/apply/...` — PASS
2. `go test -race ./...` — all pass

## Commit

```
git add internal/apply/railway.go
git commit -m "refactor: use cache-aside pattern for service ID resolution in applier"
```

```

---

### Task 23b: Add `workspace` as optional top-level config key

```

Implement Task 23b from docs/plans/2026-03-04-todo-backlog.md.

## What to do

Add `workspace` as an optional top-level config key. Parse it, merge it, and
use it as fallback when `globals.Workspace` is empty.

## Files to modify

- `internal/config/desired.go` — add `Workspace` field
- `internal/config/parse.go` — add `"workspace"` to `knownTopLevelKeys`, extract it
- `internal/config/merge.go` — merge workspace (non-empty overrides)
- `internal/config/parse_test.go` — add tests
- CLI commands — use `desired.Workspace` as fallback

## Implementation

Follow the exact same pattern as `project`/`environment`:

1. Add `Workspace string` to `DesiredConfig`
2. Add `"workspace": true` to `knownTopLevelKeys`
3. Extract with type checking (same as Task 1's pattern for project/environment)
4. Merge: `if cfg.Workspace != "" { result.Workspace = cfg.Workspace }`
5. In CLI (either `loadAndFetch` from Task 9, or directly in Run methods):

   ```go
   workspace := globals.Workspace
   if workspace == "" {
       workspace = desired.Workspace
   }
   ```

## Tests

```go
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
}
```

## Verification

1. `go test -race ./internal/config/...` — PASS
2. `go test -race ./...` — all pass

## Commit

```
git add internal/config/desired.go internal/config/parse.go internal/config/merge.go internal/config/parse_test.go
git commit -m "feat: add workspace as optional top-level config key"
```

```

---

### Task 23c: Include deploy/build settings in live state fetches

```

Implement Task 23c from docs/plans/2026-03-04-todo-backlog.md.

## What to do

`FetchLiveConfig` doesn't populate `Deploy` on `ServiceConfig`. The
`ServiceInstance` GraphQL query already exists in `operations.graphql` (lines
73-81) and has generated Go code. Call it to populate deploy settings.

## Files to modify

- `internal/railway/state.go` — `FetchLiveConfig` function
- Tests: add httptest-based test

## Implementation

In `state.go`, the `FetchLiveConfig` function (line 11) loops through services.
After fetching variables for each service, add a `ServiceInstance` call:

```go
instance, err := ServiceInstance(ctx, client.GQL(), environmentID, edge.Node.Id)
if err != nil {
    return nil, fmt.Errorf("fetching deploy settings for %s: %w", edge.Node.Name, err)
}
```

Map the response fields to `config.Deploy`:

```go
svc.Deploy = config.Deploy{
    Builder:        string(instance.ServiceInstance.Builder),
    DockerfilePath: deref(instance.ServiceInstance.DockerfilePath),
    RootDirectory:  deref(instance.ServiceInstance.RootDirectory),
    StartCommand:   deref(instance.ServiceInstance.StartCommand),
    HealthcheckPath: deref(instance.ServiceInstance.HealthcheckPath),
}
```

Read the generated types in `generated.go` to find the exact field names and
types for `ServiceInstanceResponse`.

You may need a `deref` helper for `*string` → `string` conversion.

## Verification

1. `go test -race ./internal/railway/...` — PASS
2. `go test -race ./...` — all pass

## Commit

```
git add internal/railway/state.go
git commit -m "feat: include deploy/build settings in live state fetches"
```

```

---

### Task 23d: Add WARNINGS.md notice (conditional)

```

Implement Task 23d from docs/plans/2026-03-04-todo-backlog.md.

## What to do

If Task 19 (warning system) has NOT been implemented yet, add a notice at the
top of `docs/WARNINGS.md` indicating the system is planned but not implemented.

If Task 19 IS already implemented (check for `internal/config/validate.go`),
skip this task entirely — the notice is unnecessary.

## Conditional check

```bash
ls internal/config/validate.go 2>/dev/null && echo "SKIP: Task 19 done" || echo "PROCEED: Add notice"
```

## Implementation (only if Task 19 is not done)

Add at the top of `docs/WARNINGS.md`, after the `#` heading:

```markdown
> **Note:** The warning system described below is planned but not yet
> implemented. Warning codes are reserved for future use.
```

## Commit (only if change was made)

```
git add docs/WARNINGS.md
git commit -m "docs: add notice that warning system is not yet implemented"
```

```

---

## 3. Quick reference: batch execution order

```

BATCH A (independent, can be parallel):
  Task 1:  non-string project/environment errors
  Task 2:  unknown top-level key errors
  Task 4:  TOML key quoting
  Task 5:  TOML output in list commands
  Task 11: OpenBrowser zombie fix
  Task 15: marshal error handling
  Task 16: ctx.Err() in apply loops
  Task 17: remove Result.Skipped

BATCH B (independent, can be parallel):
  Task 3:  parse sensitive_keywords etc.
  Task 6:  config get path filtering
  Task 7:  config set/delete confirmation
  Task 12: remove browserCommand variable

BATCH C (depends on batch A/B being stable):
  Task 8:  extract newClient helper

BATCH D (depends on Task 8 or independent):
  Task 9:  extract loadAndFetch helper
  Task 10: deploy/resource key constants
  Task 13: context.Context in auth functions
  Task 18: CI and build improvements

BATCH E (depends on Tasks 3, 8, 13):
  Task 14: auth safety improvements
  Task 19: config validation + validate command
  Task 20: --timeout flag
  Task 21: --verbose/--quiet flags

BATCH F (final features):
  Task 22: batch variable upserts
  Task 23a: resolveServiceID cache-aside
  Task 23b: workspace config key
  Task 23c: deploy settings in live fetches
  Task 23d: WARNINGS.md notice (conditional)

```
