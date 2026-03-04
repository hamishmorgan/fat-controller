# TODO Backlog Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement all remaining TODO items from `docs/TODO.md` — config validation warnings, bug fixes, safety improvements, output format handling, auth hardening, and code quality improvements.

**Architecture:** Work is organised into 23 tasks grouped by theme. Each task is self-contained: write failing test, implement, verify, commit. Tasks are ordered so that foundational changes (config parsing validation, shared helpers) land first, and dependent work (warning system, output improvements) builds on them.

**Tech Stack:** Go 1.25, kong (CLI), BurntSushi/toml, genqlient (Railway GraphQL), lipgloss (styled output), stdlib testing, httptest

---

## Context for the implementor

### How the codebase works

- **CLI layer** (`internal/cli/`): Each command has a `Run(globals *Globals) error` method that wires up real dependencies, and a testable `RunXxx(...)` function that accepts interfaces. All commands embed `Globals` for shared flags.
- **Auth bootstrap** is identical in every `Run()` method: `NewTokenStore → ResolveAuth → NewClient → interface wrapper`. This is the boilerplate referenced in the TODO.
- **Auth commands are different**: `auth login` uses `OAuthClient` + `TokenStore` + `BrowserOpener` directly; `auth logout` only needs `TokenStore`; `auth status` builds its own transport and `OAuthClient`. None of these use `railway.NewClient`, so they cannot share the same helper as config commands.
- **Config pipeline**: `LoadConfigs` (base + local + extras) → `Merge` → `Interpolate` → `Fetch` live → `diff.Compute` → `diff.Format` or `apply.Apply`.
- **Testing style**: External test packages (`package cli_test`), `fakeFetcher`/`recordingMutator` in `helpers_test.go`, `httptest.NewServer` for API mocks, obviously-fake values to avoid gitleaks false positives.
- **Pre-commit hooks** run gitleaks and formatters. Use `mise run check` for full suite. Use `go test -race ./...` for tests.
- **Config parsing** (`internal/config/parse.go`): TOML → `map[string]any` → manual extraction. `knownTopLevelKeys` map tracks non-service keys. Unknown non-table keys are silently skipped (line 69).
- **Config rendering** (`internal/config/render.go`): `tomlQuote` escapes values but does NOT quote keys. Section headers and keys are written bare.
- **Config merging** (`internal/config/merge.go`): `Merge()` uses a local variable named `result` (not `merged`) for the accumulated config.
- **Diff** (`internal/diff/`): `Compute` does additive-only comparison. `Format` uses lipgloss styles.
- **Apply** (`internal/apply/`): Three-phase (settings → shared vars → per-service vars). `Result` has `Applied`, `Failed`, `Skipped` fields — `Skipped` is never incremented.
- **Applier interface** (`internal/apply/apply.go:19-24`):

  ```go
  type Applier interface {
      UpsertVariable(ctx context.Context, service, key, value string, skipDeploys bool) error
      DeleteVariable(ctx context.Context, service, key string) error
      UpdateServiceSettings(ctx context.Context, service string, deploy *config.DesiredDeploy) error
      UpdateServiceResources(ctx context.Context, service string, res *config.DesiredResources) error
  }
  ```

- **Auth** (`internal/auth/`): OAuth 2.0 PKCE flow. `browserCommand` in `login.go` is a **function** variable (`var browserCommand = exec.Command`) — NOT a string. `OpenBrowser` calls `browserCommand(...)` then `cmd.Start()` without `cmd.Wait()`. `CallbackServer` already stores `*http.Server` as a field and has a `Shutdown()` method that calls `s.server.Shutdown(context.Background())` — it just needs a timeout context instead.
- **Auth function signatures**:
  - `RegisterClient(redirectURI string) (*RegistrationResponse, error)` — takes redirectURI, no context
  - `ExchangeCode(clientID, code, redirectURI, codeVerifier string) (*TokenResponse, error)` — note parameter order
  - `ResolveAuth(flagToken string, store *TokenStore) (*ResolvedAuth, error)` — no context
- **Transport** (`internal/railway/transport.go`): `ResolvedAuth.Token` is mutated under `mu` lock inside `RoundTrip` but the field is public and accessible externally.
- **List commands** (`project_list.go`, `environment_list.go`, `workspace_list.go`): Output format switch handles `"json"` and default (text). TOML case is missing — falls through to text. Info structs have `json` tags but no `toml` tags.
- **GraphQL operations**: `variableCollectionUpsert` exists in `schema.graphql` (the introspected schema) but is NOT yet defined in `operations.graphql` and has no generated Go code. The `ServiceInstance` query IS already in `operations.graphql` and has generated Go code, but `FetchLiveConfig` doesn't call it.

### Running tests

```bash
go test -race ./...                           # all tests
go test -race ./internal/config/...           # config package
go test -race ./internal/cli/...              # CLI tests
go test -race -run TestSpecificName ./pkg/... # single test
mise run check                                # full lint + test + build
```

### Hazards

- gitleaks pre-commit hook: use obviously-fake values like `fakekeyfakekeyfakekey` in tests
- TOML formatting: `taplo format` runs on pre-commit, may reformat TOML test fixtures
- Config keys containing dots/spaces: TOML spec requires quoting keys with special chars
- `browserCommand` is a **function** variable (`var browserCommand = exec.Command`), not a string — proposed code must call it as a function
- `TestOpenBrowser` in `login_test.go` uses `SetBrowserCommand` — removing it requires updating this test
- List command structs need `toml` struct tags added for TOML marshalling to work correctly
- `variableCollectionUpsert` must be added to `operations.graphql` and code generation re-run before the batch upsert task

### TODO items explicitly deferred (not in this plan)

These are marked as "future" in TODO.md or require significant API/design work beyond the current scope:

- Extend `config init` with optional service selection (choose which services to include)
- Add `config init` support for environment-specific files (e.g. generate overlay files)
- Resolve project/environment IDs to names when `config init` is passed UUIDs
- Add volume, domain, and TCP proxy management to config (future)
- Make `config init` interactive bootstrap (future)
- Support parsing, validating, and updating standard `railway.toml` native service configurations natively
- Add shell completions (kong custom completers or external generator)
- Improve test coverage for `cmd/fat-controller`, `internal/apply`, and `internal/railway` (ongoing effort, not a single task)

---

## Task 1: Return errors for non-string `project`/`environment` values in config

**Files:**

- Modify: `internal/config/parse.go:39-45`
- Test: `internal/config/parse_test.go`

**Step 1: Write the failing test**

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

**Step 2: Run test to verify it fails**

Run: `go test -race -run TestParse_RejectsNonString ./internal/config/...`
Expected: FAIL — currently these values are silently ignored.

**Step 3: Write minimal implementation**

In `internal/config/parse.go`, replace the project/environment extraction (lines 39-45) with type-checked extraction that returns errors for non-string values:

```go
// Extract project/environment metadata (must be strings if present).
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

**Step 4: Run test to verify it passes**

Run: `go test -race -run TestParse_RejectsNonString ./internal/config/...`
Expected: PASS

**Step 5: Run full test suite**

Run: `go test -race ./...`
Expected: All pass (no existing tests depend on silent ignore behaviour).

**Step 6: Commit**

```bash
git add internal/config/parse.go internal/config/parse_test.go
git commit -m "fix: return errors for non-string project/environment config values"
```

---

## Task 2: Return errors for unrecognised non-table top-level config keys

**Files:**

- Modify: `internal/config/parse.go:63-79`
- Test: `internal/config/parse_test.go`

**Step 1: Write the failing test**

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
	// Tables are service names — should not error.
	cfg, err := config.Parse([]byte("[my_service.variables]\nFOO = \"bar\""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := cfg.Services["my_service"]; !ok {
		t.Error("expected my_service in services")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -race -run TestParse_RejectsUnknownScalar ./internal/config/...`
Expected: FAIL — currently line 69 does `continue` for non-table values.

**Step 3: Write minimal implementation**

In `internal/config/parse.go`, replace the `continue` on line 69 with an error:

