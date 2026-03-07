# Commands

## Command Structure

fat-controller has two interaction modes: **declarative** (config-file-driven
diff and apply) and **imperative** (one-off operations against live Railway).

### Core Declarative Commands

```sh
fat-controller adopt                # Pull live state into a config file
fat-controller diff                 # Compare config against live Railway state
fat-controller apply                # Push config changes to Railway
fat-controller validate             # Check config for errors (offline)
fat-controller show                 # Display live Railway state
```

### Discovery

```sh
fat-controller list services        # List services in the project
fat-controller list deployments     # List recent deployments
fat-controller list domains         # List domains across services
fat-controller list all             # Full inventory
```

### Imperative Commands

```sh
fat-controller deploy [service]     # Trigger a deployment
fat-controller redeploy [id]        # Redeploy an existing deployment
fat-controller restart [id]         # Restart a deployment
fat-controller rollback [id]        # Rollback a deployment
fat-controller stop [id]            # Cancel a deployment
fat-controller status [services]    # Show service deployment status
fat-controller logs [service]       # Fetch logs
fat-controller open                 # Open Railway dashboard
```

### Scaffolding

```sh
fat-controller new project          # Create a new Railway project
fat-controller new environment      # Create a new environment
```

### Auth

```sh
fat-controller auth login           # Browser-based OAuth login
fat-controller auth logout          # Clear stored credentials
fat-controller auth status          # Show current auth state
```

### Legacy (deprecated, hidden)

```sh
fat-controller config get           # → use "show"
fat-controller config diff          # → use "diff"
fat-controller config apply         # → use "apply"
fat-controller config init          # → use "adopt"
fat-controller config validate      # → use "validate"
```

## Scope Resolution

Railway has five scope levels: **user > workspace > project > environment >
service**. Scope is determined by context:

1. **Auth token** — a project access token implicitly sets project +
   environment. An account-level token accesses any workspace/project.
2. **Config file** — `[workspace]` and `[project]` tables in
   `fat-controller.toml` set context.
3. **Flags** — `--workspace`, `--project`, `--environment`, `--service`.

When using an account-level token:

1. If flags (or env vars) are set, use them.
2. If only one workspace/project/environment exists, auto-select it.
3. If a TTY is attached, show an interactive picker.
4. Otherwise, error with a listing of available options.

## Settings

Every setting can be specified at up to five levels. Higher levels
override lower ones:

1. **Compiled-in defaults** (lowest)
2. **Global config** — `$XDG_CONFIG_HOME/fat-controller/config.toml`
3. **Config file** — `[tool]` table in `fat-controller.toml`
4. **Environment variable**
5. **CLI flag** (highest)

| Setting | CLI flag | Env var | Config key | Default | Description |
|---------|----------|---------|------------|---------|-------------|
| Token | `--token` | `RAILWAY_TOKEN` / `RAILWAY_API_TOKEN` | — | — | Auth token |
| Workspace | `--workspace` | `FAT_CONTROLLER_WORKSPACE` | `[workspace] name` | — | Workspace name |
| Project | `--project` | `FAT_CONTROLLER_PROJECT` | `[project] name` | — | Project name |
| Environment | `--environment` | `FAT_CONTROLLER_ENVIRONMENT` | `name` | — | Environment name |
| Output format | `--output`, `-o` | `FAT_CONTROLLER_OUTPUT` | `output` | `text` | `text`, `json`, `toml` |
| Color | `--color` | `FAT_CONTROLLER_COLOR` | `color` | `auto` | `auto`, `always`, `never` |
| Timeout | `--timeout` | `FAT_CONTROLLER_TIMEOUT` | `timeout` | `30s` | API request timeout |
| Yes | `--yes`, `-y` | `FAT_CONTROLLER_YES` | — | `false` | Skip confirmation prompts |
| Dry run | `--dry-run` | `FAT_CONTROLLER_DRY_RUN` | — | `false` | Preview without executing |
| Config file | `--config` | `FAT_CONTROLLER_CONFIG` | — | `fat-controller.toml` | Config file path(s) |
| Service | `--service` | `FAT_CONTROLLER_SERVICE` | — | — | Scope to a single service |
| Skip deploys | `--skip-deploys` | `FAT_CONTROLLER_SKIP_DEPLOYS` | `deploy` | `false` | Don't trigger redeployments |
| Fail fast | `--fail-fast` | `FAT_CONTROLLER_FAIL_FAST` | `fail_fast` | `false` | Stop on first error |
| Show secrets | `--show-secrets` | `FAT_CONTROLLER_SHOW_SECRETS` | `show_secrets` | `false` | Show secret values |
| Allow create | `--allow-create` | — | `allow_create` | `true` | Allow creating new resources |
| Allow update | `--allow-update` | — | `allow_update` | `true` | Allow updating resources |
| Allow delete | `--allow-delete` | — | `allow_delete` | `false` | Allow deleting resources |
| Verbose | `--verbose`, `-v` | — | — | `false` | Debug output |
| Quiet | `--quiet`, `-q` | — | — | `false` | Suppress informational output |

## Confirmation and dry-run

All mutations (`adopt`, `apply`) require confirmation:

- **Interactive (TTY):** prompts for confirmation before writing/mutating.
  Pass `--yes` (`-y`) to skip all confirmation prompts.
- **Non-interactive (piped):** previews changes without executing.
  Pass `--yes` to execute.
- **`--dry-run`:** always previews, never persists. Overrides `--yes`.

`NO_COLOR` (any value) disables color output regardless of `--color`.
