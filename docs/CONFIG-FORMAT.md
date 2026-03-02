# Config File Format

## Live state, single config file

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

## Config file format

`fat-controller.toml` contains only mutable fields. Read-only fields
(IDs, `current_size_mb`, deployment metadata) are silently ignored if
present.

```toml
[shared.variables]
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

Shared variables (`[shared.variables]`) follow the same rules as
per-service variables. Railway's own precedence applies: per-service
overrides shared when both define the same key.

## Multi-file config

Multiple config files are merged in order (later values override earlier):

- `fat-controller.toml` — base config (committed)
- `fat-controller.local.toml` — auto-discovered if present (gitignored,
  for local overrides and secrets)
- Additional files via `--config` flags

```bash
fat-controller config diff
fat-controller config diff --config base.toml --config overrides.toml
```

## Variable interpolation

Two interpolation syntaxes in config values:

- `${{service.VAR}}` — **Railway reference**. Passed through as-is.
  Railway resolves at runtime. Safe to commit.
- `${VAR}` — **Local environment variable**. Resolved at apply time from
  the local shell environment. Missing env var = error. Useful for secrets
  in CI.

## Secret handling

With additive-only semantics, secrets that aren't in the config are simply
ignored. Three patterns for managing secrets:

1. **Don't mention them** — set in the dashboard, untouched by this tool.
   Works because unmentioned = ignored.
2. **Railway references** — `DATABASE_URL = "${{postgres.DATABASE_URL}}"`.
   Safe to commit. Railway resolves at runtime.
3. **Local env interpolation** — `STRIPE_KEY = "${STRIPE_KEY}"`. Resolved
   from local environment at apply time. Config file is safe to commit;
   actual value comes from CI env vars or a `.env` file.

See also: [SECRET-MASKING.md](SECRET-MASKING.md) for output masking,
[WARNINGS.md](WARNINGS.md) for config validation warnings.

## Apply ordering and redeployment

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
