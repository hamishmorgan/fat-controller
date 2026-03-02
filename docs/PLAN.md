# Fat Controller — Project Plan

A CLI for managing [Railway](https://railway.com) projects. The initial
focus is declarative configuration management — pull live state, diff
against a desired-state config, apply the difference — but the tool is
designed to grow into a comprehensive Railway CLI over time.

## Motivation

Railway's `railway.toml` files cover build and deploy settings (Dockerfile
path, watch patterns, healthchecks, restart policy), but a large portion of
project configuration lives only in the dashboard: environment variables,
resource limits, regions, replicas, domains, volumes, and TCP proxies. For
multi-service projects this means:

- No version control or audit trail for env var changes
- No way to review configuration changes in a PR
- Manual, error-prone setup when recreating or cloning a project
- No mechanism to detect configuration drift

Fat Controller treats Railway project configuration as code: pull the live
state, declare the desired state in a TOML file, diff, and apply.

## Scope and command structure

Railway has five scope levels: **user > workspace > project > environment >
service**. A user account can access multiple workspaces, each containing
multiple projects. Rather than encoding these as nested subcommands, scope
is determined by context:

1. **Auth token** — a project access token implicitly sets project +
   environment (narrowest). An account-level token could access any
   workspace/project the user belongs to (broadest).
2. **Flags** — `--service <name>` narrows to a single service.
3. **Future**: a local context file, workspace-level auth, or account-level
   auth could broaden scope.

The default is as broad as the auth allows. `config pull` fetches all
services in the project+environment; `--service` narrows when needed.

Commands are grouped by **domain**, not by scope. There are two
interaction modes: **imperative** (one-off CRUD against live Railway)
and **declarative** (config-file-driven diff and apply).

```sh
fat-controller auth login       # Browser-based OAuth login
fat-controller auth logout      # Clear stored credentials
fat-controller auth status      # Show current auth state

# Imperative — read/write live Railway directly
fat-controller config get                         # all config (pipe to file to bootstrap)
fat-controller config get api.variables           # all variables for a service
fat-controller config get api.variables.PORT      # one specific value
fat-controller config get --full                  # everything including IDs and read-only fields
fat-controller config set api.variables.PORT 8080 # set a value
fat-controller config delete api.variables.OLD    # delete a value

# Declarative — config file driven
fat-controller config diff      # compare fat-controller.toml against live state
fat-controller config apply     # push differences from config file
fat-controller config validate  # check config for warnings (no API calls)
```

Dot-path addressing (`service.section.key`) is used universally: in
`get/set/delete` arguments, in `--service` scoping for diff/apply, and
in config file section headers.

Future command groups (not in scope for initial release):

```sh
fat-controller deploy list      # List deployments
fat-controller deploy trigger   # Trigger a redeploy
fat-controller service list     # List services in the project
fat-controller logs tail        # Stream logs
```

### Settings

Every setting can be specified at up to five levels. Higher levels
override lower ones:

1. **Compiled-in defaults** (lowest)
2. **Global config** — `$XDG_CONFIG_HOME/fat-controller/config.toml`
3. **Local config** — `.fat-controller.toml` in working dir or git root
4. **Environment variable**
5. **CLI flag** (highest)

The full settings table:

