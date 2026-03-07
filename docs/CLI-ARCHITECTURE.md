# CLI Architecture Design

**Goal:** Define the end-state command structure, scope model, and config
file conventions for fat-controller as a comprehensive Railway management
tool.

**Status:** Draft.

---

## Principles

1. **Scope lives in the file, not the command.** `fat-controller apply`
   works the same whether the file describes a whole environment or a
   single service. The file declares what it manages.

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

6. **One file, one scope.** Each config file is self-contained and
   targets one scope. Multi-environment setups use one file per
   environment, not overlays.

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

Every level has configurable state.

---

## Scope model

A config file targets exactly one scope. The scope is inferred from
which keys are present:

| Scope | Required keys | What it manages |
|-------|--------------|-----------------|
| **User** | (none — implied by auth) | API tokens, SSH keys, preferences |
| **Workspace** | `workspace` (no `project`) | Workspace settings, trusted domains, notification rules, members |
| **Project** | `project` (no `environment`) | Project settings, tokens, members, environments |
| **Environment** | `project` + `environment` | All services in one environment: shared vars, per-service vars, deploy settings, resources, domains, volumes, networking |
| **Service** | `project` + `environment` + `service` | One service: its vars, deploy settings, resources, domains, volumes |

### Environment scope

Manages all services within a single project+environment.

```toml
workspace = "Hamish Morgan's Projects"
project = "Life"
environment = "production"

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

### Service scope

A subset of environment scope, targeting one service. Useful in
monorepos where each service team owns their own config file.

```toml
project = "Life"
environment = "production"
service = "api"

[variables]
PORT = "8080"

[deploy]
builder = "NIXPACKS"
start_command = "node dist/server.js"

[resources]
vcpus = 2
memory_gb = 4

[domains]
"api.example.com" = { port = 8080 }
```

Note: no service name prefix on section headers — the `service` key
establishes the scope. `[variables]` instead of `[api.variables]`.

### Project scope

Manages project-level settings and can declare which environments
should exist. When `apply` runs against a project-scope file and an
environment doesn't exist, it creates it.

```toml
workspace = "Hamish Morgan's Projects"
project = "Life"

[project]
pr_deploys = true
base_environment = "production"

[project.environments]
production = {}
staging = {}
```

### Workspace scope

```toml
workspace = "Hamish Morgan's Projects"

[workspace]
preferred_region = "us-west1"
two_factor_required = true

