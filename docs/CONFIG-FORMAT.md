# Config File Format

## Live state, single config file

There is no state file. `diff` and `apply` always query Railway's API for
current live state. This means diffs are never stale, and secrets are
never written to disk.

The only file the user manages is `fat-controller.toml` — the desired
state.

`show` with no arguments outputs the full live config — pipe it to a file
to bootstrap your config, or inspect what's deployed.

Service names in the config are resolved to Railway IDs via the live API
at diff/apply time.

## Config file format

`fat-controller.toml` uses TOML with the `[[service]]` array-of-tables
pattern. Each service is a `[[service]]` block with its name, variables,
deploy settings, and sub-resources.

```toml
# Environment identity
name = "production"

# Workspace and project context
[workspace]
name = "acme-corp"

[project]
name = "my-app"

# Shared variables (available to all services)
[variables]
NODE_ENV = "production"

# Tool behavior settings
[tool]
env_file = ".env.fat-controller"
fail_fast = true

# --- Services ---

[[service]]
name = "api"

[service.deploy]
builder = "NIXPACKS"
start_command = "node server.js"
healthcheck_path = "/health"

[service.resources]
vcpus = 2
memory_gb = 4

[service.variables]
PORT = "8080"
DATABASE_URL = "${{postgres.DATABASE_URL}}"
STRIPE_KEY = "${STRIPE_KEY}"   # resolved from local env at apply time
OLD_VAR = ""                   # explicit delete

# Sub-resources
[service.domains]
"api.example.com" = { port = 8080 }

[service.volumes]
data = { mount = "/app/data" }

[[service]]
name = "worker"

[service.deploy]
builder = "DOCKERFILE"
dockerfile_path = "./worker/Dockerfile"

[service.variables]
DATABASE_URL = "${{api.DATABASE_URL}}"
QUEUE_NAME = "default"
```

## Service sub-resources

Each service can declare sub-resources:

| Sub-resource | Format | Example |
|-------------|--------|---------|
| **Domains** | `[service.domains]` table of `{ port }` | `"api.example.com" = { port = 8080 }` |
| **Volumes** | `[service.volumes]` table of `{ mount }` | `data = { mount = "/app/data" }` |
| **TCP proxies** | `tcp_proxies` array of ports | `tcp_proxies = [5432, 6379]` |
| **Network** | `network` boolean | `network = true` |
| **Triggers** | `[[service.triggers]]` array | `repository = "org/repo"`, `branch = "main"` |
| **Egress** | `egress` array of regions | `egress = ["us-west1", "eu-west1"]` |

To delete a domain or volume, use `delete = true`:

```toml
[service.domains]
"old.example.com" = { delete = true }
```

## Diff semantics

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

Sub-resources use create/delete semantics based on presence in config vs.
live state. TCP proxies also detect live proxies not in config (deletes).

## Multi-file config

Multiple config files are merged in order (later values override earlier):

- `fat-controller.toml` — base config (committed)
- Additional files via `--config` flags
- `.env.fat-controller` or files specified in `[tool] env_file`

```bash
fat-controller diff
fat-controller diff --config base.toml --config overrides.toml
```

## Variable interpolation

Two interpolation syntaxes in config values:

- `${{service.VAR}}` — **Railway reference**. Passed through as-is.
  Railway resolves at runtime. Safe to commit.
- `${VAR}` — **Local environment variable**. Resolved at apply time from
  the local shell environment or `.env.fat-controller`. Missing env var =
  error. Useful for secrets in CI.

## Secret handling

With additive-only semantics, secrets that aren't in the config are simply
ignored. Three patterns for managing secrets:

1. **Don't mention them** — set in the dashboard, untouched by this tool.
   Works because unmentioned = ignored.
2. **Railway references** — `DATABASE_URL = "${{postgres.DATABASE_URL}}"`.
   Safe to commit. Railway resolves at runtime.
3. **Local env interpolation** — `STRIPE_KEY = "${STRIPE_KEY}"`. Resolved
   from local environment at apply time. Config file is safe to commit;
   actual value comes from `.env.fat-controller` or CI env vars.

`adopt` (or `config init`) generates a `.env.fat-controller` file
(gitignored) with actual secret values pulled from Railway. Load it into
your environment before running `apply` (e.g. `source .env.fat-controller`,
direnv, or CI pipeline secrets).

See also: [SECRET-MASKING.md](SECRET-MASKING.md) for output masking,
[WARNINGS.md](WARNINGS.md) for config validation warnings.

## Apply ordering and redeployment

Apply runs in five phases:

1. **Service CRUD** — create new services, delete marked ones.
2. **Service settings** — deploy and resource settings for all services
   (via `serviceInstanceUpdate` / `serviceInstanceLimitsUpdate`),
   alphabetically by service name.
3. **Shared variables** — project-wide variables (no `serviceId`).
4. **Per-service variables** — alphabetically by service name.
5. **Sub-resources** — domains, volumes, TCP proxies, network, triggers,
   egress for each service.

This ordering ensures that the triggered redeploy from variable
upserts picks up settings changes already applied.

Apply is **best-effort, non-transactional**. By default, a failure on
one service does not stop processing of remaining services. Use
`--fail-fast` to stop on first error. On completion, a summary reports
what was applied and what failed. Exit code is non-zero if any service
failed.

The `--skip-deploys` flag passes `skipDeploys: true` to variable upserts
to defer redeployment until all changes are applied.
