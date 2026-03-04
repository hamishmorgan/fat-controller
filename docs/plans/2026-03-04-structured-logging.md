# Structured Logging with log/slog

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace ad-hoc `fmt.Fprintf(os.Stderr, ...)` debug output with `log/slog`, wired to `--verbose` (debug level) and `--quiet` (warn-only), with structured logging throughout the CLI, transport, apply, and config-loading layers.

**Architecture:** Create a `slog.Logger` in `main.go` based on `--verbose`/`--quiet` flags, store it in context via `context.WithValue`, and retrieve it with a helper. All packages pull the logger from context — no globals, no passing `*Globals` into library code. The `TextHandler` writes to `stderr` with no timestamp (clean CLI output).

**Tech Stack:** `log/slog` (stdlib, Go 1.21+), no new dependencies.

---

## Task 1: Add logger package with context helpers

**Files:**

- Create: `internal/logger/logger.go`
- Create: `internal/logger/logger_test.go`

**Step 1: Create `internal/logger/logger.go`**

```go
// Package logger provides context-based slog helpers for the CLI.
package logger

import (
	"context"
	"log/slog"
)

type contextKey struct{}

// NewContext returns a new context carrying the given logger.
func NewContext(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, contextKey{}, l)
}

// From returns the logger stored in ctx, or slog.Default() if none.
func From(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(contextKey{}).(*slog.Logger); ok {
		return l
	}
	return slog.Default()
}
```

**Step 2: Create `internal/logger/logger_test.go`**

```go
package logger_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/logger"
)

func TestFrom_ReturnsDefault_WhenNoLoggerInContext(t *testing.T) {
	l := logger.From(context.Background())
	if l != slog.Default() {
		t.Fatal("expected slog.Default()")
	}
}

func TestNewContext_RoundTrips(t *testing.T) {
	var buf bytes.Buffer
	l := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx := logger.NewContext(context.Background(), l)
	got := logger.From(ctx)
	got.DebugContext(ctx, "hello")
	if !strings.Contains(buf.String(), "hello") {
		t.Fatalf("expected logger to write, got: %s", buf.String())
	}
}
```

**Step 3: Run tests**

Run: `go test ./internal/logger/...`
Expected: PASS

**Step 4: Commit**

```text
feat: add logger package with context-based slog helpers
```

---

## Task 2: Create logger in main.go and inject into context

**Files:**

- Modify: `cmd/fat-controller/main.go`
- Modify: `internal/cli/cli.go`

**Step 1: Add `NewLogger` factory to `internal/cli/cli.go`**

Add after the `TimeoutContext` method on `Globals`:

```go
// Logger returns a slog.Logger configured for the current verbosity level.
// Output goes to stderr with no timestamps for clean CLI output.
func (g *Globals) Logger() *slog.Logger {
	level := slog.LevelInfo
	if g.Verbose {
		level = slog.LevelDebug
	} else if g.Quiet {
		level = slog.LevelWarn
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level:     level,
		AddSource: false,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// Remove timestamp for clean CLI output.
			if a.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return a
		},
	}))
}
```

Add `"log/slog"` and `"os"` to the imports of `cli.go`.

**Step 2: Wire logger into context in `main.go`**

In `main.go`, after `kong.Parse`, before `ctx.Run`, inject the logger into a background context. But since kong calls `Run(globals)` and each command creates its own context, the simplest approach is to set `slog.SetDefault` so that `slog.Default()` (the fallback in `logger.From`) picks it up. Update `main.go`:

```go
func main() {
	applyColorMode()

	var c cli.CLI
	ctx := kong.Parse(&c,
		kong.Name("fat-controller"),
		kong.Description("CLI for managing Railway projects. Pull live config, diff against desired state, apply the difference."),
		kong.Vars{"version": version.String()},
		kong.UsageOnError(),
		kong.Help(cli.ColorHelpPrinter),
	)

	// Configure structured logging based on --verbose / --quiet.
	slog.SetDefault(c.Globals.Logger())

	if err := ctx.Run(&c.Globals); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
```

Add `"log/slog"` to `main.go` imports.

**Step 3: Run build to verify**

Run: `go build ./cmd/fat-controller`
Expected: success

**Step 4: Commit**

```text
feat: configure slog logger from --verbose/--quiet flags
```

---

## Task 3: Replace debug() with slog in CLI layer

**Files:**

- Modify: `internal/cli/output.go`
- Modify: `internal/cli/config_common.go`

**Step 1: Replace `debug()` in `output.go`**

Replace the entire file with a comment pointing to slog:

```go
package cli
```

