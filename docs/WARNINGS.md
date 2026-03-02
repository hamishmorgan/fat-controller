# Config Validation and Warnings

When loading `fat-controller.toml` (and `fat-controller.local.toml`), the
tool runs a series of checks and emits warnings to stderr. These are
advisory — they never block execution. Warnings appear on `diff`, `apply`,
and `config validate` (a dedicated command for checking config without
hitting the API).

## Structural warnings

| Code | Warning | When |
|------|---------|------|
| `W001` | Unknown top-level key | Key is not `shared`, `services`, or a known setting (catches typos like `servics`) |
| `W002` | Unknown key in service block | Key inside `[services.X]` is not `variables` or a recognized service setting |
| `W003` | Empty service block | `[services.X]` exists but defines no variables or settings |

## Variable value warnings

| Code | Warning | When |
|------|---------|------|
| `W010` | Unresolved local interpolation | `${VAR}` where `VAR` is not set in the local environment |
| `W011` | Suspicious reference syntax | `${service.X}` looks like it was meant to be `${{service.X}}` (single vs double braces) |
| `W012` | Empty string is explicit delete | `VAR = ""` — reminder that this will delete the variable in Railway |

## Duplicate / conflict warnings

| Code | Warning | When |
|------|---------|------|
| `W020` | Variable in both shared and service | Variable appears in `[shared]` and `[services.X]` — service value wins |
| `W021` | Variable overridden by local file | Same variable defined in both `fat-controller.toml` and `fat-controller.local.toml` — local wins |

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
| `W051` | Local override file not gitignored | `fat-controller.local.toml` exists but is not in `.gitignore` |

## Reference warnings

| Code | Warning | When |
|------|---------|------|
| `W060` | Reference to unknown service | `${{service.VAR}}` references a service name not defined in the config (may still be valid if the service exists in Railway but isn't managed) |

## Controlling warnings

Warnings can be suppressed per-code:

```toml
# fat-controller.toml or .fat-controller.toml
suppress_warnings = ["W012", "W030"]   # suppress specific warnings
```

The `--quiet` flag suppresses all warnings. `config validate` ignores
`--quiet` (its whole purpose is to show warnings).
