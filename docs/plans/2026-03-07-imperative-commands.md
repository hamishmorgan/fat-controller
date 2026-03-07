# Imperative Commands

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement the operational commands that don't touch config: `deploy`, `redeploy`, `restart`, `rollback`, `stop`, `logs`, `status`, `ssh`, and `open`.

**Architecture:** Imperative commands act on live Railway state. They accept service arguments to narrow scope (no service = all services in the environment). `logs` streams by default; switches to fetch mode with `--lines`/`--since`/`--until`. `ssh` opens a WebSocket shell. `open` launches the Railway dashboard in a browser.

**Tech Stack:** Go 1.26, Kong CLI, Railway GraphQL API, WebSocket (gorilla/websocket or nhooyr.io/websocket for SSH), charmbracelet/bubbletea (optional for log streaming TUI).

**Depends on:** Plan 3 (Command Restructure) and Plan 4 (GraphQL Operations).

---

## Context for the implementer

### Key files

| File | Role |
|------|------|
| `internal/cli/cli.go` | Root CLI struct (commands added in Plan 3) |
| `internal/railway/operations.graphql` | GraphQL operations (expanded in Plan 4) |
| `internal/railway/mutate.go` | Mutation wrappers |
| `internal/auth/login.go` | Browser opening (reuse for `open` command) |

### Railway API capabilities

All deployment lifecycle operations are available:

- `serviceInstanceDeploy(envId, serviceId)` — trigger deploy
- `deploymentRedeploy(id)` — redeploy current image
- `deploymentRestart(id)` — restart running deployments
- `deploymentRollback(id)` — rollback to previous
- `deploymentCancel(id)` — cancel in-progress
- Log queries: `deploymentLogs`, `buildLogs`, `environmentLogs`
- WebSocket endpoint for SSH: `wss://backboard.railway.com/...`

---

## Task 1: Implement `deploy` command

**Files:**

- Create: `internal/cli/deploy.go`
- Test: `internal/cli/deploy_test.go`

### Step 1: Write tests

```go
func TestDeploy_AllServices(t *testing.T) {
	// No service args → deploys all services in environment.
}

func TestDeploy_SpecificServices(t *testing.T) {
	// deploy api worker → deploys only api and worker.
}

func TestDeploy_DryRun(t *testing.T) {
	// --dry-run → preview only, no mutations.
}

func TestDeploy_Confirmation(t *testing.T) {
	// Interactive → shows preview, asks confirmation.
	// --yes → skips confirmation.
}
```

### Step 2: Implement

```go
type DeployCmd struct {
	EnvironmentFlags `kong:"embed"`
	MutationFlags    `kong:"embed"`
	Services         []string `arg:"" optional:"" help:"Services to deploy (default: all)."`
}

func (c *DeployCmd) Run(globals *Globals) error {
	// 1. Resolve context (workspace/project/environment)
	// 2. If no services specified, fetch all service IDs
	// 3. Confirm (unless --yes or --dry-run)
	// 4. For each service: call TriggerDeploy
	// 5. Report results
}
```

### Step 3: Run tests — expect pass

### Step 4: Commit

---

## Task 2: Implement `redeploy`, `restart`, `rollback`, `stop`

These follow the same pattern as `deploy` but call different mutations.
Factor out a common `deploymentAction` helper.

**Files:**

- Create: `internal/cli/deployment_actions.go`
- Test: `internal/cli/deployment_actions_test.go`

### Step 1: Write tests for each command

### Step 2: Implement shared helper

```go
// deploymentAction handles the common pattern: resolve services,
// find latest deployment for each, call the action mutation.
type deploymentAction struct {
	name   string // "redeploy", "restart", "rollback", "stop"
	action func(ctx context.Context, client *railway.Client, deploymentID string) error
}
```

### Step 3: Wire each command

```go
type RedeployCmd struct { /* same shape as DeployCmd */ }
type RestartCmd  struct { /* same shape as DeployCmd */ }
type RollbackCmd struct { /* same shape as DeployCmd */ }
type StopCmd     struct { /* same shape as DeployCmd */ }
```

### Step 4: Run tests — expect pass

### Step 5: Commit

---

## Task 3: Implement `logs` command

**Files:**

- Create: `internal/cli/logs.go`
- Test: `internal/cli/logs_test.go`

### Step 1: Write tests

