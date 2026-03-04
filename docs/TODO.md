# TODO

## Features

- [ ] Extend `config init` with optional service selection (choose which services to include).
- [ ] Add `config init` support for environment-specific files (e.g. generate overlay files).
- [ ] Resolve project/environment IDs to names when `config init` is passed UUIDs.
- [ ] Add volume, domain, and TCP proxy management to config (future).
- [ ] Make `config init` interactive bootstrap (future).
- [ ] Support parsing, validating, and updating standard `railway.toml` native service configurations natively.

## Code Quality & Testing

- [ ] Improve test coverage for `cmd/fat-controller`, `internal/apply`, and `internal/railway` (currently ~0-60%).
- [ ] Tie auth callback server goroutine lifecycle to context/cancellation.
- [ ] Add shell completions (kong custom completers or external generator).

## Done

- [x] Implement config validation warnings (W003-W041) and wire up `config validate` command.
- [x] Include deploy/build settings in live state fetches so `config get --full` and diffs reflect them.
- [x] Batch variable updates in apply using `variableCollectionUpsert` instead of per-variable mutations.
- [x] Add `workspace` as an optional top-level config key (parsing, merge, and resolution fallback).
- [x] `config get` path argument filters by section/key (e.g. `config get api.variables.PORT`).
- [x] `config set` and `config delete` offer interactive confirmation like `config apply` does.
- [x] Handle `toml` output format in list commands (`environment list`, `project list`, `workspace list`).
- [x] Wire up `--verbose` and `--quiet` flags to control output verbosity across all commands.
- [x] Parse and validate `sensitive_keywords`, `sensitive_allowlist`, and `suppress_warnings` config keys.
- [x] Return errors for unrecognised non-table top-level config keys (typos like `projct` are rejected).
- [x] Return errors for non-string `project`/`environment` values in config.
- [x] Quote TOML keys in rendered output — bare keys containing `.`, spaces, or special chars are quoted.
- [x] Apply CLI `--timeout` flag to derived contexts in command `Run` methods.
- [x] Make OAuth login wait bounded by context/timeout to avoid indefinite blocking.
- [x] Shutdown auth callback server with a timeout context to prevent hangs.
- [x] Surface token refresh failures from transport (return wrapped error instead of silent 401).
- [x] Add `context.Context` parameter to `RegisterClient` and `ExchangeCode` in `auth/oauth.go`.
- [x] Add `context.Context` parameter to `ResolveAuth` in `auth/resolver.go`.
- [x] Include response body in error messages for `RegisterClient` and `ExchangeCode` failures.
- [x] Extract repeated auth/client boilerplate in CLI `Run` methods into a shared `newClient` helper.
- [x] Extract shared config-load/resolve/fetch/filter logic into `loadAndFetch` common function.
- [x] Define constants for deploy/resource setting keys shared between `diff` and `apply` packages.
- [x] Add `ctx.Err()` check in apply best-effort loops to avoid wasted network calls on cancellation.
- [x] Remove `apply.Result.Skipped` field (was declared but never incremented).
- [x] Fix `OpenBrowser` zombie process leak — `cmd.Start()` now followed by background `cmd.Wait()`.
- [x] Remove mutable package-level `browserCommand` variable in `auth/login.go`.
- [x] Handle `json.MarshalIndent` / `toml.Marshal` errors in `config_apply.go`.
- [x] Refactor `resolveServiceID` to cache-aside pattern (no mutex held across network calls).
- [x] `ResolvedAuth.Token` thread-safe accessors (`Token()` / `SetToken()`).
- [x] Pin GitHub Actions to commit SHAs for supply-chain security.
- [x] Add `concurrency` with `cancel-in-progress` to CI workflows.
- [x] Pin mise tool versions to specific releases for reproducible builds.
- [x] Add an interactive confirmation prompt for apply when `--confirm` is not set and stdin is a TTY.
- [x] Support JSON/TOML output formats for `config apply`.
- [x] Update `.gitignore` automatically when creating `fat-controller.local.toml` (optional safety).
- [x] Add GoReleaser pipeline for prebuilt binaries.
- [x] Add end-to-end integration tests using a mocked GraphQL API.
- [x] Automate CLI reference documentation generation.
- [x] Add JSON Schema and annotated example for `fat-controller.toml` config format.
- [x] CI freshness check for generated CLI reference docs.
- [x] Refactor repeated config CLI tests into table-driven subtests.
