# Configuration and Storage

## File locations (XDG-compliant via adrg/xdg)

| Path | Purpose | Example (Linux) |
|------|---------|-----------------|
| `$XDG_CONFIG_HOME/fat-controller/config.toml` | User preferences, defaults | `~/.config/fat-controller/config.toml` |
| `$XDG_CONFIG_HOME/fat-controller/auth.json` | Token fallback (mode 0600, used when keyring unavailable) | `~/.config/fat-controller/auth.json` |
| `$XDG_STATE_HOME/fat-controller/` | Logs, command history | `~/.local/state/fat-controller/` |
| `$XDG_CACHE_HOME/fat-controller/` | Cached schema, etc. | `~/.cache/fat-controller/` |
| `.fat-controller.toml` | Project-level config overrides (in working dir or git root) | `.fat-controller.toml` |

## Token storage

OAuth tokens are stored using a keyring-first strategy (same pattern as
the `gh` CLI):

1. **`--token` flag or env vars** — highest priority. `RAILWAY_TOKEN`
   (project-scoped) or `RAILWAY_API_TOKEN` (account/workspace-scoped).
2. **OS keyring** — primary persistent storage. Encrypted at rest by the OS.
   Service name: `fat-controller`, key: `oauth-token`.
3. **Fallback file** — `$XDG_CONFIG_HOME/fat-controller/auth.json` with mode
   0600. Used when no keyring daemon is available (headless, SSH, containers).
   A warning is printed when falling back to plaintext storage.

## Config loading

All settings (see the settings table in [COMMANDS.md](COMMANDS.md)) can be
specified at five levels. Each level is loaded in order, later values
override earlier ones:

1. Compiled-in defaults
2. Global config — `$XDG_CONFIG_HOME/fat-controller/config.toml`
3. Local config — `.fat-controller.toml` in working directory or git root
4. Environment variables — `FAT_CONTROLLER_*` / `RAILWAY_TOKEN` / `RAILWAY_API_TOKEN`
5. CLI flags
