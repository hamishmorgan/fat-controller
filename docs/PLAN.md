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

Commands are grouped by **domain**, not by scope:

```
fat-controller auth login       # Browser-based OAuth login, stores account-level token
fat-controller auth logout      # Clear stored credentials
fat-controller auth status      # Show current auth state (who am I, what scope)

fat-controller config pull      # Show live config (pipe to file to bootstrap)
fat-controller config pull --full  # Show everything including IDs and read-only fields
fat-controller config diff      # Compare fat-controller.toml against live state
fat-controller config apply     # Push differences (dry-run by default, --confirm to execute)
```

Future command groups (not in scope for initial release):

```
fat-controller deploy list      # List deployments
fat-controller deploy trigger   # Trigger a redeploy
fat-controller service list     # List services in the project
fat-controller logs tail        # Stream logs
```

### Flags

- `--config <path>` — path to config file (default `fat-controller.toml`);
  repeatable to merge multiple files
- `--service <name>` — scope to a single service
- `--full` — show all fields including IDs and read-only (pull only)
- `--confirm` — actually execute mutations (apply only; without this, dry-run)
- `--skip-deploys` — don't trigger redeployments after variable changes
- `--fail-fast` — stop on first error during apply (default: continue)

## Architecture

### Live state, single config file

There is no state file. `diff` and `apply` always query Railway's API for
current live state. This means diffs are never stale, and secrets are
never written to disk.

The only file the user manages is `fat-controller.toml` — the desired
state. An optional `fat-controller.local.toml` (gitignored) provides
overrides for secrets or local values.

`pull` is an adoption/inspection tool: it outputs the current live config
in the same format as `fat-controller.toml`, so you can pipe it to a file
to bootstrap your config or inspect what's deployed.

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

Endpoint: `https://backboard.railway.com/graphql/v2`

### Authentication

The tool supports two authentication methods, resolved in order:

1. **Environment variable** — `RAILWAY_TOKEN` (or a `.env` file) can hold
   either a project access token or an account-level token. Intended for CI
   and non-interactive use.
2. **Stored OAuth credentials** — `fat-controller auth login` performs a
   browser-based OAuth 2.0 flow and persists the token locally.

**Project access tokens** use the `Project-Access-Token` header and
implicitly scope to one project + environment. The `projectToken` query
returns the project and environment IDs.

**Account-level tokens** (from OAuth or manually created in the dashboard)
use the `Authorization: Bearer` header and can access any resource the user
is authorized for. Commands that need a project/environment will require
`--project` and `--environment` flags (or a local context file, future).

#### OAuth 2.0 flow (auth login)

Railway exposes a full OAuth 2.0 + OIDC system:

- Authorization endpoint: `https://backboard.railway.com/oauth/auth`
- Token endpoint: `https://backboard.railway.com/oauth/token`
- Dynamic client registration: `POST https://backboard.railway.com/oauth/register`
- OIDC discovery: `https://backboard.railway.com/oauth/.well-known/openid-configuration`

The login flow:

1. Register as a native (public) client via dynamic registration if needed
   (one-time, client ID stored locally).
2. Start a local HTTP server on a random port for the callback.
3. Open the browser to the authorization endpoint with PKCE (`S256`),
   redirect URI `http://127.0.0.1:<port>/callback`.
4. Exchange the authorization code for an access token + refresh token.
5. Store tokens in OS keychain (primary) or fallback file (see
   "Configuration and storage" below).
6. Use the refresh token to renew the access token transparently (1hr TTL).

`auth logout` clears stored tokens from keychain and fallback file.
`auth status` calls the `me` query and displays the authenticated user +
available scopes.

### Queries for pull

All data needed for `pull` is available via GQL — no Railway CLI dependency.

