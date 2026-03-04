# TODO

## Features

- [ ] Implement config validation warnings (M4: unknown keys/typos) and wire up `config validate`.
- [ ] Include deploy/build settings in live state fetches so `config get --full` and diffs reflect them.
- [ ] Batch variable updates in apply using `variableCollectionUpsert` instead of per-variable mutations.
- [ ] Add `workspace` as an optional top-level config key (parsing, merge, and resolution fallback).
- [ ] Extend `config init` with optional service selection (choose which services to include).
- [ ] Add `config init` support for environment-specific files (e.g. generate overlay files).
- [ ] Resolve project/environment IDs to names when `config init` is passed UUIDs.
- [ ] Add volume, domain, and TCP proxy management to config (future).
- [ ] Make `config init` interactive bootstrap (future).
- [ ] Support parsing, validating, and updating standard `railway.toml` native service configurations natively.
- [ ] `config get` path argument should filter by section/key, not just service (currently `config get api.variables.PORT` returns all of `api`).
- [ ] `config set` and `config delete` should offer interactive confirmation like `config apply` does (currently they default to dry-run with no prompt).
- [ ] Handle `toml` output format in list commands (`environment list`, `project list`, `workspace list`) — currently silently falls to text.
- [ ] Wire up `--verbose` and `--quiet` flags to control output verbosity across all commands.
- [ ] Parse and validate `sensitive_keywords`, `sensitive_allowlist`, and `suppress_warnings` config keys (currently accepted but silently ignored).
- [ ] Return errors for unrecognised non-table top-level config keys (typos like `projct` are silently ignored).
- [ ] Return errors for non-string `project`/`environment` values in config (e.g. `project = 123` is silently ignored).
- [ ] Quote TOML keys in rendered output — bare keys containing `.`, spaces, or other special chars produce invalid TOML.

## Code Quality & Testing

- [ ] Improve test coverage for `cmd/fat-controller`, `internal/apply`, and `internal/railway` (currently ~0-60%).
- [ ] Apply CLI `--timeout` flag to derived contexts in command `Run` methods and set per-client HTTP timeouts (flag is declared but unused).
- [ ] Make OAuth login wait bounded by context/timeout to avoid indefinite blocking when callback never arrives.
- [ ] Shutdown auth callback server with a timeout context to prevent hangs.
- [ ] Surface token refresh failures from transport (return wrapped error instead of silent 401).
- [ ] Tie auth callback server goroutine lifecycle to context/cancellation.
- [ ] Add shell completions (kong custom completers or external generator).
- [ ] Add `context.Context` parameter to `RegisterClient` and `ExchangeCode` in `auth/oauth.go` (currently uncancellable I/O).
- [ ] Add `context.Context` parameter to `ResolveAuth` in `auth/resolver.go` (keyring access can block on some Linux systems).
- [ ] Include response body in error messages for `RegisterClient` and `ExchangeCode` failures (only `RefreshToken` currently does this).
- [ ] Extract repeated auth/client boilerplate in CLI `Run` methods into a shared helper (repeated in every command).
- [ ] Extract shared config-load/resolve/fetch/filter logic from `config_diff.go` and `config_apply.go` into a common function.
- [ ] Define constants for deploy/resource setting keys shared between `diff` and `apply` packages (currently hard-coded strings must be kept in sync).
- [ ] Add `ctx.Err()` check in apply best-effort loops to avoid wasted network calls on context cancellation.
- [ ] `apply.Result.Skipped` field is declared and serialised but never incremented — wire it up or remove it.
- [ ] `OpenBrowser` in `auth/login.go` calls `cmd.Start()` without `cmd.Wait()`, leaking zombie processes.
- [ ] Remove mutable package-level `browserCommand` variable in `auth/login.go` — tests already inject `BrowserOpener` via function parameter.
- [ ] Handle `json.MarshalIndent` / `toml.Marshal` errors in `config_apply.go` instead of discarding them.
- [ ] `resolveServiceID` in `apply/railway.go` holds a mutex across network calls — refactor to cache-aside pattern if concurrent apply is added.
- [ ] `ResolvedAuth.Token` is mutated inside transport's mutex but readable externally without synchronisation — consider a safe accessor.
- [ ] Add `docs/WARNINGS.md` notice that the warning system (W001-W060) is planned but not yet implemented.
- [ ] Pin GitHub Actions to commit SHAs instead of mutable version tags for supply-chain security.
- [ ] Add `concurrency` with `cancel-in-progress` to CI workflows to avoid redundant PR runs.
- [ ] Pin mise tool versions to specific releases instead of `latest` for reproducible builds.

## Done

- [x] Add an interactive confirmation prompt for apply when `--confirm` is not set and stdin is a TTY.
- [x] Support JSON/TOML output formats for `config apply`.
- [x] Update `.gitignore` automatically when creating `fat-controller.local.toml` (optional safety).
- [x] Add GoReleaser pipeline for prebuilt binaries.
- [x] Add end-to-end integration tests using a mocked GraphQL API.
- [x] Automate CLI reference documentation generation.
- [x] Add JSON Schema and annotated example for `fat-controller.toml` config format.
- [x] CI freshness check for generated CLI reference docs.
- [x] Refactor repeated config CLI tests into table-driven subtests.
