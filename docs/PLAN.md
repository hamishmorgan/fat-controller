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

See [docs/COMMANDS.md](COMMANDS.md) — command groups (`auth`, `config`),
imperative CRUD (`get/set/delete`), declarative (`diff/apply/validate`),
dot-path addressing, full settings table, config examples, and
confirmation mode.

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