```go
func TestLogs_FetchMode(t *testing.T) {
	// --lines 100 → fetches 100 lines, no streaming.
}

func TestLogs_SinceUntil(t *testing.T) {
	// --since 5m → fetches logs from 5 minutes ago.
}

func TestLogs_BuildLogs(t *testing.T) {
	// --build → fetches build logs instead of deploy logs.
}
```

### Step 2: Implement

```go
type LogsCmd struct {
	EnvironmentFlags `kong:"embed"`
	Services         []string `arg:"" optional:""`
	Build            bool     `help:"Show build logs." short:"b"`
	Deploy           bool     `help:"Show deploy logs." short:"d"`
	Lines            *int     `help:"Fetch N lines (disables streaming)." short:"n"`
	Since            string   `help:"Start time: relative (5m, 2h) or ISO 8601." short:"S"`
	Until            string   `help:"End time." short:"U"`
	Filter           string   `help:"Filter expression." short:"f"`
}
```

**Streaming mode** (default when no `--lines`/`--since`/`--until`):
Uses Railway's log subscription WebSocket or polling.

**Fetch mode** (when any time/count flag is set):
Uses `deploymentLogs` / `buildLogs` / `environmentLogs` queries.

### Step 3: Implement time parsing

Support relative durations (`5m`, `2h`, `1d`) and ISO 8601 timestamps.

### Step 4: Run tests — expect pass

### Step 5: Commit

---

## Task 4: Implement `status` command

**Files:**

- Create: `internal/cli/status.go`
- Test: `internal/cli/status_test.go`

### Step 1: Write tests

### Step 2: Implement

```go
type StatusCmd struct {
	EnvironmentFlags `kong:"embed"`
	Services         []string `arg:"" optional:""`
}
```

Fetches and displays:

- Deployment state per service (deploying, running, crashed, etc.)
- Domain verification and certificate status
- Volume state
- Healthcheck results
- Actionable problems (e.g., DNS not propagated + required CNAME)

### Step 3: Run tests — expect pass

### Step 4: Commit

---

## Task 5: Implement `ssh` command

**Files:**

- Create: `internal/cli/ssh.go`
- Create: `internal/railway/ssh.go` (WebSocket client)
- Test: `internal/cli/ssh_test.go`

### Step 1: Write tests

```go
func TestSSH_ServiceRequired(t *testing.T) {
	// No service in non-interactive mode → error.
}

func TestSSH_ServicePicker(t *testing.T) {
	// Interactive mode, no service → picker shown.
}
```

### Step 2: Implement WebSocket SSH client

Railway SSH uses a WebSocket connection. The client:

1. Connects to Railway's WebSocket endpoint
2. Sets up stdin/stdout piping
3. Handles terminal raw mode (if interactive)
4. Runs optional command or opens shell

### Step 3: Wire into CLI

```go
type SSHCmd struct {
	EnvironmentFlags `kong:"embed"`
	Service          string   `arg:"" optional:"" help:"Service to connect to."`
	Command          []string `arg:"" optional:"" help:"Command to run."`
}
```

### Step 4: Run tests — expect pass

### Step 5: Commit

---

## Task 6: Implement `open` command

**Files:**

- Create: `internal/cli/open.go`
- Test: `internal/cli/open_test.go`

### Step 1: Write tests

```go
func TestOpen_PrintURL(t *testing.T) {
	// --print → outputs URL to stdout instead of opening browser.
}

func TestOpen_URLFormat(t *testing.T) {
	// Verify URL format: https://railway.com/project/<id>/environment/<id>
}
```

### Step 2: Implement

```go
type OpenCmd struct {
	EnvironmentFlags `kong:"embed"`
	Print            bool `help:"Print URL instead of opening browser." short:"p"`
}

func (c *OpenCmd) Run(globals *Globals) error {
	// 1. Resolve context
	// 2. Build URL: https://railway.com/project/{projectID}/environment/{envID}
	// 3. If --print, write to stdout
	// 4. Else, open browser (reuse auth.OpenBrowser or use pkg/browser)
}
```

### Step 3: Run tests — expect pass

### Step 4: Commit

---

## Task 7: Final verification

### Step 1: Run `mise run check`

### Step 2: Run `go test -race ./...`

### Step 3: Manual smoke test

```bash
go run ./cmd/fat-controller deploy --help
go run ./cmd/fat-controller logs --help
go run ./cmd/fat-controller status --help
go run ./cmd/fat-controller ssh --help
go run ./cmd/fat-controller open --help
```
