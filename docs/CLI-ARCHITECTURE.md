# CLI Architecture Design

**Goal:** Define the end-state command structure, config file format, and
interaction model for fat-controller as a comprehensive Railway management
tool.

**Status:** Draft.

---

## Principles

1. **Environment scope.** Each config file **is** an environment.
   Top-level keys are the environment's identity, shared variables,
   and tool settings. `[workspace]` and `[project]` provide parent
   context. `[[service]]` arrays declare services.

2. **Config is state, not actions.** The config file expresses what
   the infrastructure should look like — variables, domains, deploy
   settings, resources. It never expresses things that happen once:
   deployments, restarts, rollbacks. Those are commands.

3. **Apply creates everything.** If the config declares a project,
   environment, service, volume, or domain that doesn't exist in
   Railway, `apply` creates it — from the project level down.
   The config file is the source of truth and `apply` converges
   reality toward it. Symmetrically, `adopt` adds anything from
   Railway that isn't in the config. The only prerequisite is
   that the workspace must already exist.

4. **Additive by default, opt-in deletion.** Unmentioned entities are
   never touched by default. If your file doesn't mention a service,
   that service is ignored — not deleted. `--delete` enables full
   convergence: entities in the target that aren't in the source get
   removed. This keeps the safe default while allowing full IaC
   control when desired. See [Merge behavior](#merge-behavior).

5. **No local state file.** Live state always comes from Railway's API.
   Diffs are never stale.

6. **Files cascade.** Multiple config files merge in precedence order
   (global config → discovered configs shallowest-first → local
   override → env vars → CLI flags). See [File cascade](#file-cascade).

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

The file **is** the environment. Top-level keys are the
environment's identity and state:

| Key | Description |
|-----|-------------|
| `name` | Environment name |
| `id` | Environment ID (optional, populated by `adopt`) |
| `variables` | Environment-wide shared variables |

**`[tool]`** holds tool settings — how fat-controller behaves,
not what it manages:

| Key | Description |
|-----|-------------|
| `timeout` | API request timeout |
| `format` | Output format: `text`, `json`, `toml` |
| `color` | Color: `auto`, `always`, `never` |
| `show_secrets` | Show secret values instead of masking |
| `sensitive_keywords` | Keywords for detecting sensitive variable names |
| `sensitive_allowlist` | Keywords that suppress false-positive secret matches |
| `suppress_warnings` | Warning codes to suppress |
| `create` | Merge flag default |
| `update` | Merge flag default |
| `delete` | Merge flag default |

**`[workspace]`** and **`[project]`** are parent context — the
workspace and project that own this environment. Each has `name`
and optional `id`. When `id` is present, it is authoritative for
matching; when absent, the tool falls back to name.

**`[[service]]`** arrays declare services within this environment.
Each entry has `name` and optional `id` fields, plus inline tables
for variables, deploy settings, etc.

### Reserved names

`tool`, `workspace`, and `project` are TOML tables and
structurally cannot collide with `[[service]]` entries. No
service name reservation is needed.

```toml
name = "production"
id = "env_abc123"
variables = { NODE_ENV = "production" }

[tool]
timeout = "60s"

[workspace]
name = "Hamish Morgan's Projects"
id = "ws_abc123"

[project]
name = "Life"
id = "proj_abc123"

[[service]]
name = "api"
id = "srv_abc123"
variables = {
    PORT = "8080",
    DATABASE_URL = "${{postgres.DATABASE_URL}}",
}
deploy = {
    builder = "NIXPACKS",
    start_command = "node dist/server.js",
}
resources = { vcpus = 2, memory_gb = 4 }
domains = { "api.example.com" = { port = 8080 } }
volumes = { data = { mount = "/data" } }
```

The `id` fields are optional everywhere. When absent, the tool
matches by name. `adopt` and `apply` populate IDs automatically
after resolving resources.

Sub-tables use TOML v1.1 multiline inline tables (supported by
BurntSushi/toml v1.6.0+). This keeps all fields visually grouped
under their parent entry. The equivalent `[service.variables]`
sub-header form also works — the parser treats both identically.

A file doesn't need to include everything — the
[cascade](#file-cascade) merges files at different directory levels.
A root config might set `[workspace]`, `[project]`, and shared
service definitions; an environment-level file adds environment
identity and per-service overrides.

---

## Command structure

### Global flags

Every command accepts these flags.

| Flag | Short | Env var | Config key | Default | Description |
|------|-------|---------|------------|---------|-------------|
| `--token` | | `RAILWAY_TOKEN` / `RAILWAY_API_TOKEN` | — | — | Auth token |
| `--json` | | `FAT_CONTROLLER_FORMAT` | `tool.format` | | Output as JSON |
| `--toml` | | `FAT_CONTROLLER_FORMAT` | `tool.format` | | Output as TOML |
| `--raw` | | `FAT_CONTROLLER_FORMAT` | `tool.format` | | Output bare value, no formatting |
| `--color` | | `FAT_CONTROLLER_COLOR` | `tool.color` | `auto` | Color: `auto`, `always`, `never`. Respects `NO_COLOR` |
| `--timeout` | | `FAT_CONTROLLER_TIMEOUT` | `tool.timeout` | `30s` | API request timeout |
| `--ask` | `-a` | — | — | | Prompt for all parameters, even those with defaults |
| `--yes` | `-y` | `FAT_CONTROLLER_YES` | — | `false` | Accept all defaults without prompting |
| `--verbose` | `-v` | — | — | | Decrease log level. Repeatable: `-v` = DEBUG, `-vv` = TRACE |
| `--quiet` | `-q` | — | — | | Increase log level. Repeatable: `-q` = WARN, `-qq` = ERROR, `-qqq` = silent |

Default format is `auto` — the tool infers the best format from
context (e.g. file extension, whether stdout is a TTY). `--json`,
`--toml`, and `--raw` are mutually exclusive. `--raw` outputs the
bare value with no quoting or structure — only valid when the
result is a single scalar (e.g. `show api.variables.PORT`); errors
if the result is a table or list.

Default log level is INFO.

### Context flags

Commands that target Railway resources accept these flags. Values are
also resolved from the config file, env vars, and token scope — see
[Context resolution](#context-resolution).

| Flag | Env var | Config key | Description |
|------|---------|------------|-------------|
| `--workspace` | `FAT_CONTROLLER_WORKSPACE` | `workspace.name` | Workspace name or ID |
| `--project` | `FAT_CONTROLLER_PROJECT` | `project.name` | Project name or ID |
| `--environment` | `FAT_CONTROLLER_ENVIRONMENT` | `name` | Environment name or ID |
| `--service` | `FAT_CONTROLLER_SERVICE` | — | Service name or ID |

Each flag accepts either a name or an ID — the tool detects which
based on format and matches accordingly.

### Config flags

Commands that read or write config files accept these flags.

| Flag | Env var | Config key | Default | Description |
|------|---------|------------|---------|-------------|
| `--config` | `FAT_CONTROLLER_CONFIG` | `tool.config` | *(auto-discover)* | Config file path. Disables upward walk — loads only this file |
| `--secrets` | `FAT_CONTROLLER_SECRETS` | `tool.secrets` | *(co-located)* | Secrets file path for `${VAR}` interpolation |

### Merge flags

`apply` and `adopt` accept these flags. See
[Merge behavior](#merge-behavior) for details.

| Flag | Env var | Config key | Default | Description |
|------|---------|------------|---------|-------------|
| `--create` / `--no-create` | `FAT_CONTROLLER_CREATE` | `tool.create` | on | Add entities that exist in source but not target |
| `--update` / `--no-update` | `FAT_CONTROLLER_UPDATE` | `tool.update` | on | Overwrite entities that exist in both |
| `--delete` / `--no-delete` | `FAT_CONTROLLER_DELETE` | `tool.delete` | off | Remove entities that exist in target but not source |

### Mutation flags

Commands that modify state (`adopt`, `apply`, `deploy`, `redeploy`,
`restart`, `rollback`, `stop`) accept these flags.

| Flag | Short | Env var | Config key | Default | Description |
|------|-------|---------|------------|---------|-------------|
| `--dry-run` | | `FAT_CONTROLLER_DRY_RUN` | `tool.dry_run` | `false` | Preview changes without executing |
| `--fail-fast` | | `FAT_CONTROLLER_FAIL_FAST` | `tool.fail_fast` | `false` | Stop on first error |

### Apply-specific flags

| Flag | Env var | Config key | Default | Description |
|------|---------|------------|---------|-------------|
| `--skip-deploys` | `FAT_CONTROLLER_SKIP_DEPLOYS` | `tool.skip_deploys` | `false` | Don't trigger redeployments after variable changes |

### Display flags

Commands that show config or state (`show`, `diff`, `adopt`) accept
these flags.

| Flag | Env var | Config key | Default | Description |
|------|---------|------------|---------|-------------|
| `--show-secrets` | `FAT_CONTROLLER_SHOW_SECRETS` | `tool.show_secrets` | `false` | Show secret values instead of masking |

### Config-only keys

These are set only in the config file (`[tool]`), not via flags
or env vars.

| Config key | Default | Description |
|------------|---------|-------------|
| `tool.sensitive_keywords` | *(built-in list)* | Keywords for detecting sensitive variable names |
| `tool.sensitive_allowlist` | *(built-in list)* | Keywords that suppress false-positive secret matches |
| `tool.suppress_warnings` | `[]` | Warning codes to suppress (e.g. `["W012"]`) |

---

## Commands

### `new`

Scaffold entries in the local config file. Creates the file if it
doesn't exist, appends to it if it does. Never calls the Railway
API — use `apply` to create the resources in Railway.

**Non-destructive:** refuses to overwrite existing entries. If the
config already has a `[project]` table, `new project` errors. If a
`[[service]]` entry with `name = "api"` exists, `new service api`
errors. To modify existing entries, edit the file directly or use
`adopt`.

```text
fat-controller new [type] [options]
```

With no arguments in interactive mode, prompts for everything:
type, then all parameters for that type. In non-interactive mode,
`type` is required.

#### `new project`

Add project context to the config file. If no config file exists,
creates one.

```text
fat-controller new project [name]
```

| Arg/flag | Description |
|----------|-------------|
| `name` | Project name |

Flags: global, config.

Interactive resolution:

| Parameter | Default | Interactive | Non-interactive |
|-----------|---------|-------------|-----------------|
| Config file (`--config`) | `fat-controller.toml` | Use default, prompt if missing | Use default |
| Workspace | From config file | Use default, picker if missing | Use default, error if missing |
| Name | — | Prompt | Error if not specified |

Writes `[workspace]` and `[project]` tables to the config file.

#### `new environment`

Add environment context to the config file.

```text
fat-controller new environment [name]
```

| Arg/flag | Description |
|----------|-------------|
| `name` | Environment name |

Flags: global, config.

Interactive resolution:

| Parameter | Default | Interactive | Non-interactive |
|-----------|---------|-------------|-----------------|
| Name | — | Prompt | Error if not specified |

Writes the `name` key to the config file.

#### `new service`

Add a service definition to the config file. Pre-fills common
settings based on the service type.

```text
fat-controller new service [name] [--database <type>] [--repo <repo>] [--image <image>]
```

| Arg/flag | Description |
|----------|-------------|
| `name` | Service name |
| `--database <type>` | Pre-fill for a database (`postgres`, `mysql`, `redis`, `mongo`) |
| `--repo <repo>` | Pre-fill source repo |
| `--image <image>` | Pre-fill Docker image |

Flags: global, config.

Interactive resolution:

| Parameter | Default | Interactive | Non-interactive |
|-----------|---------|-------------|-----------------|
| Type | Empty service | Picker: empty, database, repo, image | Empty unless flag set |
| Name | — | Prompt (auto-suggested for databases) | Error if not specified |

Writes a `[[service]]` entry to the config file with `name` and
appropriate defaults for the service type. The `id` field is left
empty — `apply` populates it after creating the service in Railway.

### `adopt`

Pull live Railway state into the local config file. Populates `id`
fields — top-level for the environment, in `[workspace]` and
`[project]` for context, and in each `[[service]]` entry. Sensitive values
are detected and written to the secrets file as `${VAR}` references.
See [Merge behavior](#merge-behavior) for how `--create`, `--update`,
and `--delete` control the merge.

Works for both first-time bootstrap (no config file yet) and ongoing
sync (config file exists). Follows the standard prompting model —
uses defaults silently, prompts only when a value is missing, and
confirms before writing.

```text
fat-controller adopt [path]
```

| Arg/flag | Description |
|----------|-------------|
| `path` | Optional dot-path to limit what is adopted (e.g. `redis`, `api.variables`). Uses service names |

Flags: global, context, config, merge, mutation, display.

Interactive resolution:

| Parameter | Default | Interactive | Non-interactive |
|-----------|---------|-------------|-----------------|
| Config file (`--config`) | Auto-discover | Use default, prompt if missing | Use default, error if missing |
| Secrets file (`--secrets`) | Co-located | Use default, prompt if missing | Use default, error if missing |
| Workspace | From config file | Use default, prompt if missing | Use default, error if missing |
| Project | From config file | Use default, prompt if missing | Use default, error if missing |
| Environment | From config file | Use default, prompt if missing | Use default, error if missing |
| Services | All | All (reported, not prompted) | All |
| Confirm changes | — | Preview + confirm | Error unless `--yes` |

With `--ask`, every parameter is prompted (with defaults pre-filled).
Without it, `adopt` reports what it's doing and confirms before
writing:

```text
$ fat-controller adopt

  Workspace: Hamish Morgan's Projects
  Project: Life
  Environment: production
  Services: api, worker, postgres (all)
  Config: fat-controller.toml

  3 services adopted. Write changes? [Y/n]
```

### `diff`

Compare local config against live Railway state. Read-only — no
changes are made. Output reflects what `apply` would do given the
current merge flag settings.

```text
fat-controller diff [path]
```

| Arg/flag | Description |
|----------|-------------|
| `path` | Optional dot-path to narrow comparison (e.g. `api`, `api.variables`) |

Flags: global, context, config, merge, display.

Interactive resolution:

| Parameter | Default | Interactive | Non-interactive |
|-----------|---------|-------------|-----------------|
| Config file (`--config`) | Auto-discover | Use default, prompt if missing | Use default |
| Workspace | From config file | Use default, prompt if missing | Use default |
| Project | From config file | Use default, prompt if missing | Use default |
| Environment | From config file | Use default, prompt if missing | Use default |

Read-only — no confirmation needed.

### `apply`

Merge local config into live Railway state. See
[Merge behavior](#merge-behavior) for how `--create`, `--update`,
and `--delete` control the merge.

```text
fat-controller apply [path]
```

| Arg/flag | Description |
|----------|-------------|
| `path` | Optional dot-path to narrow scope (e.g. `api`, `api.variables`) |

Flags: global, context, config, merge, mutation, apply-specific.

Interactive resolution:

| Parameter | Default | Interactive | Non-interactive |
|-----------|---------|-------------|-----------------|
| Config file (`--config`) | Auto-discover | Use default, prompt if missing | Use default |
| Workspace | From config file | Use default, prompt if missing | Use default |
| Project | From config file | Use default, prompt if missing | Use default |
| Environment | From config file | Use default, prompt if missing | Use default |
| Confirm changes | — | Preview + confirm | Error unless `--yes` |

**Creation ordering.** When bootstrapping from scratch, `apply`
follows the resource hierarchy: project → environment → services
→ service sub-resources (variables, domains, volumes, etc.).
Services within an environment have no ordering dependency on
each other — Railway resolves `${{service.VAR}}` references at
deploy time, not at variable-set time. Services can be created
and configured in parallel.

### `validate`

Check config file for warnings without making API calls.

```text
fat-controller validate [path]
```

| Arg/flag | Description |
|----------|-------------|
| `path` | Optional dot-path to narrow validation (e.g. `api`) |

Flags: global, config.

Interactive resolution:

| Parameter | Default | Interactive | Non-interactive |
|-----------|---------|-------------|-----------------|
| Config file (`--config`) | Auto-discover | Use default, prompt if missing | Use default |

No API calls, no context flags needed.

### `show`

Display live Railway state for a resource. Read-only.

```text
fat-controller show [path]
```

| Path | What it shows |
|------|---------------|
| *(none)* | Everything in the current environment |
| `variables` | Shared variables for this environment |
| `api` | Everything about the `api` service |
| `api.variables` | Just `api`'s variables |
| `api.variables.PORT` | Single value |
| `workspace` | Peek up: workspace metadata (name, ID, members, settings) |
| `project` | Peek up: project metadata (name, ID, settings, tokens) |

The environment is the implicit scope — the tool always operates
within one environment. Paths without a qualifier refer to things
*in* the environment: `variables` for shared variables, service
names for services. `workspace` and `project` navigate upward to
parent resources. All other top-level paths are resolved as
service names, matched to `[[service]]` entries by `id` or `name`.

Flags: global, context, display.

Interactive resolution:

| Parameter | Default | Interactive | Non-interactive |
|-----------|---------|-------------|-----------------|
| Workspace | From config file | Use default, prompt if missing | Use default, error if missing |
| Project | From config file | Use default, prompt if missing | Use default, error if missing |
| Environment | From config file | Use default, prompt if missing | Use default, error if missing |

Context is always resolved the same way — from the config file,
env vars, or flags. `show workspace` and `show project` navigate
upward from the current environment's context, not across to
other workspaces or projects.

Use `--environment` to look at a different environment without
changing config:

```bash
fat-controller show --environment staging
fat-controller show api.variables --environment staging
```

`show` includes read-only fields from the API that the config file
does not express — for example, volume current size, deployment
status, or creation timestamps.

Read-only — no confirmation needed.

### `deploy`

Trigger a deployment. No arguments = all services in the environment.

```text
fat-controller deploy [service...]
```

Flags: global, context, mutation.

Interactive resolution:

| Parameter | Default | Interactive | Non-interactive |
|-----------|---------|-------------|-----------------|
| Workspace | From config file | Use default, prompt if missing | Use default, error if missing |
| Project | From config file | Use default, prompt if missing | Use default, error if missing |
| Environment | From config file | Use default, prompt if missing | Use default, error if missing |
| Services | All | All (reported, not prompted) | All |
| Confirm | — | Preview + confirm | Error unless `--yes` |

### `redeploy`

Redeploy the current image.

```text
fat-controller redeploy [service...]
```

Flags: global, context, mutation.

Interactive resolution: same as `deploy`.

### `restart`

Restart running deployments.

```text
fat-controller restart [service...]
```

Flags: global, context, mutation.

Interactive resolution: same as `deploy`.

### `rollback`

Rollback to the previous deployment.

```text
fat-controller rollback [service...]
```

Flags: global, context, mutation.

Interactive resolution: same as `deploy`.

### `stop`

Stop running deployments.

```text
fat-controller stop [service...]
```

Flags: global, context, mutation.

Interactive resolution: same as `deploy`.

### `logs`

View or stream logs. No service arguments = all services in the
environment. Streams by default; switches to fetch mode when
`--lines`, `--since`, or `--until` is set.

```text
fat-controller logs [service...] [--build | --deploy] [--lines N]
                    [--since <time>] [--until <time>] [--filter <query>]
```

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--build` | `-b` | | Show build logs |
| `--deploy` | `-d` | | Show deploy logs (default) |
| `--lines` | `-n` | | Fetch N lines (disables streaming) |
| `--since` | `-S` | | Start time: relative (`5m`, `2h`, `1d`) or ISO 8601 |
| `--until` | `-U` | | End time: same formats as `--since` |
| `--filter` | `-f` | | Filter expression (e.g. `@level:error`) |

Flags: global, context.

Interactive resolution:

| Parameter | Default | Interactive | Non-interactive |
|-----------|---------|-------------|-----------------|
| Workspace | From config file | Use default, prompt if missing | Use default, error if missing |
| Project | From config file | Use default, prompt if missing | Use default, error if missing |
| Environment | From config file | Use default, prompt if missing | Use default, error if missing |
| Services | All | All (reported, not prompted) | All |

Read-only — no confirmation needed.

### `status`

Show operational health for services. No arguments = all services
in the environment.

```text
fat-controller status [service...]
```

Includes deployment state, domain verification and certificate
status, volume state, and healthcheck results. Surfaces
actionable problems — for example, a custom domain with
`DNS not propagated` and the required CNAME record.

Flags: global, context.

Interactive resolution: same as `logs`.

### `ssh`

Open an interactive shell inside a running service container.
WebSocket-based — does not support SCP, SFTP, or port forwarding.

```text
fat-controller ssh [service] [command...]
```

| Arg/flag | Description |
|----------|-------------|
| `service` | Service to connect to (prompted if omitted) |
| `command...` | Optional command to run instead of interactive shell |

Flags: global, context.

Interactive resolution:

| Parameter | Default | Interactive | Non-interactive |
|-----------|---------|-------------|-----------------|
| Workspace | From config file | Use default, prompt if missing | Use default, error if missing |
| Project | From config file | Use default, prompt if missing | Use default, error if missing |
| Environment | From config file | Use default, prompt if missing | Use default, error if missing |
| Service | — | Picker | Error if not specified |

### `open`

Open the Railway dashboard in the browser for the current context.

```text
fat-controller open [--print]
```

| Flag | Short | Description |
|------|-------|-------------|
| `--print` | `-p` | Print URL instead of opening browser |

Flags: global, context.

### `list`

List entities. Takes an optional noun argument for the entity type.

```text
fat-controller list [type]
```

| Type | Context required | Description |
|------|-----------------|-------------|
| `all` | None | Full hierarchy: workspaces → projects → environments → services |
| `workspaces` | None | |
| `projects` | Workspace | |
| `environments` | Workspace + project | |
| `services` | Workspace + project + environment | |
| `deployments` | Workspace + project + environment | |
| `volumes` | Workspace + project | |
| `domains` | Workspace + project + environment | |

Flags: global, context.

**No argument behavior:** lists services in the current environment
(same as `list services`). Both `list` and `show` are
environment-scoped by default.

`list all` outputs the full hierarchy from workspaces down to
services as a tree:

```text
Hamish Morgan's Projects
  Life
    production
      api, worker, postgres
    staging
      api, worker, postgres
  Other Project
    production
      web
```

Context flags follow the standard pattern — prompt with default
if interactive, use default or error if non-interactive. Only the
context required for the entity type is resolved. `list all` and
`list workspaces` need no context. `list projects` needs a
workspace. `list services` (and the no-argument default) needs
workspace + project + environment.

### `auth login`

Authenticate via browser-based OAuth. Opens a browser by default.
Use `--browserless` for headless environments (SSH sessions, CI
containers) — displays a pairing code to enter at a URL.

```text
fat-controller auth login [--browserless]
```

| Flag | Short | Description |
|------|-------|-------------|
| `--browserless` | `-b` | Login via pairing code instead of opening a browser |

Flags: global.

### `auth logout`

Clear stored credentials.

```text
fat-controller auth logout
```

Flags: global.

Interactive resolution:

| Parameter | Default | Interactive | Non-interactive |
|-----------|---------|-------------|-----------------|
| Confirm | Yes | "Clear credentials? [Y/n]" | Error unless `--yes` |

### `auth status`

Show current authentication state.

```text
fat-controller auth status
```

Flags: global.

Read-only — no interactive resolution needed.

### `completion`

Generate shell completion scripts.

```text
fat-controller completion <shell>
```

Supported shells: `bash`, `zsh`, `fish`, `powershell`.

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
`adopt` writes, and where the local override is resolved.

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

### Local overrides

The `.local` file has the same schema as the main config file. It
merges on top — any key can be overridden. Use it for personal
preferences that shouldn't be committed:

```toml
# fat-controller.local.toml
[tool]
format = "json"
show_secrets = true
```

### File cascade

Multiple config files are loaded and merged in precedence order,
lowest priority first:

1. **Compiled-in defaults** — built into the binary.
2. **Global config** — `$XDG_CONFIG_HOME/fat-controller/config.toml`.
   Always at this fixed path. Useful for setting `[tool]` preferences
   (`format`, `color`, `timeout`) or a default `[workspace]` across
   all projects.
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
$XDG_CONFIG_HOME/fat-controller/config.toml   # [tool] timeout = "60s"
repo-root/fat-controller.toml                  # [workspace], [project], [[service]]
environments/production/fat-controller.toml    # name, variables, [[service]] overrides
environments/production/fat-controller.local.toml  # [tool] show_secrets = true
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

- **Top-level scalar keys** (`name`, `id`): later values replace
  earlier ones.
- **Top-level `variables`**: deep merge. Individual keys from
  higher-precedence files override lower-precedence ones.
- **`[tool]`**: deep merge. A global config can set `timeout` and
  a project config can override it.
- **`[workspace]`, `[project]`**: deep merge. A root config
  typically sets these; environment configs inherit them.
- **`[[service]]` entries**: matched by `id` (if present) or
  `name`. When the same service appears in multiple files, inline
  tables are deep-merged — a root config can set `deploy` and an
  environment config can add `resources` or override individual
  deploy fields. A higher-precedence file's values win.
- **Environment variables and CLI flags** only set tool settings
  and context (workspace/project/environment). They do not express
  Railway state.

---

## Entity coverage

### What the TOML can express

| Entity | Section | Fields |
|--------|---------|--------|
| Variables (shared) | `variables` (top-level) | key-value pairs |
| Variables (per-service) | `[service.variables]` | key-value pairs |
| Deploy settings | `[service.deploy]` | See below |
| Resources | `[service.resources]` | `vcpus`, `memory_gb` |
| Scaling | `[service.scale]` | Per-region instance counts |
| Custom domains | `[service.domains]` | hostname, target port |
| Service domains | `[service.domains]` | railway.app subdomain, target port |
| Volumes | `[service.volumes]` | name, mount path (size is read-only, visible via `show`) |
| TCP proxies | `[service.tcp_proxies]` | application port |
| Private network endpoints | `[service.network]` | DNS name |
| Deployment triggers | `[service.triggers]` | branch, repo, check suites |
| Egress gateways | `[service.egress]` | service association |

Each `[[service]]` entry has `name` (required) and `id` (optional,
populated by `adopt`/`apply`). Sub-tables use `[service.X]` and
attach to the preceding `[[service]]` entry.

`[service.deploy]` fields: `repo`, `image`, `builder`,
`build_command`, `start_command`, `dockerfile_path`,
`root_directory`, `healthcheck_path`, `healthcheck_timeout`,
`cron_schedule`, `draining_seconds`, `num_replicas`,
`overlap_seconds`, `pre_deploy_command`, `region`,
`restart_policy`, `restart_policy_max_retries`,
`sleep_application`, `watch_patterns`.

`repo` and `image` are mutually exclusive source types. `repo` is
a GitHub repo (e.g. `"railwayapp/starters"`); `image` is a Docker
image (e.g. `"postgres:16"`). If neither is specified, `apply`
creates the service with no source.

The minimal service definition is just a `[[service]]` entry with
a `name` — no sub-tables required. The service is created empty
in Railway; sub-tables are applied after creation.

`[service.scale]` expresses multi-region scaling as region =
instance count pairs:

```toml
[[service]]
name = "api"
scale = { us-west1 = 3, europe-west4 = 2 }
```

This replaces `num_replicas` and `region` in `deploy` for
services that scale across multiple regions. Single-region
services can use either `deploy.region` or `scale`.

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
fat-controller.toml                        # workspace, project, shared variables
environments/
  production/fat-controller.toml           # name = "production", overrides
  staging/fat-controller.toml              # name = "staging", overrides
```

The root config sets `[workspace]`, `[project]`, and shared service
definitions. Each environment directory's config sets the
environment identity and overrides only what differs (e.g. resource
limits, replica counts).

```toml
# fat-controller.toml (root — shared base)
[workspace]
name = "Hamish Morgan's Projects"
id = "ws_abc123"

[project]
name = "Life"
id = "proj_abc123"

[[service]]
name = "api"
id = "srv_abc123"
deploy = {
    builder = "NIXPACKS",
    start_command = "node dist/server.js",
}
domains = { "api.example.com" = { port = 8080 } }
```

```toml
# environments/production/fat-controller.toml
name = "production"
id = "env_prod123"
variables = { NODE_ENV = "production" }

[[service]]
name = "api"
id = "srv_abc123"
resources = { vcpus = 4, memory_gb = 8 }
```

```toml
# environments/staging/fat-controller.toml
name = "staging"
id = "env_stg123"
variables = { NODE_ENV = "staging" }

[[service]]
name = "api"
id = "srv_xyz789"
resources = { vcpus = 1, memory_gb = 2 }
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
  api/fat-controller.toml           # declares only the api [[service]]
  worker/fat-controller.toml        # declares only the worker [[service]]
```

Each file is environment-scoped but only declares one service.
Because `apply` is additive by default, each file only touches
the services it mentions. A CI pipeline applies each file
independently. Shared variables live in a root-level config (picked
up via cascade) or are duplicated across files.

Best for: large teams where each service team owns their config.

---

## Context resolution

Commands that target Railway resources need `workspace`, `project`,
`environment`, and optionally `service`. These are resolved using the
[file cascade](#file-cascade) — CLI flags, then env vars, then config
files — with two additional fallback sources:

- **Token scope.** A project-scoped `RAILWAY_TOKEN` implies a
  specific project and environment.
- **Interactive picker.** If a value is still missing and stdin is a
  TTY, the user picks from a list of available options. Otherwise,
  the command errors with available options listed.

---

## Interactive vs non-interactive mode

**Detection:** interactive mode is active when stdin is a TTY.
Piped or redirected stdin = non-interactive. This is not a flag — it
is determined by the terminal environment.

**Core principle:** every command parameter is resolved the same way,
but the behavior for "unspecified" depends on the mode and prompting
level:

| | Interactive (TTY) | Non-interactive (piped/CI) |
|---|---|---|
| Specified via flag | Use flag value | Use flag value |
| Has a default | Use default silently | Use default silently |
| No default, options available | Picker (select from list) | Error with available options |
| No default, no options | Prompt for free-text input | Error |
| Mutation | Preview + confirmation (default: yes) | Error unless `--yes` |
| Colors | Auto-detected | Off (unless `--color=always`) |

**Prompting levels** control how aggressively the tool prompts in
interactive mode:

| Flag | Has a default | No default | Mutation |
|------|---------------|------------|----------|
| `--ask` | Prompt, pre-filled with default | Prompt/picker | Confirm |
| *(default)* | Use default silently | Prompt/picker | Confirm |
| `--yes` | Use default silently | Error if missing | Skip confirmation |

`--ask` is only valid in interactive mode — it errors on a
non-interactive terminal. `--yes` works in both modes.

**Flags pin values.** If `--project`, `--environment`, or any other
flag is specified with a value, that value is locked in — no prompt
is shown for it regardless of `--ask`. Everything unspecified follows
the prompting level.

This means you can use `--ask` to explore interactively even when
the config file has defaults:

```text
$ fat-controller show --ask

  Workspace: Hamish Morgan's Projects  (1 of 1)
  Project: > Life
            Other Project
  Environment: > production
                staging
```

Or pin specific values and only be prompted for the rest:

```text
$ fat-controller show --ask --project Life

  Environment: > production
                staging
```

Without `--ask`, if the config file has `project = "Life"` and
`environment = "production"`, `show` uses those silently — no
prompts at all.

**`--ask`, `--yes`, and `--dry-run`:**

`--dry-run` prevents all mutations. When combined with other flags,
`--dry-run` always wins.

| Flags | Interactive | Non-interactive |
|-------|-------------|-----------------|
| (none) | Use defaults, prompt if missing | Use defaults, error if missing |
| `--ask` | Prompt for everything | Error (requires TTY) |
| `--yes` | Use defaults, skip confirmations | Use defaults, skip confirmations |
| `--dry-run` | Use defaults, prompt if missing, preview only | Preview only, no mutations |
| `--ask --dry-run` | Prompt for everything, preview only | Error (requires TTY) |
| `--yes --dry-run` | Use defaults, preview only | Use defaults, preview only |

`--ask` and `--yes` are mutually exclusive — specifying both is an
error.

The goal: interactive mode is convenient for humans — `--ask` lets
you explore, defaults keep things quick for the common case.
Non-interactive mode is safe for CI — deterministic, no prompts,
fails loudly on missing values.

---

## Merge behavior

`apply` and `adopt` share three merge flags (`--create`, `--update`,
`--delete`) that control what the merge does. See
[Merge flags](#merge-flags) for flag details and defaults.

### Identity matching

All Railway resources — workspace, project, environment, and
services — are matched between config and Railway by **ID when
present**, falling back to **name when not**. This means:

- A resource with an `id` field matches the Railway resource with
  that ID, regardless of name changes on either side.
- A resource with only `name` (no `id`) matches by name.
- After `adopt` or `apply` resolves a resource, it writes the `id`
  back to the config file so subsequent operations are ID-based.

If a resource has an `id` but that ID doesn't exist in Railway,
the tool errors — the ID is stale. Use `adopt` to re-sync, or
remove the `id` to fall back to name matching.

### Merge direction

`apply` and `adopt` are symmetric — the same flags have parallel
meaning in opposite directions:

| Flag | `apply` (config → Railway) | `adopt` (Railway → config) |
|------|---------------------------|---------------------------|
| `--create` | Create Railway entities not in Railway | Add config entries not in config |
| `--update` | Update Railway entities that differ from config | Update config entries that differ from Railway |
| `--delete` | Delete Railway entities not in config | Remove config entries not in Railway |

**Path scoping.** Both `apply` and `adopt` accept an optional
dot-path that narrows the operation. Merge flags apply only within
the scoped path — everything outside it is untouched.

```bash
apply                              # create + update everything (default)
apply --delete                     # full convergence, everything
apply --delete api.variables       # full convergence, api variables only
apply api                          # create + update the api service only
apply --no-update                  # create only, don't touch existing
apply --no-create                  # update only, don't add new
adopt api                          # adopt only the api service
adopt --delete api.variables       # sync api variables, remove stale
```

This eliminates the need for per-section `managed` keys — scope
`--delete` with a path to get per-section delete granularity.

Without `--delete`, explicit delete markers handle one-off removals:

```toml
[[service]]
name = "api"
id = "srv_abc123"
variables = { OLD_VAR = { delete = true } }
volumes = { old-data = { delete = true } }

[[service]]
name = "old-service"
id = "srv_def456"
delete = true
```

`diff` reflects the active flags: it shows what `apply` would do
given the current create/update/delete settings.

---

## Open questions

1. **Buckets (S3-compatible object storage).** Railway supports
   managed S3-compatible buckets with their own lifecycle (create,
   delete, rename, credentials). Should these be declarative
   (`[service.buckets]`) or imperative-only? They have state (name,
   region) but also credentials that are more like secrets.

2. **Functions (serverless).** Railway supports serverless functions
   with their own deploy/push model. These are a different resource
   type from services. Should they be a new table type
   (`[fn.name]` or `[functions.name]`)? Or are they similar enough
   to services to use the same `[[service]]` structure?

3. **`scale` vs `deploy` overlap.** `[service.scale]` handles
   multi-region instance counts, while `[service.deploy]` has
   `num_replicas` and `region` for single-region. Should
   `num_replicas`/`region` be removed from `[service.deploy]` in
   favor of always using `[service.scale]`?