```go
for key, val := range raw {
    if knownTopLevelKeys[key] {
        continue
    }
    svcMap, ok := val.(map[string]any)
    if !ok {
        return nil, fmt.Errorf("unrecognised config key %q (not a known setting or service table)", key)
    }
    // ... rest of service parsing
```

**Step 4: Run tests to verify they pass**

Run: `go test -race -run "TestParse_Rejects|TestParse_Accepts" ./internal/config/...`
Expected: PASS

**Step 5: Run full test suite**

Run: `go test -race ./...`
Expected: All pass. Check if any existing tests rely on non-table keys being silently ignored — fix if so.

**Step 6: Commit**

```bash
git add internal/config/parse.go internal/config/parse_test.go
git commit -m "fix: return errors for unrecognised non-table top-level config keys"
```

---

## Task 3: Parse and validate `sensitive_keywords`, `sensitive_allowlist`, and `suppress_warnings` config keys

These keys are in `knownTopLevelKeys` but are never extracted from the parsed TOML. They need to be stored in `DesiredConfig` and threaded through to the masker and (future) warning system.

**Files:**

- Modify: `internal/config/desired.go`
- Modify: `internal/config/parse.go`
- Modify: `internal/config/merge.go`
- Test: `internal/config/parse_test.go`
- Test: `internal/config/merge_test.go`

**Step 1: Write the failing test for parsing**

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

**Step 2: Run test to verify it fails**

Run: `go test -race -run TestParse_ExtractsSensitive ./internal/config/...`
Expected: FAIL — `DesiredConfig` has no such fields yet.

**Step 3: Add fields to DesiredConfig**

In `internal/config/desired.go`:

```go
type DesiredConfig struct {
	Project     string
	Environment string
	Shared      *DesiredVariables
	Services    map[string]*DesiredService

	SensitiveKeywords  []string
	SensitiveAllowlist []string
	SuppressWarnings   []string
}
```

**Step 4: Add parsing logic**

In `internal/config/parse.go`, after the project/environment extraction, add:

```go
// Extract sensitive_keywords, sensitive_allowlist, suppress_warnings.
if v, ok := raw["sensitive_keywords"]; ok {
    list, err := toStringSlice(v, "sensitive_keywords")
    if err != nil {
        return nil, err
    }
    cfg.SensitiveKeywords = list
}
if v, ok := raw["sensitive_allowlist"]; ok {
    list, err := toStringSlice(v, "sensitive_allowlist")
    if err != nil {
        return nil, err
    }
    cfg.SensitiveAllowlist = list
}
if v, ok := raw["suppress_warnings"]; ok {
    list, err := toStringSlice(v, "suppress_warnings")
    if err != nil {
        return nil, err
    }
    cfg.SuppressWarnings = list
}
```

Add new helper:

```go
// toStringSlice converts a []any (TOML array) to []string.
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

**Step 5: Update merge logic**

In `internal/config/merge.go`, in the `Merge` function, add after the project/environment merging (after line 20, before `result.Shared = ...`):

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

Note: The merge function uses the variable name `result`, not `merged`.

**Step 6: Run test to verify it passes**

Run: `go test -race -run TestParse_ExtractsSensitive ./internal/config/...`
Expected: PASS

**Step 7: Run full test suite**

Run: `go test -race ./...`
Expected: All pass.

**Step 8: Commit**

```bash
git add internal/config/desired.go internal/config/parse.go internal/config/parse_test.go internal/config/merge.go internal/config/merge_test.go
git commit -m "feat: parse and validate sensitive_keywords, sensitive_allowlist, and suppress_warnings config keys"
```

---

## Task 4: Quote TOML keys in rendered output

Bare keys containing `.`, spaces, or other special chars produce invalid TOML. The `renderTOML` function writes keys bare — e.g., a variable named `my.key` would render as `my.key = "value"` which TOML interprets as nested tables.

**Files:**

- Modify: `internal/config/render.go`
- Test: `internal/config/render_test.go`

**Step 1: Write the failing test**

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
		t.Errorf("dotted key should be quoted in output:\n%s", output)
	}
	if !strings.Contains(output, `"key with spaces"`) {
		t.Errorf("key with spaces should be quoted in output:\n%s", output)
	}
	// Normal keys should remain bare.
	if strings.Contains(output, `"NORMAL_KEY"`) {
		t.Errorf("normal key should not be quoted:\n%s", output)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -race -run TestRenderTOML_QuotesSpecialKeys ./internal/config/...`
Expected: FAIL

**Step 3: Add key quoting helper and use it**

In `internal/config/render.go`, add:

```go
// tomlKey returns a bare key if it contains only [A-Za-z0-9_-],
// otherwise returns a quoted key.
func tomlKey(key string) string {
	for _, r := range key {
		if !((r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-') {
			return tomlQuote(key)
		}
	}
	return key
}
```

Then replace all bare key writes in `renderTOML` and `writeTOMLDeploy`. For example, in `renderTOML` change:

```go
out.WriteString(k + " = " + tomlQuote(cfg.Shared[k]) + "\n")
```

to:

```go
out.WriteString(tomlKey(k) + " = " + tomlQuote(cfg.Shared[k]) + "\n")
```

Apply this pattern to all variable key writes in `renderTOML` (shared and per-service variables). Section headers (`[name.variables]`) use service names which come from Railway — these are typically safe but could also be quoted for safety.

**Step 4: Run test to verify it passes**

Run: `go test -race -run TestRenderTOML_QuotesSpecialKeys ./internal/config/...`
Expected: PASS

**Step 5: Run full test suite**

Run: `go test -race ./...`
Expected: All pass. Some golden tests may need updating if they relied on bare key output.

**Step 6: Commit**

```bash
git add internal/config/render.go internal/config/render_test.go
git commit -m "fix: quote TOML keys containing dots, spaces, or special characters"
```

---

## Task 5: Handle `toml` output format in list commands

Currently `project list`, `environment list`, and `workspace list` switch on `globals.Output` for `"json"` and default (text). The `"toml"` case silently falls through to text.

**Files:**

- Modify: `internal/cli/project_list.go`
- Modify: `internal/cli/environment_list.go`
- Modify: `internal/cli/workspace_list.go`
- Test: `internal/cli/project_list_test.go`
- Test: `internal/cli/environment_list_test.go`
- Test: `internal/cli/workspace_list_test.go`

**Prerequisite:** The `ProjectInfo`, `EnvironmentInfo`, and `WorkspaceInfo` structs only have `json` struct tags. You must add `toml` struct tags for `toml.Marshal` to produce correctly-named fields. For example:

```go
type ProjectInfo struct {
	ID   string `json:"id" toml:"id"`
	Name string `json:"name" toml:"name"`
}
```

Apply the same to `EnvironmentInfo` and `WorkspaceInfo`.

**Step 1: Write failing tests**

```go
// In project_list_test.go:
func TestRunProjectList_TOMLOutput(t *testing.T) {
	lister := &fakeProjectLister{
		projects: []cli.ProjectInfo{
			{ID: "proj-1", Name: "my-app"},
		},
	}
	var buf bytes.Buffer
	globals := &cli.Globals{Output: "toml"}
	err := cli.RunProjectList(context.Background(), globals, lister, &buf)
	if err != nil {
		t.Fatal(err)
	}
	output := buf.String()
	if !strings.Contains(output, "my-app") {
		t.Errorf("expected project name in TOML output: %s", output)
	}
	if !strings.Contains(output, "proj-1") {
		t.Errorf("expected project ID in TOML output: %s", output)
	}
}
```

Write similar tests for environment and workspace list commands.

**Step 2: Run tests to verify they fail**

Run: `go test -race -run TestRunProjectList_TOMLOutput ./internal/cli/...`
Expected: FAIL — TOML case falls through to text (no TOML structure).

**Step 3: Add TOML output to list commands**

In each list command's `Run*` function, add a `"toml"` case to the output switch. Use `github.com/BurntSushi/toml` to marshal a wrapper struct:

```go
case "toml":
    wrapper := struct {
        Projects []ProjectInfo `toml:"projects"`
    }{Projects: projects}
    return toml.NewEncoder(out).Encode(wrapper)
```

