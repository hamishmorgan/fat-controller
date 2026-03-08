# Config Validation and Warnings

When loading `fat-controller.toml`, the
tool runs a series of checks and emits warnings to stderr. These are
advisory — they never block execution. Warnings appear on `diff`, `apply`,
and `config validate` (a dedicated command for checking config without
hitting the API).

## Structural warnings

| Code | Warning | When |
|------|---------|------|
| `W002` | Unknown key in service block | Key inside a service block is not `variables`, `resources`, or `deploy` |
| `W003` | Empty service block | Service block exists but defines no variables, resources, or deploy settings |

> **Note:** Unknown non-table top-level keys (e.g. `shaerd = ...`) are rejected
> as parse errors, not warnings.

## Variable value warnings

| Code | Warning | When |
|------|---------|------|
| `W011` | Suspicious reference syntax | `${service.X}` looks like it was meant to be `${{service.X}}` (single vs double braces) |
| `W012` | Empty string is explicit delete | `VAR = ""` — reminder that this will delete the variable in Railway |

> **Note:** Unresolved local environment variables (`${VAR}` where `VAR` is not
> set) are treated as errors, not warnings.

## Duplicate / conflict warnings

| Code | Warning | When |
|------|---------|------|
| `W020` | Variable in both shared and service | Variable appears in `[shared.variables]` and `[X.variables]` — service value wins |
| `W021` | Variable overridden by overlay file | Same variable defined in base config and override file — later value wins |

## Naming warnings

| Code | Warning | When |
|------|---------|------|
| `W030` | Lowercase variable name | Variable name contains lowercase letters (convention is `UPPER_SNAKE_CASE`) |
| `W031` | Invalid variable name characters | Variable name contains spaces or characters that may not work in Railway |

## Scope warnings

| Code | Warning | When |
|------|---------|------|
| `W040` | Unknown service name | Config references a service that doesn't exist in Railway (checked at `diff`/`apply` time) |
| `W041` | No services or shared variables | Config file exists but defines nothing actionable |

## Secret hygiene warnings

| Code | Warning | When |
|------|---------|------|
| `W050` | Hardcoded secret in config | A value matches the secret detection heuristics (name + entropy) and is not using `${VAR}` interpolation — likely a plaintext secret in a committed file |
| `W052` | Deprecated local override file | `fat-controller.local.toml` exists — migrate to ${VAR} references |

## Reference warnings

| Code | Warning | When |
|------|---------|------|
| `W060` | Reference to unknown service | `${{service.VAR}}` references a service name not defined in the config (may still be valid if the service exists in Railway but isn't managed) |

> **Note:** References to `${{shared.VAR}}` are always treated as valid because
> Railway uses `shared` as the pseudo-service for environment-level variables
> (represented by this tool's top-level `[variables]` table).

## Controlling warnings

Warnings can be suppressed per-code:

```toml
# fat-controller.toml or .fat-controller.toml
suppress_warnings = ["W012", "W030"]   # suppress specific warnings
```

The `--quiet` flag suppresses all warnings. `config validate` ignores
`--quiet` (its whole purpose is to show warnings).
