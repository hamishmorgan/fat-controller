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
   (`init`, `diff`, `apply`, `validate`) converge toward desired state.
   Imperative commands (`deploy`, `restart`, `logs`) perform actions on a
   running system. They share context resolution (project/env/service)
   but not mechanics.

3. **Apply creates everything.** If the config declares a service,
   environment, volume, or domain that doesn't exist in Railway,
   `apply` creates it. The config file is the source of truth and
   `apply` converges reality toward it.

4. **Additive by default, opt-in ownership.** Unmentioned entities are
   never touched by default. If your file doesn't mention a service,
   that service is ignored — not deleted. Opt-in ownership mode
   (`--prune` or a config key) enables full convergence: entities in
   an owned scope that aren't in the config get deleted. This keeps
   the safe default while allowing full IaC control when desired.

5. **No local state file.** Live state always comes from Railway's API.
   Diffs are never stale.

6. **Files are composable.** Multiple files can be merged
   (`--config a.toml --config b.toml`). This supports base+overlay
   patterns for multi-environment setups.

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
staging = { source = "production" }   # cloned from production
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

### Top-level declarative commands

These are the primary workflow. They operate on whatever scope the
config file declares.

```text
fat-controller init              Bootstrap a config file
fat-controller diff              Compare config against live state
fat-controller apply             Push config changes to Railway
fat-controller validate          Check config for warnings (no API)
```

### Top-level convenience commands

Quick imperative reads/writes against live Railway. These use the
config file (or flags) for context resolution but don't use the
declarative model.

```text
fat-controller get [path]        Read live state
fat-controller set <path> <val>  Set a single value
fat-controller delete <path>     Delete a single value
```

### Imperative action commands

Actions on running services. Read config file for project/env/service
context.

```text
fat-controller deploy [service]     Trigger a deployment
fat-controller redeploy [service]   Redeploy current image
fat-controller restart [service]    Restart running deployment
fat-controller rollback [service]   Rollback to previous deployment
fat-controller stop [service]       Stop running deployment
fat-controller logs [service]       Tail logs
fat-controller status [service]     Show deployment status
```

### Discovery commands

Read-only listing.

```text
fat-controller workspace list
fat-controller project list
fat-controller environment list
fat-controller service list
```

### Auth commands

```text
fat-controller auth login
fat-controller auth logout
fat-controller auth status
```

### Init modes

`fat-controller init` creates a config file. The source of data is a
sub-choice:

- **From remote (default when remote exists):** Interactive prompts to
  select workspace/project/environment/services, pulls live state,
  writes config + `.env.fat-controller`.

- **From scratch (when no remote exists, or `--new`):** Scaffolds a
  minimal config file. Prompts for project name, environment name,
  service names. Writes a skeleton that you then `apply` to create
  resources upstream.

- **From template:** `init --template <name>` scaffolds from a Railway
  template definition.

---

## File conventions

### Config file (desired state)

| File | Purpose | Committed? |
|------|---------|-----------|
| `fat-controller.toml` | Primary desired state file | Yes |
| `fat-controller.production.toml` | Environment overlay | Yes |
| `fat-controller.staging.toml` | Environment overlay | Yes |
| `.env.fat-controller` | Secret values for `${VAR}` interpolation | No (gitignored) |

The base file name is `fat-controller.toml` by default, overridable
with `--config`. Multiple `--config` flags merge in order.

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
| Environments | `[project.environments]` | name, source |

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
- Notification rules
- Integrations
- Observability dashboards

---

## Multi-environment patterns

### Pattern 1: Base + overlays (recommended)

```text
fat-controller.toml                 # shared across environments
fat-controller.production.toml      # production overrides
fat-controller.staging.toml         # staging overrides
.env.fat-controller                 # local secrets
```

```bash
# Apply to production
fat-controller apply \
  --config fat-controller.toml \
  --config fat-controller.production.toml

# Apply to staging
fat-controller apply \
  --config fat-controller.toml \
  --config fat-controller.staging.toml
```

The base file contains service definitions and variables that are the
same everywhere. The overlay file sets `environment = "production"` and
overrides values that differ.

### Pattern 2: Directory per environment

```text
environments/
  production/fat-controller.toml
  staging/fat-controller.toml
```

Each file is self-contained. Simpler but more duplication.

### Pattern 3: Service-scoped files (monorepo)

```text
services/
  api/fat-controller.toml           # service = "api"
  worker/fat-controller.toml        # service = "worker"
fat-controller.toml                 # shared variables only
```

Each service team owns their own file. A CI pipeline applies them all.

---

## Context resolution

Imperative commands (`deploy`, `logs`, `status`, etc.) need to know
which project/environment/service to target. Resolution order:

1. CLI flags (`--project`, `--environment`, `--service`)
2. Environment variables (`FAT_CONTROLLER_PROJECT`, etc.)
3. Config file keys (`project`, `environment` in `fat-controller.toml`)
4. Settings file keys (`.fat-controller.toml`)
5. Token scope (project-scoped `RAILWAY_TOKEN` implies project + env)
6. Interactive picker (if TTY)
7. Error with available options

Imperative commands also read the config file for context.

---

## Ownership and deletion

By default, `apply` is additive: it creates and updates but never
deletes by omission. To delete, you use explicit markers:

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

**Opt-in ownership mode** enables full convergence. When enabled for a
scope, entities that exist in Railway but aren't in the config are
deleted. This could be enabled via:

- A flag: `fat-controller apply --prune`
- A config key: `managed = true` (at top level or per-section)
- Per-section: `[api.variables] managed = true` means "I own all of
  api's variables; delete any not listed here"

The exact mechanism needs design, but the principle is: additive by
default, opt-in to full ownership. The diff output must clearly show
deletions that would result from prune mode.

### Creation semantics

`apply` creates anything declared in the config that doesn't exist:

| Entity | Created by | Notes |
|--------|-----------|-------|
| Project | Project-scope `apply` | Requires workspace context |
| Environment | Environment-scope `apply` | Requires project context |
| Service | Environment-scope `apply` | Service appears in config but not in Railway |
| Variable | Any scope `apply` | |
| Volume | Environment/service-scope `apply` | mount path required |
| Domain | Environment/service-scope `apply` | DNS verification separate |

This means `init --new` + `apply` is the full bootstrap path: scaffold
a config from scratch, then apply it to create everything in Railway.

---

## Open questions

1. **Prune mechanism.** `--prune` flag vs `managed = true` config key vs
   per-section ownership. Need to decide granularity. Per-section is
   most flexible but most complex.

2. **`get` output format.** `get` outputs TOML matching the config file
   format by default. Should it also support outputting just the raw
   values for scripting? e.g. `fat-controller get api.variables.PORT`
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