| Query | Returns |
|-------|---------|
| `projectToken` | Project ID + environment ID from the token |
| `project(id).services` | All service names + IDs |
| `project(id).volumes` | All volumes |
| `variables(projectId, environmentId, unrendered: true)` | Shared variables (unrendered preserves `${{ref}}` syntax) |
| `variables(projectId, environmentId, serviceId, unrendered: true)` | Per-service variables |
| `serviceInstance(environmentId, serviceId)` | Service config + domains + `latestDeployment.meta` |
| `serviceInstanceLimitOverride(environmentId, serviceId)` | Resource limits (CPU, memory) |
| `tcpProxies(serviceId, environmentId)` | TCP proxy config |

**Important nuance**: `serviceInstance` returns `null` for fields set via
`railway.toml` (e.g. healthcheck, watch patterns). The *effective* merged
values are in `latestDeployment.meta.serviceManifest`. Pull uses the manifest
for the state snapshot.

### Mutations for apply

| Mutation | Input type | Purpose |
|----------|-----------|---------|
| `variableCollectionUpsert` | `VariableCollectionUpsertInput` | Atomically set all variables for a service or shared. Has `replace: bool` (true = delete vars not in the set) and `skipDeploys: bool`. |
| `serviceInstanceUpdate` | `ServiceInstanceUpdateInput` | Update deploy/build settings: builder, dockerfilePath, rootDirectory, region, numReplicas, healthcheckPath/Timeout, restartPolicy, startCommand, preDeployCommand, cronSchedule, sleepApplication, watchPatterns, etc. |
| `serviceInstanceLimitsUpdate` | `ServiceInstanceLimitsUpdateInput` | Update resource limits: `vCPUs: Float`, `memoryGB: Float`. |

### Reference implementation