Repeat for environments and workspaces with appropriate field names.

**Step 4: Run tests to verify they pass**

Run: `go test -race -run "TOMLOutput" ./internal/cli/...`
Expected: PASS

**Step 5: Run full test suite**

Run: `go test -race ./...`
Expected: All pass.

**Step 6: Commit**

```bash
git add internal/cli/project_list.go internal/cli/environment_list.go internal/cli/workspace_list.go internal/cli/project_list_test.go internal/cli/environment_list_test.go internal/cli/workspace_list_test.go
git commit -m "feat: handle TOML output format in list commands"
```

---

## Task 6: `config get` path argument should filter by section/key

Currently `config get api.variables.PORT` parses the path to extract the service name for filtering, but returns the entire service config — not just the requested variable.

**Files:**

- Modify: `internal/cli/config_get.go`
- Test: `internal/cli/config_get_test.go`

**Step 1: Write the failing test**

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
		t.Errorf("expected PORT value in output: %s", output)
	}
	if strings.Contains(output, "DEBUG") {
		t.Errorf("should not contain other variables: %s", output)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -race -run TestRunConfigGet_FiltersByPathSectionAndKey ./internal/cli/...`
Expected: FAIL — output contains all variables for the service.

**Step 3: Implement path-based filtering in RunConfigGet**

Restructure `RunConfigGet` to handle the path after fetching. When a specific key is requested, output just the value (not the full config structure). When a section is requested (e.g., `api.variables`), filter to just that section:

```go
// After fetching cfg...
if path != "" {
    parsed, parseErr := config.ParsePath(path)
    if parseErr != nil {
        return parseErr
    }
    // Single variable lookup — output just the value.
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
    // Section-level filtering (e.g., api.variables) — filter cfg
    // to only the requested section data before rendering.
}
```

**Step 4: Run test to verify it passes**

Run: `go test -race -run TestRunConfigGet_FiltersByPathSectionAndKey ./internal/cli/...`
Expected: PASS

**Step 5: Run full test suite**

Run: `go test -race ./...`
Expected: All pass.

**Step 6: Commit**

```bash
git add internal/cli/config_get.go internal/cli/config_get_test.go
git commit -m "feat: filter config get output by section/key path"
```

---

## Task 7: `config set` and `config delete` should offer interactive confirmation

Currently `config set` and `config delete` use a single condition `if !globals.Confirm || globals.DryRun` that goes straight to dry-run output. They should match `config apply`'s behaviour: separate `--dry-run`, non-TTY dry-run, and TTY-interactive-prompt branches.

**Files:**

- Modify: `internal/cli/config_set.go` (the `RunConfigSet` function, approximately lines 24-40)
- Modify: `internal/cli/config_delete.go` (the `RunConfigDelete` function, approximately lines 24-40)
- Test: `internal/cli/config_set_test.go`
- Test: `internal/cli/config_delete_test.go`

**Step 1: Write the failing test for config set**

```go
func TestRunConfigSet_PromptsWhenInteractive(t *testing.T) {
	mut := &recordingMutator{}
	var buf bytes.Buffer
	globals := &cli.Globals{Confirm: false}
	// In non-TTY (test) mode, should still output dry-run message.
	err := cli.RunConfigSet(context.Background(), globals, "api.variables.PORT", "8080", mut, &buf)
	if err != nil {
		t.Fatal(err)
	}
	output := buf.String()
	if !strings.Contains(output, "dry run") {
		t.Error("expected dry-run message in non-TTY mode")
	}
	if mut.called {
		t.Error("should not have called setter in non-TTY mode without --confirm")
	}
}
```

**Step 2: Run test to verify current behaviour**

Run: `go test -race -run TestRunConfigSet_PromptsWhenInteractive ./internal/cli/...`
Expected: PASS (this test validates the non-TTY path still works).

**Step 3: Implement interactive confirmation**

In `RunConfigSet` and `RunConfigDelete`, replace the single `if !globals.Confirm || globals.DryRun` block with the three-branch pattern from `config_apply.go`:

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

Note: `RunConfigSet`/`RunConfigDelete` currently do NOT read from stdin — the interactive prompt via `prompt.ConfirmRW(os.Stdin, out, ...)` is new. This matches `config_apply.go:161`'s pattern.

**Step 4: Run full test suite**

Run: `go test -race ./...`
Expected: All pass. Existing tests run in non-TTY (CI), so the non-interactive path is exercised.

**Step 5: Commit**

```bash
git add internal/cli/config_set.go internal/cli/config_delete.go internal/cli/config_set_test.go internal/cli/config_delete_test.go
git commit -m "feat: add interactive confirmation to config set and config delete"
```

---

## Task 8: Extract shared auth/client boilerplate into a helper

Every config/list CLI `Run` method repeats: `NewTokenStore → ResolveAuth → NewClient`. Extract this into a shared helper.

**Files:**

- Create: `internal/cli/client.go`
- Modify: `internal/cli/config_get.go`
- Modify: `internal/cli/config_set.go`
- Modify: `internal/cli/config_delete.go`
- Modify: `internal/cli/config_diff.go`
- Modify: `internal/cli/config_apply.go`
- Modify: `internal/cli/config_init.go`
- Modify: `internal/cli/project_list.go`
- Modify: `internal/cli/environment_list.go`
- Modify: `internal/cli/workspace_list.go`

**Do NOT modify** `internal/cli/auth.go` — the auth commands (`login`, `logout`, `status`) have different patterns that don't fit a shared helper. `login` needs `OAuthClient` + `TokenStore` + `BrowserOpener` directly; `logout` only needs `TokenStore`; `status` builds a custom transport. None use `railway.NewClient`.

**Step 1: Create the helper**

In `internal/cli/client.go`:

```go
package cli

import (
	"github.com/hamishmorgan/fat-controller/internal/auth"
	"github.com/hamishmorgan/fat-controller/internal/platform"
	"github.com/hamishmorgan/fat-controller/internal/railway"
)

// newClient creates an authenticated Railway client from the globals.
// This consolidates the auth bootstrap boilerplate repeated in every
// config and list command's Run() method.
//
// Not used by auth commands (login/logout/status) which have different
// auth patterns — see internal/cli/auth.go.
func newClient(globals *Globals) (*railway.Client, error) {
	store := auth.NewTokenStore(auth.WithFallbackPath(platform.AuthFilePath()))
	resolved, err := auth.ResolveAuth(globals.Token, store)
	if err != nil {
		return nil, err
	}
	return railway.NewClient(railway.Endpoint, resolved, store, auth.NewOAuthClient()), nil
}
```

**Step 2: Replace boilerplate in config and list Run methods**

In each `Run()` method (config_get.go, config_set.go, config_delete.go, config_diff.go, config_apply.go, config_init.go, project_list.go, environment_list.go, workspace_list.go), replace:

```go
store := auth.NewTokenStore(auth.WithFallbackPath(platform.AuthFilePath()))
resolved, err := auth.ResolveAuth(globals.Token, store)
if err != nil {
    return err
}
client := railway.NewClient(railway.Endpoint, resolved, store, auth.NewOAuthClient())
```

with:

```go
client, err := newClient(globals)
if err != nil {
    return err
}
```

Remove unused imports (`auth`, `platform`) from each file where they're no longer needed.

**Step 3: Run full test suite**

Run: `go test -race ./...`
Expected: All pass — this is a pure refactor, no behaviour change.

**Step 4: Commit**

```bash
git add internal/cli/client.go internal/cli/config_get.go internal/cli/config_set.go internal/cli/config_delete.go internal/cli/config_diff.go internal/cli/config_apply.go internal/cli/config_init.go internal/cli/project_list.go internal/cli/environment_list.go internal/cli/workspace_list.go
git commit -m "refactor: extract shared auth/client bootstrap into newClient helper"
```

---

## Task 9: Extract shared config-load/resolve/fetch/filter logic from diff and apply

`config_diff.go` and `config_apply.go` share nearly identical code for loading configs, interpolating, resolving project/environment, fetching live state, and filtering by service. Extract this into a shared function.

**Files:**

- Create: `internal/cli/config_common.go`
- Modify: `internal/cli/config_diff.go`
- Modify: `internal/cli/config_apply.go`
- Test: `internal/cli/config_common_test.go`

**Step 1: Create the shared function**

In `internal/cli/config_common.go`:

```go
package cli

import (
	"context"

	"github.com/hamishmorgan/fat-controller/internal/config"
)

// configPair holds the loaded desired config and fetched live config,
// ready for diffing or applying.
type configPair struct {
	Desired       *config.DesiredConfig
	Live          *config.LiveConfig
	ProjectID     string
	EnvironmentID string
}

// loadAndFetch loads config files, interpolates, resolves project/environment,
// fetches live state, and filters by service. This is the shared pipeline
// for config diff and config apply.
func loadAndFetch(ctx context.Context, globals *Globals, configDir string, extraFiles []string, fetcher configFetcher) (*configPair, error) {
	// 1. Load and merge config files.
	desired, err := config.LoadConfigs(configDir, extraFiles)
	if err != nil {
		return nil, err
	}

	// 2. Interpolate local env vars.
	if err := config.Interpolate(desired); err != nil {
		return nil, err
	}

	// 3. Use config-file project/environment as fallback.
	project := globals.Project
	if project == "" {
		project = desired.Project
	}
	environment := globals.Environment
	if environment == "" {
		environment = desired.Environment
	}

	// 4. Resolve and fetch live state.
	projID, envID, err := fetcher.Resolve(ctx, globals.Workspace, project, environment)
	if err != nil {
		return nil, err
	}
	live, err := fetcher.Fetch(ctx, projID, envID, globals.Service)
	if err != nil {
		return nil, err
	}

	// 5. Filter desired config by --service if set.
	if globals.Service != "" {
		filtered := &config.DesiredConfig{
			Shared:   desired.Shared,
			Services: make(map[string]*config.DesiredService),
		}
		if svc, ok := desired.Services[globals.Service]; ok {
			filtered.Services[globals.Service] = svc
		}
		desired = filtered
	}

	return &configPair{
		Desired:       desired,
		Live:          live,
		ProjectID:     projID,
		EnvironmentID: envID,
	}, nil
}
```

**Step 2: Refactor config_diff.go to use the shared function**

```go
func RunConfigDiff(ctx context.Context, globals *Globals, configDir string, extraFiles []string, fetcher configFetcher, out io.Writer) error {
	if out == nil {
		out = os.Stdout
	}
	pair, err := loadAndFetch(ctx, globals, configDir, extraFiles, fetcher)
	if err != nil {
		return err
	}
	result := diff.Compute(pair.Desired, pair.Live)
	formatted := diff.Format(result, globals.ShowSecrets)
	_, err = fmt.Fprintln(out, formatted)
	return err
}
```

**Step 3: Refactor config_apply.go to use the shared function**

The `Run()` method in `config_apply.go` needs `projID`/`envID` to construct the `RailwayApplier`. Use `pair.ProjectID` and `pair.EnvironmentID` from the returned `configPair`.

`RunConfigApply` can use `loadAndFetch` for the load/resolve/fetch/filter steps, then proceed with the no-changes check, dry-run/confirm logic, and apply.

**Step 4: Run full test suite**

Run: `go test -race ./...`
Expected: All pass — pure refactor.

**Step 5: Commit**

```bash
git add internal/cli/config_common.go internal/cli/config_diff.go internal/cli/config_apply.go
git commit -m "refactor: extract shared config load/resolve/fetch/filter pipeline"
```

---

## Task 10: Define constants for deploy/resource setting keys

Hard-coded string keys like `"builder"`, `"dockerfile_path"`, `"start_command"` etc. appear in `internal/diff/diff.go`, `internal/apply/convert.go`, and `internal/config/parse.go` and must be kept in sync.

**Files:**

- Create: `internal/config/keys.go`
- Modify: `internal/diff/diff.go` (use constants)
- Modify: `internal/apply/convert.go` (use constants)
- Modify: `internal/config/parse.go` (use constants)
- Test: build verification only (constants are compile-time)

**Step 1: Create constants file**

In `internal/config/keys.go`:

```go
package config

// Deploy setting keys shared across config parsing, diff, and apply.
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

**Step 2: Replace hard-coded strings**

In `internal/diff/diff.go`, `internal/apply/convert.go`, and `internal/config/parse.go`, replace string literals with the constants.

**Step 3: Run full test suite**

Run: `go test -race ./...`
Expected: All pass.

**Step 4: Commit**

```bash
git add internal/config/keys.go internal/diff/diff.go internal/apply/convert.go internal/config/parse.go
git commit -m "refactor: define constants for deploy/resource setting keys"
```

---

## Task 11: Fix `OpenBrowser` zombie process leak

`OpenBrowser` in `auth/login.go` calls `cmd.Start()` without `cmd.Wait()`, leaking zombie processes.

**Files:**

- Modify: `internal/auth/login.go`
- Test: `internal/auth/login_test.go` (verify existing tests still pass)

**Current code** (`login.go:20-31`):

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

Note: `browserCommand` is a **function** variable (`var browserCommand = exec.Command`), not a string.

**Step 1: Fix OpenBrowser**

Add a goroutine to wait on the child process after starting it:

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
    // Wait in a goroutine to reap the child process and avoid zombies.
    go cmd.Wait() //nolint:errcheck
    return nil
}
```

**Step 2: Run full test suite**

Run: `go test -race ./...`
Expected: All pass.

**Step 3: Commit**

```bash
git add internal/auth/login.go
git commit -m "fix: prevent zombie processes in OpenBrowser by waiting on child"
```

---

## Task 12: Remove mutable `browserCommand` package variable

`auth/login.go` has a mutable package-level `browserCommand` variable (`var browserCommand = exec.Command`). The `Login` function already accepts a `BrowserOpener` function parameter for test injection, making the mutable variable unnecessary for most tests. However, `TestOpenBrowser` in `login_test.go` directly uses `SetBrowserCommand` — this test must be updated.

**Files:**

- Modify: `internal/auth/login.go`
- Modify: `internal/auth/login_test.go` (update `TestOpenBrowser` to not use `SetBrowserCommand`)

**Step 1: Replace OpenBrowser with a fixed implementation**

Make `OpenBrowser` use `exec.Command` directly, removing the mutable variable:

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

Remove `browserCommand`, `SetBrowserCommand`, and `BrowserCommand`.

Note: This preserves the existing Windows command (`rundll32 url.dll,FileProtocolHandler`) rather than changing it.

**Step 2: Update TestOpenBrowser**

`TestOpenBrowser` currently uses `SetBrowserCommand` to inject a stub command. Since `OpenBrowser` now uses `exec.Command` directly, this test needs to change. Options:

1. **Remove TestOpenBrowser** — the function is a thin wrapper around `exec.Command` + `Start`. The real browser opening is tested indirectly via `Login` tests that inject a `BrowserOpener`.
2. **Test with a real command** — use `echo` or `true` as the browser command via environment manipulation (fragile).

Recommend option 1: remove `TestOpenBrowser` since `Login` tests already exercise the `BrowserOpener` injection point.

**Step 3: Run full test suite**

Run: `go test -race ./...`
Expected: All pass.

**Step 4: Commit**

```bash
git add internal/auth/login.go internal/auth/login_test.go
git commit -m "refactor: remove mutable browserCommand variable in auth/login.go"
```

---

## Task 13: Add `context.Context` to auth functions and include response body in errors

Several auth functions perform network I/O without accepting a `context.Context`, making them uncancellable. Additionally, `RegisterClient` and `ExchangeCode` don't include response body in error messages (only `RefreshToken` does).

**Files:**

- Modify: `internal/auth/oauth.go` — `RegisterClient`, `ExchangeCode`
- Modify: `internal/auth/resolver.go` — `ResolveAuth`
- Modify: `internal/auth/login.go` — update call sites
- Modify: `internal/cli/auth.go` — update call sites
- Modify: all other CLI `Run()` methods that call `ResolveAuth`
- Test: `internal/auth/oauth_test.go`, `internal/auth/resolver_test.go`

**Step 1: Add context.Context to RegisterClient**

Current signature: `RegisterClient(redirectURI string) (*RegistrationResponse, error)`
New signature: `RegisterClient(ctx context.Context, redirectURI string) (*RegistrationResponse, error)`

Replace `c.httpClient().Post(...)` with `http.NewRequestWithContext(ctx, ...)` + `c.httpClient().Do(req)`.

Also add response body to error message:

```go
if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
    body, _ := io.ReadAll(resp.Body)
    return nil, fmt.Errorf("registration failed with status %d: %s", resp.StatusCode, string(body))
}
```

**Step 2: Add context.Context to ExchangeCode**

Current signature: `ExchangeCode(clientID, code, redirectURI, codeVerifier string) (*TokenResponse, error)`
New signature: `ExchangeCode(ctx context.Context, clientID, code, redirectURI, codeVerifier string) (*TokenResponse, error)`

**Important:** Preserve the existing parameter order (`clientID, code, redirectURI, codeVerifier`) — only prepend `ctx`.

Replace `c.httpClient().PostForm(...)` with `http.NewRequestWithContext(ctx, ...)` + `c.httpClient().Do(req)`.

Add response body to error message (same pattern as `RegisterClient`).

**Step 3: Add context.Context to ResolveAuth**

Current signature: `ResolveAuth(flagToken string, store *TokenStore) (*ResolvedAuth, error)`
New signature: `ResolveAuth(ctx context.Context, flagToken string, store *TokenStore) (*ResolvedAuth, error)`

No network calls currently, but keyring access can block on Linux. Accept ctx for future-proofing and consistency.

**Step 4: Update all call sites**

- `Login` in `login.go` calls `RegisterClient` and `ExchangeCode` — pass context from the Login function (which should also accept `ctx` — see Task 14).
- Every config/list `Run()` method calls `auth.ResolveAuth` — add `context.Background()` as the first argument. If Task 8 is done, update `newClient` instead.
- `auth status` in `auth.go` calls `ResolveAuth` — add `context.Background()`.

**Step 5: Run full test suite**

Run: `go test -race ./...`
Expected: All pass.

**Step 6: Commit**

```bash
git add internal/auth/oauth.go internal/auth/resolver.go internal/auth/login.go internal/cli/
git commit -m "feat: add context.Context to RegisterClient, ExchangeCode, and ResolveAuth; include response body in auth errors"
```

---

## Task 14: Auth callback server and login safety improvements

Multiple related auth safety fixes:

- Change `CallbackServer.Shutdown()` to use a timeout context instead of `context.Background()`
- Tie callback server goroutine lifecycle to context/cancellation
- Make OAuth login wait bounded by context/timeout
- Surface token refresh failures from transport
- Address `ResolvedAuth.Token` thread safety

**Files:**

- Modify: `internal/auth/callback.go`
- Modify: `internal/auth/login.go`
- Modify: `internal/railway/transport.go`
- Modify: `internal/auth/resolver.go`
- Test: `internal/auth/callback_test.go`
- Test: `internal/auth/login_test.go`
- Test: `internal/railway/transport_test.go`

**Step 1: Add timeout to CallbackServer.Shutdown**

`CallbackServer` already stores `*http.Server` as a field (`callback.go:23`). The `Shutdown` method already calls `s.server.Shutdown(context.Background())` (`callback.go:82`). Change to use a timeout context:

```go
// Shutdown gracefully stops the callback server with a timeout.
func (s *CallbackServer) Shutdown() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s.server.Shutdown(ctx) //nolint:errcheck
}
```

**Step 2: Make Login accept context and bound the wait**

```go
func Login(ctx context.Context, oauth *OAuthClient, store *TokenStore, openBrowser BrowserOpener) error {
    // ...
    select {
    case result := <-cb.Result:
        // handle result
    case <-ctx.Done():
        cb.Shutdown()
        return ctx.Err()
    }
}
```

Update the call site in `auth.go` `AuthLoginCmd.Run` to pass a context (e.g., `context.Background()` or a timeout context from `globals.Timeout`).

**Step 3: Surface refresh failures in transport**

In `internal/railway/transport.go`, the `RoundTrip` method at line ~88 silently returns the original 401 response when refresh fails. Return the refresh error instead:

```go
newTokens, refreshErr := t.tryRefresh(req.Context())
if refreshErr != nil {
    resp.Body.Close() //nolint:errcheck
    return nil, fmt.Errorf("authentication failed (token refresh error: %w)", refreshErr)
}
```

**Step 4: Address ResolvedAuth.Token thread safety**

`ResolvedAuth.Token` is mutated under `AuthTransport.mu` in `RoundTrip` but readable externally without synchronisation. The proper fix is to make `Token` unexported with a synchronized accessor, but this is invasive. For now:

1. Add a `sync.Mutex` field to `ResolvedAuth`
2. Add `SetToken(token string)` and `GetToken() string` methods
3. Have the transport use `SetToken` and have external readers use `GetToken`

Alternatively, use `atomic.Value` for the token string.

**Step 5: Run full test suite**

Run: `go test -race ./...`
Expected: All pass.

**Step 6: Commit**

```bash
git add internal/auth/callback.go internal/auth/login.go internal/railway/transport.go internal/auth/resolver.go
git commit -m "fix: add timeout to callback shutdown, bound login wait, surface refresh errors"
```

---

## Task 15: Handle marshal errors in config_apply.go

`config_apply.go` discards errors from `json.MarshalIndent` and `toml.Marshal` with `_`. There are two locations: the "no changes" block (approximately lines 122-138) and the apply results block (approximately lines 179-189).

**Files:**

- Modify: `internal/cli/config_apply.go`

**Step 1: Fix error handling**

Replace all instances of:

```go
b, _ := json.MarshalIndent(...)
```

with:

```go
b, err := json.MarshalIndent(...)
if err != nil {
    return fmt.Errorf("marshalling result: %w", err)
}
```

Apply the same fix to `toml.Marshal` calls.

There are four total: two in the "no changes" output block, two in the apply results output block.

**Step 2: Run full test suite**

Run: `go test -race ./...`
Expected: All pass.

**Step 3: Commit**

```bash
git add internal/cli/config_apply.go
git commit -m "fix: handle json/toml marshal errors in config apply instead of discarding"
```

---

## Task 16: Add `ctx.Err()` check in apply best-effort loops

The apply engine loops through services in `internal/apply/apply.go`. When the context is cancelled, it continues making network calls.

**Files:**

- Modify: `internal/apply/apply.go`
- Test: `internal/apply/apply_test.go`

**Step 1: Write the failing test**

Define a `countingApplier` test double (or reuse `recordingApplier` from existing tests):

```go
type countingApplier struct {
	upsertCount int
}

