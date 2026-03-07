# Secrets Handling Refactor

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to
> implement this plan task-by-task.

**Goal:** Replace the two-file secrets model (`fat-controller.toml` with
`"********"` + gitignored `fat-controller.local.toml` with `${VAR}` refs) with
a single-file model where secrets are `${VAR}` references inline in
`fat-controller.toml`, and `config init` writes actual secret values to a
gitignored `.env.fat-controller` file.

**Architecture:** Remove `LocalConfigFile` auto-discovery from `LoadConfigs`.
Change `RenderInitTOML` to emit `${VAR_NAME}` instead of `"********"` for
sensitive values. Add `renderEnvFile` to generate `.env.fat-controller` with
actual secret values. Emit a deprecation warning if `fat-controller.local.toml`
is detected. Use `MaskValue` (name + value + entropy) instead of `IsSensitive`
(name-only) for secret classification during init — this avoids false positives
on Railway `${{}}` references and non-secret values that happen to match
sensitive name patterns.

**Tech Stack:** Go 1.26, TOML config, `config.Masker`

**Key design decisions:**

- Users load `.env.fat-controller` into their environment themselves (direnv,
  `source`, CI pipeline secrets). fat-controller does NOT auto-load it.
- The `--config` flag for explicit overlay files is unchanged.
- `config get` display masking (`"********"`) is unchanged.
- W050 (hardcoded secret) validation remains — it catches secrets that should
  use `${VAR}` but don't.
- The `Masker`, `MaskValue`, `IsSensitive` types/functions are unchanged.

---

## Tasks

### Task 1: Remove local config auto-discovery from LoadConfigs

**Files:**

- Modify: `internal/config/load.go:11-56`
- Modify: `internal/config/load_test.go:30-56`

**Step 1: Write the failing test — deprecation warning for local file**

In `internal/config/load_test.go`, replace `TestLoadConfigs_BaseWithLocal`
with a test that verifies the local file is NOT loaded and a deprecation
warning is logged:

```go
func TestLoadConfigs_IgnoresLocalFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "fat-controller.toml"), []byte(`
[api.variables]
PORT = "8080"
APP_ENV = "staging"
`), 0o644); err != nil {
		t.Fatalf("write base: %v", err)
	}
	// Create a local file — it should be ignored.
	if err := os.WriteFile(filepath.Join(dir, "fat-controller.local.toml"), []byte(`
[api.variables]
APP_ENV = "production"
`), 0o644); err != nil {
		t.Fatalf("write local: %v", err)
	}

	cfg, err := config.LoadConfigs(dir, nil)
	if err != nil {
		t.Fatalf("LoadConfigs() error: %v", err)
	}
	// Local file should NOT be merged — APP_ENV should remain "staging".
	if cfg.Services["api"].Variables["APP_ENV"] != "staging" {
		t.Errorf("APP_ENV = %q, want staging (local file should be ignored)",
			cfg.Services["api"].Variables["APP_ENV"])
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -run TestLoadConfigs_IgnoresLocalFile -v`
Expected: FAIL — `APP_ENV = "production"` because local file is still loaded.

**Step 3: Implement — remove local file loading**

In `internal/config/load.go`:

1. Keep the `LocalConfigFile` constant (needed for deprecation detection).
2. Replace lines 47-56 (local file auto-discovery block) with a deprecation
   warning:

```go
	localPath := filepath.Join(dir, LocalConfigFile)
	if _, err := os.Stat(localPath); err == nil {
		slog.Warn("fat-controller.local.toml is deprecated — move secrets "+
			"to ${VAR} references in fat-controller.toml and use "+
			".env.fat-controller for secret values",
			"path", localPath)
	}
```

1. Update the doc comment (lines 18-26) to remove step 2 mentioning the
   local file:

```go
// LoadConfigs loads and merges config files:
//  1. fat-controller.toml from dir (required)
//  2. Extra files from --config flags (in order)
//
// If fat-controller.local.toml exists, a deprecation warning is logged.
// Migrate secrets to ${VAR} references in the base config file.
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/config/ -run TestLoadConfigs -v`
Expected: All pass including `TestLoadConfigs_IgnoresLocalFile`.

**Step 5: Commit**

```bash
git add internal/config/load.go internal/config/load_test.go
git commit -m "refactor: stop loading fat-controller.local.toml, emit deprecation warning"
```

---

### Task 2: Replace `"********"` with `${VAR}` in RenderInitTOML

**Files:**

- Modify: `internal/config/render.go:160-183`
- Modify: `internal/config/render_test.go:275-294`

**Step 1: Write the failing test**

In `internal/config/render_test.go`, update `TestRenderInitTOML_MasksSecrets`
to expect `${VAR}` references instead of masked values:

```go
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
```

Also add a test for Railway references being preserved (not turned into
`${VAR}` refs):

```go
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
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/config/ -run TestRenderInitTOML -v`
Expected: `TestRenderInitTOML_MasksSecrets` FAILS — finds `"********"` instead
of `"${DATABASE_PASSWORD}"`.

**Step 3: Implement — replace masking with env refs in RenderInitTOML**

In `internal/config/render.go`, add a new function `envRefConfig` and modify
`RenderInitTOML`:

```go
// envRefConfig returns a copy of cfg with sensitive variable values replaced
// by ${VAR_NAME} environment references. Railway references (${{...}}) are
// preserved. Non-sensitive values are left as-is.
func envRefConfig(cfg LiveConfig) LiveConfig {
	masker := NewMasker(nil, nil)
	out := LiveConfig{
		ProjectID:     cfg.ProjectID,
		EnvironmentID: cfg.EnvironmentID,
		Shared:        envRefVars(cfg.Shared, masker),
		Services:      make(map[string]*ServiceConfig, len(cfg.Services)),
	}
	for name, svc := range cfg.Services {
		out.Services[name] = &ServiceConfig{
			ID:        svc.ID,
			Name:      svc.Name,
			Variables: envRefVars(svc.Variables, masker),
			Deploy:    svc.Deploy,
		}
	}
	return out
}

// envRefVars replaces sensitive values with ${VAR_NAME} references.
func envRefVars(vars map[string]string, masker *Masker) map[string]string {
	if len(vars) == 0 {
		return vars
	}
	out := make(map[string]string, len(vars))
	for k, v := range vars {
		if masker.MaskValue(k, v) == MaskedValue {
			out[k] = "${" + k + "}"
		} else {
			out[k] = v
		}
	}
	return out
}
```

Then change `RenderInitTOML` to use `envRefConfig` instead of `maskConfig`:

```go
func RenderInitTOML(workspace, project, environment string, cfg LiveConfig) string {
	replaced := envRefConfig(cfg)

	var out strings.Builder
	if workspace != "" {
		out.WriteString("workspace = " + tomlQuote(workspace) + "\n")
	}
	out.WriteString("project = " + tomlQuote(project) + "\n")
	out.WriteString("environment = " + tomlQuote(environment) + "\n")

	body := renderTOML(replaced, false)
	if body != "" {
		out.WriteString("\n")
		out.WriteString(body)
	}

	return out.String()
}
```

