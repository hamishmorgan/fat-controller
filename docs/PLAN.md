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

Railway has four scope levels: **workspace > project > environment >
service**. Rather than encoding these as nested subcommands, scope is
determined by context:

1. **Auth token** — a project access token implicitly sets project +
   environment.
2. **Flags** — `--service <name>` narrows to a single service.
3. **Future**: a local context file or workspace-level auth could broaden
   scope.

The default is as broad as the auth allows. `config pull` fetches all
services in the project+environment; `--service` narrows when needed.

Commands are grouped by **domain**, not by scope:

```
fat-controller config pull      # Fetch live state -> railway-state.toml
fat-controller config diff      # Compare railway-config.toml against live state
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

## Open Questions

### Secret handling gap

The current options for managing secrets are limited. Option 1 (omit from
config) means you can't manage *any* variables for a service if some are
secrets — it's all-or-nothing at the section level. Option 3 (secrets file)
is punted to Future.

Consider supporting environment variable interpolation in config values
(e.g. `SECRET_KEY = "${SECRET_KEY}"` or a `$env{}` syntax distinct from
Railway's `${{}}` references). This would let you commit the config with
placeholders and resolve from the local environment at apply time.

### Deletion safety

"Key in state, not in config, but section exists in config → Delete" is
correct but dangerous. A typo or accidental removal of a line deletes a
production variable. The dry-run default helps, but consider:

- Printing deletions prominently with explicit "WILL DELETE" language
- An `--allow-deletes` flag or confirmation prompt specifically for deletions
- Requiring an explicit deletion marker rather than implicit deletion by
  omission

### Volumes, domains, and TCP proxies in apply

The pull queries include volumes, domains, and TCP proxies, but the
mutations section only covers variables, service instance settings, and
resource limits. Are volumes/domains/TCP proxies read-only in this tool, or
is that a gap? Should be explicitly stated either way.

### Shared variable semantics

The config example shows `[shared_variables]` but the diff semantics don't
address:

- Does `variableCollectionUpsert` work differently for shared variables
  vs per-service?
- If a shared variable and a per-service variable collide, what wins?
- Does the "section presence = ownership" deletion rule apply to
  `[shared_variables]` too?

### Error handling and partial apply

What happens when apply succeeds for service A but fails for service B?
The plan says "mutations are applied one service at a time" but doesn't
address rollback, whether to continue or stop on first error, or what to
report. Suggested stance: "Apply is best-effort, non-transactional. On
failure, report what was applied and what failed, then exit non-zero."

### Apply logic needs a package

The project structure has `internal/config/`, `internal/railway/`, and
`internal/diff/`, but apply orchestration (read config, run diff, execute
mutations) doesn't have a home. Neither `diff/` nor `railway/` is a clean
fit. Consider `internal/apply/` or `internal/engine/`.

### CLI framework

No CLI framework is mentioned. For three subcommands with shared flags,
something like `cobra` or `kong` would reduce boilerplate. Worth deciding
up front.

### Testing strategy

No testing approach is described. Even a brief note would help — e.g.
unit tests for diff logic, integration tests with recorded GQL responses.

### Milestone scoping

- `--service` flag is listed in Flags but punted to Future. It's simple
  to implement and useful immediately — consider pulling into M3/M4.
- Config validation (warn on unknown keys, typos like `varaibles`) is
  listed as Future but is a meaningful footgun. Consider pulling into M3.
