# Technology

## Language: Go

- `go run github.com/hamishmorgan/fat-controller@latest` — zero-install
- Static binary via GoReleaser if distribution is needed later

## CLI framework: kong

[kong](https://github.com/alecthomas/kong) — struct-based CLI parser.
Commands and flags are defined as Go structs with tags. Cleaner than cobra
for nested subcommand groups, less boilerplate.

## GraphQL: genqlient

[genqlient](https://github.com/Khan/genqlient) generates typed Go functions
from `.graphql` operation files against the schema. Workflow:

1. Fetch schema via introspection -> `schema.graphql`
2. Write queries/mutations in `.graphql` files
3. `go generate` -> `generated.go` with typed functions and structs

## TOML: BurntSushi/toml

[BurntSushi/toml](https://github.com/BurntSushi/toml) — the standard Go TOML
library. Supports both encoding and decoding, preserves key order.

## Configuration: koanf

[koanf](https://github.com/knadh/koanf) — layered configuration library.
Modular (zero deps in core), case-sensitive keys, explicit merge order.
Replaces viper without the baggage (forced lowercase, global singleton,
massive dep tree).

## XDG directories: adrg/xdg

[adrg/xdg](https://github.com/adrg/xdg) — full XDG Base Directory spec
implementation. Cross-platform (Linux, macOS, Windows). Handles
`CONFIG_HOME`, `DATA_HOME`, `STATE_HOME`, `CACHE_HOME`, `RUNTIME_DIR`.

## Keyring: zalando/go-keyring

[go-keyring](https://github.com/zalando/go-keyring) — OS keychain access.
macOS Keychain, Linux Secret Service (GNOME Keyring / KWallet), Windows
Credential Manager. Pure Go, no CGo.