Note: `MaskValue` already handles Railway references correctly — if a value
contains `${{`, it returns the value as-is (not `MaskedValue`), so
`envRefVars` will preserve Railway references.

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/config/ -run TestRenderInitTOML -v`
Expected: All pass.

**Step 5: Commit**

```bash
git add internal/config/render.go internal/config/render_test.go
git commit -m "refactor: emit \${VAR} env references instead of ******** in config init"
```

---

### Task 3: Add RenderInitTOML return of detected secrets

`RenderInitTOML` currently returns only the TOML string. We also need to know
which variables were classified as secrets (and their original values) so
`config init` can generate `.env.fat-controller`. Rather than making
`RenderInitTOML` do double-duty, add a separate exported function.

**Files:**

- Modify: `internal/config/render.go`
- Create test: `internal/config/render_test.go`

**Step 1: Write the failing test**

```go
func TestCollectSecrets(t *testing.T) {
	cfg := config.LiveConfig{
		Shared: map[string]string{
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
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -run TestCollectSecrets -v`
Expected: FAIL — `CollectSecrets` does not exist.

**Step 3: Implement CollectSecrets**

In `internal/config/render.go`:

```go
// CollectSecrets returns a map of variable names to their actual values for
// all variables classified as secrets. A variable is a secret if MaskValue
// would mask it (sensitive name or high entropy) AND the value is not a
// Railway reference. The returned map is flat — shared and per-service
// variables are merged (last wins if duplicated, which matches the env var
// namespace).
func CollectSecrets(cfg LiveConfig) map[string]string {
	masker := NewMasker(nil, nil)
	secrets := make(map[string]string)
	for k, v := range cfg.Shared {
		if masker.MaskValue(k, v) == MaskedValue {
			secrets[k] = v
		}
	}
	for _, svc := range cfg.Services {
		for k, v := range svc.Variables {
			if masker.MaskValue(k, v) == MaskedValue {
				secrets[k] = v
			}
		}
	}
	return secrets
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/config/ -run TestCollectSecrets -v`
Expected: PASS.

**Step 5: Commit**

```bash
git add internal/config/render.go internal/config/render_test.go
git commit -m "feat: add CollectSecrets to extract secret name/value pairs from live config"
```

---

### Task 4: Replace renderLocalTOML with renderEnvFile in config init

**Files:**

- Modify: `internal/cli/config_init.go:380-496`
- Modify: `internal/cli/config_init_test.go:164-277,379-420`

**Step 1: Rewrite the config init tests for .env.fat-controller**

Replace the four local-TOML tests in `config_init_test.go`:

```go
const envFile = ".env.fat-controller"

func TestRunConfigInit_CreatesEnvFileWithSecrets(t *testing.T) {
	dir := t.TempDir()
	resolver := newFakeResolver(&config.LiveConfig{
		Services: map[string]*config.ServiceConfig{
			"api": {
				Name: "api",
				Variables: map[string]string{
					"PORT":           "8080",
					"DATABASE_URL":   "postgres://user:pass@host/db",
					"STRIPE_API_KEY": "sk_live_xxx",
					"SESSION_SECRET": "abc123",
					"APP_NAME":       "my-app",
				},
			},
		},
	})
	var buf bytes.Buffer
	err := cli.RunConfigInit(context.Background(), dir, "", "", "", resolver, false, false, true, &buf)
	if err != nil {
		t.Fatalf("RunConfigInit() error: %v", err)
	}

	// .env.fat-controller should contain actual secret values.
	content, err := os.ReadFile(filepath.Join(dir, envFile))
	if err != nil {
		t.Fatalf("reading env file: %v", err)
	}
	got := string(content)

	// Should contain actual values for sensitive vars.
	for _, want := range []string{
		"DATABASE_URL=postgres://user:pass@host/db",
		"SESSION_SECRET=abc123",
		"STRIPE_API_KEY=sk_live_xxx",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in env file, got:\n%s", want, got)
		}
	}

	// Non-sensitive vars should NOT be in the env file.
	if strings.Contains(got, "PORT=") {
		t.Errorf("PORT is not sensitive — should not be in env file:\n%s", got)
	}
	if strings.Contains(got, "APP_NAME=") {
		t.Errorf("APP_NAME is not sensitive — should not be in env file:\n%s", got)
	}

	// The base config should have ${VAR} refs, not ******** or literal secrets.
	base, _ := os.ReadFile(filepath.Join(dir, "fat-controller.toml"))
	baseStr := string(base)
	if strings.Contains(baseStr, "sk_live_xxx") {
		t.Errorf("secret value should not appear in base config:\n%s", baseStr)
	}
	if strings.Contains(baseStr, "********") {
		t.Errorf("masked placeholder should not appear in base config:\n%s", baseStr)
	}
}

func TestRunConfigInit_EnvFileSharedSecrets(t *testing.T) {
	dir := t.TempDir()
	resolver := newFakeResolver(&config.LiveConfig{
		Shared: map[string]string{
			"GLOBAL_SECRET": "s3cr3t",
			"APP_MODE":      "production",
		},
		Services: map[string]*config.ServiceConfig{
			"api": {Name: "api", Variables: map[string]string{"PORT": "8080"}},
		},
	})
	var buf bytes.Buffer
	err := cli.RunConfigInit(context.Background(), dir, "", "", "", resolver, false, false, true, &buf)
	if err != nil {
		t.Fatalf("RunConfigInit() error: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, envFile))
	if err != nil {
		t.Fatalf("reading env file: %v", err)
	}
	got := string(content)

	if !strings.Contains(got, "GLOBAL_SECRET=s3cr3t") {
		t.Errorf("expected GLOBAL_SECRET in env file:\n%s", got)
	}
	if strings.Contains(got, "APP_MODE=") {
		t.Errorf("APP_MODE is not sensitive — should not be in env file:\n%s", got)
	}
}

func TestRunConfigInit_NoSecretsNoEnvFile(t *testing.T) {
	dir := t.TempDir()
	resolver := newFakeResolver(&config.LiveConfig{
		Services: map[string]*config.ServiceConfig{
			"api": {Name: "api", Variables: map[string]string{"PORT": "8080", "APP_NAME": "hello"}},
		},
	})
	var buf bytes.Buffer
	err := cli.RunConfigInit(context.Background(), dir, "", "", "", resolver, false, false, true, &buf)
	if err != nil {
		t.Fatalf("RunConfigInit() error: %v", err)
	}

	// No secrets → no .env.fat-controller file.
	if _, err := os.Stat(filepath.Join(dir, envFile)); !os.IsNotExist(err) {
		t.Error("should not create .env.fat-controller when no secrets detected")
	}
}

func TestRunConfigInit_DryRunWritesNoFiles(t *testing.T) {
	dir := t.TempDir()
	resolver := newFakeResolver(&config.LiveConfig{
		ProjectID:     "proj-1",
		EnvironmentID: "env-1",
		Services: map[string]*config.ServiceConfig{
			"api": {
				Name:      "api",
				Variables: map[string]string{"PORT": "8080", "DATABASE_URL": "postgres://..."},
			},
		},
	})
	var buf bytes.Buffer
	err := cli.RunConfigInit(context.Background(), dir, "", "", "", resolver, false, true, false, &buf)
	if err != nil {
		t.Fatalf("RunConfigInit() error: %v", err)
	}

	// No files should be written.
	if _, err := os.Stat(filepath.Join(dir, "fat-controller.toml")); !os.IsNotExist(err) {
		t.Error("dry-run should not create fat-controller.toml")
	}
	if _, err := os.Stat(filepath.Join(dir, envFile)); !os.IsNotExist(err) {
		t.Error("dry-run should not create .env.fat-controller")
	}
	if _, err := os.Stat(filepath.Join(dir, ".gitignore")); !os.IsNotExist(err) {
		t.Error("dry-run should not create .gitignore")
	}

	got := buf.String()
	if !strings.Contains(got, "dry run") {
		t.Errorf("expected 'dry run' in output, got:\n%s", got)
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/cli/ -run 'TestRunConfigInit_(CreatesEnvFile|EnvFileShared|NoSecretsNoEnv|DryRun)' -v`
Expected: FAIL — old code creates local.toml, not .env.fat-controller.

**Step 3: Implement — replace renderLocalTOML with renderEnvFile**

In `internal/cli/config_init.go`:

1. Delete the entire `renderLocalTOML` function (lines 427-496).

2. Add `renderEnvFile`:

```go
// renderEnvFile generates a .env file with KEY=VALUE lines for each secret
// detected in the live config. Returns empty string if no secrets found.
func renderEnvFile(cfg *config.LiveConfig) string {
	secrets := config.CollectSecrets(*cfg)
	if len(secrets) == 0 {
		return ""
	}

	keys := make([]string, 0, len(secrets))
	for k := range secrets {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var out strings.Builder
	out.WriteString("# Secret values for fat-controller (gitignored).\n")
	out.WriteString("# Load into your environment before running config apply.\n")
	out.WriteString("# e.g. source .env.fat-controller\n\n")
	for _, k := range keys {
		_, _ = fmt.Fprintf(&out, "%s=%s\n", k, secrets[k])
	}
	return out.String()
}
```

1. Replace lines 380-422 (dry-run block, local file creation, gitignore
   update) with the new flow. The updated section in `RunConfigInit` after
   the config file is rendered (line 378 onwards):

```go
	// Collect secrets for .env.fat-controller.
	envContent := renderEnvFile(filtered)
	envFileName := ".env.fat-controller"

	if dryRun {
		_, _ = fmt.Fprintf(out, "dry run: would write %s (%d services)\n\n%s\n",
			config.BaseConfigFile, len(filtered.Services), content)
		if envContent != "" {
			_, _ = fmt.Fprintf(out, "\ndry run: would write %s\n\n%s\n",
				envFileName, envContent)
			_, _ = fmt.Fprintf(out, "dry run: would ensure %s is in .gitignore\n",
				envFileName)
		}
		return nil
	}

	if !yes && !interactive {
		_, _ = fmt.Fprintf(out, "would write %s (%d services)\n\n%s\n",
			config.BaseConfigFile, len(filtered.Services), content)
		_, _ = fmt.Fprintf(out, "use --yes to write files\n")
		return nil
	}

	// 6. Write the config file.
	if err := os.WriteFile(configPath, []byte(content+"\n"), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", config.BaseConfigFile, err)
	}
	_, _ = fmt.Fprintf(out, "wrote %s (%d services)\n",
		config.BaseConfigFile, len(filtered.Services))

	// 7. Write .env.fat-controller with actual secret values.
	if envContent != "" {
		envPath := filepath.Join(dir, envFileName)
		if err := os.WriteFile(envPath, []byte(envContent), 0o600); err != nil {
			return fmt.Errorf("writing %s: %w", envFileName, err)
		}
		_, _ = fmt.Fprintf(out, "wrote %s (secret values — do not commit)\n",
			envFileName)

		added, err := ensureGitignoreHasLine(dir, envFileName)
		if err != nil {
			return fmt.Errorf("updating .gitignore: %w", err)
		}
		slog.Debug("gitignore check", "line", envFileName, "added", added)
		if added {
			_, _ = fmt.Fprintf(out, "updated .gitignore (added %s)\n",
				envFileName)
		}
	}

	return nil
```

Note: The env file is written with mode `0o600` (owner-only) since it
contains actual secrets.

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/cli/ -run TestRunConfigInit -v`
Expected: All pass.

**Step 5: Commit**

```bash
git add -f internal/cli/config_init.go internal/cli/config_init_test.go
git commit -m "refactor: replace local.toml with .env.fat-controller for secret values"
```

---

### Task 5: Remove W051, add deprecation warning in ValidateFiles

**Files:**

- Modify: `internal/config/validate.go:292-323`
- Modify: `internal/config/validate_test.go:467-507,578-615`

**Step 1: Write the failing tests**

In `internal/config/validate_test.go`, replace the W051 tests and the W021
local-override integration test:

```go
// --- W051: Deprecated — removed (local file no longer auto-discovered) ---

func TestValidateFiles_DeprecationWarning_LocalFileExists(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, config.LocalConfigFile),
		[]byte("# local"), 0o644); err != nil {
		t.Fatal(err)
	}
	warnings := config.ValidateFiles(dir)
	assertHasWarning(t, warnings, "W052")
}

func TestValidateFiles_NoWarning_NoLocalFile(t *testing.T) {
	dir := t.TempDir()
	warnings := config.ValidateFiles(dir)
	assertNoWarning(t, warnings, "W052")
}
```

Delete these tests entirely:

- `TestValidateFiles_W051_LocalNotGitignored`
- `TestValidateFiles_W051_GitignoreContainsLocal`
- `TestValidateFiles_W051_NoLocalFile`
- `TestValidateFiles_W051_GitignoreWithoutLocal`
- `TestLoadConfigs_W021_LocalOverride`

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/config/ -run TestValidateFiles -v`
Expected: FAIL — W052 code not emitted.

**Step 3: Implement — replace W051 with W052 deprecation**

In `internal/config/validate.go`, replace `ValidateFiles`:

```go
// ValidateFiles checks filesystem conditions that can't be detected from
// the parsed config alone.
func ValidateFiles(dir string) []Warning {
	var warnings []Warning
	localPath := filepath.Join(dir, LocalConfigFile)
	if _, err := os.Stat(localPath); err == nil {
		warnings = append(warnings, Warning{
			Code: "W052",
			Message: fmt.Sprintf("%s is deprecated — move secrets to ${VAR} "+
				"references in %s and use .env.fat-controller for secret values",
				LocalConfigFile, BaseConfigFile),
			Path: LocalConfigFile,
		})
	}
	return warnings
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/config/ -v`
Expected: All pass.

**Step 5: Commit**

```bash
git add internal/config/validate.go internal/config/validate_test.go
git commit -m "refactor: replace W051 with W052 deprecation warning for local config file"
```

---

### Task 6: Remove e2e test for local overlay

**Files:**

- Modify: `internal/cli/e2e_mocked_graphql_test.go:1264-1306`

**Step 1: Remove the subtest**

Delete the entire `t.Run("config apply with local overlay merges values", ...)`
block (lines 1264-1306). No replacement needed — the local file auto-discovery
is removed so there is nothing to test.

**Step 2: Run e2e tests to verify nothing breaks**

Run: `go test ./internal/cli/ -run TestCLIE2E -v`
Expected: All remaining subtests pass.

**Step 3: Commit**

```bash
git add internal/cli/e2e_mocked_graphql_test.go
git commit -m "test: remove e2e test for deprecated local overlay"
```

---

### Task 7: Update the project's own config files

**Files:**

- Modify: `fat-controller.toml`
- Remove: `fat-controller.local.toml`
- Modify: `.gitignore`

**Step 1: Update fat-controller.toml — replace `"********"` with `${VAR}`**

Replace all five masked values:

| Line | Old | New |
|------|-----|-----|
| 6 | `MEILI_MASTER_KEY = "********"` | `MEILI_MASTER_KEY = "${MEILI_MASTER_KEY}"` |
| 18 | `HANKO_SECRET_KEY = "********"` | `HANKO_SECRET_KEY = "${HANKO_SECRET_KEY}"` |
| 21 | `SMTP_PASSWORD = "********"` | `SMTP_PASSWORD = "${SMTP_PASSWORD}"` |
| 44 | `POSTGRES_PASSWORD = "********"` | `POSTGRES_PASSWORD = "${POSTGRES_PASSWORD}"` |
| 47 | `SSL_CERT_DAYS = "********"` | `SSL_CERT_DAYS = "${SSL_CERT_DAYS}"` |

**Step 2: Delete fat-controller.local.toml**

```bash
rm fat-controller.local.toml
```

**Step 3: Update .gitignore**

Remove the explicit `fat-controller.local.toml` line (the `*.local.*` glob
already covers it). Add `.env.fat-controller`:

Replace:

```text
fat-controller.local.toml
```

With:

```text
.env.fat-controller
```

**Step 4: Commit**

```bash
git add fat-controller.toml .gitignore
git rm fat-controller.local.toml
git commit -m "chore: migrate config to \${VAR} references, remove local.toml"
```

---

### Task 8: Update docs

**Files:**

- Modify: `docs/WARNINGS.md`
- Modify: `docs/CONFIG-FORMAT.md`
- Modify: `docs/DECISIONS.md`
- Modify: `docs/TODO.md:67`
- Modify: `internal/config/desired.go:6` (comment only)

**Step 1: Update WARNINGS.md**

- Line 3: `When loading fat-controller.toml (and fat-controller.local.toml)`
  → `When loading fat-controller.toml`
- Line 34: W021 row — change description to remove local file mention:
  `Same variable defined in base config and override file — later value wins`
- Line 55: Replace W051 row with W052:
  `W052 | Deprecated local override file | fat-controller.local.toml exists — migrate to ${VAR} references`

**Step 2: Update CONFIG-FORMAT.md**

- Lines 10-11: Remove `An optional fat-controller.local.toml ...` sentence.
- Lines 73-75: Remove the `fat-controller.local.toml` line from the multi-file
  config list.
- Lines 93-104: Update the secret handling section to describe the new model —
  `${VAR}` inline in the committed config, `.env.fat-controller` for actual
  values:

```markdown
## Secret handling

With additive-only semantics, secrets that aren't in the config are simply
ignored. Three patterns for managing secrets:

1. **Don't mention them** — set in the dashboard, untouched by this tool.
   Works because unmentioned = ignored.
2. **Railway references** — `DATABASE_URL = "${{postgres.DATABASE_URL}}"`.
   Safe to commit. Railway resolves at runtime.
3. **Local env interpolation** — `STRIPE_KEY = "${STRIPE_KEY}"`. Resolved
   from local environment at apply time. Config file is safe to commit;
   actual value comes from CI env vars or `.env.fat-controller`.

`config init` generates a `.env.fat-controller` file (gitignored) with
actual secret values pulled from Railway. Load it into your environment
before running `config apply` (e.g. `source .env.fat-controller`, direnv,
or CI pipeline secrets).
```

**Step 3: Update DECISIONS.md**

Lines 21-23: Replace:

```text
Multi-file merging provides additional flexibility: a gitignored
`fat-controller.local.toml` is auto-discovered, and `--config` can be
repeated for explicit layering.
```

With:

```text
`config init` generates `.env.fat-controller` (gitignored) with actual
secret values pulled from Railway. The base config uses `${VAR}` references
for secrets. `--config` can be repeated for explicit overlay layering.
```

**Step 4: Update TODO.md line 67**

Change:

```text
- [x] Update `.gitignore` automatically when creating `fat-controller.local.toml` (optional safety).
```

To:

```text
- [x] Update `.gitignore` automatically when creating `.env.fat-controller` (optional safety).
```

**Step 5: Update desired.go line 6 comment**

Change:

```go
Source string // e.g. "local override"
```

To:

```go
Source string // e.g. "extra.toml"
```

**Step 6: Commit**

```bash
git add docs/WARNINGS.md docs/CONFIG-FORMAT.md docs/DECISIONS.md docs/TODO.md internal/config/desired.go
git commit -m "docs: update for secrets refactor — \${VAR} references replace local.toml"
```

---

### Task 9: Regenerate CLI docs and run full check

**Step 1: Regenerate CLI docs**

```bash
mise run docs:cli
```

**Step 2: Run full check**

```bash
mise run check
```

Expected: All linters, tests, and build pass.

**Step 3: Commit if docs changed**

```bash
git add docs/cli/
git diff --cached --quiet || git commit -m "docs: regenerate CLI reference"
```

**Step 4: Push**

```bash
git push
```

---