The [community Terraform provider](https://github.com/terraform-community-providers/terraform-provider-railway)
uses `genqlient` against the same API. Its `internal/provider/` directory
contains per-resource `.graphql` files with exact queries, mutations, and
`genqlient` annotations.

## Technology

### Language: Go

- `go run github.com/hamishmorgan/fat-controller@latest` — zero-install
- Static binary via GoReleaser if distribution is needed later

### CLI framework: kong

[kong](https://github.com/alecthomas/kong) — struct-based CLI parser.
Commands and flags are defined as Go structs with tags. Cleaner than cobra
for nested subcommand groups, less boilerplate.

### GraphQL: genqlient

[genqlient](https://github.com/Khan/genqlient) generates typed Go functions
from `.graphql` operation files against the schema. Workflow:

1. Fetch schema via introspection -> `schema.graphql`
2. Write queries/mutations in `.graphql` files
3. `go generate` -> `generated.go` with typed functions and structs

### TOML: BurntSushi/toml

[BurntSushi/toml](https://github.com/BurntSushi/toml) — the standard Go TOML
library. Supports both encoding and decoding, preserves key order.

### Configuration: koanf

[koanf](https://github.com/knadh/koanf) — layered configuration library.
Modular (zero deps in core), case-sensitive keys, explicit merge order.
Replaces viper without the baggage (forced lowercase, global singleton,
massive dep tree).

### XDG directories: adrg/xdg

[adrg/xdg](https://github.com/adrg/xdg) — full XDG Base Directory spec
implementation. Cross-platform (Linux, macOS, Windows). Handles
`CONFIG_HOME`, `DATA_HOME`, `STATE_HOME`, `CACHE_HOME`, `RUNTIME_DIR`.

### Keyring: zalando/go-keyring

[go-keyring](https://github.com/zalando/go-keyring) — OS keychain access.
macOS Keychain, Linux Secret Service (GNOME Keyring / KWallet), Windows
Credential Manager. Pure Go, no CGo.

## Configuration and storage

### File locations (XDG-compliant via adrg/xdg)

| Path | Purpose | Example (Linux) |
|------|---------|-----------------|
| `$XDG_CONFIG_HOME/fat-controller/config.toml` | User preferences, defaults | `~/.config/fat-controller/config.toml` |
| `$XDG_CONFIG_HOME/fat-controller/auth.json` | Token fallback (mode 0600, used when keyring unavailable) | `~/.config/fat-controller/auth.json` |
| `$XDG_STATE_HOME/fat-controller/` | Logs, command history | `~/.local/state/fat-controller/` |
| `$XDG_CACHE_HOME/fat-controller/` | Cached schema, etc. | `~/.cache/fat-controller/` |
| `.fat-controller.toml` | Project-level config overrides (in working dir or git root) | `.fat-controller.toml` |

### Token storage

OAuth tokens are stored using a keyring-first strategy (same pattern as
the `gh` CLI):

1. **`RAILWAY_TOKEN` env var** — highest priority, for CI and non-interactive
   use. Can be a project access token or account-level token.
2. **OS keyring** — primary persistent storage. Encrypted at rest by the OS.
   Service name: `fat-controller`, key: hostname or user identifier.
3. **Fallback file** — `$XDG_CONFIG_HOME/fat-controller/auth.json` with mode
   0600. Used when no keyring daemon is available (headless, SSH, containers).
   A warning is printed when falling back to plaintext storage.

### Config precedence (lowest to highest)

1. Compiled-in defaults
2. User config — `$XDG_CONFIG_HOME/fat-controller/config.toml`
3. Project config — `.fat-controller.toml` in working directory or git root
4. Environment variables — `FAT_CONTROLLER_*`
5. CLI flags

Layering is handled by koanf: each level is loaded in order, later values
override earlier ones.

## Project structure

```
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

### M3: Pull

- Implement all pull queries (services, variables, instances, limits,
  volumes, domains, TCP proxies)
- Output live config to stdout in `fat-controller.toml` format (default)
- `--full` flag for verbose output including IDs and read-only fields
- No Railway CLI dependency, no state file

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

- Dry-run by default, `--confirm` to execute
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

Resolved during planning. Rationale preserved for future reference.

### Variable ownership: additive-only

Variables are additive-only by default. Only variables explicitly listed in
config are managed. Unmentioned variables are left alone — no implicit
deletion by omission. To delete a variable, set it to empty string:
`OLD_VAR = ""`. This eliminates the previous "section presence = ownership"
model and avoids accidental deletions.

### Secret handling: local env interpolation + multi-file

Secrets are handled through three complementary mechanisms: don't mention
them (unmanaged), use Railway references (`${{service.VAR}}`), or use
local env interpolation (`${VAR}`). The `${VAR}` syntax (single braces) is
deliberately distinct from Railway's `${{}}` (double braces) — Railway
chose double braces specifically to avoid shell variable collision.

Multi-file merging provides additional flexibility: a gitignored
`fat-controller.local.toml` is auto-discovered, and `--config` can be
repeated for explicit layering.

### Deletion safety: dry-run default is sufficient

With additive-only semantics, deletions are always explicit (`KEY = ""`).
The dry-run default on apply plus prominent diff output (showing "DELETE"
clearly) provides sufficient safety without extra flags.

### Volumes, domains, TCP proxies: pull-only for now

These are included in the state file for visibility but are not manageable
via config in the initial release. The focus is on the variable/settings
gap. Management can be added in a future milestone — the additive-only
model makes it safe when we do.

### Shared variables: same semantics as per-service

Shared variables follow the same additive-only rules. The API call is the
same mutation (`variableCollectionUpsert`) without a `serviceId`. Railway
handles precedence: per-service overrides shared when both define the
same key.

### Error handling: continue by default, --fail-fast option

Apply is best-effort and non-transactional. By default, a failure on one
service does not stop processing of remaining services. `--fail-fast` stops
on first error. A summary reports what was applied and what failed. Exit
code is non-zero if anything failed.

### Orchestration: thick cmd/ layer

Command handlers in `cmd/` directly call `internal/` packages. No separate
engine or orchestration package. Extract if complexity warrants it later.

### CLI framework: kong

[kong](https://github.com/alecthomas/kong) for struct-based CLI parsing.
Less boilerplate than cobra for nested subcommand groups.

### Testing strategy

- Unit tests for pure logic: diff, config parsing, interpolation, PKCE
- HTTP mock tests (`httptest.NewServer`) for OAuth and GraphQL
- Keyring mock tests (`go-keyring MockInit()`) for token storage
- Golden file tests for diff output formatting
- No live Railway integration tests in CI
