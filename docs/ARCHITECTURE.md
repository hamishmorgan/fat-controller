# Architecture

Command structure, config file format, and interaction model for
fat-controller.

---

## Principles

1. **One file, one environment.** Each config file describes a single
   environment. Not a project, not a workspace — one environment
   and its services. Multi-environment setups use multiple files.

2. **Config is state, not actions.** The config file expresses what
   the infrastructure should look like — variables, domains, deploy
   settings, resources. It never expresses things that happen once:
   deployments, restarts, rollbacks. Those are commands.

3. **Additive by default.** Unmentioned entities are never touched.
   If your file doesn't mention a service, that service is ignored
   — not deleted. `--delete` opts in to full convergence.

4. **No local state file.** Live state always comes from Railway's
   API. Diffs compare config against live, never against a cached
   snapshot.

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
        Buckets
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
| `volumes` | Unattached volumes (name → mount path, optional region) |
| `buckets` | S3-compatible object storage buckets |

**`[tool]`** holds tool settings — how fat-controller behaves,
not what it manages:

| Key | Description |
|-----|-------------|
| `api_timeout` | Overall time limit per API request (connect through response) |
| `log_level` | Log level: `trace`, `debug`, `info`, `warn`, `error`, `silent` |
| `output_format` | Output format: `auto`, `text`, `json`, `toml`, `raw` |
| `output_color` | Color: `auto`, `always`, `never` |
| `prompt` | Prompting mode: `all`, `default`, `none` |
| `show_secrets` | Show secret values instead of masking |
| `sensitive_keywords` | Keywords for detecting sensitive variable names |
| `sensitive_allowlist` | Keywords that suppress false-positive secret matches |
| `suppress_warnings` | Warning codes to suppress |
| `fail_fast` | Stop on first error instead of trying all operations |
| `deploy` | Deploy after apply: `run`, `skip` |
| `allow_create` | Merge flag default: add entities that exist in source but not target |
| `allow_update` | Merge flag default: overwrite entities that exist in both |
| `allow_delete` | Merge flag default: remove entities that exist in target but not source |

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
api_timeout = "60s"

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
matches by name. Any command that resolves a resource records its
ID in the config file as bookkeeping — this is not a merge
direction, just pinning the match for future operations.

Sub-tables use TOML v1.1 multiline inline tables (supported by
BurntSushi/toml v1.6.0+). This keeps all fields visually grouped
under their parent entry. The equivalent `[service.variables]`
sub-header form also works — the parser treats both identically.

### Variable interpolation

Two interpolation syntaxes serve different purposes:

- **`${VAR}`** — variable substitution. Resolved by fat-controller
  at load time from env files (if declared) or the process
  environment. The value never appears in the config file. Use for
  any values that should be sourced externally — secrets, but also
  environment-specific settings, feature flags, etc.

- **`${{service.VAR}}`** — Railway reference variable. Passed
  through to Railway verbatim. Railway resolves the reference at
  deploy time, substituting the value from the named service. Use
  for cross-service references like database URLs.

```toml
[[service]]
name = "api"
variables = {
    SECRET_KEY = "${SECRET_KEY}",
    DATABASE_URL = "${{postgres.DATABASE_URL}}",
}
```

In this example, `SECRET_KEY` is resolved from an env file or the
process environment and sent to Railway as a literal value.
`DATABASE_URL` is stored in Railway as a reference — Railway
resolves it to the `postgres` service's `DATABASE_URL` at deploy
time.

**`${VAR}` resolution order:**

1. **Env files** — checked in declaration order if declared via
   `tool.env_file`, `--env-file`, or `FAT_CONTROLLER_ENV_FILE`.
   First match wins.
2. **Process environment** — `os.Getenv`. In CI, variables are
   typically injected by the provider.
3. **Error** — `validate` warns, `apply` errors.

No magic file discovery. If the process environment already has
the variable, no env file is needed.

### Env files

Env files are optional — a convenience for loading multiple
`${VAR}` values at once. They use dotenv format (`KEY=value`, one
per line):

```text
SECRET_KEY=super-secret-value
DATABASE_HOST=db.example.com
```

A config file can declare one or more env files:

```toml
[tool]
env_file = [".env", ".env.production"]
```

A single file can be specified as a string instead of a list:

```toml
[tool]
env_file = ".env.production"
```