| Setting | CLI flag | Env var | Config key | Default | Description |
|---------|----------|---------|------------|---------|-------------|
| Token | `--token` | `RAILWAY_TOKEN` / `RAILWAY_API_TOKEN` | — | — | Auth token. `RAILWAY_TOKEN` = project-scoped. `RAILWAY_API_TOKEN` = account/workspace. |
| Project | `--project` | `FAT_CONTROLLER_PROJECT` | `project` | — | Project ID or name. Required with account-level tokens. |
| Environment | `--environment` | `FAT_CONTROLLER_ENVIRONMENT` | `environment` | — | Environment name. Required with account-level tokens. |
| Output format | `--output`, `-o` | `FAT_CONTROLLER_OUTPUT` | `output` | `text` | Output format: `text`, `json`, `toml`. |
| Color | `--color` | `FAT_CONTROLLER_COLOR` | `color` | `auto` | Color: `auto`, `always`, `never`. Respects `NO_COLOR`. |
| Timeout | `--timeout` | `FAT_CONTROLLER_TIMEOUT` | `timeout` | `30s` | API request timeout. |
| Confirm | `--confirm` | `FAT_CONTROLLER_CONFIRM` | `confirm` | `false` | Auto-execute mutations (dangerous mode). |
| Dry run | `--dry-run` | `FAT_CONTROLLER_DRY_RUN` | `dry_run` | `false` | Force preview of mutations. |
| Config file | `--config` | `FAT_CONTROLLER_CONFIG` | `config` | `fat-controller.toml` | Railway config file path. Repeatable. |
| Service | `--service` | `FAT_CONTROLLER_SERVICE` | `service` | — | Scope to a single service. |
| Skip deploys | `--skip-deploys` | `FAT_CONTROLLER_SKIP_DEPLOYS` | `skip_deploys` | `false` | Don't trigger redeployments. |
| Fail fast | `--fail-fast` | `FAT_CONTROLLER_FAIL_FAST` | `fail_fast` | `false` | Stop on first error during apply. |
| Show secrets | `--show-secrets` | `FAT_CONTROLLER_SHOW_SECRETS` | `show_secrets` | `false` | Show secret values instead of masking them. |
| Sensitive keywords | — | — | `sensitive_keywords` | *(see below)* | Keywords for detecting sensitive variable names (substring match). |
| Sensitive allowlist | — | — | `sensitive_allowlist` | *(see below)* | Keywords that suppress false-positive secret matches. |
| Suppress warnings | — | — | `suppress_warnings` | `[]` | List of warning codes to suppress (e.g. `["W012", "W030"]`). |
| Full output | `--full` | — | — | `false` | Include IDs and read-only fields (get only). |
| Verbose | `--verbose`, `-v` | — | — | `false` | Debug output (HTTP requests, timing). |
| Quiet | `--quiet`, `-q` | — | — | `false` | Suppress informational output. |

**Token precedence:** `--token` flag > `RAILWAY_API_TOKEN` env var >
`RAILWAY_TOKEN` env var > stored OAuth credentials (keyring/file).
`RAILWAY_TOKEN` uses the `Project-Access-Token` header (project-scoped).
`RAILWAY_API_TOKEN` uses `Authorization: Bearer` (account/workspace-scoped).

#### Example: global config file

```toml
# ~/.config/fat-controller/config.toml
# User-wide defaults

output = "json"          # prefer JSON output everywhere
color = "auto"
timeout = "60s"
```

#### Example: local config file

```toml
# .fat-controller.toml (in project root, committed)
# Project-specific settings

project = "my-railway-project"
environment = "production"
config = "infra/fat-controller.toml"    # non-default config location
skip_deploys = true                      # batch changes, deploy separately
sensitive_keywords = ["SECRET", "TOKEN", "PASSWORD", "KEY",
  "SIGNING"]                             # replaces all defaults
sensitive_allowlist = ["KEYSTROKE"]       # replaces all defaults
```

### Confirmation mode

All mutations (`set`, `delete`, `apply`) respect the `confirm` setting:

- **Safe mode (default, `confirm = false`):** mutations are dry-run.
  Pass `--confirm` to execute.
- **Dangerous mode (`confirm = true`):** mutations execute immediately.
  Pass `--dry-run` to preview.

This can be set at any level: global config, local config, env var
(`FAT_CONTROLLER_CONFIRM=true`), or CLI flag. Flags always win.

`NO_COLOR` (any value) disables color output regardless of `--color`.

## Architecture

### Live state, single config file

There is no state file. `diff` and `apply` always query Railway's API for
current live state. This means diffs are never stale, and secrets are
never written to disk.

The only file the user manages is `fat-controller.toml` — the desired
state. An optional `fat-controller.local.toml` (gitignored) provides
overrides for secrets or local values.

`config get` with no arguments outputs the full live config in
`fat-controller.toml` format — pipe it to a file to bootstrap your
config, or inspect what's deployed.

Service names in the config are resolved to Railway IDs via the live API
at diff/apply time.

### Config file format

