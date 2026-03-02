# TODO

- [ ] Implement config validation warnings (M4: unknown keys/typos) and wire up `config validate`.
- [ ] Include deploy/build settings in live state fetches so `config get --full` and diffs reflect them.
- [ ] Batch variable updates in apply using `variableCollectionUpsert` instead of per-variable mutations.
- [ ] Add an interactive confirmation prompt for apply when `--confirm` is not set and stdin is a TTY.
- [ ] Support JSON/TOML output formats for `config apply`.
- [ ] Add TOML output for `project list`, `environment list`, and `workspace list`.
- [ ] Avoid double project/environment resolution in `config apply` Run path by loading config first.
- [ ] Add `workspace` as an optional top-level config key (parsing, merge, and resolution fallback).
- [ ] Extend `config init` with optional service selection (choose which services to include).
- [ ] Add `config init` support for environment-specific files (e.g. generate overlay files).
- [ ] Resolve project/environment IDs to names when `config init` is passed UUIDs.
- [ ] Update `.gitignore` automatically when creating `fat-controller.local.toml` (optional safety).
- [ ] Add volume, domain, and TCP proxy management to config (Future).
- [ ] Add GoReleaser pipeline for prebuilt binaries (Future).
- [ ] Make `config init` interactive bootstrap (Future).

## Code Quality & Testing

- [ ] Improve test coverage for `cmd/fat-controller`, `internal/apply`, and `internal/railway` (currently ~0-60%).
- [ ] Add end-to-end integration tests using a staging Railway project or a mocked GraphQL API.
- [ ] Automate CLI reference documentation generation (e.g., using `cobra/doc`).

## Features

- [ ] Add a `--dry-run` flag to `config apply` to simulate and validate mutations without making changes.
- [ ] Support parsing, validating, and updating standard `railway.toml` native service configurations natively.