Paths are relative to the config file that declares them. This
participates in the cascade normally — a root config could set
`env_file = ".env"` and an environment config could override it.
`--env-file` and `FAT_CONTROLLER_ENV_FILE` override everything,
relative to the working directory.

During `adopt`, sensitive values are detected and written as
`${VAR}` references in the config. The actual values are written
to the last env file in the list — the file is created if it
doesn't exist. If no env file is declared, `adopt` prompts for
a path — or errors in non-interactive mode.

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
| `--json` | | `FAT_CONTROLLER_OUTPUT_FORMAT` | `tool.output_format` | | Output as JSON |
| `--toml` | | `FAT_CONTROLLER_OUTPUT_FORMAT` | `tool.output_format` | | Output as TOML |
| `--raw` | | `FAT_CONTROLLER_OUTPUT_FORMAT` | `tool.output_format` | | Output bare value, no formatting |
| `--color` | | `FAT_CONTROLLER_OUTPUT_COLOR` | `tool.output_color` | `auto` | Color: `auto`, `always`, `never`. Respects `NO_COLOR` |
| `--timeout` | | `FAT_CONTROLLER_API_TIMEOUT` | `tool.api_timeout` | `30s` | Time limit per API request |
| `--ask` | `-a` | `FAT_CONTROLLER_PROMPT` | `tool.prompt` | | Set prompt mode to `all` |
| `--yes` | `-y` | `FAT_CONTROLLER_PROMPT` | `tool.prompt` | | Set prompt mode to `none` |
| `--verbose` | `-v` | `FAT_CONTROLLER_LOG_LEVEL` | `tool.log_level` | | Decrease log level. Repeatable: `-v` = DEBUG, `-vv` = TRACE |
| `--quiet` | `-q` | `FAT_CONTROLLER_LOG_LEVEL` | `tool.log_level` | | Increase log level. Repeatable: `-q` = WARN, `-qq` = ERROR, `-qqq` = silent |

Default format is `auto` — the tool picks the best format from
context (e.g. text for TTY, JSON for piped output). `--json`,
`--toml`, and `--raw` are mutually exclusive. `--raw` outputs the
bare value with no quoting or structure — only valid when the
result is a single scalar (e.g. `show api.variables.PORT`); errors
if the result is a table or list.

Default log level is `info`. The env var and config key set the
base level (`trace`, `debug`, `info`, `warn`, `error`, `silent`);
`--verbose` and `--quiet` adjust it relative to whatever the base
is.

### Context flags

