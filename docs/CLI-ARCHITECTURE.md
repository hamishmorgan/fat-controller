# CLI Architecture Design

**Goal:** Define the end-state command structure, config file format, and
interaction model for fat-controller as a comprehensive Railway management
tool.

**Status:** Draft.

---

## Principles

1. **Environment scope.** Every config file has the same schema and
   manages services within a single project+environment. Top-level
   keys are tool settings and context. TOML tables are services.

2. **Declarative and imperative are separate.** Declarative commands
   (`init`, `adopt`, `diff`, `apply`, `show`, `validate`) manage
   desired state. Imperative commands (`deploy`, `restart`, `logs`)
   perform actions on a running system. They share context resolution
   (project/env/service) but not mechanics.

3. **Apply creates everything.** If the config declares a service,
   environment, volume, or domain that doesn't exist in Railway,
   `apply` creates it. The config file is the source of truth and
   `apply` converges reality toward it. Symmetrically, `adopt` adds
   anything from Railway that isn't in the config.

4. **Additive by default, opt-in deletion.** Unmentioned entities are
   never touched by default. If your file doesn't mention a service,
   that service is ignored — not deleted. `--delete` enables full
   convergence: entities in the target that aren't in the source get
   removed. This keeps the safe default while allowing full IaC
   control when desired. See [Merge behavior](#merge-behavior).

5. **No local state file.** Live state always comes from Railway's API.
   Diffs are never stale.

6. **Files cascade.** Multiple config files merge in precedence order
   (global → project → directory → local). Later files override
   earlier ones. Multi-environment setups use one file per environment.

---

## Railway object hierarchy

```text
User
  API Tokens
  SSH Keys
  Preferences
  Workspace
    Settings (preferred region, 2FA enforcement)
    Trusted Domains
    Notification Rules
    Members
    Project
      Settings (PR deploys, base environment)
      Tokens
      Members
      Environment
        Shared Variables
        Private Networks
        Service
          Variables
          Deploy Settings
          Resources
          Domains (custom + service)
          Volumes
          TCP Proxies
          Private Network Endpoints
          Egress Gateways
          Deployment Triggers
```

Every level has configurable state. fat-controller manages the
**Environment** level and below declaratively. Higher levels (User,
Workspace, Project) are context for targeting — set via config keys,
env vars, or CLI flags — but not managed as desired state.

---

## Config file schema

Every config file has the same schema. There is one scope:
environment. A file manages services within a single
project+environment.

**Top-level keys** are tool settings and context:

| Key | Description |
|-----|-------------|
| `workspace` | Workspace name or ID |
| `project` | Project name or ID |
| `environment` | Environment name |
| `timeout` | API request timeout |
| `output` | Output format: `text`, `json`, `toml` |
| `color` | Color: `auto`, `always`, `never` |
| `show_secrets` | Show secret values instead of masking |
| `sensitive_keywords` | Keywords for detecting sensitive variable names |
| `sensitive_allowlist` | Keywords that suppress false-positive secret matches |
| `suppress_warnings` | Warning codes to suppress |
| `create` | Merge flag default |
| `update` | Merge flag default |
| `delete` | Merge flag default |

**TOML tables** are Railway state — services and their entities:

```toml
workspace = "Hamish Morgan's Projects"
project = "Life"
environment = "production"
timeout = "60s"

[shared.variables]
NODE_ENV = "production"

[api.variables]
PORT = "8080"
DATABASE_URL = "${{postgres.DATABASE_URL}}"

[api.deploy]
builder = "NIXPACKS"
start_command = "node dist/server.js"

[api.resources]
vcpus = 2
memory_gb = 4

[api.domains]
"api.example.com" = { port = 8080 }

[api.volumes]
data = { mount = "/data", size_gb = 10 }
```

A file doesn't need to include everything. A global config might only
set `timeout` and `output`. A project-level file might set `workspace`,
`project`, and `[shared.variables]`. An environment-level file sets
`environment` and per-service config. The cascade merges them all.

---

## Command structure

### Global flags

Every command accepts these flags.

| Flag | Short | Env var | Config key | Default | Description |
|------|-------|---------|------------|---------|-------------|
| `--token` | | `RAILWAY_TOKEN` / `RAILWAY_API_TOKEN` | — | — | Auth token |
| `--output` | `-o` | `FAT_CONTROLLER_OUTPUT` | `output` | `text` | Output format: `text`, `json`, `toml` |
| `--color` | | `FAT_CONTROLLER_COLOR` | `color` | `auto` | Color: `auto`, `always`, `never`. Respects `NO_COLOR` |
| `--timeout` | | `FAT_CONTROLLER_TIMEOUT` | `timeout` | `30s` | API request timeout |
| `--verbose` | `-v` | — | — | | Decrease log level. Repeatable: `-v` = DEBUG, `-vv` = TRACE |
| `--quiet` | `-q` | — | — | | Increase log level. Repeatable: `-q` = WARN, `-qq` = ERROR, `-qqq` = silent |

Default log level is INFO.

### Context flags

Commands that target Railway resources accept these flags. Values are
also resolved from the config file, env vars, and token scope — see
[Context resolution](#context-resolution).

| Flag | Env var | Config key | Description |
|------|---------|------------|-------------|
| `--workspace` | `FAT_CONTROLLER_WORKSPACE` | `workspace` | Workspace name or ID |
| `--project` | `FAT_CONTROLLER_PROJECT` | `project` | Project name or ID |
| `--environment` | `FAT_CONTROLLER_ENVIRONMENT` | `environment` | Environment name |
| `--service` | `FAT_CONTROLLER_SERVICE` | `service` | Service name (filters to one service) |

### Config flags

Commands that read or write config files accept these flags.

| Flag | Env var | Config key | Default | Description |
|------|---------|------------|---------|-------------|
| `--config` | `FAT_CONTROLLER_CONFIG` | `config` | `fat-controller.toml` | Config file path |
| `--secrets` | `FAT_CONTROLLER_SECRETS` | `secrets` | `.env.fat-controller` | Secrets file path for `${VAR}` interpolation |

### Merge flags

`apply` and `adopt` accept these flags. See
[Merge behavior](#merge-behavior) for details.

| Flag | Env var | Config key | Default | Description |
|------|---------|------------|---------|-------------|
| `--create` / `--no-create` | `FAT_CONTROLLER_CREATE` | `create` | on | Add entities that exist in source but not target |
| `--update` / `--no-update` | `FAT_CONTROLLER_UPDATE` | `update` | on | Overwrite entities that exist in both |
| `--delete` / `--no-delete` | `FAT_CONTROLLER_DELETE` | `delete` | off | Remove entities that exist in target but not source |

### Mutation flags

Commands that modify Railway state (`apply`, `deploy`, `redeploy`,
`restart`, `rollback`, `stop`) accept these flags.

| Flag | Short | Env var | Config key | Default | Description |
|------|-------|---------|------------|---------|-------------|
| `--yes` | `-y` | `FAT_CONTROLLER_YES` | — | `false` | Skip confirmation prompts |
| `--dry-run` | | `FAT_CONTROLLER_DRY_RUN` | `dry_run` | `false` | Preview changes without executing |
| `--fail-fast` | | `FAT_CONTROLLER_FAIL_FAST` | `fail_fast` | `false` | Stop on first error |

### Apply-specific flags

| Flag | Env var | Config key | Default | Description |
|------|---------|------------|---------|-------------|
| `--skip-deploys` | `FAT_CONTROLLER_SKIP_DEPLOYS` | `skip_deploys` | `false` | Don't trigger redeployments after variable changes |

### Display flags

Commands that show config or state (`show`, `diff`, `adopt`) accept
these flags.

| Flag | Env var | Config key | Default | Description |
|------|---------|------------|---------|-------------|
| `--show-secrets` | `FAT_CONTROLLER_SHOW_SECRETS` | `show_secrets` | `false` | Show secret values instead of masking |

### Config-only keys

These are set only in the config file, not via flags or env vars.

| Config key | Default | Description |
|------------|---------|-------------|
| `sensitive_keywords` | *(built-in list)* | Keywords for detecting sensitive variable names |
| `sensitive_allowlist` | *(built-in list)* | Keywords that suppress false-positive secret matches |
| `suppress_warnings` | `[]` | Warning codes to suppress (e.g. `["W012"]`) |

---

### Commands

#### `init`

First-time setup. In "from remote" mode, resolves context (workspace,
project, environment, services) and then runs `adopt` to pull live
state. In `--new` mode, scaffolds an empty config file.

```text
fat-controller init [--new] [--template <name>]
```

| Arg/flag | Description |
|----------|-------------|
| `--new` | Scaffold from scratch instead of importing from remote |
| `--template <name>` | Scaffold from a Railway template |

After the initial bootstrap, use `adopt` to bring additional resources
into an existing config (e.g. `adopt redis` after adding a service in
the dashboard).

Flags: global, context, config, mutation (`--yes`, `--dry-run`).

Interactive resolution:

| Parameter | Default | Interactive | Non-interactive |
|-----------|---------|-------------|-----------------|
| Mode (`--new`) | From remote | Prompt: import or scaffold | From remote |
| Config file (`--config`) | `fat-controller.toml` | Prompt with default | Use default |
| Secrets file (`--secrets`) | `.env.fat-controller` | Prompt with default | Use default |
| Workspace (`--workspace`) | — | Picker (skip if only one) | Error if ambiguous |
| Project (`--project`) | — | Picker | Error if not specified |
| Environment (`--environment`) | — | Picker | Error if not specified |
| Services | All | Checkbox list, all selected | All |

In `--new` mode, only config file path and secrets file path are
resolved — no API calls, no context needed.

#### `adopt`

Merge live Railway state into the local config file. Detects sensitive
values and writes them to the secrets file as `${VAR}` references.
Additive by default — adds missing entries, updates changed entries,
does not remove entries unless `--delete` is set.

```text
fat-controller adopt [path]
```

| Arg/flag | Description |
|----------|-------------|
| `path` | Optional dot-path to narrow scope (e.g. `redis`, `api.variables`) |

Flags: global, context, config, merge, mutation, display.

Interactive resolution:

| Parameter | Default | Interactive | Non-interactive |
|-----------|---------|-------------|-----------------|
| Config file (`--config`) | `fat-controller.toml` | Prompt with default | Use default |
| Secrets file (`--secrets`) | `.env.fat-controller` | Prompt with default | Use default |
| Workspace | From config file | Prompt with default | Use default |
| Project | From config file | Prompt with default | Use default |
| Environment | From config file | Prompt with default | Use default |
| Confirm changes | Yes | Preview + confirm | Error unless `--yes` |

#### `diff`

Compare local config against live Railway state. Read-only — no
changes are made. Output reflects what `apply` would do given the
current merge flag settings.

```text
fat-controller diff
```

Flags: global, context, config, merge, display.

Interactive resolution:

| Parameter | Default | Interactive | Non-interactive |
|-----------|---------|-------------|-----------------|
| Config file (`--config`) | `fat-controller.toml` | Prompt with default | Use default |
| Workspace | From config file | Prompt with default | Use default |
| Project | From config file | Prompt with default | Use default |
| Environment | From config file | Prompt with default | Use default |

Read-only — no confirmation needed.

#### `apply`

Merge local config into live Railway state. Additive by default —
creates missing entities, updates changed entities, does not delete
unless `--delete` is set.

```text
fat-controller apply
```

Flags: global, context, config, merge, mutation, apply-specific.

Interactive resolution:

| Parameter | Default | Interactive | Non-interactive |
|-----------|---------|-------------|-----------------|
| Config file (`--config`) | `fat-controller.toml` | Prompt with default | Use default |
| Workspace | From config file | Prompt with default | Use default |
| Project | From config file | Prompt with default | Use default |
| Environment | From config file | Prompt with default | Use default |
| Confirm changes | Yes | Preview + confirm | Error unless `--yes` |

#### `validate`

Check config file for warnings without making API calls.

```text
fat-controller validate
```

Flags: global, config.

Interactive resolution:

| Parameter | Default | Interactive | Non-interactive |
|-----------|---------|-------------|-----------------|
| Config file (`--config`) | `fat-controller.toml` | Prompt with default | Use default |

No API calls, no context flags needed.

#### `show`

Display live Railway state. Read-only. No path = full overview.
Dot-path = narrow to a specific section or value.

```text
fat-controller show [path]
```

| Arg/flag | Description |
|----------|-------------|
| `path` | Optional dot-path (e.g. `api`, `api.variables.PORT`) |

Flags: global, context, display.

Interactive resolution:

| Parameter | Default | Interactive | Non-interactive |
|-----------|---------|-------------|-----------------|
| Workspace | From config file | Prompt with default | Use default, error if missing |
| Project | From config file | Prompt with default | Use default, error if missing |
| Environment | From config file | Prompt with default | Use default, error if missing |

Read-only — no confirmation needed.

#### `deploy`

Trigger a deployment. No arguments = all services in the environment.

```text
fat-controller deploy [service...]
```

Flags: global, context, mutation.

Interactive resolution:

| Parameter | Default | Interactive | Non-interactive |
|-----------|---------|-------------|-----------------|
| Workspace | From config file | Prompt with default | Use default, error if missing |
| Project | From config file | Prompt with default | Use default, error if missing |
| Environment | From config file | Prompt with default | Use default, error if missing |
| Services | All | Checkbox list, all selected | All |
| Confirm | Yes | "Deploy N services? [Y/n]" | Error unless `--yes` |

#### `redeploy`

Redeploy the current image.

```text
fat-controller redeploy [service...]
```

Flags: global, context, mutation.

Interactive resolution: same as `deploy`.

#### `restart`

Restart running deployments.

```text
fat-controller restart [service...]
```

Flags: global, context, mutation.

Interactive resolution: same as `deploy`.

#### `rollback`

Rollback to the previous deployment.

```text
fat-controller rollback [service...]
```

Flags: global, context, mutation.

Interactive resolution: same as `deploy`.

#### `stop`

Stop running deployments.

```text
fat-controller stop [service...]
```

Flags: global, context, mutation.

Interactive resolution: same as `deploy`.

#### `logs`

Tail logs. No arguments = all services in the environment.

```text
fat-controller logs [service...]
```

Flags: global, context.

Interactive resolution:

| Parameter | Default | Interactive | Non-interactive |
|-----------|---------|-------------|-----------------|
| Workspace | From config file | Prompt with default | Use default, error if missing |
| Project | From config file | Prompt with default | Use default, error if missing |
| Environment | From config file | Prompt with default | Use default, error if missing |
| Services | All | Checkbox list, all selected | All |

Read-only — no confirmation needed.

#### `status`

Show deployment status. No arguments = all services in the
environment.

```text
fat-controller status [service...]
```

Flags: global, context.

Interactive resolution: same as `logs`.

#### `list`

List entities. Takes a noun argument for the entity type.

```text
fat-controller list <type>
```

| Type | Context required |
|------|-----------------|
| `workspaces` | None |
| `projects` | Workspace |
| `environments` | Workspace + project |
| `services` | Workspace + project |
| `deployments` | Workspace + project + environment |
| `volumes` | Workspace + project |
| `domains` | Workspace + project + environment |

Flags: global, context.

Interactive resolution: context flags follow the standard pattern —
prompt with default if interactive, use default or error if
non-interactive. Only the context required for the entity type is
resolved. `list workspaces` needs no context. `list projects` needs
a workspace. `list services` needs a workspace and project.

In interactive mode with no `<type>` argument, prompt with a picker
for the entity type. In non-interactive mode, error.

#### `auth login`

Authenticate via browser-based OAuth. Opens a browser.

```text
fat-controller auth login
```

Flags: global.

No interactive resolution — the OAuth flow is always browser-based.

#### `auth logout`

Clear stored credentials.

```text
fat-controller auth logout
```

Flags: global.

Interactive resolution:

| Parameter | Default | Interactive | Non-interactive |
|-----------|---------|-------------|-----------------|
| Confirm | Yes | "Clear credentials? [Y/n]" | Error unless `--yes` |

#### `auth status`

Show current authentication state.

```text
fat-controller auth status
```

Flags: global.

Read-only — no interactive resolution needed.

---

## File conventions

### Config file discovery

When `--config` is not specified, config files are found by walking
from the working directory upward to the git root (or current
directory only if not in a git repo). At each directory, three
locations are checked in order — first match at that level wins:

1. `[path]/fat-controller.toml`
2. `[path]/.config/fat-controller.toml`
3. `[path]/.config/fat-controller/config.toml`

Visible beats hidden. Simple beats nested. At most one config file
is found per directory level.

**All config files found in the walk are collected and merged,
shallowest first.** The deepest file (closest to the working
directory) has the highest priority. This enables inheritance:
a root-level config sets shared values, and a subdirectory config
overrides only what differs.

Example walk from `environments/production/`:

```text
environments/production/fat-controller.toml  → found (deepest)
environments/                                → no config file
repo-root/fat-controller.toml                → found (shallowest)
```

Merge order: repo-root config, then environments/production config
on top.

The **primary config file** is the deepest one found — this is where
`adopt` and `init` write, and where the local override is resolved.

The secrets file is co-located with the primary (deepest) config file:

| Config location | Secrets location |
|----------------|-----------------|
| `[path]/fat-controller.toml` | `[path]/.env.fat-controller` |
| `[path]/.config/fat-controller.toml` | `[path]/.config/.env.fat-controller` |
| `[path]/.config/fat-controller/config.toml` | `[path]/.config/fat-controller/.env` |

Both are overridable with `--config` and `--secrets`. When `--config`
is specified, only that single file is loaded — no upward walk.

### File locations summary

| File | Purpose | Committed? |
|------|---------|-----------|
| `fat-controller.toml` | Desired state + shared settings | Yes |
| `fat-controller.local.toml` | Personal overrides | No (gitignored) |
| `.env.fat-controller` | Secret values for `${VAR}` interpolation | No (gitignored) |

When using the `.config/fat-controller/` directory form:

| File | Purpose | Committed? |
|------|---------|-----------|
| `.config/fat-controller/config.toml` | Desired state + shared settings | Yes |
| `.config/fat-controller/config.local.toml` | Personal overrides | No (gitignored) |
| `.config/fat-controller/.env` | Secret values for `${VAR}` interpolation | No (gitignored) |

### Settings in the config file

Tool settings are top-level keys in the same file as desired state.
There is no separate settings file — settings and state live together.

```toml
# Scope
workspace = "Hamish Morgan's Projects"
project = "Life"
environment = "production"

# Tool settings
timeout = "60s"
output = "text"
show_secrets = false

# Railway desired state
[shared.variables]
NODE_ENV = "production"

[api.variables]
PORT = "8080"
```

Top-level keys are always tool configuration (scope, settings).
TOML tables (`[api]`, `[shared]`, etc.) are always Railway state.
No collision is possible.

### Local overrides

The `.local` file has the same schema as the main config file. It
merges on top — any key can be overridden. Use it for personal
preferences that shouldn't be committed:

```toml
# fat-controller.local.toml
output = "json"
show_secrets = true
```

### File cascade

Multiple config files are loaded and merged in precedence order,
lowest priority first:

1. **Compiled-in defaults** — built into the binary.
2. **Global config** — `$XDG_CONFIG_HOME/fat-controller/config.toml`.
   Always at this fixed path. Useful for setting `output`, `color`,
   `timeout`, or a default `workspace` across all projects.
3. **Discovered config files** — all config files found by walking
   upward from the working directory to the git root (see
   [Config file discovery](#config-file-discovery)). Merged
   shallowest first, so deeper (more specific) files win.
4. **Local override** — co-located with the primary (deepest) config
   file, with `.local` inserted before the extension (e.g.
   `fat-controller.local.toml`). Gitignored. Personal preferences
   or environment-specific overrides.
5. **Environment variables** — `FAT_CONTROLLER_*` and `RAILWAY_*`.
6. **CLI flags** — highest priority, always wins.

Concrete example with this directory structure:

```text
$XDG_CONFIG_HOME/fat-controller/config.toml   # timeout = "60s"
repo-root/fat-controller.toml                  # workspace, project, [shared.variables]
environments/production/fat-controller.toml    # environment = "production", [api.resources]
environments/production/fat-controller.local.toml  # show_secrets = true
```

Running from `environments/production/`, the merge order is:

1. Compiled defaults
2. `$XDG_CONFIG_HOME/fat-controller/config.toml`
3. `repo-root/fat-controller.toml`
4. `environments/production/fat-controller.toml`
5. `environments/production/fat-controller.local.toml`
6. Environment variables
7. CLI flags

**Merge rules:**

- **Top-level keys** (settings, context): later values replace earlier
  ones. If the root config sets `timeout = "60s"` and the environment
  config sets `timeout = "30s"`, the environment config wins.
- **TOML tables** (Railway state): deep merge. Keys within a table
  from a higher-precedence file override the same keys from a
  lower-precedence file. Keys only present in the lower-precedence
  file are preserved. This means a root config can set
  `[shared.variables]` and an environment config can add to or
  override individual variables without replacing the entire table.
- **Environment variables and CLI flags** only set top-level keys
  (settings, context). They do not express Railway state.

---

## Entity coverage

### What the TOML can express

| Entity | Section | Fields |
|--------|---------|--------|
| Variables (shared) | `[shared.variables]` | key-value pairs |
| Variables (per-service) | `[svc.variables]` | key-value pairs |
| Deploy settings | `[svc.deploy]` | `builder`, `build_command`, `start_command`, `dockerfile_path`, `root_directory`, `healthcheck_path`, `healthcheck_timeout`, `cron_schedule`, `draining_seconds`, `num_replicas`, `overlap_seconds`, `pre_deploy_command`, `region`, `restart_policy`, `restart_policy_max_retries`, `sleep_application`, `watch_patterns` |
| Resources | `[svc.resources]` | `vcpus`, `memory_gb` |
| Custom domains | `[svc.domains]` | hostname, target port |
| Service domains | `[svc.domains]` | railway.app subdomain, target port |
| Volumes | `[svc.volumes]` | name, mount path |
| TCP proxies | `[svc.tcp_proxies]` | application port |
| Private network endpoints | `[svc.network]` | DNS name |
| Deployment triggers | `[svc.triggers]` | branch, repo, check suites |
| Egress gateways | `[svc.egress]` | service association |

### What stays imperative-only

These are actions, not state — no declarative equivalent:

- Deployments (trigger, cancel, rollback, restart, stop)
- Logs (tail, fetch)
- Volume backups (create, restore)
- Template deployment

---

## Multi-environment patterns

### Pattern 1: Shared base with environment overrides (cascade)

```text
fat-controller.toml                        # workspace, project, shared services
environments/
  production/fat-controller.toml           # environment = "production", overrides
  staging/fat-controller.toml              # environment = "staging", overrides
```

The root config sets `workspace`, `project`, and shared service
definitions. Each environment directory's config sets `environment`
and overrides only what differs (e.g. resource limits, replica
counts).

```toml
# fat-controller.toml (root — shared base)
workspace = "Hamish Morgan's Projects"
project = "Life"

[shared.variables]
NODE_ENV = "production"

[api.deploy]
builder = "NIXPACKS"
start_command = "node dist/server.js"

[api.domains]
"api.example.com" = { port = 8080 }
```

```toml
# environments/production/fat-controller.toml
environment = "production"

[api.resources]
vcpus = 4
memory_gb = 8
```

```toml
# environments/staging/fat-controller.toml
environment = "staging"

[shared.variables]
NODE_ENV = "staging"

[api.resources]
vcpus = 1
memory_gb = 2
```

Running `fat-controller apply` from `environments/production/`
discovers both files via the upward walk and merges them — root
first, then the environment override on top.

```bash
# From environments/production/:
fat-controller apply
# Or from anywhere, using --config (skips walk, loads only this file):
fat-controller apply --config environments/production/fat-controller.toml
```

Note: `--config` loads a single file with no upward walk. For CI
pipelines that need the cascade, run from the environment directory
rather than using `--config`.

Best for: most projects. Keeps shared config DRY, with per-environment
differences clearly separated.

### Pattern 2: Self-contained files per environment

```text
environments/
  production/fat-controller.toml
  staging/fat-controller.toml
```

Each file is fully self-contained with all settings and service
definitions. No cascade — use `--config` to target a specific file.

```bash
fat-controller apply --config environments/production/fat-controller.toml
fat-controller apply --config environments/staging/fat-controller.toml
```

Best for: projects where environments differ substantially, or teams
that prefer explicit duplication over inheritance.

### Pattern 3: Per-service files (monorepo)

```text
services/
  api/fat-controller.toml           # declares only [api.*] tables
  worker/fat-controller.toml        # declares only [worker.*] tables
```

Each file is environment-scoped but only declares one service's
tables. Because `apply` is additive by default, each file only
touches the services it mentions. A CI pipeline applies each file
independently. Shared variables live in a root-level config (picked
up via cascade) or are duplicated across files.

Best for: large teams where each service team owns their config.

---

## Context resolution

All commands need to know which project/environment/service to target.
Resolution order:

1. CLI flags (`--project`, `--environment`, `--service`)
2. Environment variables (`FAT_CONTROLLER_PROJECT`, etc.)
3. Local override keys (`fat-controller.local.toml`)
4. Config file keys (`fat-controller.toml`)
5. Global config keys (`$XDG_CONFIG_HOME/fat-controller/config.toml`)
6. Token scope (project-scoped `RAILWAY_TOKEN` implies project + env)
7. Interactive picker (if TTY) — see below
8. Error with available options

---

## Interactive vs non-interactive mode

**Detection:** interactive mode is active when stdin is a TTY.
Piped or redirected stdin = non-interactive. This is not a flag — it
is determined by the terminal environment.

**Core principle:** every command parameter is resolved the same way,
but the behavior for "unspecified" depends on the mode:

| | Interactive (TTY) | Non-interactive (piped/CI) |
|---|---|---|
| Specified via flag | Use flag value | Use flag value |
| Has a default | Prompt, pre-filled with default | Use default silently |
| No default, options available | Picker (select from list) | Error with available options |
| No default, no options | Prompt for free-text input | Error |
| Mutation | Preview + confirmation (default: yes) | Error unless `--yes` |
| Colors | Auto-detected | Off (unless `--color=always`) |

**Flags pin values.** If `--config`, `--project`, `--environment`, or
any other flag is specified, that value is locked in — no prompt is
shown for it in either mode. Everything else is prompted in
interactive mode, with defaults pre-filled where they exist.

This means you can run any command with zero flags in interactive mode
and the tool walks you through every decision:

```text
$ fat-controller init

  Config file: fat-controller.toml
  Secrets file: .env.fat-controller
  Workspace: Hamish Morgan's Projects  (1 of 1)
  Project: > Life
            Other Project
  Environment: > production
                staging
  Services: [x] api
            [x] worker
            [ ] postgres
```

Or pin specific values and only be prompted for the rest:

```text
$ fat-controller init --project Life --environment production

  Config file: fat-controller.toml
  Secrets file: .env.fat-controller
  Services: [x] api
            [x] worker
            [ ] postgres
```

**`--yes` and `--dry-run`:**

`--yes` accepts the default for every prompt. It makes interactive
mode behave like non-interactive: use defaults, error on missing
required values with no default.

| Flags | Interactive | Non-interactive |
|-------|-------------|-----------------|
| (none) | Prompt for everything | Use defaults, error if missing |
| `--yes` | Use defaults, error if missing | Use defaults, error if missing |
| `--dry-run` | Prompt, but preview only | Preview only, no mutations |
| `--yes --dry-run` | Use defaults, preview only | Use defaults, preview only |

The goal: interactive mode is convenient for humans — you can run
`fat-controller apply` with zero flags and the tool guides you
through every decision. Non-interactive mode is safe for CI —
deterministic, no prompts, fails loudly on missing values.

---

## Merge behavior

`apply` and `adopt` share three merge flags (`--create`, `--update`,
`--delete`) that control what the merge does. See
[Merge flags](#merge-flags) for flag details and defaults.

`apply` and `adopt` are symmetric — the same flags have parallel
meaning in opposite directions:

| Flag | `apply` (config → Railway) | `adopt` (Railway → config) |
|------|---------------------------|---------------------------|
| `--create` | Create Railway entities not in Railway | Add config entries not in config |
| `--update` | Update Railway entities that differ from config | Update config entries that differ from Railway |
| `--delete` | Delete Railway entities not in config | Remove config entries not in Railway |

Common patterns:

```bash
apply                              # create + update (default)
apply --delete                     # full convergence
apply --no-update                  # create only, don't touch existing
apply --no-create                  # update only, don't add new
apply --no-create --delete         # update + delete, don't add
adopt --no-update                  # add new entries, don't touch existing
adopt --delete                     # add + update + remove stale
adopt --no-create --delete         # update + remove stale, don't add
```

Without `--delete`, explicit delete markers handle one-off removals:

```toml
# Delete a variable
OLD_VAR = ""

# Delete a service (environment-scope file)
[old-service]
delete = true

# Delete a volume
[api.volumes]
old-data = { delete = true }
```

`diff` reflects the active flags: it shows what `apply` would do
given the current create/update/delete settings.

---

## Open questions

1. **Per-section delete granularity.** `--delete` applies globally.
   Should there be a per-section equivalent (e.g. a `managed = true`
   key within `[api.variables]` meaning "delete any variables not
   listed here")? Per-section is more flexible but more complex.

2. **`show` output format.** `show` outputs TOML matching the config
   file format by default. Should it also support outputting just the
   raw values for scripting? e.g. `fat-controller show api.variables.PORT`
   outputs `8080` with no formatting.

3. **Volume sizing.** Volumes have a size, but Railway doesn't expose
   size in the create/update mutations — they auto-scale. Should the
   config file express a size, or just mount path?

4. **Domain verification.** Custom domains require DNS verification.
   `apply` can create the domain record in Railway, but the user still
   needs to set up DNS. Should `status` show verification state?

5. **Service creation defaults.** When `apply` creates a new service,
   what source does it use? Empty service (no repo)? The config would
   need to specify source (repo URL + branch, or Docker image) for
   new services. What's the minimal viable service definition?

6. **Ordering of creation.** When bootstrapping from scratch, `apply`
   may need to create the project, then the environment, then services,
   then configure them. The apply engine needs to handle dependency
   ordering (e.g. Railway references `${{postgres.VAR}}` require
   postgres to exist first).
