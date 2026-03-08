![Fat Controller Logo](docs/logo.svg)

[![CI](https://github.com/hamishmorgan/fat-controller/actions/workflows/ci.yml/badge.svg)](https://github.com/hamishmorgan/fat-controller/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/hamishmorgan/fat-controller)](https://github.com/hamishmorgan/fat-controller/releases)
[![Go](https://img.shields.io/github/go-mod/go-version/hamishmorgan/fat-controller)](go.mod)
[![License](https://img.shields.io/github/license/hamishmorgan/fat-controller)](LICENSE)

# Fat Controller

A CLI for managing [Railway](https://railway.com) environments with a
config-as-code workflow.

Fat Controller pulls live _environment_ state into a local config file (such as `fat-controller.toml`), lets you edit it in version control, then diffs and applies changes back to Railway:

```sh
fat-controller adopt            # pull live state into config
fat-controller diff             # compare config against live Railway state
fat-controller apply            # push differences
```

It also supports imperative, one-off operations against live Railway:

```sh
fat-controller deploy [service]
fat-controller logs [service]
fat-controller status [services]
```

## Installation

### Download a release binary (recommended)

Grab the latest binary from the
[Releases](https://github.com/hamishmorgan/fat-controller/releases) page.
Archives are available for Linux, macOS, and Windows on amd64 and arm64.

Each release also publishes `checksums.txt`.

### Install with Go (from source)

```sh
go install github.com/hamishmorgan/fat-controller/cmd/fat-controller@latest
```

This installs `fat-controller` into `$(go env GOBIN)` (or `$(go env GOPATH)/bin`).
Make sure that directory is on your `PATH`.

To pin a specific version, replace `@latest` with a tag (for example
`@vX.Y.Z`).

### Run without installing

```sh
go run github.com/hamishmorgan/fat-controller/cmd/fat-controller@latest
```

### Install with [Mise](https://mise.jdx.dev/)

```toml
[tools]
"go:github.com/hamishmorgan/fat-controller/cmd/fat-controller" = "latest"
```

```sh
mise install
```

## Getting started

1. **Authenticate** with your Railway account:

    ```sh
    fat-controller auth login
    ```

2. **Bootstrap** a config file from your live project:

    ```sh
    fat-controller adopt
    ```

    This writes `fat-controller.toml` with your current live environment configuration.

    If secrets are detected, they are written to `fat-controller.secrets` (gitignored).

3. **Edit** the TOML file to declare your desired state, then **diff** and
    **apply**:

     ```sh
     fat-controller diff
     fat-controller apply
     ```

See [docs/COMMANDS.md](docs/COMMANDS.md) for the full command reference.