Commands that target Railway resources accept these flags. Values are
also resolved from the config file, env vars, and token scope — see
[Context resolution](#context-resolution).

| Flag | Env var | Config key | Description |
|------|---------|------------|-------------|
| `--workspace` | `FAT_CONTROLLER_WORKSPACE` | `workspace.id` / `workspace.name` | Workspace name or ID |
| `--project` | `FAT_CONTROLLER_PROJECT` | `project.id` / `project.name` | Project name or ID |
| `--environment` | `FAT_CONTROLLER_ENVIRONMENT` | `id` / `name` | Environment name or ID |
| `--service` | `FAT_CONTROLLER_SERVICE` | — | Service name or ID |

Flags and env vars accept either a name or an ID — the tool
detects which based on format. Config files have separate `id` and
`name` fields; `id` is authoritative when present, `name` is the
fallback. IDs are stable across renames — prefer `id` in any
long-lived config.

### Config flags

Commands that read or write config files accept these flags.

| Flag | Env var | Config key | Default | Description |
|------|---------|------------|---------|-------------|
| `--config-file` | `FAT_CONTROLLER_CONFIG_FILE` | — | *(auto-discover)* | Config file path. Disables upward walk — loads only this file |
| `--env-file` | `FAT_CONTROLLER_ENV_FILE` | `tool.env_file` | *(none)* | Env file path(s) for `${VAR}` interpolation |

### Merge flags

`apply` and `adopt` accept these flags. See
[Merge behavior](#merge-behavior) for details.

| Flag | Env var | Config key | Default | Description |
|------|---------|------------|---------|-------------|
| `--create` / `--no-create` | `FAT_CONTROLLER_ALLOW_CREATE` | `tool.allow_create` | on | Add entities that exist in source but not target |
| `--update` / `--no-update` | `FAT_CONTROLLER_ALLOW_UPDATE` | `tool.allow_update` | on | Overwrite entities that exist in both |
| `--delete` / `--no-delete` | `FAT_CONTROLLER_ALLOW_DELETE` | `tool.allow_delete` | off | Remove entities that exist in target but not source |

### Mutation flags

Commands that modify state (`adopt`, `apply`, `deploy`, `redeploy`,
`restart`, `rollback`, `stop`) accept these flags.

| Flag | Short | Env var | Config key | Default | Description |
|------|-------|---------|------------|---------|-------------|
| `--dry-run` | | `FAT_CONTROLLER_DRY_RUN` | — | `false` | Preview changes without executing |
| `--fail-fast` | | — | `tool.fail_fast` | `false` | Stop on first error |

### Apply-specific flags

| Flag | Env var | Config key | Default | Description |
|------|---------|------------|---------|-------------|
| `--skip-deploys` | `FAT_CONTROLLER_DEPLOY` | `tool.deploy` | `run` | Set deploy mode to `skip` — don't trigger redeployments after changes |

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
| Config file (`--config-file`) | `fat-controller.toml` | Use default, prompt if missing | Use default |
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
empty — once the service is created in Railway, the resolved ID
is recorded as bookkeeping.

### `adopt`

Pull live Railway state into the local config file. Sensitive
values are detected and written as `${VAR}` references in the
config, with the actual values written to an env file (see
[Env files](#env-files)).
See [Merge behavior](#merge-behavior) for how `--create`, `--update`,
and `--delete` control the merge.

Works for both first-time bootstrap (no config file yet) and ongoing
sync (config file exists). Follows the standard prompting model —
uses defaults silently, prompts only when a value is missing, and
confirms before writing.

**Env file interaction.** When `adopt` writes sensitive values, it
adds `${VAR}` references to the config and appends the actual
values to the env file. When `adopt --delete` removes a variable
from the config, the corresponding env file entry is left intact
— env files are user-managed. `validate` warns about env file
entries that are no longer referenced by any config file in the
cascade.

```text
fat-controller adopt [path]
```

| Arg/flag | Description |
|----------|-------------|
| `path` | Optional dot-path to limit what is adopted (e.g. `redis`, `api.variables`). Matches against config-file service names |

Flags: global, context, config, merge, mutation, display.

Interactive resolution:

| Parameter | Default | Interactive | Non-interactive |
|-----------|---------|-------------|-----------------|
| Config file (`--config-file`) | Auto-discover | Use default, prompt if missing | Use default, error if missing |
| Env file (`--env-file`) | From `tool.env_file` | Use default, prompt if missing | Use default, error if missing |
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
| Config file (`--config-file`) | Auto-discover | Use default, prompt if missing | Use default |
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
| Config file (`--config-file`) | Auto-discover | Use default, prompt if missing | Use default |
| Workspace | From config file | Use default, prompt if missing | Use default |
| Project | From config file | Use default, prompt if missing | Use default |
| Environment | From config file | Use default, prompt if missing | Use default |
| Confirm changes | — | Preview + confirm | Error unless `--yes` |

**Creation ordering.** `apply` can create projects, environments,
services, and service sub-resources. It cannot create workspaces
— the workspace must already exist. When bootstrapping from
scratch, `apply` follows the resource hierarchy: project →
environment → services → service sub-resources (variables,
domains, volumes, etc.).
Services within an environment have no ordering dependency on
each other — Railway resolves `${{service.VAR}}` references at
deploy time, not at variable-set time. Services can be created
and configured in parallel.

**Concurrent runs.** There is no local lock file. Two `apply`
runs against the same environment race at the API level —
last write wins per resource, same as two users editing the
Railway dashboard simultaneously. `diff` before `apply` to
check for unexpected drift. CI pipelines should serialize
`apply` runs per environment (e.g. via job-level concurrency
controls).

### `validate`

Check config files for errors and warnings without making API
calls. Operates on the merged cascade — all discovered config
files are loaded and merged before validation. Catches problems
before `apply`:

- TOML syntax and schema errors
- Unknown keys or invalid value types
- Duplicate service names
- Broken `${{service.VAR}}` references (referencing services
  not defined in the config)
- Unresolvable `${VAR}` references (not in env files or process
  environment)
- Mutually exclusive fields (`repo` + `image`, `scale` +
  `deploy.region`)

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
| Config file (`--config-file`) | Auto-discover | Use default, prompt if missing | Use default |

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
| `volumes` | Unattached volumes for this environment |
| `buckets` | S3-compatible buckets (name, credentials, endpoint, size) |
| `api` | Everything about the `api` service |
| `api.variables` | Just `api`'s variables |
| `api.variables.PORT` | Single value |
| `workspace` | Peek up: workspace metadata (name, ID, members, settings) |
| `project` | Peek up: project metadata (name, ID, settings, tokens) |

The environment is the implicit scope. Unqualified paths refer to
things *in* the environment; `workspace` and `project` navigate
upward to parent resources. All other top-level paths are resolved
as service names, matched by `id` or `name`.

Flags: global, context, config, display.

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

Flags: global, context, config, mutation.

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

Flags: global, context, config, mutation.

Interactive resolution: same as `deploy`.

### `restart`

Restart running deployments.

```text
fat-controller restart [service...]
```

Flags: global, context, config, mutation.

Interactive resolution: same as `deploy`.

### `rollback`

Rollback to the previous deployment.

```text
fat-controller rollback [service...]
```

Flags: global, context, config, mutation.

Interactive resolution: same as `deploy`.

### `stop`

Stop running deployments.

```text
fat-controller stop [service...]
```

Flags: global, context, config, mutation.

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

Flags: global, context, config.

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

Flags: global, context, config.

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

Flags: global, context, config.

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

Flags: global, context, config.

Interactive resolution:

| Parameter | Default | Interactive | Non-interactive |
|-----------|---------|-------------|-----------------|
| Workspace | From config file | Use default, prompt if missing | Use default, error if missing |
| Project | From config file | Use default, prompt if missing | Use default, error if missing |
| Environment | From config file | Use default, prompt if missing | Use default, error if missing |

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
| `deployments` | Workspace + project + environment | Recent deployments across all services. Use `--service` to narrow to one |
| `volumes` | Workspace + project | |
| `buckets` | Workspace + project | |
| `domains` | Workspace + project + environment | |

Flags: global, context, config.

**No argument behavior:** lists services in the current environment
(same as `list services`).

`show` is always environment-scoped — it shows what's in your
environment. `list` is broader — `list volumes` and `list buckets`
are project-scoped (across all environments), useful for seeing
the full inventory. Most `list` types are environment-scoped.

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
Credentials are stored in
`$XDG_DATA_HOME/fat-controller/credentials.json`.

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

When `--config-file` is not specified, config files are found by walking
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

The **primary config file** is the deepest one found — this is
where `adopt` writes state, where ID bookkeeping is recorded, and
where the local override is resolved.

When `--config-file` is specified, only that single file is loaded — no
upward walk.

### File locations summary

| File | Purpose | Committed? |
|------|---------|-----------|
| `fat-controller.toml` | Desired state + shared settings | Yes |
| `fat-controller.local.toml` | Personal overrides | No (gitignored) |

When using the `.config/fat-controller/` directory form:

| File | Purpose | Committed? |
|------|---------|-----------|
| `.config/fat-controller/config.toml` | Desired state + shared settings | Yes |
| `.config/fat-controller/config.local.toml` | Personal overrides | No (gitignored) |

Env files are not convention-based — declare them explicitly via
`tool.env_file` or pass `--env-file`. See [Env files](#env-files).

### Local overrides

The `.local` file has the same schema as the main config file. It
merges on top — any key can be overridden. Use it for personal
preferences that shouldn't be committed:

```toml
# fat-controller.local.toml
[tool]
output_format = "json"
show_secrets = true
```

### File cascade

Multiple config files are loaded and merged in precedence order,
lowest priority first:

1. **Compiled-in defaults** — built into the binary.
2. **Global config** — `$XDG_CONFIG_HOME/fat-controller/config.toml`.
   Always at this fixed path. Useful for setting `[tool]` preferences
   (`output_format`, `output_color`, `api_timeout`) or a default `[workspace]` across
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
$XDG_CONFIG_HOME/fat-controller/config.toml   # [tool] api_timeout = "60s"
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
- **`[tool]`**: deep merge. A global config can set `api_timeout` and
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

**Cascade edge cases:**

- **Mutually exclusive fields** (`repo`/`image`). If a root config
  sets `image` and an environment config sets `repo`, the deep
  merge produces both — `validate` catches this as an error on the
  merged result. To switch source type in an override, explicitly
  clear the other: `image = ""`.
- **`delete = true` in cascade.** A `delete = true` marker in a
  higher-precedence file wins. If a root config defines a service
  and an environment config marks it `delete = true`, the service
  is deleted in that environment.
- **`--config-file` and local overrides.** `--config-file` disables
  the upward walk (cascade items 3) *and* skips the local override
  (item 4). Only the specified file is loaded.

---

## Entity coverage

### What the TOML can express

| Entity | Section | Fields |
|--------|---------|--------|
| Variables (shared) | `variables` (top-level) | key-value pairs |
| Variables (per-service) | `service.variables` | key-value pairs |
| Deploy settings | `service.deploy` | See below |
| Resources | `service.resources` | `vcpus`, `memory_gb` |
| Scaling | `service.scale` | Per-region instance counts |
| Custom domains | `service.domains` | hostname → target port |
| Service domains | `service.domains` | enabled (boolean), target port. Subdomain is assigned by Railway (read-only via `show`) |
| Volumes (attached) | `service.volumes` | name, mount path, optional region (size is read-only, visible via `show`) |
| Volumes (unattached) | `volumes` (top-level) | name, mount path, optional region |
| Buckets | `buckets` (top-level) | name (credentials, endpoint, region are assigned by Railway, read-only via `show`) |
| TCP proxies | `service.tcp_proxies` | application port. Proxy port and domain are assigned by Railway (read-only via `show`) |
| Private network endpoints | `service.network` | enabled (boolean). DNS name is assigned by Railway (read-only via `show`) |
| Deployment triggers | `service.triggers` | branch, repository, provider, check suites, root directory |
| Egress gateways | `service.egress` | region (Railway assigns the static IPv4, read-only via `show`) |

Each `[[service]]` entry has `name` (required), `id` (optional,
populated by `adopt`/`apply`), and `icon` (optional — the service's
icon identifier in the Railway dashboard). Sub-tables use
`service.X` keys and attach to the preceding `[[service]]` entry.

`service.deploy` fields:

- **Source:** `repo`, `image`, `branch`, `registry_credentials`
- **Build:** `builder`, `build_command`, `dockerfile_path`,
  `root_directory`, `nixpacks_plan`, `watch_patterns`
- **Run:** `start_command`, `pre_deploy_command`, `cron_schedule`
- **Health:** `healthcheck_path`, `healthcheck_timeout`,
  `restart_policy`, `restart_policy_max_retries`
- **Deploy strategy:** `draining_seconds`, `overlap_seconds`,
  `sleep_application`
- **Placement:** `num_replicas`, `region`
- **Networking:** `ipv6_egress`

`repo` and `image` are mutually exclusive source types. `repo` is
a GitHub repo (e.g. `"railwayapp/starters"`); `image` is a Docker
image (e.g. `"postgres:16"`). `branch` sets the Git branch for
repo-sourced services (e.g. `"main"`). If neither `repo` nor
`image` is specified, `apply` creates the service with no source.

`registry_credentials` authenticates to a private Docker registry.
It takes `username` and `password` — use `${VAR}` interpolation
for credentials to keep them out of the config file:

```toml
deploy = {
    image = "registry.example.com/app:latest",
    registry_credentials = {
        username = "deploy",
        password = "${REGISTRY_PASSWORD}",
    },
}
```

`nixpacks_plan` is a TOML inline table matching the Nixpacks plan
schema — custom providers, phases, and build settings for
Nixpacks-built services.

The minimal service definition is just a `[[service]]` entry with
a `name` — no sub-tables required. The service is created empty
in Railway; sub-tables are applied after creation.

`service.scale` expresses multi-region scaling as region =
instance count pairs:

```toml
[[service]]
name = "api"
scale = { us-west1 = 3, europe-west4 = 2 }
```

`deploy.num_replicas` and `deploy.region` handle the common
single-region case. `service.scale` handles multi-region. When
both are present, `scale` takes precedence — `deploy.region` and
`deploy.num_replicas` are ignored. Single-region services can use
either form:

```toml
# These are equivalent:
deploy = { region = "us-west1", num_replicas = 2 }
scale = { us-west1 = 2 }
```

### Comprehensive service example

A service using all sub-resource types:

```toml
[[service]]
name = "api"
id = "srv_abc123"
icon = "server"
variables = {
    PORT = "8080",
    SECRET_KEY = "${SECRET_KEY}",
    DATABASE_URL = "${{postgres.DATABASE_URL}}",
}
deploy = {
    repo = "org/api",
    branch = "main",
    builder = "NIXPACKS",
    build_command = "npm run build",
    start_command = "node dist/server.js",
    pre_deploy_command = ["npx", "prisma", "migrate", "deploy"],
    root_directory = "apps/api",
    dockerfile_path = "Dockerfile",
    healthcheck_path = "/health",
    healthcheck_timeout = 30,
    restart_policy = "ON_FAILURE",
    restart_policy_max_retries = 5,
    cron_schedule = "",
    draining_seconds = 30,
    overlap_seconds = 5,
    sleep_application = false,
    watch_patterns = ["apps/api/**", "packages/shared/**"],
    ipv6_egress = false,
}
resources = { vcpus = 2, memory_gb = 4 }
scale = { us-west1 = 3, europe-west4 = 2 }
domains = {
    "api.example.com" = { port = 8080 },
    service_domain = { port = 8080 },
}
volumes = {
    data = { mount = "/data" },
    cache = { mount = "/cache", region = "us-west1" },
}
tcp_proxies = [5432]
network = true
triggers = [
    { branch = "main", repository = "org/api", check_suites = true },
]
egress = ["us-west1"]
```

Most services need only a few of these. The
[minimal service definition](#what-the-toml-can-express) is just
`name`.

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
# Or from anywhere, using --config-file (skips walk, loads only this file):
fat-controller apply --config-file environments/production/fat-controller.toml
```

Note: `--config-file` loads a single file with no upward walk. For CI
pipelines that need the cascade, run from the environment directory
rather than using `--config-file`.

Best for: most projects. Keeps shared config DRY, with per-environment
differences clearly separated.

### Pattern 2: Self-contained files per environment

```text
environments/
  production/fat-controller.toml
  staging/fat-controller.toml
```

Each file is fully self-contained with all settings and service
definitions. No cascade — use `--config-file` to target a specific file.

```bash
fat-controller apply --config-file environments/production/fat-controller.toml
fat-controller apply --config-file environments/staging/fat-controller.toml
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
| Mutation | Preview + confirmation (default: yes) | Error unless `prompt = "none"` |
| Colors | Auto-detected | Off (unless `--color=always`) |

**Prompting mode** (`tool.prompt`) controls how aggressively the
tool prompts in interactive mode. CLI flags `--ask` and `--yes`
are shortcuts for `prompt = "all"` and `prompt = "none"`:

| Mode | Has a default | No default | Mutation |
|------|---------------|------------|----------|
| `all` (`--ask`) | Prompt, pre-filled with default | Prompt/picker | Confirm |
| `default` | Use default silently | Prompt/picker | Confirm |
| `none` (`--yes`) | Use default silently | Error if missing | Skip confirmation |

`all` is only valid in interactive mode — it errors on a
non-interactive terminal. `none` works in both modes.

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

Without `--ask`, if the config file sets the project name to
`"Life"` and the environment name to `"production"`, `show` uses
those silently — no prompts at all.

**Prompt mode and `--dry-run`:**

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
error. In config, `prompt` accepts only one value.

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
- When any command resolves a resource, it records the `id` in the
  config file as bookkeeping — pinning the match so subsequent
  operations are ID-based. This is not a merge direction; it's the
  same kind of side effect as `git commit` updating `HEAD`.

If a resource has an `id` but that ID doesn't exist in Railway,
the tool errors — the ID is stale. Use `adopt` to re-sync, or
remove the `id` to fall back to name matching.

### Merge direction

`apply` and `adopt` are symmetric — the same flags have parallel
meaning in opposite directions:

| Flag | `apply` (config → Railway) | `adopt` (Railway → config) |
|------|---------------------------|---------------------------|
| `--create` | Create in Railway what exists in config but not Railway | Add to config what exists in Railway but not config |
| `--update` | Update Railway to match config where both exist | Update config to match Railway where both exist |
| `--delete` | Delete from Railway what exists in Railway but not config | Remove from config what exists in config but not Railway |

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

Without `--delete`, individual resources can be marked for removal
with `delete = true`. This is a config-file directive, not Railway
state — apply removes the resource from Railway and then removes
the marker from the config file. Works at any level: a service, a
variable, a volume, a domain.

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
