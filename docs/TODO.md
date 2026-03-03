# TODO

- [ ] Implement config validation warnings (M4: unknown keys/typos) and wire up `config validate`.
- [ ] Include deploy/build settings in live state fetches so `config get --full` and diffs reflect them.
- [ ] Batch variable updates in apply using `variableCollectionUpsert` instead of per-variable mutations.
- [x] Add an interactive confirmation prompt for apply when `--confirm` is not set and stdin is a TTY.
- [x] Support JSON/TOML output formats for `config apply`.
- [ ] Add `workspace` as an optional top-level config key (parsing, merge, and resolution fallback).
- [ ] Extend `config init` with optional service selection (choose which services to include).
- [ ] Add `config init` support for environment-specific files (e.g. generate overlay files).
- [ ] Resolve project/environment IDs to names when `config init` is passed UUIDs.
- [x] Update `.gitignore` automatically when creating `fat-controller.local.toml` (optional safety).
- [ ] Add volume, domain, and TCP proxy management to config (Future).
- [x] Add GoReleaser pipeline for prebuilt binaries.
- [ ] Make `config init` interactive bootstrap (Future).

## Code Quality & Testing

- [ ] Improve test coverage for `cmd/fat-controller`, `internal/apply`, and `internal/railway` (currently ~0-60%).
- [ ] Add end-to-end integration tests using a staging Railway project or a mocked GraphQL API.
- [ ] Automate CLI reference documentation generation (e.g., using `cobra/doc`).
- [ ] Apply CLI timeout flag to derived contexts in command `Run` methods and set per-client HTTP timeouts.
- [ ] Make OAuth login wait bounded by context/timeout to avoid indefinite blocking when callback never arrives.
- [ ] Shutdown auth callback server with a timeout context to prevent hangs.
- [ ] Surface token refresh failures from transport (return wrapped error instead of silent 401).
- [ ] Tie auth callback server goroutine lifecycle to context/cancellation.
- [ ] Refactor repeated config CLI tests into table-driven subtests.

## Features

- [ ] Support parsing, validating, and updating standard `railway.toml` native service configurations natively.
