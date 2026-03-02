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

```
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

```
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

Variables are automatically masked in output when they appear to contain
secrets. Detection uses two layers: **name-based** and **entropy-based**.

#### Layer 1: Name-based detection

Variable names are matched case-insensitively using **substring matching**
against a keyword list, plus a false-positive **allowlist** to suppress
known non-secret matches. This is the same approach used by gitleaks and
Yelp's detect-secrets.

**Default sensitive keywords** (case-insensitive substring match):

```
# Passwords & passphrases
PASSWORD, PASSWD, PASS, PWD

# Secrets & keys
SECRET, PRIVATE_KEY, SIGNING_KEY, ENCRYPTION_KEY, MASTER_KEY,
DEPLOY_KEY, KEY

# API & access credentials
API_KEY, APIKEY, API_SECRET, ACCESS_KEY, AUTH_TOKEN, AUTH_KEY,
CLIENT_SECRET, SERVICE_KEY, ACCOUNT_KEY

# Tokens
TOKEN

# Credentials
CREDENTIAL, CREDS, AUTH

# Certificates
CERT, PEM, PFX, KEYSTORE, STOREPASS

# Cryptographic material
HMAC, SALT, PEPPER, NONCE, SEED, CIPHER

# Connection strings (often embed credentials)
CONNECTION_STRING, DATABASE_URL, REDIS_URL, MONGODB_URI,
MYSQL_URL, POSTGRES_URL, DSN

# Webhooks & sessions
WEBHOOK_SECRET, WEBHOOK_URL, SESSION_SECRET, COOKIE_SECRET,
JWT_SECRET
```

**Default false-positive allowlist** (suppress matches on these):

```
# KEY false positives
KEYBOARD, KEYSTROKE, KEYFRAME, KEYSTONE, KEYPRESS, KEYWORD,
MONKEY, DONKEY, TURKEY, PRIMARY_KEY, FOREIGN_KEY, SORT_KEY,
PARTITION_KEY, PUBLIC_KEY, KEY_ID, KEY_NAME, KEY_FILE,
KEY_LENGTH, KEY_SIZE, KEY_TYPE, KEY_FORMAT, KEY_VAULT_NAME

# AUTH false positives
AUTHOR, AUTHORITY, AUTHORIZE, AUTHENTICATION_RESULTS

# PASS false positives
PASSENGER, PASSIVE, COMPASS, BYPASS, PASSPORT_STRATEGY

# TOKEN false positives
TOKEN_URL, TOKEN_ENDPOINT, TOKEN_FILE, TOKEN_TYPE, TOKEN_EXPIRY

# CREDENTIAL false positives
CREDENTIAL_ID, CREDENTIALS_URL, CREDENTIALS_ENDPOINT

# SECRET false positives
SECRET_NAME, SECRET_LENGTH, SECRET_VERSION

# SEED false positives
SEED_DATA, SEED_FILE
```

Both lists are configurable. Setting `sensitive_keywords` or
`sensitive_allowlist` in config replaces the respective defaults.

#### Layer 2: Entropy-based detection

Values that pass name-based checks are tested for high Shannon entropy,
which indicates random/generated strings typical of API keys and tokens.
Uses the same thresholds as truffleHog and Yelp's detect-secrets:

| Charset | Characters | Threshold | Min length |
|---------|-----------|-----------|------------|
| Base64 | `A-Za-z0-9+/=` | > 4.5 bits/char | 20 chars |
| Hex | `0-9a-fA-F` | > 3.0 bits/char | 20 chars |

The Shannon entropy formula: `H = -Σ p(x) * log₂(p(x))` where `p(x)` is
the frequency of character `x` in the string. Random strings approach the
theoretical maximum for their charset; structured strings (English words,
URLs, paths) score much lower.

This catches secrets with non-obvious names like
`SETTING_X = "sk_live_4eC39HqL..."` that name-based detection would miss.

#### Combined masking logic

1. The tool always fetches the unrendered value from Railway (needed to
   detect `${{}}` references and compute diffs correctly).
2. If the value contains `${{` — it's a Railway reference template.
   **Show as-is** regardless of name or entropy.
3. If the name matches a sensitive pattern — **mask as `********`**.
4. If the value has high entropy (base64 > 4.5 or hex > 3.0, min 20
   chars) — **mask as `********`**.
5. Otherwise — **show**.
6. `--show-secrets` overrides all masking.

**Examples:**

