# Fat Controller — Project Plan

Declarative configuration management for [Railway](https://railway.com)
projects. Pull live state from Railway's GraphQL API, diff against a local
desired-state config, apply the difference back.

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

## Commands

```
fat-controller pull     # Fetch live state -> railway-state.toml
fat-controller diff     # Compare railway-config.toml against live state
fat-controller apply    # Push differences (dry-run by default, --confirm to execute)
```

### Flags

- `--config <path>` — path to config file (default `railway-config.toml`)
- `--state <path>` — path to state file (default `railway-state.toml`)
- `--service <name>` — scope to a single service
- `--confirm` — actually execute mutations (apply only; without this, dry-run)
- `--skip-deploys` — don't trigger redeployments after variable changes

## Architecture

### Two-file model

| File | Purpose | Committed? |
|------|---------|------------|
| `railway-state.toml` | Full snapshot of live state (read-only, contains secrets + IDs) | No (gitignored) |
| `railway-config.toml` | Declarative desired state (mutable fields only) | Yes (if using `${{references}}` for secrets) |

The tool resolves service names to IDs via the pulled state at diff/apply time.

### Config file format

`railway-config.toml` is a **subset** of the state file format. It contains
only mutable fields. Read-only fields (IDs, `current_size_mb`, deployment
metadata) are silently ignored if present.

```toml
[shared_variables]
SHARED_SECRET = "some-value"

[api.variables]
APP_ENV = "production"
DATABASE_URL = "postgresql://${{postgres.PGUSER}}:${{postgres.PGPASSWORD}}@${{postgres.PGHOST}}:5432/${{postgres.PGDATABASE}}"
REDIS_URL = "${{redis.REDIS_URL}}"
PORT = "8080"

[api.resources]
vcpus = 2
memory_gb = 4

[worker.variables]
DATABASE_URL = "${{api.DATABASE_URL}}"
QUEUE_NAME = "default"
```

### Diff semantics

Comparing config (desired) against state (live):

| Situation | Behaviour |
|-----------|-----------|
| Key in config, not in state | **Create** |
| Key in both, different value | **Update** |
| Key in state, not in config, but section exists in config | **Delete** (variables only) |
| Key in state, not in config, section absent from config | **Ignore** (unmanaged service) |
| Read-only field in config | **Ignore** |

The "section presence = ownership" rule applies **only to variables**. For
settings (resources, deploy config), only explicitly specified fields are
diffed — omitted fields are never zeroed out.

This means you can manage a subset of services. If a service has no section
in `railway-config.toml`, it is entirely unmanaged.

### Secret handling

Variables using Railway reference syntax (`${{service.VAR}}`) are safe to
commit — Railway resolves them at runtime. Raw secrets can be handled three
ways:

1. **Omit from config** — if a `[service.variables]` section exists, omitted
   keys are deletion candidates. To manage *some* vars while leaving secrets
   untouched, don't include the `[service.variables]` section and manage that
   service's variables entirely in the dashboard.
2. **Include in config** — acceptable for non-sensitive values or if the config
   file is gitignored.
3. **Future: separate secrets file** — a gitignored overrides file merged at
   apply time.

### Apply ordering and redeployment

- `variableCollectionUpsert` triggers a redeployment by default. The
  `--skip-deploys` flag passes `skipDeploys: true` to defer redeployment.
- When applying both variables and settings, settings are applied first
  (via `serviceInstanceUpdate`), then variables. This way the triggered
  redeploy picks up both changes.
- Mutations are applied one service at a time.

## Railway GraphQL API

Endpoint: `https://backboard.railway.com/graphql/v2`

### Authentication

Uses a **project access token** with the `Project-Access-Token` header (not
`Authorization: Bearer`). This is a per-project token, not an account token.

The token implicitly scopes all operations to one project + environment.
The `projectToken` query returns the project and environment IDs.

Token is read from `RAILWAY_TOKEN` env var or a `.env` file.

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

### GraphQL: genqlient

[genqlient](https://github.com/Khan/genqlient) generates typed Go functions
from `.graphql` operation files against the schema. Workflow:

1. Fetch schema via introspection -> `schema.graphql`
2. Write queries/mutations in `.graphql` files
3. `go generate` -> `generated.go` with typed functions and structs

### TOML: BurntSushi/toml

[BurntSushi/toml](https://github.com/BurntSushi/toml) — the standard Go TOML
library. Supports both encoding and decoding, preserves key order.

## Project structure

```
fat-controller/
├── main.go                       # Entry point, command dispatch
├── internal/
│   ├── config/                   # TOML config/state types + parsing
│   ├── railway/                  # GQL client
│   │   ├── schema.graphql        # Introspected schema (checked in)
│   │   ├── operations.graphql    # Queries + mutations
│   │   ├── generated.go          # genqlient output (checked in)
│   │   └── client.go             # Client setup, auth
│   └── diff/                     # Diffing logic + display
├── genqlient.yaml
├── go.mod
├── docs/PLAN.md
└── README.md
```

## Milestones

### M1: Scaffold + schema

- `go mod init`, genqlient setup
- Fetch Railway schema via introspection
- Wire up GQL client with project-access-token auth
- Verify basic query (`projectToken`) works

### M2: Pull

- Implement all pull queries (services, variables, instances, limits, etc.)
- Generate `railway-state.toml`
- No Railway CLI dependency

### M3: Diff

- Define config types (subset of state types)
- Parse both files, compute differences
- Display: coloured terminal output, grouped by service
- Handle variable ownership semantics (section present = managed)

### M4: Apply

- Dry-run by default, `--confirm` to execute
- `variableCollectionUpsert` for variables (shared + per-service)
- `serviceInstanceUpdate` for deploy/build settings
- `serviceInstanceLimitsUpdate` for resource limits
- `--skip-deploys` flag
- Apply settings before variables (so one redeploy catches both)

### Future

- Config validation (warn on unknown keys, type mismatches)
- `--service` flag to scope operations
- GoReleaser for prebuilt binaries
- Secrets file support (gitignored, merged at apply time)
- `init` command to bootstrap `railway-config.toml` from current state
