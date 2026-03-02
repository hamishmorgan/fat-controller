# Fat Controller — Project Plan

## Scope and command structure

See [docs/COMMANDS.md](COMMANDS.md) — command groups (`auth`, `config`),
imperative CRUD (`get/set/delete`), declarative (`diff/apply/validate`),
dot-path addressing, full settings table, config examples, and
confirmation mode.

## Architecture

See [docs/CONFIG-FORMAT.md](CONFIG-FORMAT.md) — config file format, diff
semantics, multi-file merging, variable interpolation, secret handling,
and apply ordering. Also links to [SECRET-MASKING.md](SECRET-MASKING.md)
and [WARNINGS.md](WARNINGS.md).

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
│   └── config/                   # config get/set/delete/diff/apply/validate
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
├── .config/
│   └── genqlient.yaml
├── go.mod
├── docs/PLAN.md
└── README.md
```

## Decisions

See [docs/DECISIONS.md](DECISIONS.md) — design decision log covering
variable ownership, secret handling, deletion safety, error handling,
orchestration, CLI framework, and testing strategy.

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