(Delete the `debug` function entirely — it is no longer needed.)

**Step 2: Replace `debug()` calls in `config_common.go`**

Replace three calls:

```go
debug(globals, "loading config from %s", configDir)
```

→

```go
slog.Debug("loading config", "dir", configDir)
```

```go
debug(globals, "resolving project=%q environment=%q", project, environment)
```

→

```go
slog.Debug("resolving project and environment", "project", project, "environment", environment)
```

```go
debug(globals, "fetching live state for project=%s environment=%s", projID, envID)
```

→

```go
slog.Debug("fetching live state", "project_id", projID, "environment_id", envID)
```

Add `"log/slog"` to imports, remove the unused `// globals` reference if `globals` was only used for `debug()` (it's still used for other fields, so keep the parameter).

**Step 3: Run tests**

Run: `go test ./internal/cli/...`
Expected: PASS

**Step 4: Commit**

```text
refactor: replace ad-hoc debug() with slog.Debug in CLI layer
```

---

## Task 4: Add logging to config loading

**Files:**

- Modify: `internal/config/load.go`

**Step 1: Add slog calls to `LoadConfigs`**

```go
func LoadConfigs(dir string, extraFiles []string) (*DesiredConfig, error) {
	basePath := filepath.Join(dir, BaseConfigFile)
	if _, err := os.Stat(basePath); err != nil {
		// ... existing error handling ...
	}

	var configs []*DesiredConfig

	slog.Debug("loading config file", "path", basePath)
	base, err := ParseFile(basePath)
	// ...

	localPath := filepath.Join(dir, LocalConfigFile)
	if _, err := os.Stat(localPath); err == nil {
		slog.Debug("loading local override", "path", localPath)
		local, err := ParseFile(localPath)
		// ...
	}

	for _, path := range extraFiles {
		slog.Debug("loading extra config", "path", path)
		extra, err := ParseFile(path)
		// ...
	}

	slog.Debug("merged config files", "count", len(configs))
	return Merge(configs...), nil
}
```

Add `"log/slog"` to imports.

**Step 2: Run tests**

Run: `go test ./internal/config/...`
Expected: PASS

**Step 3: Commit**

```text
feat: add structured logging to config file loading
```

---

## Task 5: Add logging to auth resolution

**Files:**

- Modify: `internal/auth/resolver.go`

**Step 1: Add slog calls to `ResolveAuth`**

At each return point, add a debug log:

```go
// 1. --token flag
if flagToken != "" {
	slog.Debug("auth resolved", "source", SourceFlag)
	return &ResolvedAuth{...}, nil
}

// 2. RAILWAY_API_TOKEN env var
if token := os.Getenv("RAILWAY_API_TOKEN"); token != "" {
	slog.Debug("auth resolved", "source", SourceEnvAPIToken)
	return &ResolvedAuth{...}, nil
}

// 3. RAILWAY_TOKEN env var
if token := os.Getenv("RAILWAY_TOKEN"); token != "" {
	slog.Debug("auth resolved", "source", SourceEnvToken)
	return &ResolvedAuth{...}, nil
}

// 4. Stored OAuth token
tokens, err := store.Load()
if err != nil {
	// ... existing error handling ...
}
slog.Debug("auth resolved", "source", SourceStored)
return &ResolvedAuth{...}, nil
```

Add `"log/slog"` to imports.

**Step 2: Run tests**

Run: `go test ./internal/auth/...`
Expected: PASS

**Step 3: Commit**

```text
feat: add structured logging to auth resolution
```

---

## Task 6: Add logging to HTTP transport

**Files:**

- Modify: `internal/railway/transport.go`

**Step 1: Add request/response logging to `RoundTrip`**

```go
func (t *AuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.mu.Lock()
	headerName := t.resolved.HeaderName
	headerValue := t.resolved.HeaderValue
	source := t.resolved.Source
	t.mu.Unlock()

	clone := req.Clone(req.Context())
	clone.Header.Set(headerName, headerValue)

	start := time.Now()
	resp, err := t.base.RoundTrip(clone)
	duration := time.Since(start)
	if err != nil {
		slog.Debug("http request failed", "method", req.Method, "url", req.URL.String(), "error", err, "duration", duration)
		return nil, err
	}

	slog.Debug("http request", "method", req.Method, "url", req.URL.String(), "status", resp.StatusCode, "duration", duration)

	if resp.StatusCode != http.StatusUnauthorized {
		return resp, nil
	}

	if !t.canRefresh(source) {
		return resp, nil
	}

	// ... existing refresh logic ...
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.resolved.HeaderValue != headerValue {
		slog.Debug("token already refreshed by another goroutine, retrying")
		resp.Body.Close()
		retry := req.Clone(req.Context())
		retry.Header.Set(headerName, t.resolved.HeaderValue)
		return t.base.RoundTrip(retry)
	}

	slog.Debug("refreshing expired token")
	newTokens, refreshErr := t.tryRefresh(req.Context())
	if refreshErr != nil {
		// ... existing error handling ...
	}

	slog.Debug("token refreshed successfully")
	// ... existing retry logic ...
}
```

Add `"log/slog"` and `"time"` to imports.

**Step 2: Run tests**

Run: `go test ./internal/railway/...`
Expected: PASS

**Step 3: Commit**

```text
feat: add HTTP request logging to auth transport
```

---

## Task 7: Add logging to Railway resolve and state fetch

**Files:**

- Modify: `internal/railway/resolve.go`
- Modify: `internal/railway/state.go`

**Step 1: Add slog calls to `resolve.go`**

In `ResolveProjectEnvironment`:

```go
slog.Debug("resolving project and environment", "workspace", workspace, "project", project, "environment", environment)
```

In `resolveProjectID` when UUID passthrough:

```go
slog.Debug("project is UUID, skipping resolution", "project_id", project)
```

In `resolveWorkspaceID` after resolution:

```go
slog.Debug("resolved workspace", "name", workspace, "id", *id)
```

In `resolveEnvironmentID` when matched:

```go
slog.Debug("resolved environment", "name", env, "id", edge.Node.Id)
```

**Step 2: Add slog calls to `state.go`**

In `FetchLiveConfig`:

```go
slog.Debug("fetching live config", "project_id", projectID, "environment_id", environmentID, "service_filter", serviceFilter)
```

After fetching shared variables:

```go
slog.Debug("fetched shared variables", "count", len(cfg.Shared))
```

Per service:

```go
slog.Debug("fetched service state", "service", edge.Node.Name, "variables", len(svc.Variables))
```

Add `"log/slog"` to imports in both files.

**Step 3: Run tests**

Run: `go test ./internal/railway/...`
Expected: PASS

**Step 4: Commit**

```text
feat: add structured logging to Railway resolve and state fetch
```

---

## Task 8: Add logging to apply operations

**Files:**

- Modify: `internal/apply/apply.go`

**Step 1: Add slog calls to `Apply` and `applyVariables`**

In `Apply`, at the start:

```go
slog.Debug("starting apply", "services", len(desired.Services))
```

Before settings update:

```go
slog.Debug("updating service settings", "service", name)
```

Before resource update:

```go
slog.Debug("updating service resources", "service", name)
```

In `applyVariables`, before batch upsert:

```go
scope := service
if scope == "" {
	scope = "shared"
}
slog.Debug("upserting variables", "scope", scope, "count", len(batch))
```

Per delete:

```go
slog.Debug("deleting variable", "scope", scope, "key", ch.Key)
```

After all apply operations, before returning from `Apply`:

```go
slog.Debug("apply complete", "applied", result.Applied, "failed", result.Failed)
```

Add `"log/slog"` to imports.

**Step 2: Run tests**

Run: `go test ./internal/apply/...`
Expected: PASS

**Step 3: Commit**

```text
feat: add structured logging to apply operations
```

---

## Task 9: Update help text and docs

**Files:**

- Modify: `internal/cli/cli.go` (help text)
- Modify: `docs/TODO.md` (mark done if listed)

**Step 1: Update `Verbose` help text**

Change:

```go
Verbose bool `help:"Debug output (HTTP requests, timing)." short:"v"`
```

To:

```go
Verbose bool `help:"Enable debug logging (config loading, auth, HTTP requests, apply operations)." short:"v"`
```

**Step 2: Update `Quiet` help text**

Change:

```go
Quiet bool `help:"Suppress informational output." short:"q"`
```

To:

```go
Quiet bool `help:"Suppress informational and debug output (warnings and errors only)." short:"q"`
```

**Step 3: Regenerate CLI docs**

Run: `mise run docs:cli`

**Step 4: Run full check**

Run: `mise run check`
Expected: all pass

**Step 5: Commit**

```text
docs: update --verbose/--quiet help text and regenerate CLI docs
```

---

## Task 10: Clean up empty output.go

**Files:**

- Delete or repurpose: `internal/cli/output.go`

If `output.go` is now an empty package declaration, either delete it (if no other functions exist there) or leave it as a future home for output helpers. If deleted, ensure the build still passes.

Run: `mise run check`
Expected: all pass

**Step 6: Commit**

```text
chore: remove empty output.go after slog migration
```
