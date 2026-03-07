# Command Restructure and Core Commands

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Reorganize the CLI from nested `config *` subcommands to top-level `adopt`, `diff`, `apply`, `validate`, `show`, `new` commands. Implement merge flags, path scoping, ID bookkeeping, and the full interactive prompting model.

**Architecture:** Commands are top-level verbs. `adopt` (Railway → config), `diff` (compare), `apply` (config → Railway), `validate` (offline), `show` (read-only live state), `new` (local scaffolding). Merge flags (`--create`/`--update`/`--delete`) control what the merge does. Path scoping narrows operations. ID bookkeeping writes resolved IDs back to config. The `tool.prompt` enum (`all`/`default`/`none`) controls interactive behavior.

**Tech Stack:** Go 1.26, Kong CLI framework, charmbracelet/huh for interactive prompts.

**Depends on:** Plan 1 (Config Schema Migration) and Plan 2 (File Cascade and Env Files).

---

## Context for the implementer

### Current command tree

```text
auth login|logout|status
config init|get|set|delete|diff|apply|validate
workspace list
project list
environment list
completion
```

### Target command tree

```text
auth login|logout|status
new project|environment|service
adopt [path]
diff [path]
apply [path]
validate [path]
show [path]
deploy|redeploy|restart|rollback|stop [service...]   (Plan 5)
logs [service...]                                      (Plan 5)
status [service...]                                    (Plan 5)
ssh [service]                                          (Plan 5)
open                                                   (Plan 5)
list [type]
completion
```

### Key files

| File | Role |
|------|------|
| `internal/cli/cli.go` | Root CLI struct, flag hierarchy |
| `internal/cli/config_*.go` | Current config subcommands (will be restructured) |
| `internal/cli/config_common.go` | `loadAndFetch`, `configPair` |
| `internal/apply/apply.go` | Apply engine |
| `internal/diff/diff.go` | Diff engine |
| `internal/prompt/pick.go` | Interactive pickers |
| `internal/prompt/tty.go` | TTY + CI detection |

### Kong patterns

- Each command is a struct with a `Run(*Globals) error` method
- Flags use Kong struct tags: `` `help:"..." short:"x" env:"VAR" default:"val"` ``
- Boolean pairs: `--create/--no-create` use Kong `negatable:""`
- Commands embed flag structs for reuse (ApiFlags, WorkspaceFlags, etc.)

---

## Task 1: Add merge flag struct

**Files:**

- Modify: `internal/cli/cli.go`

### Step 1: Write test confirming flags parse

```go
func TestMergeFlags_Parse(t *testing.T) {
	var c struct {
		cli.MergeFlags `kong:"embed"`
	}
	parser, err := kong.New(&c)
	if err != nil {
		t.Fatal(err)
	}
	_, err = parser.Parse([]string{"--delete", "--no-create"})
	if err != nil {
		t.Fatal(err)
	}
	if !c.Delete {
		t.Error("Delete = false, want true")
	}
	if c.Create {
		t.Error("Create = true, want false")
	}
}
```

### Step 2: Run test — expect fail

### Step 3: Add MergeFlags to cli.go

```go
// MergeFlags controls what a merge operation does.
type MergeFlags struct {
	Create bool `help:"Add entities that exist in source but not target." negatable:"" default:"true" env:"FAT_CONTROLLER_ALLOW_CREATE"`
	Update bool `help:"Overwrite entities that exist in both." negatable:"" default:"true" env:"FAT_CONTROLLER_ALLOW_UPDATE"`
	Delete bool `help:"Remove entities that exist in target but not source." negatable:"" default:"false" env:"FAT_CONTROLLER_ALLOW_DELETE"`
}
```

### Step 4: Run test — expect pass

### Step 5: Commit

---

## Task 2: Add prompt mode flag struct

**Files:**

- Modify: `internal/cli/cli.go`

### Step 1: Write test for --ask/--yes parsing

### Step 2: Run test — expect fail

### Step 3: Add PromptFlags

Replace the current `MutationFlags.Yes bool` with a proper prompt
mode system:

```go
type PromptFlags struct {
	Ask bool `help:"Prompt for all parameters." short:"a" xor:"prompt"`
	Yes bool `help:"Skip all confirmation prompts." short:"y" xor:"prompt" env:"FAT_CONTROLLER_PROMPT"`
}

// PromptMode returns the effective prompt mode.
func (f *PromptFlags) PromptMode() string {
	if f.Ask {
		return "all"
	}
	if f.Yes {
		return "none"
	}
	return "default"
}
```

Kong's `xor:"prompt"` ensures `--ask` and `--yes` are mutually exclusive.

### Step 4: Run test — expect pass

### Step 5: Commit

---

## Task 3: Restructure CLI root — add top-level commands

**Files:**

- Modify: `internal/cli/cli.go`
- Create: `internal/cli/adopt.go`
- Create: `internal/cli/diff.go`
- Create: `internal/cli/apply.go`
- Create: `internal/cli/validate.go`
- Create: `internal/cli/show.go`
- Create: `internal/cli/new.go`
- Create: `internal/cli/list.go`