func (a *countingApplier) UpsertVariable(_ context.Context, _, _, _ string, _ bool) error {
	a.upsertCount++
	return nil
}
func (a *countingApplier) DeleteVariable(_ context.Context, _, _ string) error   { return nil }
func (a *countingApplier) UpdateServiceSettings(_ context.Context, _ string, _ *config.DesiredDeploy) error {
	return nil
}
func (a *countingApplier) UpdateServiceResources(_ context.Context, _ string, _ *config.DesiredResources) error {
	return nil
}

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
	applier := &countingApplier{}
	_, err := apply.Apply(ctx, desired, live, applier, apply.Options{})
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
	if applier.upsertCount > 0 {
		t.Errorf("expected no upsert calls on cancelled context, got %d", applier.upsertCount)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -race -run TestApply_StopsOnContextCancellation ./internal/apply/...`
Expected: FAIL — apply proceeds with network calls.

**Step 3: Add ctx.Err() checks**

In `internal/apply/apply.go`, add at the top of each loop iteration and before each applier call:

```go
if err := ctx.Err(); err != nil {
    return result, err
}
```

**Step 4: Run test to verify it passes**

Run: `go test -race -run TestApply_StopsOnContextCancellation ./internal/apply/...`
Expected: PASS

**Step 5: Run full test suite**

Run: `go test -race ./...`
Expected: All pass.

**Step 6: Commit**

```bash
git add internal/apply/apply.go internal/apply/apply_test.go
git commit -m "fix: check ctx.Err() in apply loops to avoid wasted calls on cancellation"
```

---

## Task 17: Remove `apply.Result.Skipped`

`Result.Skipped` is declared and serialised but never incremented. The current diff-then-apply approach means only actual changes reach the apply engine — there are no semantic "skips". Remove it as dead code.

**Files:**

- Modify: `internal/apply/result.go`
- Modify: `internal/apply/apply.go` (if any reference)
- Test: `internal/apply/apply_test.go` (fix any tests referencing `Skipped`)
- Modify: `internal/cli/e2e_mocked_graphql_test.go` (if it checks `Skipped`)

**Step 1: Remove the field**

In `internal/apply/result.go`, remove `Skipped`:

```go
type Result struct {
	Applied int `json:"applied" toml:"applied"`
	Failed  int `json:"failed" toml:"failed"`
}
```

Update `Summary()` to remove the skipped count from its output.

**Step 2: Run full test suite**

Run: `go test -race ./...`
Expected: Fix any tests that reference `Skipped`, then all pass.

**Step 3: Commit**

```bash
git add internal/apply/result.go internal/apply/apply.go internal/apply/apply_test.go
git commit -m "refactor: remove unused Result.Skipped field"
```

---

## Task 18: CI and build improvements

Three related CI/build improvements:

- Pin GitHub Actions to commit SHAs
- Add `concurrency` with `cancel-in-progress` to CI workflows
- Pin mise tool versions to specific releases

**Files:**

- Modify: `.github/workflows/test.yml`
- Modify: `.github/workflows/build.yml`
- Modify: `.github/workflows/lint-go.yml`
- Modify: `.github/workflows/lint-docs.yml`
- Modify: `.github/workflows/release.yml`
- Modify: `.github/workflows/secrets.yml`
- Modify: `.config/mise/config.toml`

**Step 1: Pin GitHub Actions to commit SHAs**

Look up current latest commit SHAs for:

- `actions/checkout@v4`
- `actions/upload-artifact@v4`
- `jdx/mise-action@v2`

Replace version tags with SHAs. Example:

```yaml
- uses: actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683 # v4.2.2
```

Add a comment with the version tag for readability.

**Step 2: Add concurrency blocks to PR-triggered workflows**

Add to each workflow file (test, build, lint-go, lint-docs):

```yaml
concurrency:
  group: ${{ github.workflow }}-${{ github.ref }}
  cancel-in-progress: true
```

**Step 3: Pin mise tool versions**

In `.config/mise/config.toml`, replace `"latest"` with specific versions. Run `mise ls` to find currently installed versions, then pin to those:

```toml
[tools]
go = "1.25"
golangci-lint = "1.64"            # was "latest" — check actual current
"npm:markdownlint-cli2" = "0.17"  # was "latest" — check actual current
"npm:prettier" = "3.5"            # was "latest" — check actual current
taplo = "0.9"                     # was "latest" — check actual current
actionlint = "1.7"                # was "latest" — check actual current
gitleaks = "8.24"                 # was "latest" — check actual current
apollo-rover = "0.30"             # was "latest" — check actual current
```

**Step 4: Run lint and build to verify**

Run: `mise run check`
Expected: All pass.

**Step 5: Commit**

```bash
git add .github/workflows/ .config/mise/config.toml
git commit -m "chore: pin CI actions to SHAs, add concurrency, pin tool versions"
```

---

## Task 19: Config validation warnings system and `config validate` command

This is the largest task. Implement the warning system described in `docs/WARNINGS.md` and wire up `config validate`.

**Dependencies:** Task 3 (parse `SensitiveKeywords`/`SensitiveAllowlist`/`SuppressWarnings` into `DesiredConfig`).

**Files:**

- Create: `internal/config/validate.go`
- Create: `internal/config/validate_test.go`
- Modify: `internal/cli/cli.go` (remove validate stub, un-hide command)
- Create: `internal/cli/config_validate.go`
- Create: `internal/cli/config_validate_test.go`

**Step 1: Write the failing tests**

```go
// internal/config/validate_test.go
package config_test

import (
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/config"
)

func TestValidate_W003_EmptyServiceBlock(t *testing.T) {
	warnings := config.Validate(&config.DesiredConfig{
		Services: map[string]*config.DesiredService{
			"api": {}, // No variables, resources, or deploy
		},
	}, nil)
	assertHasWarning(t, warnings, "W003")
}

func TestValidate_W012_EmptyStringDelete(t *testing.T) {
	warnings := config.Validate(&config.DesiredConfig{
		Services: map[string]*config.DesiredService{
			"api": {Variables: map[string]string{"OLD_VAR": ""}},
		},
	}, nil)
	assertHasWarning(t, warnings, "W012")
}

func TestValidate_W020_SharedAndServiceConflict(t *testing.T) {
	warnings := config.Validate(&config.DesiredConfig{
		Shared: &config.DesiredVariables{Vars: map[string]string{"PORT": "3000"}},
		Services: map[string]*config.DesiredService{
			"api": {Variables: map[string]string{"PORT": "8080"}},
		},
	}, nil)
	assertHasWarning(t, warnings, "W020")
}

func TestValidate_W030_LowercaseVarName(t *testing.T) {
	warnings := config.Validate(&config.DesiredConfig{
		Services: map[string]*config.DesiredService{
			"api": {Variables: map[string]string{"myVar": "value"}},
		},
	}, nil)
	assertHasWarning(t, warnings, "W030")
}

func TestValidate_W040_UnknownServiceName(t *testing.T) {
	warnings := config.Validate(&config.DesiredConfig{
		Services: map[string]*config.DesiredService{
			"typo-svc": {Variables: map[string]string{"FOO": "bar"}},
		},
	}, []string{"api", "worker"})
	assertHasWarning(t, warnings, "W040")
}

func TestValidate_W041_NothingActionable(t *testing.T) {
	warnings := config.Validate(&config.DesiredConfig{}, nil)
	assertHasWarning(t, warnings, "W041")
}

func TestValidate_SuppressedWarnings(t *testing.T) {
	warnings := config.Validate(&config.DesiredConfig{
		SuppressWarnings: []string{"W041"},
	}, nil)
	assertNoWarning(t, warnings, "W041")
}

func assertHasWarning(t *testing.T, warnings []config.Warning, code string) {
	t.Helper()
	for _, w := range warnings {
		if w.Code == code {
			return
		}
	}
	t.Errorf("expected warning %s, got: %v", code, warnings)
}

func assertNoWarning(t *testing.T, warnings []config.Warning, code string) {
	t.Helper()
	for _, w := range warnings {
		if w.Code == code {
			t.Errorf("did not expect warning %s", code)
		}
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test -race -run TestValidate ./internal/config/...`
Expected: FAIL — `Validate` function doesn't exist yet.

**Step 3: Create the Warning type and Validate function**

In `internal/config/validate.go`:

```go
package config

import (
	"fmt"
	"regexp"
	"strings"
)

// Warning represents a config validation warning.
type Warning struct {
	Code    string // e.g. "W003"
	Message string // human-readable description
	Path    string // dot-path to the problematic item, if applicable
}

// Validate checks a DesiredConfig for common issues and returns warnings.
// liveServiceNames can be nil for offline-only checks (e.g., config validate
// without API calls). When non-nil, enables W040 checks.
func Validate(cfg *DesiredConfig, liveServiceNames []string) []Warning {
	var warnings []Warning

	// W003: Empty service block
	for name, svc := range cfg.Services {
		if len(svc.Variables) == 0 && svc.Resources == nil && svc.Deploy == nil {
			warnings = append(warnings, Warning{
				Code:    "W003",
				Message: fmt.Sprintf("empty service block [%s] — defines no variables, resources, or deploy settings", name),
				Path:    name,
			})
		}
	}

	// W011: Suspicious reference syntax — ${word.word} that probably should be ${{word.word}}
	for name, svc := range cfg.Services {
		for key, val := range svc.Variables {
			if strings.Contains(val, "${") && !strings.Contains(val, "${{") {
				if matched, _ := regexp.MatchString(`\$\{[a-zA-Z_]+\.[a-zA-Z_]+\}`, val); matched {
					warnings = append(warnings, Warning{
						Code:    "W011",
						Message: fmt.Sprintf("suspicious reference syntax in %s.variables.%s: %q looks like a Railway reference — did you mean ${{...}}?", name, key, val),
						Path:    name + ".variables." + key,
					})
				}
			}
		}
	}

	// W012: Empty string is explicit delete
	checkEmptyDelete := func(scope, key, val string) {
		if val == "" {
			warnings = append(warnings, Warning{
				Code:    "W012",
				Message: fmt.Sprintf("%s.%s is set to empty string — this will delete the variable in Railway", scope, key),
				Path:    scope + "." + key,
			})
		}
	}
	for name, svc := range cfg.Services {
		for key, val := range svc.Variables {
			checkEmptyDelete(name+".variables", key, val)
		}
	}
	if cfg.Shared != nil {
		for key, val := range cfg.Shared.Vars {
			checkEmptyDelete("shared.variables", key, val)
		}
	}

	// W020: Variable in both shared and service
	if cfg.Shared != nil {
		for name, svc := range cfg.Services {
			for key := range svc.Variables {
				if _, ok := cfg.Shared.Vars[key]; ok {
					warnings = append(warnings, Warning{
						Code:    "W020",
						Message: fmt.Sprintf("variable %s defined in both shared and %s — service value wins", key, name),
						Path:    name + ".variables." + key,
					})
				}
			}
		}
	}

	// W030: Lowercase variable name
	for name, svc := range cfg.Services {
		for key := range svc.Variables {
			if key != strings.ToUpper(key) {
				warnings = append(warnings, Warning{
					Code:    "W030",
					Message: fmt.Sprintf("variable name %s.variables.%s contains lowercase letters — convention is UPPER_SNAKE_CASE", name, key),
					Path:    name + ".variables." + key,
				})
			}
		}
	}

	// W040: Unknown service name (requires live data)
	if liveServiceNames != nil {
		liveSet := make(map[string]bool, len(liveServiceNames))
		for _, name := range liveServiceNames {
			liveSet[name] = true
		}
		for name := range cfg.Services {
			if !liveSet[name] {
				warnings = append(warnings, Warning{
					Code:    "W040",
					Message: fmt.Sprintf("service %q not found in Railway project", name),
					Path:    name,
				})
			}
		}
	}

	// W041: No services or shared variables
	if len(cfg.Services) == 0 && (cfg.Shared == nil || len(cfg.Shared.Vars) == 0) {
		warnings = append(warnings, Warning{
			Code:    "W041",
			Message: "config defines no services or shared variables",
		})
	}

	// W050: Hardcoded secret in config (requires Task 3 fields)
	masker := NewMasker(cfg.SensitiveKeywords, cfg.SensitiveAllowlist)
	for name, svc := range cfg.Services {
		for key, val := range svc.Variables {
			if val != "" && !strings.Contains(val, "${") && masker.MaskValue(key, val) == MaskedValue {
				warnings = append(warnings, Warning{
					Code:    "W050",
					Message: fmt.Sprintf("possible hardcoded secret in %s.variables.%s — consider using ${VAR} interpolation", name, key),
					Path:    name + ".variables." + key,
				})
			}
		}
	}

	// Filter suppressed warnings.
	if len(cfg.SuppressWarnings) > 0 {
		suppress := make(map[string]bool, len(cfg.SuppressWarnings))
		for _, code := range cfg.SuppressWarnings {
			suppress[code] = true
		}
		filtered := warnings[:0]
		for _, w := range warnings {
			if !suppress[w.Code] {
				filtered = append(filtered, w)
			}
		}
		warnings = filtered
	}

	return warnings
}
```

**Step 4: Run tests to verify they pass**

Run: `go test -race -run TestValidate ./internal/config/...`
Expected: PASS

**Step 5: Implement `config validate` CLI command**

In `internal/cli/config_validate.go`:

```go
package cli

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/hamishmorgan/fat-controller/internal/config"
)

// Run implements `config validate`.
func (c *ConfigValidateCmd) Run(globals *Globals) error {
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}
	return RunConfigValidate(context.Background(), globals, wd, globals.ConfigFiles, os.Stdout)
}

// RunConfigValidate is the testable core of `config validate`.
func RunConfigValidate(ctx context.Context, globals *Globals, configDir string, extraFiles []string, out io.Writer) error {
	if out == nil {
		out = os.Stdout
	}

	desired, err := config.LoadConfigs(configDir, extraFiles)
	if err != nil {
		return err
	}

	warnings := config.Validate(desired, nil) // No live data for offline validation.

	if len(warnings) == 0 {
		_, err := fmt.Fprintln(out, "No warnings.")
		return err
	}

	for _, w := range warnings {
		if w.Path != "" {
			fmt.Fprintf(out, "%s: %s (%s)\n", w.Code, w.Message, w.Path)
		} else {
			fmt.Fprintf(out, "%s: %s\n", w.Code, w.Message)
		}
	}
	return nil
}
```

**Step 6: Remove the stub from cli.go and un-hide the command**

In `internal/cli/cli.go`, remove the stub `Run` method:

```go
func (c *ConfigValidateCmd) Run(globals *Globals) error {
	fmt.Println("config validate: not yet implemented")
	return nil
}
```

Change the `ConfigValidateCmd` field tag from `hidden:""` to just `cmd:""`:

```go
Validate ConfigValidateCmd `cmd:"" help:"Check config file for warnings (no API calls)."`
```

**Step 7: Write CLI test**

```go
// internal/cli/config_validate_test.go
func TestRunConfigValidate_EmptyServiceWarning(t *testing.T) {
	dir := t.TempDir()
	writeTOML(t, dir, "fat-controller.toml", `
project = "test"
environment = "production"

[api.variables]
`)
	var buf bytes.Buffer
	globals := &cli.Globals{}
	err := cli.RunConfigValidate(context.Background(), globals, dir, nil, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "W003") {
		t.Errorf("expected W003 warning: %s", buf.String())
	}
}
```

**Step 8: Run full test suite**

Run: `go test -race ./...`
Expected: All pass.

**Step 9: Commit**

```bash
git add internal/config/validate.go internal/config/validate_test.go internal/cli/config_validate.go internal/cli/config_validate_test.go internal/cli/cli.go
git commit -m "feat: implement config validation warning system and config validate command"
```

---

## Task 20: Apply `--timeout` flag to derived contexts and set per-client HTTP timeouts

The `--timeout` CLI flag is declared but unused. It should be applied to the `context.Background()` calls in command `Run` methods.

**Files:**

- Modify: all CLI command `Run()` methods
- Modify: `internal/cli/client.go` (if Task 8 done)

**Step 1: Apply timeout in Run methods**

In each `Run()` method, replace `context.Background()` with a timeout context:

```go
func (c *ConfigGetCmd) Run(globals *Globals) error {
	ctx, cancel := context.WithTimeout(context.Background(), globals.Timeout)
	defer cancel()
	// ... use ctx instead of context.Background()
}
```

Apply this to all commands that make API calls (config get/set/delete/diff/apply/init, project/environment/workspace list).

**Step 2: Run full test suite**

Run: `go test -race ./...`
Expected: All pass.

**Step 3: Commit**

```bash
git add internal/cli/
git commit -m "feat: apply --timeout flag to command contexts"
```

---

## Task 21: Wire up `--verbose` and `--quiet` flags

These flags are declared but not wired to anything. `--verbose` should enable debug output (HTTP requests, timing). `--quiet` should suppress informational output.

**Files:**

- Create: `internal/cli/output.go`
- Modify: CLI commands as needed
- Test: `internal/cli/output_test.go`

**Step 1: Create output helpers**

In `internal/cli/output.go`:

```go
package cli

import (
	"fmt"
	"os"
)

// info writes informational output to stderr unless quiet mode is active.
func info(globals *Globals, format string, args ...any) {
	if globals.Quiet {
		return
	}
	fmt.Fprintf(os.Stderr, format+"\n", args...)
}

// debug writes debug output to stderr only when verbose mode is active.
func debug(globals *Globals, format string, args ...any) {
	if !globals.Verbose {
		return
	}
	fmt.Fprintf(os.Stderr, "debug: "+format+"\n", args...)
}
```

**Step 2: Add debug logging to key operations**

In config commands (particularly `loadAndFetch` from Task 9, or directly in diff/apply), add `debug` calls:

```go
debug(globals, "loading config from %s", configDir)
debug(globals, "resolving project=%q environment=%q", project, environment)
debug(globals, "fetching live state for project=%s environment=%s", projID, envID)
```

**Step 3: Suppress informational output in quiet mode**

In `config validate`, use `info` for "No warnings." message.
In `config apply`, suppress the summary in quiet mode (but still return non-zero exit code on failures).

**Step 4: Run full test suite**

Run: `go test -race ./...`
Expected: All pass.

**Step 5: Commit**

```bash
git add internal/cli/output.go internal/cli/
git commit -m "feat: wire up --verbose and --quiet flags for output control"
```

---

## Task 22: Batch variable updates using `variableCollectionUpsert`

Instead of calling `variableUpsert` per variable, use `variableCollectionUpsert` for bulk updates. This reduces API calls and triggers only one redeployment.

**Prerequisites:** The `variableCollectionUpsert` mutation exists in `schema.graphql` but is NOT yet defined in `operations.graphql`. You must add it and re-run code generation before implementing.

**Files:**

- Modify: `internal/railway/operations.graphql` (add mutation definition)
- Regenerate: `internal/railway/generated.go` (via `go generate` / `mise run generate`)
- Modify: `internal/railway/mutate.go` (add batch mutation function)
- Modify: `internal/apply/apply.go` (batch variables before calling applier)
- Modify: `internal/apply/railway.go` (implement batch method)
- Test: `internal/apply/apply_test.go`
- Test: `internal/railway/mutate_test.go`

**Step 1: Add the mutation to operations.graphql**

Check `schema.graphql` for `VariableCollectionUpsertInput` type definition, then add to `operations.graphql`:

```graphql
mutation VariableCollectionUpsert($input: VariableCollectionUpsertInput!) {
  variableCollectionUpsert(input: $input)
}
```

**Step 2: Re-run code generation**

Run: `mise run generate`
Verify: `internal/railway/generated.go` now has `VariableCollectionUpsert` function and `VariableCollectionUpsertInput` type.

**Step 3: Add batch mutation function**

In `internal/railway/mutate.go`:

```go
// UpsertVariables sets multiple variables in a single API call.
func UpsertVariables(ctx context.Context, client *Client, projectID, environmentID, serviceID string, variables map[string]string, skipDeploys bool) error {
	input := VariableCollectionUpsertInput{
		ProjectId:     projectID,
		EnvironmentId: environmentID,
		Variables:     variables,
		SkipDeploys:   &skipDeploys,
	}
	if serviceID != "" {
		input.ServiceId = &serviceID
	}
	_, err := VariableCollectionUpsert(ctx, client.GQL(), input)
	return err
}
```

**Step 4: Add batch method to Applier interface**

Add `UpsertVariables` to the interface, preserving the existing `UpsertVariable` for backward compatibility:

```go
type Applier interface {
	UpsertVariable(ctx context.Context, service, key, value string, skipDeploys bool) error
	UpsertVariables(ctx context.Context, service string, variables map[string]string, skipDeploys bool) error
	DeleteVariable(ctx context.Context, service, key string) error
	UpdateServiceSettings(ctx context.Context, service string, deploy *config.DesiredDeploy) error
	UpdateServiceResources(ctx context.Context, service string, res *config.DesiredResources) error
}
```

Note: `skipDeploys bool` must be included (it's part of the existing `UpsertVariable` signature).

**Step 5: Update apply engine to batch upserts**

In `internal/apply/apply.go`, collect all create/update variables for a service/shared scope into a map, then call `UpsertVariables` once. Deletes still happen individually (no batch delete API).

**Step 6: Implement in RailwayApplier**

In `internal/apply/railway.go`, implement `UpsertVariables` using the new `railway.UpsertVariables` function.

**Step 7: Update all test doubles to implement the new method**

All test applier implementations (in apply_test.go, helpers_test.go, e2e tests) need the new `UpsertVariables` method.

**Step 8: Write tests**

**Step 9: Run full test suite**

Run: `go test -race ./...`
Expected: All pass.

**Step 10: Commit**

```bash
git add internal/railway/operations.graphql internal/railway/generated.go internal/railway/mutate.go internal/apply/ internal/cli/
git commit -m "feat: batch variable upserts using variableCollectionUpsert"
```

---

## Task 23: Remaining smaller fixes

These are small, independent fixes that can each be done in a single commit.

### 23a: `resolveServiceID` mutex across network calls

**File:** `internal/apply/railway.go`

The `RailwayApplier.resolveServiceID` method holds a mutex across network calls. Refactor to cache-aside pattern:

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

Commit: `refactor: use cache-aside pattern for service ID resolution in applier`

### 23b: Add workspace as optional top-level config key

**Files:** `internal/config/desired.go`, `internal/config/parse.go`, `internal/config/merge.go`, `internal/config/parse_test.go`

1. Add `Workspace` field to `DesiredConfig`
2. Add `"workspace"` to `knownTopLevelKeys` in `parse.go`
3. Extract workspace in `Parse` (same pattern as project/environment, with non-string error from Task 1)
4. Merge workspace in `Merge` (non-empty overrides, same pattern as project/environment)
5. Thread through as fallback in CLI commands: in `loadAndFetch` (Task 9) or in `Run()` methods, fall back to `desired.Workspace` when `globals.Workspace` is empty
6. Write tests for parsing, merging, and resolution fallback

Commit: `feat: add workspace as optional top-level config key`

### 23c: Include deploy/build settings in live state fetches

**File:** `internal/railway/state.go`

`FetchLiveConfig` currently doesn't populate `Deploy` on `ServiceConfig`. The `ServiceInstance` GraphQL query already exists in `operations.graphql` and has generated Go code — it just needs to be called.

In the service loop in `FetchLiveConfig`, after fetching variables, call `ServiceInstance`:

```go
instance, err := ServiceInstance(ctx, client.GQL(), environmentID, edge.Node.Id)
if err != nil {
    return nil, err
}
svc.Deploy = config.Deploy{
    Builder: string(instance.ServiceInstance.Builder),
    // ... map other fields
}
```

Write tests with `httptest.NewServer` to verify deploy settings are populated.

Commit: `feat: include deploy/build settings in live state fetches`

### 23d: Add WARNINGS.md notice (conditional)

**File:** `docs/WARNINGS.md`

If Task 19 hasn't been implemented yet, add a notice at the top:

```markdown
> **Note:** The warning system described below is planned but not yet
> implemented. Warning codes are reserved for future use.
```

If Task 19 IS implemented, skip this — the notice is unnecessary.

Commit: `docs: add notice that warning system is not yet implemented`

---

## Summary of task dependencies

```text
Task 1  (non-string errors)     ──┐
Task 2  (unknown key errors)    ──┤── Foundation (do first)
Task 3  (parse sensitive keys)  ──┘
                                  │
Task 19 (warnings + validate)  ←──┘ depends on Task 3

Task 8  (auth boilerplate)     ──→ Task 9  (config pipeline)
                                ──→ Task 20 (--timeout)
                                ──→ Task 21 (--verbose/--quiet)

Task 11 (zombie fix)           ──→ Task 12 (remove browserCommand)
Task 13 (context in auth)      ──→ Task 14 (auth safety)
```

**Recommended execution order:**

1. Tasks 1-7 can be done in parallel (independent)
2. Task 8 then Task 9 (sequential refactors)
3. Tasks 10-18 can be done in parallel (independent)
4. Task 19 after Task 3 (depends on parsed fields)
5. Tasks 20-23 can be done in parallel (independent)
