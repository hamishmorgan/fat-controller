# Scope and Command Structure

Railway has five scope levels: **user > workspace > project > environment >
service**. A user account can access multiple workspaces, each containing
multiple projects. Rather than encoding these as nested subcommands, scope
is determined by context:

1. **Auth token** — a project access token implicitly sets project +
   environment (narrowest). An account-level token could access any
   workspace/project the user belongs to (broadest).
2. **Flags** — `--service <name>` narrows to a single service.
3. **Future**: a local context file, workspace-level auth, or account-level
   auth could broaden scope.

The default is as broad as the auth allows. `config get` fetches all
services in the project+environment; `--service` narrows when needed.

Commands are grouped by **domain**, not by scope. There are two
interaction modes: **imperative** (one-off CRUD against live Railway)
and **declarative** (config-file-driven diff and apply).

```sh
fat-controller auth login       # Browser-based OAuth login
fat-controller auth logout      # Clear stored credentials
fat-controller auth status      # Show current auth state

# Project / environment discovery
fat-controller workspace list                     # list available workspaces
fat-controller project list                       # list available projects (within workspace)
fat-controller environment list --project my-app  # list environments for a project

# Imperative — read/write live Railway directly
fat-controller config get                         # all config (pipe to file to bootstrap)
fat-controller config get api.variables           # all variables for a service
fat-controller config get api.variables.PORT      # one specific value
fat-controller config get --full                  # everything including IDs and read-only fields
fat-controller config set api.variables.PORT 8080 # set a value
fat-controller config delete api.variables.OLD    # delete a value
# Note: In M3, config set/delete supports variables only (*.variables.KEY).
# Other sections (resources, deploy settings) will be added in later milestones.

# Declarative — config file driven
fat-controller config diff      # compare fat-controller.toml against live state
fat-controller config apply     # push differences from config file
fat-controller config validate  # check config for warnings (no API calls)
```

Dot-path addressing (`service.section.key`) is used universally: in
`get/set/delete` arguments, in `--service` scoping for diff/apply, and
in config file section headers.

Future command groups (not in scope for initial release):

```sh
fat-controller deploy list      # List deployments
fat-controller deploy trigger   # Trigger a redeploy
fat-controller service list     # List services in the project
fat-controller logs tail        # Stream logs
```

## Settings

Every setting can be specified at up to five levels. Higher levels
override lower ones:

1. **Compiled-in defaults** (lowest)
2. **Global config** — `$XDG_CONFIG_HOME/fat-controller/config.toml`
3. **Local config** — `.fat-controller.toml` in working dir or git root
4. **Environment variable**
5. **CLI flag** (highest)

The full settings table:

| Setting | CLI flag | Env var | Config key | Default | Description |
|---------|----------|---------|------------|---------|-------------|
| Token | `--token` | `RAILWAY_TOKEN` / `RAILWAY_API_TOKEN` | — | — | Auth token. `RAILWAY_TOKEN` = project-scoped. `RAILWAY_API_TOKEN` = account/workspace. |
| Workspace | `--workspace` | `FAT_CONTROLLER_WORKSPACE` | `workspace` | — | Workspace ID or name. Required with account-level tokens. |
| Project | `--project` | `FAT_CONTROLLER_PROJECT` | `project` | — | Project ID or name. Required with account-level tokens. |
| Environment | `--environment` | `FAT_CONTROLLER_ENVIRONMENT` | `environment` | — | Environment name. Required with account-level tokens. |
| Output format | `--output`, `-o` | `FAT_CONTROLLER_OUTPUT` | `output` | `text` | Output format: `text`, `json`, `toml`. |
| Color | `--color` | `FAT_CONTROLLER_COLOR` | `color` | `auto` | Color: `auto`, `always`, `never`. Respects `NO_COLOR`. |
| Timeout | `--timeout` | `FAT_CONTROLLER_TIMEOUT` | `timeout` | `30s` | API request timeout. |
| Confirm | `--confirm` | `FAT_CONTROLLER_CONFIRM` | `confirm` | `false` | Auto-execute mutations (dangerous mode). |
| Dry run | `--dry-run` | `FAT_CONTROLLER_DRY_RUN` | `dry_run` | `false` | Force preview of mutations. |
| Config file | `--config` | `FAT_CONTROLLER_CONFIG` | `config` | `fat-controller.toml` | Railway config file path. Repeatable. |
| Service | `--service` | `FAT_CONTROLLER_SERVICE` | `service` | — | Scope to a single service. |
| Skip deploys | `--skip-deploys` | `FAT_CONTROLLER_SKIP_DEPLOYS` | `skip_deploys` | `false` | Don't trigger redeployments. |
| Fail fast | `--fail-fast` | `FAT_CONTROLLER_FAIL_FAST` | `fail_fast` | `false` | Stop on first error during apply. |
| Show secrets | `--show-secrets` | `FAT_CONTROLLER_SHOW_SECRETS` | `show_secrets` | `false` | Show secret values instead of masking them. |
| Sensitive keywords | — | — | `sensitive_keywords` | *(see below)* | Keywords for detecting sensitive variable names (boundary match). |
| Sensitive allowlist | — | — | `sensitive_allowlist` | *(see below)* | Keywords that suppress false-positive secret matches. |
| Suppress warnings | — | — | `suppress_warnings` | `[]` | List of warning codes to suppress (e.g. `["W012", "W030"]`). |
| Full output | `--full` | — | — | `false` | Include IDs and read-only fields (get only). |
| Verbose | `--verbose`, `-v` | — | — | `false` | Debug output (HTTP requests, timing). |
| Quiet | `--quiet`, `-q` | — | — | `false` | Suppress informational output. |

**Token precedence:** `--token` flag > `RAILWAY_API_TOKEN` env var >
`RAILWAY_TOKEN` env var > stored OAuth credentials (keyring/file).
`RAILWAY_TOKEN` uses the `Project-Access-Token` header (project-scoped).
`RAILWAY_API_TOKEN` uses `Authorization: Bearer` (account/workspace-scoped).

## Workspace, project, and environment resolution

When using an account-level token (`RAILWAY_API_TOKEN` or stored OAuth),
`config get/set/delete` need a workspace, project, and environment. Resolution works as:

1. If `--workspace`/`--project`/`--environment` flags (or env vars) are set, use them.
2. If only one workspace/project/environment exists, auto-select it.
3. If a TTY is attached, show an interactive picker.
4. Otherwise, error with a listing of available options.

Use `workspace list`, `project list`, and `environment list` to discover available options.

### Example: global config file

```toml
# ~/.config/fat-controller/config.toml
# User-wide defaults

output = "json"          # prefer JSON output everywhere
color = "auto"
timeout = "60s"
```

### Example: local config file

```toml
# .fat-controller.toml (in project root, committed)
# Project-specific settings

project = "my-railway-project"
environment = "production"
config = "infra/fat-controller.toml"    # non-default config location
skip_deploys = true                      # batch changes, deploy separately
sensitive_keywords = ["SECRET", "TOKEN", "PASSWORD", "KEY",
  "SIGNING"]                             # replaces all defaults
sensitive_allowlist = ["KEYSTROKE"]       # replaces all defaults
```

## Confirmation mode

All mutations (`set`, `delete`, `apply`) respect the `confirm` setting:

- **Safe mode (default, `confirm = false`):** mutations are dry-run.
  Pass `--confirm` to execute.
- **Dangerous mode (`confirm = true`):** mutations execute immediately.
  Pass `--dry-run` to preview.

This can be set at any level: global config, local config, env var
(`FAT_CONTROLLER_CONFIRM=true`), or CLI flag. Flags always win.

`NO_COLOR` (any value) disables color output regardless of `--color`.
