# Output, Polish, and CI Hardening

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Polish output formatting, implement the full color precedence chain, add `--raw` output format, harden CI testing, and add shell completions for all new commands.

**Architecture:** Output follows the `auto`/`text`/`json`/`toml`/`raw` format model. Color follows a strict precedence: CLI flag > `NO_COLOR`/`FORCE_COLOR` > `CLICOLOR`/`CLICOLOR_FORCE` > `TERM=dumb` > auto-detect. All commands support `--json`/`--toml` for machine-readable output. `--raw` outputs a bare scalar value.

**Tech Stack:** Go 1.26, charmbracelet/lipgloss (color), Kong CLI.

**Depends on:** All other plans (this is the final polish pass).

---

## Context for the implementer

### Key files

| File | Role |
|------|------|
| `cmd/fat-controller/main.go` | `applyColorMode()` — color initialization |
| `internal/cli/cli.go` | Global flags, output format |
| `internal/cli/help.go` | Help text formatting, terminal width |
| `internal/config/render.go` | Config output rendering |
| `internal/diff/format.go` | Diff output formatting |

### Current color handling

`applyColorMode()` in `main.go` checks:

1. `--color` flag (manual os.Args scan)
2. `FAT_CONTROLLER_COLOR` env var
3. `NO_COLOR` env var
4. Auto-detect via lipgloss/termenv

Missing: `FORCE_COLOR`, `CLICOLOR`, `CLICOLOR_FORCE`, `TERM=dumb`.

---

## Task 1: Implement full color precedence chain

**Files:**

- Modify: `cmd/fat-controller/main.go`
- Test: `cmd/fat-controller/main_test.go` (or `internal/cli/color_test.go`)

### Step 1: Write tests for color precedence

```go
func TestColorPrecedence_NoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("FORCE_COLOR", "1")
	// NO_COLOR should win over FORCE_COLOR.
	mode := resolveColorMode("")
	if mode != "never" {
		t.Errorf("mode = %q, want never", mode)
	}
}

func TestColorPrecedence_ForceColor(t *testing.T) {
	t.Setenv("FORCE_COLOR", "1")
	mode := resolveColorMode("")
	if mode != "always" {
		t.Errorf("mode = %q, want always", mode)
	}
}

func TestColorPrecedence_CliColor0(t *testing.T) {
	t.Setenv("CLICOLOR", "0")
	mode := resolveColorMode("")
	if mode != "never" {
		t.Errorf("mode = %q, want never", mode)
	}
}

func TestColorPrecedence_CliColorForce(t *testing.T) {
	t.Setenv("CLICOLOR_FORCE", "1")
	mode := resolveColorMode("")
	if mode != "always" {
		t.Errorf("mode = %q, want always", mode)
	}
}

func TestColorPrecedence_TermDumb(t *testing.T) {
	t.Setenv("TERM", "dumb")
	mode := resolveColorMode("")
	if mode != "never" {
		t.Errorf("mode = %q, want never", mode)
	}
}

func TestColorPrecedence_FlagWins(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	mode := resolveColorMode("always")
	if mode != "always" {
		t.Errorf("mode = %q, want always (flag wins)", mode)
	}
}
```

### Step 2: Run tests — expect fail

### Step 3: Implement resolveColorMode

Extract the color resolution logic into a testable function:

```go
// resolveColorMode determines the color mode from flag and env vars.
// Precedence: flag > NO_COLOR/FORCE_COLOR > CLICOLOR/CLICOLOR_FORCE > TERM=dumb > auto.
func resolveColorMode(flag string) string {
	// 1. CLI flag (explicit)
	if flag != "" {
		return flag
	}

	// 2. NO_COLOR (any value = never)
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		return "never"
	}

	// 3. FORCE_COLOR (non-zero = always)
	if v := os.Getenv("FORCE_COLOR"); v != "" && v != "0" {
		return "always"
	}

	// 4. CLICOLOR (0 = never)
	if os.Getenv("CLICOLOR") == "0" {
		return "never"
	}

	// 5. CLICOLOR_FORCE (non-zero = always)
	if v := os.Getenv("CLICOLOR_FORCE"); v != "" && v != "0" {
		return "always"
	}

	// 6. TERM=dumb
	if os.Getenv("TERM") == "dumb" {
		return "never"
	}

	// 7. Auto-detect
	return "auto"
}
```

### Step 4: Run tests — expect pass

### Step 5: Commit

---

## Task 2: Implement `--raw` output format

**Files:**

- Modify: `internal/cli/cli.go` (add `--raw` flag)
- Modify: `internal/cli/show.go` (handle raw output)
- Test: `internal/cli/show_test.go`

### Step 1: Write tests

```go
func TestShow_Raw_SingleValue(t *testing.T) {
	// show api.variables.PORT --raw → "8080" (no quoting, no newline decorations)
}

func TestShow_Raw_Table_Errors(t *testing.T) {
	// show api --raw → error (can't raw-output a table)
}
```

### Step 2: Implement

`--raw` output: the bare value with no quoting, structure, or
formatting. Only valid when the result is a single scalar. For `show`,
this means the path must resolve to a single key-value pair (e.g.,
`show api.variables.PORT`). If the result is a table or list, error
with a clear message.

### Step 3: Run tests — expect pass

### Step 4: Commit

---

## Task 3: Ensure all commands support --json/--toml

Audit every command and ensure structured output works:

**Files:**

- Various CLI files

### Step 1: Audit each command

For each command, verify that `--json` outputs valid JSON and `--toml`
outputs valid TOML. Commands to check:

- `show` (all path variants)
- `diff`
- `apply` (result summary)
- `adopt` (result summary)
- `validate` (warnings as structured data)
- `list` (all entity types)
- `status` (per-service health)
- `deploy`/`redeploy`/`restart`/`rollback`/`stop` (result)
- `auth status`

### Step 2: Add missing format support

### Step 3: Write tests for each format

### Step 4: Commit

---

## Task 4: Secret masking in all outputs

Ensure secrets are masked consistently across all commands and
output formats.

**Files:**

- Modify: `internal/config/render.go`
- Modify: `internal/diff/format.go`

### Step 1: Audit masking

Verify that:

- `show` masks secrets by default, `--show-secrets` reveals
- `diff` masks live values by default
- `adopt` masks values in preview
- `apply` masks values in preview
- JSON/TOML output also masks

### Step 2: Fix any gaps

### Step 3: Write tests

### Step 4: Commit

---

## Task 5: Env file orphan warnings in validate

**Files:**

- Modify: `internal/config/validate.go`
- Test: `internal/config/validate_test.go`

### Step 1: Write test

```go
func TestValidate_W080_OrphanedEnvFileEntry(t *testing.T) {
	// Env file has KEY=value but no ${KEY} reference in any config.
	// validate should warn W080.
}
```

### Step 2: Implement W080

In `Validate`, if env file paths are known, load them and check
that every key in the env file is referenced by at least one
`${KEY}` pattern in the merged config.

### Step 3: Run tests — expect pass

### Step 4: Commit

---

## Task 6: Shell completions for new commands

**Files:**

- Modify: `internal/cli/completion.go`

### Step 1: Verify Kong completions auto-discover new commands

Kong + kong-completion should automatically generate completions for
all registered commands. Verify by running:

```bash
go run ./cmd/fat-controller completion bash
go run ./cmd/fat-controller completion zsh
go run ./cmd/fat-controller completion fish
```

### Step 2: Add custom completers for dynamic values

Where possible, add shell completion for:

- Service names (from config file)
- Entity types for `list`
- `--color` values (`auto`, `always`, `never`)

### Step 3: Commit

---

## Task 7: Update documentation

**Files:**

- Modify: `docs/COMMANDS.md` (update to reflect new command tree)
- Modify: `docs/CONFIG-FORMAT.md` (update to reflect new schema)
- Modify: `docs/TODO.md` (mark completed items, add remaining)

### Step 1: Rewrite COMMANDS.md

Replace the current command reference with the new structure.
Remove the old `config *` subcommand documentation. Add all new
commands with their flags and examples.

### Step 2: Rewrite CONFIG-FORMAT.md

Update to reflect the `[[service]]` schema, `[tool]` table,
`[workspace]`/`[project]` tables, env files, file cascade.

### Step 3: Update TODO.md

Mark completed features, remove obsolete items, add any remaining
gaps discovered during implementation.

### Step 4: Commit

---

## Task 8: Update E2E tests

**Files:**

- Modify: `internal/cli/e2e_mocked_graphql_test.go`

### Step 1: Update mock server for new operations

The mock GraphQL server needs to handle all new queries and mutations
added in Plan 4. Add response handlers for:

- Service CRUD
- Domain CRUD
- Volume CRUD
- Deployment lifecycle
- Expanded ServiceInstance query

### Step 2: Add E2E tests for new commands

Write E2E tests that exercise the full pipeline:

- `adopt` → creates config file from mocked Railway state
- `diff` → shows differences
- `apply` → pushes changes (with merge flags)
- `show` → displays state
- `deploy` → triggers deployment

### Step 3: Run tests — expect pass

### Step 4: Commit

---

## Task 9: Update CI workflow

**Files:**

- Modify: `.github/workflows/ci.yml`

### Step 1: Update docs freshness check

The docs freshness check validates the example config against a JSON
schema. Update the schema for the new config format, or remove the
check if the schema is not yet updated.

### Step 2: Ensure coverage threshold is still met

The coverage threshold is 40%. With all the new code, verify it's
still met. Adjust if needed.

### Step 3: Commit

---

## Task 10: Update example config and schema

**Files:**

- Modify: `docs/fat-controller.example.toml` (if not done in Plan 1)
- Modify: `docs/fat-controller.schema.json` (update for new format)

### Step 1: Write a comprehensive example config

The example should demonstrate all major features:

- Environment identity
- Workspace and project context
- Tool settings
- Multiple services with different types
- Variables with `${VAR}` and `${{service.VAR}}`
- Deploy settings
- Resources
- Domains (custom + service)
- Volumes
- TCP proxies
- Triggers

### Step 2: Update JSON schema

### Step 3: Verify with taplo

Run: `taplo check --schema "file://$(pwd)/docs/fat-controller.schema.json" docs/fat-controller.example.toml`

### Step 4: Commit

---

## Task 11: Final comprehensive verification

### Step 1: Run `mise run check`

All linters, tests, and checks must pass.

### Step 2: Run `go test -race ./...`

No race conditions.

### Step 3: Build for all platforms

Run: `go build -o /dev/null ./cmd/fat-controller`

### Step 4: Manual smoke test

```bash
fat-controller --help
fat-controller auth status
fat-controller show --help
fat-controller diff --help
fat-controller apply --help
fat-controller adopt --help
fat-controller new --help
fat-controller list --help
fat-controller deploy --help
fat-controller logs --help
fat-controller status --help
fat-controller ssh --help
fat-controller open --help
fat-controller validate --help
fat-controller completion bash | head -5
```

### Step 5: Verify --version output

```bash
fat-controller --version
```