[workspace.trusted_domains]
"example.com" = {}
```

### User scope

```toml
[user.preferences]
# notification settings, etc.
```

Smallest surface area. Most user settings are better managed in the
dashboard.

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
also resolved from the config file, settings file, env vars, and token
scope — see [Context resolution](#context-resolution).

| Flag | Env var | Config key | Description |
|------|---------|------------|-------------|
| `--workspace` | `FAT_CONTROLLER_WORKSPACE` | `workspace` | Workspace name or ID |
| `--project` | `FAT_CONTROLLER_PROJECT` | `project` | Project name or ID |
| `--environment` | `FAT_CONTROLLER_ENVIRONMENT` | `environment` | Environment name |
| `--service` | `FAT_CONTROLLER_SERVICE` | `service` | Service name (narrows scope) |

### Config flags

Commands that read or write config files accept these flags.

| Flag | Env var | Config key | Default | Description |
|------|---------|------------|---------|-------------|
| `--config` | `FAT_CONTROLLER_CONFIG` | `config` | `fat-controller.toml` | Config file path |

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

Guided first-time config bootstrap. A guided version of `adopt` —
selects workspace/project/environment/services, pulls live state,
writes config file and `.env.fat-controller`.

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
| Config file (`--config`) | `fat-controller.toml` | Prompt with default | Use default |
| Secrets file | `.env.fat-controller` | Prompt with default | Use default |
| Workspace (`--workspace`) | — | Picker (skip if only one) | Error if ambiguous |
| Project (`--project`) | — | Picker | Error if not specified |
| Environment (`--environment`) | — | Picker | Error if not specified |
| Services | All | Checkbox list, all selected | All |
| Mode (`--new`) | From remote | Prompt: import or scaffold | From remote |

#### `adopt`

Merge live Railway state into the local config file. Additive by
default — adds missing entries, updates changed entries, does not
remove entries unless `--delete` is set.

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

### Config file (desired state)

| File | Purpose | Committed? |
|------|---------|-----------|
| `fat-controller.toml` | Desired state file | Yes |
| `.env.fat-controller` | Secret values for `${VAR}` interpolation | No (gitignored) |

The default file name is `fat-controller.toml`, overridable with
`--config`.

### Settings file (tool behavior)

| File | Purpose |
|------|---------|
| `$XDG_CONFIG_HOME/fat-controller/config.toml` | Global user preferences |
| `.fat-controller.toml` | Project-level tool settings |

These configure the tool itself (output format, timeout, default
project/env). They are distinct from the desired state file.

The naming convention: dot-prefix (`.fat-controller.toml`) = tool
settings. No dot-prefix (`fat-controller.toml`) = Railway desired
state.

---

## Entity coverage

### What the TOML can express

**Environment / service scope:**

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

**Project scope:**

| Entity | Section | Fields |
|--------|---------|--------|
| Project settings | `[project]` | PR deploys, base environment |
| Environments | `[project.environments]` | name |

**Workspace scope:**

| Entity | Section | Fields |
|--------|---------|--------|
| Workspace settings | `[workspace]` | Preferred region, 2FA enforcement |
| Trusted domains | `[workspace.trusted_domains]` | domain |
| Notification rules | `[workspace.notifications]` | TBD |

**User scope:**

| Entity | Section | Fields |
|--------|---------|--------|
| Preferences | `[user.preferences]` | TBD |

### What stays imperative-only

These are actions, not state — no declarative equivalent:

- Deployments (trigger, cancel, rollback, restart, stop)
- Logs (tail, fetch)
- Volume backups (create, restore)
- Template deployment

### Potentially declarative

These have state but small surface area:

- Project/workspace membership (members, roles)
- Integrations
- Observability dashboards

---

## Multi-environment patterns

### Pattern 1: Directory per environment

```text
environments/
  production/fat-controller.toml
  staging/fat-controller.toml
```

Each file is self-contained. Values that differ between environments
use `${VAR}` interpolation or are set directly.

```bash
fat-controller apply --config environments/production/fat-controller.toml
fat-controller apply --config environments/staging/fat-controller.toml
```

### Pattern 2: Service-scoped files (monorepo)

```text
services/
  api/fat-controller.toml           # service = "api"
  worker/fat-controller.toml        # service = "worker"
```

Each service team owns their own file. A CI pipeline applies each
file independently.

---

## Context resolution

All commands need to know which project/environment/service to target.
Resolution order:

1. CLI flags (`--project`, `--environment`, `--service`)
2. Environment variables (`FAT_CONTROLLER_PROJECT`, etc.)
3. Config file keys (`project`, `environment` in `fat-controller.toml`)
4. Settings file keys (`.fat-controller.toml`)
5. Token scope (project-scoped `RAILWAY_TOKEN` implies project + env)
6. Interactive picker (if TTY) — see below
7. Error with available options

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

5. **Config file discovery.** Should `fat-controller diff` (with no
   `--config` flag) search upward for `fat-controller.toml` like git
   searches for `.git`? Or only look in the current directory?

6. **Service creation defaults.** When `apply` creates a new service,
   what source does it use? Empty service (no repo)? The config would
   need to specify source (repo URL + branch, or Docker image) for
   new services. What's the minimal viable service definition?

7. **Ordering of creation.** When bootstrapping from scratch, `apply`
   may need to create the project, then the environment, then services,
   then configure them. The apply engine needs to handle dependency
   ordering (e.g. Railway references `${{postgres.VAR}}` require
   postgres to exist first).