```
DATABASE_PASSWORD = "********"              # masked (name matches PASSWORD)
DATABASE_URL = "${{postgres.DATABASE_URL}}" # shown (reference template)
APP_ENV = "production"                      # shown (no name match, low entropy)
SETTING_X = "********"                      # masked (high entropy value)
BUILD_HASH = "abc123"                       # shown (too short for entropy check)
```

**Custom keywords and allowlist** (each replaces its defaults entirely):

```toml
# .fat-controller.toml
sensitive_keywords = ["SECRET", "TOKEN", "PASSWORD", "MY_CUSTOM_FIELD"]
sensitive_allowlist = ["KEYSTROKE", "MY_SAFE_TOKEN_NAME"]
```

### Config validation and warnings

When loading `fat-controller.toml` (and `fat-controller.local.toml`), the
tool runs a series of checks and emits warnings to stderr. These are
advisory — they never block execution. Warnings appear on `diff`, `apply`,
and `config validate` (a dedicated command for checking config without
hitting the API).

#### Structural warnings

| Code | Warning | When |
|------|---------|------|
| `W001` | Unknown top-level key | Key is not `shared`, `services`, or a known setting (catches typos like `servics`) |
| `W002` | Unknown key in service block | Key inside `[services.X]` is not `variables` or a recognized service setting |
| `W003` | Empty service block | `[services.X]` exists but defines no variables or settings |

#### Variable value warnings

| Code | Warning | When |
|------|---------|------|
| `W010` | Unresolved local interpolation | `${VAR}` where `VAR` is not set in the local environment |
| `W011` | Suspicious reference syntax | `${service.X}` looks like it was meant to be `${{service.X}}` (single vs double braces) |
| `W012` | Empty string is explicit delete | `VAR = ""` — reminder that this will delete the variable in Railway |

#### Duplicate / conflict warnings

| Code | Warning | When |
|------|---------|------|
| `W020` | Variable in both shared and service | Variable appears in `[shared]` and `[services.X]` — service value wins |
| `W021` | Variable overridden by local file | Same variable defined in both `fat-controller.toml` and `fat-controller.local.toml` — local wins |

#### Naming warnings

| Code | Warning | When |
|------|---------|------|
| `W030` | Lowercase variable name | Variable name contains lowercase letters (convention is `UPPER_SNAKE_CASE`) |
| `W031` | Invalid variable name characters | Variable name contains spaces or characters that may not work in Railway |

#### Scope warnings

| Code | Warning | When |
|------|---------|------|
| `W040` | Unknown service name | Config references a service that doesn't exist in Railway (checked at `diff`/`apply` time) |
| `W041` | No services or shared variables | Config file exists but defines nothing actionable |

#### Secret hygiene warnings

| Code | Warning | When |
|------|---------|------|
| `W050` | Hardcoded secret in config | A value matches the secret detection heuristics (name + entropy) and is not using `${VAR}` interpolation — likely a plaintext secret in a committed file |
| `W051` | Local override file not gitignored | `fat-controller.local.toml` exists but is not in `.gitignore` |

#### Reference warnings

| Code | Warning | When |
|------|---------|------|
| `W060` | Reference to unknown service | `${{service.VAR}}` references a service name not defined in the config (may still be valid if the service exists in Railway but isn't managed) |

#### Controlling warnings

Warnings can be suppressed per-code:

```toml
# fat-controller.toml or .fat-controller.toml
suppress_warnings = ["W012", "W030"]   # suppress specific warnings
```

The `--quiet` flag suppresses all warnings. `config validate` ignores
`--quiet` (its whole purpose is to show warnings).

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

1. **`--token` flag or env vars** — highest priority. `RAILWAY_TOKEN`
   (project-scoped) or `RAILWAY_API_TOKEN` (account/workspace-scoped).
2. **OS keyring** — primary persistent storage. Encrypted at rest by the OS.
   Service name: `fat-controller`, key: hostname or user identifier.
3. **Fallback file** — `$XDG_CONFIG_HOME/fat-controller/auth.json` with mode
   0600. Used when no keyring daemon is available (headless, SSH, containers).
   A warning is printed when falling back to plaintext storage.

### Config loading

All settings (see "Settings" table above) can be specified at five
levels. Layering is handled by koanf — each level is loaded in order,
later values override earlier ones:

1. Compiled-in defaults
2. Global config — `$XDG_CONFIG_HOME/fat-controller/config.toml`
3. Local config — `.fat-controller.toml` in working directory or git root
4. Environment variables — `FAT_CONTROLLER_*` / `RAILWAY_TOKEN` / `RAILWAY_API_TOKEN`
5. CLI flags

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
