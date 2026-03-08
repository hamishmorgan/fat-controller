# SSH Command

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement the `ssh` command that opens an interactive shell (or runs a one-shot command) inside a running Railway service via WebSocket.

**Architecture:** Railway exposes SSH access through a WebSocket endpoint. The client connects, pipes stdin/stdout/stderr, and manages terminal raw mode for interactive sessions. Non-interactive mode runs a command and exits.

**Tech Stack:** Go 1.26, Kong CLI, nhooyr.io/websocket (or gorilla/websocket), terminal raw mode via golang.org/x/term.

**Extracted from:** Plan 5 (Imperative Commands), Task 5 — all other tasks in that plan are complete.

---

## Context for the implementer

### Key files

| File | Role |
|------|------|
| `internal/cli/cli.go` | Root CLI struct — `SSH` field needs to be added |
| `internal/railway/operations.graphql` | May need a query to resolve the WebSocket endpoint URL |
| `internal/cli/deploy.go` | Example of a command that resolves services — follow same pattern |

### Railway SSH WebSocket protocol

Railway SSH uses a WebSocket connection to `wss://backboard.railway.com/...`. The exact endpoint and authentication mechanism need to be reverse-engineered from the Railway CLI source or API docs. Key considerations:

- Authentication: likely uses the same bearer token as GraphQL
- Terminal sizing: SIGWINCH handling for resize events
- Signal forwarding: Ctrl+C should send to remote, not kill the client

---

## Task 1: Research Railway SSH WebSocket protocol

**Files:**

- Read-only research task

### Step 1: Examine Railway CLI source

Check the [Railway CLI](https://github.com/railwayapp/cli) for their SSH implementation to understand:

- The exact WebSocket endpoint URL format
- Authentication headers required
- Message framing (binary vs text, any envelope format)
- Terminal resize protocol
- How service/environment targeting works

### Step 2: Document findings

Record the protocol details as comments in the implementation.

---

## Task 2: Implement WebSocket SSH client

**Files:**

- Create: `internal/railway/ssh.go`
- Test: `internal/railway/ssh_test.go`

### Step 1: Add WebSocket dependency

```bash
go get nhooyr.io/websocket
```

### Step 2: Implement SSH client

```go
// SSHOptions configures an SSH session.
type SSHOptions struct {
    ServiceID     string
    EnvironmentID string
    ProjectID     string
    Command       []string // empty = interactive shell
}

// SSH opens a WebSocket SSH session to a Railway service.
func SSH(ctx context.Context, client *Client, opts SSHOptions) error {
    // 1. Build WebSocket URL
    // 2. Connect with auth headers
    // 3. If interactive: set terminal to raw mode, handle resize
    // 4. Pipe stdin → ws, ws → stdout
    // 5. If command: send command, collect output, exit
}
```

### Step 3: Write tests (connection refused / auth error paths)

### Step 4: Commit

---

## Task 3: Wire SSH command into CLI

**Files:**

- Create: `internal/cli/ssh.go`
- Test: `internal/cli/ssh_internal_test.go`

### Step 1: Write tests

```go
func TestSSH_ServiceRequired(t *testing.T) {
    // No service in non-interactive mode → error.
}

func TestSSH_ServicePicker(t *testing.T) {
    // Interactive mode, no service → picker shown.
}
```

### Step 2: Implement CLI command

```go
type SSHCmd struct {
    EnvironmentFlags `kong:"embed"`
    Service          string   `arg:"" optional:"" help:"Service to connect to."`
    Command          []string `arg:"" optional:"" passthrough:"" help:"Command to run (omit for interactive shell)."`
}
```

### Step 3: Register in CLI struct

Add `SSH SSHCmd` to the `CLI` struct in `cli.go`.

### Step 4: Run tests, commit

---

## Verification

```bash
go build ./...
go test ./...
mise check
go run ./cmd/fat-controller ssh --help
```