### Step 1: Define the new CLI struct

```go
type CLI struct {
	Globals     `kong:"embed"`
	Version     kong.VersionFlag `help:"Print version." short:"V"`

	// Core declarative commands
	Adopt    AdoptCmd    `cmd:"" help:"Pull live Railway state into config."`
	Diff     DiffCmd     `cmd:"" help:"Compare config against live Railway state."`
	Apply    ApplyCmd    `cmd:"" help:"Push config changes to Railway."`
	Validate ValidateCmd `cmd:"" help:"Check config for errors (offline)."`
	Show     ShowCmd     `cmd:"" help:"Display live Railway state."`
	New      NewCmd      `cmd:"" help:"Scaffold config entries."`

	// Discovery
	List ListCmd `cmd:"" help:"List Railway entities."`

	// Auth
	Auth AuthCmd `cmd:"" help:"Manage authentication."`

	// Legacy (hidden, deprecated)
	Config ConfigCmd `cmd:"" help:"Declarative configuration management." hidden:""`

	// Utility
	Completion CompletionCmd `cmd:"" help:"Generate shell completions." hidden:""`
}
```

### Step 2: Create stub command files

Each new command file gets a minimal struct and `Run` method that
returns `errors.New("not implemented")`. This lets the CLI parse
and route to the right command while we implement each one.

### Step 3: Run `go build ./cmd/fat-controller` — expect success

### Step 4: Commit

---

## Task 4: Implement `show` command

Replace `config get` with top-level `show`. Uses the path table from
ARCHITECTURE.md: no path = full environment, `variables` = shared vars,
`api` = service, `api.variables.PORT` = single value, `workspace` /
`project` = peek upward.

**Files:**

- Modify: `internal/cli/show.go`
- Test: `internal/cli/show_test.go`

### Step 1: Write tests

Test the key paths:

- No path → full environment
- `"variables"` → shared variables
- `"api"` → service
- `"api.variables.PORT"` → single value
- `"workspace"` → workspace metadata
- `"project"` → project metadata
- `--raw` for single scalar

### Step 2: Implement

Port logic from `config_get.go` `RunConfigGet`, adapting for the
new path semantics and output formatting.

### Step 3: Run tests — expect pass

### Step 4: Commit

---

## Task 5: Implement `diff` command (top-level)

Port from `config_diff.go` to a top-level command. Add merge flag
awareness — the diff should reflect what `apply` would do given the
current `--create`/`--update`/`--delete` settings.

**Files:**

- Modify: `internal/cli/diff.go`
- Test: `internal/cli/diff_test.go`
- Modify: `internal/diff/diff.go` (add merge flag filtering)

### Step 1: Write tests

### Step 2: Update diff.Compute to accept merge options

```go
type Options struct {
	Create bool
	Update bool
	Delete bool
}

func Compute(desired *DesiredConfig, live *LiveConfig, opts Options) *Result
```

When `Delete=false` (default), deletions are suppressed. When
`Create=false`, creations are suppressed. When `Update=false`,
updates are suppressed.

### Step 3: Wire into CLI

### Step 4: Run tests — expect pass

### Step 5: Commit

---

## Task 6: Implement `apply` command (top-level)

Port from `config_apply.go`. Add merge flags, path scoping, ID
bookkeeping, and deploy triggering.

**Files:**

- Modify: `internal/cli/apply.go`
- Test: `internal/cli/apply_test.go`
- Modify: `internal/apply/apply.go` (accept merge options)

### Step 1: Write tests

### Step 2: Update apply engine for merge flags

The apply engine must respect `--create`/`--update`/`--delete`.
Currently it processes all diff results. Add filtering based on
`diff.Action` and the merge flags.

### Step 3: Implement path scoping

Path argument narrows scope. `apply api` only applies changes
for the `api` service. `apply api.variables` only applies variable
changes for `api`.

### Step 4: Implement ID bookkeeping

After successful resource resolution, write the resolved ID back
to the primary config file. Use TOML manipulation (read file, find
the right section, add/update `id` field, write back).

### Step 5: Wire into CLI with deploy triggering

After apply, if `tool.deploy != "skip"` and `--skip-deploys` not
set, trigger deployments for changed services.

### Step 6: Run tests — expect pass

### Step 7: Commit

---

## Task 7: Implement `adopt` command

New command — the reverse of `apply`. Pulls live Railway state into
the config file. Replaces `config init`.

**Files:**

- Create: `internal/cli/adopt.go` (replace stub)
- Test: `internal/cli/adopt_test.go`

### Step 1: Write tests

Test cases:

- Bootstrap (no config file) → creates file
- Incremental (config exists) → merges
- Path scoping: `adopt api` only adopts the api service
- `--delete` removes config entries not in Railway
- Sensitive values → `${VAR}` refs + env file
- Interactive resolution (workspace/project/environment picker)

### Step 2: Implement adopt

The flow:

1. Load existing config (if any) via LoadCascade
2. Fetch live state from Railway
3. Compute what to adopt based on merge flags
4. Preview changes, confirm
5. Write config file + env file
6. ID bookkeeping

### Step 3: Run tests — expect pass

### Step 4: Commit

---

## Task 8: Implement `validate` command (top-level)

Port from `config_validate.go` to top-level. Operates on the merged
cascade — loads all discovered config files before validating.

**Files:**

- Modify: `internal/cli/validate.go`
- Test: `internal/cli/validate_test.go`

### Step 1: Write tests

### Step 2: Implement — mostly a port of ConfigValidateCmd

### Step 3: Run tests — expect pass

### Step 4: Commit

---

## Task 9: Implement `new` command

Scaffolds entries in the local config file. Three subcommands:
`new project`, `new environment`, `new service`.

**Files:**

- Modify: `internal/cli/new.go`
- Test: `internal/cli/new_test.go`

### Step 1: Write tests for `new project`

```go
func TestNewProject_CreatesConfig(t *testing.T) {
	dir := t.TempDir()
	// Run new project "my-app" in dir.
	// Should create fat-controller.toml with [project] table.
}

func TestNewProject_RefusesToOverwrite(t *testing.T) {
	// Config file already has [project] → error.
}
```

### Step 2: Implement `new project`

- If no config file exists, create one
- If config file exists but has no `[project]`, add it
- If config file exists and has `[project]`, error
- Interactive: prompt for workspace, then project name
- Writes `[workspace]` and `[project]` tables

### Step 3: Write and implement `new environment`

### Step 4: Write and implement `new service`

Service scaffolding with type selection:

- `--database postgres` → pre-fills image + default variables
- `--repo org/app` → pre-fills repo source
- `--image nginx:latest` → pre-fills image source
- Empty (default) → just `[[service]]` with `name`

### Step 5: Run tests — expect pass

### Step 6: Commit

---

## Task 10: Implement `list` command (unified)

Replace separate `workspace list`, `project list`, `environment list`
with a unified `list [type]` command.

**Files:**

- Modify: `internal/cli/list.go`
- Test: `internal/cli/list_test.go`

### Step 1: Write tests

```go
func TestList_NoArg_DefaultsToServices(t *testing.T) { ... }
func TestList_Workspaces(t *testing.T) { ... }
func TestList_Projects(t *testing.T) { ... }
func TestList_Environments(t *testing.T) { ... }
func TestList_Services(t *testing.T) { ... }
func TestList_All_TreeOutput(t *testing.T) { ... }
func TestList_Deployments(t *testing.T) { ... }
```

### Step 2: Implement

The `list` command takes a type argument (enum: `all`, `workspaces`,
`projects`, `environments`, `services`, `deployments`, `volumes`,
`buckets`, `domains`). No argument defaults to `services`.

Context resolution depends on the type:

- `all`, `workspaces` → no context needed
- `projects` → workspace
- `environments` → workspace + project
- `services`, `deployments`, `domains` → workspace + project + environment
- `volumes`, `buckets` → workspace + project

### Step 3: Run tests — expect pass

### Step 4: Commit

---

## Task 11: Update global flags per architecture

**Files:**

- Modify: `internal/cli/cli.go`
- Modify: `cmd/fat-controller/main.go`

### Step 1: Update Globals

Rename/add flags per architecture:

- `--output`/`-o` → `--json`/`--toml`/`--raw` (mutually exclusive)
- Add `--verbose` repeatable (`-v` = debug, `-vv` = trace)
- Add `--quiet` repeatable (`-q` = warn, `-qq` = error, `-qqq` = silent)
- `--color` env var: `FAT_CONTROLLER_OUTPUT_COLOR` (was `FAT_CONTROLLER_COLOR`)
- `--timeout` env var: `FAT_CONTROLLER_API_TIMEOUT` (was `FAT_CONTROLLER_TIMEOUT`)
- Add `--config-file` (was `--config`/`-f`)
- Add `--env-file`

### Step 2: Update main.go color handling

Add `FORCE_COLOR` support to `applyColorMode()`.

### Step 3: Run tests — expect pass

### Step 4: Commit

---

## Task 12: Deprecate old `config *` commands

Mark old commands as hidden but keep them working as aliases for
a transition period. They delegate to the new top-level commands.

**Files:**

- Modify: `internal/cli/cli.go` (hide Config)
- Modify: `internal/cli/config_*.go` (delegate to new commands)

### Step 1: Add deprecation warnings

Each old command's `Run` method prints a deprecation warning to stderr
then delegates to the new command's logic.

### Step 2: Run full test suite

### Step 3: Commit

---

## Task 13: Final verification

### Step 1: Run `mise run check`

### Step 2: Run `go test -race ./...`

### Step 3: Test CLI manually

```bash
go run ./cmd/fat-controller --help
go run ./cmd/fat-controller show --help
go run ./cmd/fat-controller apply --help
go run ./cmd/fat-controller new --help
go run ./cmd/fat-controller list --help
```