`fat-controller.toml` contains only mutable fields. Read-only fields
(IDs, `current_size_mb`, deployment metadata) are silently ignored if
present.

```toml
[shared_variables]
SHARED_SECRET = "some-value"

[api.variables]
APP_ENV = "production"
DATABASE_URL = "postgresql://${{postgres.PGUSER}}:${{postgres.PGPASSWORD}}@${{postgres.PGHOST}}:5432/${{postgres.PGDATABASE}}"
REDIS_URL = "${{redis.REDIS_URL}}"
STRIPE_KEY = "${STRIPE_KEY}"    # resolved from local environment at apply time
PORT = "8080"
OLD_VAR = ""                    # explicit delete

[api.resources]
vcpus = 2
memory_gb = 4

[worker.variables]
DATABASE_URL = "${{api.DATABASE_URL}}"
QUEUE_NAME = "default"
```

### Diff semantics

Variables are **additive-only** by default. Only variables explicitly
mentioned in config are diffed — unmentioned variables are left alone.

| Situation | Behaviour |
|-----------|-----------|
| Key in config with value, not in state | **Create** |
| Key in both, different value | **Update** |
| Key in both, same value | **No-op** |
| Key in config with empty string `""` | **Delete** |
| Key in state, not in config | **Ignore** |
| Read-only field in config | **Ignore** |

For settings (resources, deploy config), the same principle applies: only
explicitly specified fields are diffed — omitted fields are never zeroed
out.

Shared variables (`[shared_variables]`) follow the same rules as
per-service variables. Railway's own precedence applies: per-service
overrides shared when both define the same key.

### Multi-file config

Multiple config files are merged in order (later values override earlier):

- `fat-controller.toml` — base config (committed)
- `fat-controller.local.toml` — auto-discovered if present (gitignored,
  for local overrides and secrets)
- Additional files via `--config` flags

```bash
fat-controller config diff
fat-controller config diff --config base.toml --config overrides.toml
```

### Variable interpolation

Two interpolation syntaxes in config values:

- `${{service.VAR}}` — **Railway reference**. Passed through as-is.
  Railway resolves at runtime. Safe to commit.
- `${VAR}` — **Local environment variable**. Resolved at apply time from
  the local shell environment. Missing env var = error. Useful for secrets
  in CI.

### Secret handling

With additive-only semantics, secrets that aren't in the config are simply
ignored. Three patterns for managing secrets:

1. **Don't mention them** — set in the dashboard, untouched by this tool.
   Works because unmentioned = ignored.
2. **Railway references** — `DATABASE_URL = "${{postgres.DATABASE_URL}}"`.
   Safe to commit. Railway resolves at runtime.
3. **Local env interpolation** — `STRIPE_KEY = "${STRIPE_KEY}"`. Resolved
   from local environment at apply time. Config file is safe to commit;
   actual value comes from CI env vars or a `.env` file.

### Secret masking

See [docs/SECRET-MASKING.md](SECRET-MASKING.md) for the full specification
including keyword lists, allowlist, entropy thresholds, and combined
masking logic.

In brief: two-layer detection (name-based keyword regex + Shannon entropy),
masked by default, `--show-secrets` to override, `${{}}` references always
shown. Keywords and allowlist are configurable via `sensitive_keywords` and
`sensitive_allowlist` in config.

### Config validation and warnings

See [docs/WARNINGS.md](WARNINGS.md) for the full list of 15 warning codes
(W001–W060) across 7 categories: structural, variable values,
duplicates/conflicts, naming, scope, secret hygiene, and references.

All warnings are advisory and never block execution. They appear on
`diff`, `apply`, and `config validate`. Suppress specific codes via
`suppress_warnings` in config, or all warnings via `--quiet`.

### Apply ordering and redeployment

- `variableCollectionUpsert` triggers a redeployment by default. The
  `--skip-deploys` flag passes `skipDeploys: true` to defer redeployment.
- When applying both variables and settings, settings are applied first
  (via `serviceInstanceUpdate`), then variables. This way the triggered
  redeploy picks up both changes.
