# Decisions

Resolved during planning. Rationale preserved for future reference.

## Variable ownership: additive-only

Variables are additive-only by default. Only variables explicitly listed in
config are managed. Unmentioned variables are left alone — no implicit
deletion by omission. To delete a variable, set it to empty string:
`OLD_VAR = ""`. This eliminates the previous "section presence = ownership"
model and avoids accidental deletions.

## Secret handling: local env interpolation + multi-file

Secrets are handled through three complementary mechanisms: don't mention
them (unmanaged), use Railway references (`${{service.VAR}}`), or use
local env interpolation (`${VAR}`). The `${VAR}` syntax (single braces) is
deliberately distinct from Railway's `${{}}` (double braces) — Railway
chose double braces specifically to avoid shell variable collision.

Multi-file merging provides additional flexibility: a gitignored
`fat-controller.local.toml` is auto-discovered, and `--config` can be
repeated for explicit layering.

## Deletion safety: dry-run default is sufficient

With additive-only semantics, deletions are always explicit (`KEY = ""`).
The dry-run default on apply plus prominent diff output (showing "DELETE"
clearly) provides sufficient safety without extra flags.

## Volumes, domains, TCP proxies: pull-only for now

These are included in `config get` output for visibility but are not manageable
via config in the initial release. The focus is on the variable/settings
gap. Management can be added in a future milestone — the additive-only
model makes it safe when we do.

## Shared variables: same semantics as per-service

Shared variables follow the same additive-only rules. The API call is the
same mutation (`variableCollectionUpsert`) without a `serviceId`. Railway
handles precedence: per-service overrides shared when both define the
same key.

## Error handling: continue by default, --fail-fast option

Apply is best-effort and non-transactional. By default, a failure on one
service does not stop processing of remaining services. `--fail-fast` stops
on first error. A summary reports what was applied and what failed. Exit
code is non-zero if anything failed.

## Orchestration: thick cmd/ layer

Command handlers in `cmd/` directly call `internal/` packages. No separate
engine or orchestration package. Extract if complexity warrants it later.

## CLI framework: kong

[kong](https://github.com/alecthomas/kong) for struct-based CLI parsing.
Less boilerplate than cobra for nested subcommand groups.

## --token flag has no env var binding in kong

The `--token` CLI flag is not bound to any env var via kong's `env:""` tag.
`RAILWAY_TOKEN` and `RAILWAY_API_TOKEN` are handled by the token resolver
(`internal/auth/resolver.go`) which reads them via `os.Getenv` and applies
the correct HTTP header for each:

- `--token` flag → `Authorization: Bearer` (explicit override, any token type)
- `RAILWAY_API_TOKEN` → `Authorization: Bearer` (account/workspace-scoped)
- `RAILWAY_TOKEN` → `Project-Access-Token` (project-scoped)

If kong bound `--token` to `RAILWAY_TOKEN`, it would route project-scoped
tokens through the Bearer header path, which is incorrect. Keeping the
resolver separate preserves the header distinction.

## Token refresh: transparent via HTTP transport

The authenticated HTTP transport (`internal/railway/transport.go`)
transparently refreshes expired OAuth tokens. On a 401 response, it:

1. Loads stored tokens (client ID + refresh token) from the token store
2. Calls the OAuth token endpoint to refresh
3. Saves the new token pair (Railway rotates refresh tokens)
4. Retries the original request with the new access token

This only applies to stored OAuth tokens (source = "stored"). Tokens
from `--token` flag or environment variables are never refreshed — a 401
is returned directly.

The transport is used by both the Railway GraphQL client and the
`auth status` userinfo call, so all commands benefit from transparent
refresh.

## Testing strategy

- Unit tests for pure logic: diff, config parsing, interpolation, PKCE
- HTTP mock tests (`httptest.NewServer`) for OAuth and GraphQL
- Keyring mock tests (`go-keyring MockInit()`) for token storage
- Golden file tests for diff output formatting
- No live Railway integration tests in CI
