```text
  ┌─────────┐
  │         │
┌─┴─────────┴─┐
╞══════════════╡
  __       _                        _             _ _
 / _| __ _| |_       ___ ___  _ __ | |_ _ __ ___ | | | ___ _ __
| |_ / _` | __|____ / __/ _ \| '_ \| __| '__/ _ \| | |/ _ \ '__|
|  _| (_| | ||_____| (_| (_) | | | | |_| | | (_) | | |  __/ |
|_|  \__,_|\__|     \___\___/|_| |_|\__|_|  \___/|_|_|\___|_|
```

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