- Shared variables are applied first, then services in alphabetical order.
- Apply is **best-effort, non-transactional**. By default, a failure on
  one service does not stop processing of remaining services. Use
  `--fail-fast` to stop on first error. On completion, a summary reports
  what was applied and what failed. Exit code is non-zero if any service
  failed.

## Railway GraphQL API

See [docs/RAILWAY-API.md](RAILWAY-API.md) for the full API reference
including authentication methods, OAuth 2.0 flow, queries, mutations,
and the Terraform provider as a reference implementation.

In brief: GraphQL at `https://backboard.railway.com/graphql/v2`. Two
token types (`RAILWAY_TOKEN` project-scoped, `RAILWAY_API_TOKEN`
account-scoped). OAuth 2.0 + PKCE for interactive login. All pull data
via queries, apply via `variableCollectionUpsert`,
`serviceInstanceUpdate`, and `serviceInstanceLimitsUpdate`.

## Technology

See [docs/TECHNOLOGY.md](TECHNOLOGY.md) — Go, kong, genqlient, koanf,
BurntSushi/toml, adrg/xdg, go-keyring.

## Configuration and storage

See [docs/CONFIGURATION.md](CONFIGURATION.md) — XDG file locations,
keyring-first token storage, 5-level config loading (defaults → global →
local → env → flags).

## Project structure

```text
fat-controller/
├── main.go                       # Entry point, kong CLI dispatch
├── cmd/                          # Command handlers (thick — orchestration lives here)
│   ├── auth/                     # auth login/logout/status
│   └── config/                   # config pull/diff/apply
├── internal/
│   ├── auth/                     # OAuth flow, keyring, token resolution
│   ├── config/                   # TOML config/state types, parsing, interpolation
│   ├── diff/                     # Diffing logic + display
│   ├── platform/                 # XDG paths, layered config loading (koanf)
│   └── railway/                  # GQL client
│       ├── schema.graphql        # Introspected schema (checked in)
│       ├── operations.graphql    # Queries + mutations
│       ├── generated.go          # genqlient output (checked in)
│       └── client.go             # Client setup, auth header resolution
├── genqlient.yaml
├── go.mod
├── docs/PLAN.md
└── README.md
```

## Milestones

### M1: Scaffold + auth

- `go mod init`, CLI framework setup
- `auth login` — OAuth 2.0 + PKCE browser flow, token storage
- `auth logout` — clear stored credentials
- `auth status` — display current user via `me` query
- Support `RAILWAY_TOKEN` env var for project access tokens
- Auth header resolution: env var takes precedence, then stored OAuth token

### M2: Schema + GQL client

- Fetch Railway schema via introspection
- genqlient setup, wire up GQL client
- Verify basic query (`projectToken`) works

### M3: Imperative CRUD (get/set/delete)

- `config get` — fetch live config (all, by service, by section, by key)
- `config get --full` — verbose output with IDs and read-only fields
- `config set` — set a single value by dot-path
- `config delete` — delete a single value by dot-path
- Confirmation mode for set/delete (safe mode default)
- No state file — all operations query/mutate live Railway

### M4: Diff

- Define config types (subset of state types)
- Multi-file config loading: auto-discover `fat-controller.local.toml`,
  support repeatable `--config` flag
- `${VAR}` local env interpolation (resolve before diffing)
- Fetch live state from Railway API, diff against config (additive-only)
- Display: coloured terminal output, grouped by service
- `--service` flag to scope to a single service
- Config validation: warn on unknown keys / typos

### M5: Apply

- Confirmation mode (safe mode default, configurable)
- `variableCollectionUpsert` for variables (shared + per-service)
- `serviceInstanceUpdate` for deploy/build settings
- `serviceInstanceLimitsUpdate` for resource limits
- `--skip-deploys` flag
- `--fail-fast` flag (default: continue on failure, report summary)
- Apply settings before variables (so one redeploy catches both)

### Future

- Volume, domain, and TCP proxy management in config
- GoReleaser for prebuilt binaries
- `init` command to interactively bootstrap `fat-controller.toml`

## Decisions

See [docs/DECISIONS.md](DECISIONS.md) — design decision log covering
variable ownership, secret handling, deletion safety, error handling,
orchestration, CLI framework, and testing strategy.
