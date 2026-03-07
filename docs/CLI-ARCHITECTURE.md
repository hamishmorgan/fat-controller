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
staging = {}
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

### Declarative commands

The primary workflow. These operate on whatever scope the config file
declares.

```text
fat-controller init              Guided first-time config bootstrap
fat-controller adopt [path]      Merge live Railway state into config
fat-controller diff              Compare config against live state
fat-controller apply             Merge config into live Railway state
fat-controller validate          Check config for warnings (no API)
fat-controller show [path]       Display live state (read-only)
```

`apply` and `adopt` are symmetric:

| Command | Direction | Behavior |
|---------|-----------|----------|
| `apply` | local config → Railway | Additive merge of config into live state. Only touches what's declared. |
| `adopt` | Railway → local config | Additive merge of live state into config. Only adds what's missing or changed. |

Neither overwrites the target wholesale. Both are additive merges in
their respective directions.

`show` is the read-only counterpart — display live state without
modifying the config file. `show` with no path gives an overview.
`show api.variables.PORT` gives a single value.

### Init

`fat-controller init` creates a config file for the first time. It is
a guided version of `adopt` — interactive prompts to select
workspace/project/environment/services, plus secret extraction into
`.env.fat-controller`.

After the initial bootstrap, use `adopt` to bring additional resources
into an existing config (e.g. `adopt redis` after adding a service in
the dashboard).

`init` modes:

- **From remote (default when remote exists):** Guided `adopt` with
  interactive selection and secret extraction.

- **From scratch (`--new`):** Scaffolds a minimal config file. Prompts
  for project name, environment name, service names. Writes a skeleton
  that you then `apply` to create resources in Railway.

- **From template:** `init --template <name>` scaffolds from a Railway
  template definition.

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

## Merge behavior

`apply` and `adopt` share three boolean flags that control what the
merge does. Each flag has both `--X` and `--no-X` forms. Defaults are
configurable via settings file, env vars, or CLI flags (highest
priority).

| Flag | Default | What it controls |
|------|---------|-----------------|
| `--create` / `--no-create` | on | Add entities that exist in source but not target |
| `--update` / `--no-update` | on | Overwrite entities that exist in both source and target |
| `--delete` / `--no-delete` | off | Remove entities that exist in target but not source |

Applied to each command:

| Flag | `apply` (config → Railway) | `adopt` (Railway → config) |
|------|---------------------------|---------------------------|
| `--create` | Create Railway entities not in Railway | Add config entries not in config |
| `--update` | Update Railway entities that differ from config | Update config entries that differ from Railway |
| `--delete` | Delete Railway entities not in config | Remove config entries not in Railway |

Defaults are configurable at every settings level:

```toml
# .fat-controller.toml or $XDG_CONFIG_HOME/fat-controller/config.toml
create = true
update = true
delete = false
```

```bash
FAT_CONTROLLER_CREATE=true
FAT_CONTROLLER_UPDATE=true
FAT_CONTROLLER_DELETE=false
```

Common patterns:

```bash
apply                              # create + update (default)
apply --delete                     # full convergence
apply --no-update                  # create only, don't touch existing
apply --no-create                  # update only, don't add new
apply --no-create --delete         # update + delete, don't add
adopt --no-update                  # add new entries, don't touch existing
adopt --delete                     # add + update + remove stale
adopt --no-create --delete         # update + remove stale, don't add
```

Without `--delete`, explicit delete markers handle one-off removals:

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

`diff` reflects the active flags: it shows what `apply` would do
given the current create/clobber/prune settings.

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
