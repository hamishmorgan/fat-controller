![Fat Controller Logo](docs/logo.svg)

[![Test](https://github.com/hamishmorgan/fat-controller/actions/workflows/test.yml/badge.svg)](https://github.com/hamishmorgan/fat-controller/actions/workflows/test.yml)
[![Build](https://github.com/hamishmorgan/fat-controller/actions/workflows/build.yml/badge.svg)](https://github.com/hamishmorgan/fat-controller/actions/workflows/build.yml)
[![Lint Go](https://github.com/hamishmorgan/fat-controller/actions/workflows/lint-go.yml/badge.svg)](https://github.com/hamishmorgan/fat-controller/actions/workflows/lint-go.yml)
[![Lint Docs](https://github.com/hamishmorgan/fat-controller/actions/workflows/lint-docs.yml/badge.svg)](https://github.com/hamishmorgan/fat-controller/actions/workflows/lint-docs.yml)
[![Secrets](https://github.com/hamishmorgan/fat-controller/actions/workflows/secrets.yml/badge.svg)](https://github.com/hamishmorgan/fat-controller/actions/workflows/secrets.yml)

[![Release](https://img.shields.io/github/v/release/hamishmorgan/fat-controller)](https://github.com/hamishmorgan/fat-controller/releases)
[![License](https://img.shields.io/github/license/hamishmorgan/fat-controller)](LICENSE)

# Fat Controller

A CLI for managing [Railway](https://railway.com) projects.

Railway's `railway.toml` files cover build and deploy settings (Dockerfile
path, watch patterns, healthchecks, restart policy), but a large portion of
project configuration lives only in the dashboard: environment variables,
resource limits, regions, replicas, domains, volumes, and TCP proxies. For
multi-service projects this means:

- No version control or audit trail for env var changes
- No way to review configuration changes in a PR
- Manual, error-prone setup when recreating or cloning a project
- No mechanism to detect configuration drift

Fat Controller treats Railway project configuration as code: pull the live
state, declare the desired state in a TOML file, diff, and apply.

```sh
fat-controller config get       # pull live config
fat-controller config diff      # compare config file against live state
fat-controller config apply     # push differences
```

## Installation

### Download a release binary

Grab the latest binary from the
[Releases](https://github.com/hamishmorgan/fat-controller/releases) page.
Archives are available for Linux, macOS, and Windows on amd64 and arm64.

### Build from source

```sh
go install github.com/hamishmorgan/fat-controller/cmd/fat-controller@latest
```

Verify the installation:

```sh
fat-controller --version
```

## Getting started

1. **Authenticate** with your Railway account:

   ```sh
   fat-controller auth login
   ```

2. **Bootstrap** a config file from your live project:

   ```sh
   fat-controller config init --project my-project --environment production
   ```

   This creates `fat-controller.toml` with your current live configuration.

3. **Edit** the TOML file to declare your desired state, then **diff** and
   **apply**:

   ```sh
   fat-controller config diff
   fat-controller config apply
   ```

See [docs/COMMANDS.md](docs/COMMANDS.md) for the full command reference.
