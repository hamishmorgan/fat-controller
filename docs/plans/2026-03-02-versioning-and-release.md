# Versioning, Build, and Release Pipeline

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Embed build-time version info in the binary (`--version` flag), configure goreleaser for multi-platform builds, and add a tag-triggered GitHub Actions release workflow that creates GitHub Releases with binaries.

**Architecture:** Version info is injected via Go `ldflags` at build time — three variables (`version`, `commit`, `date`) set on a `version` package. Kong's built-in `VersionFlag` displays this. Goreleaser handles cross-compilation, checksums, and GitHub Release creation. A new CI workflow triggers on `v*` tags and runs goreleaser. The local `mise run build` task is updated to inject version info from `git describe`.

**Tech Stack:** Go ldflags, kong `VersionFlag`, goreleaser, GitHub Actions

---

## Context for the implementor

### How the build works today

- `mise run build` runs `go build -o .build/fat-controller ./cmd/fat-controller` — no ldflags, no version injection.
- `main.go` uses `kong.Parse()` with `kong.Name("fat-controller")` and `kong.Description(...)`. No `kong.Vars` map.
- The `CLI` struct in `internal/cli/cli.go` has `Globals` embedded. There is no `Version` field.
- Kong has a built-in `VersionFlag` type (in `util.go`). When added to a struct as `Version kong.VersionFlag`, it reads from `kong.Vars{"version": "..."}` and prints it before exiting.
- CI workflows use `jdx/mise-action@v2` to install mise/Go. Existing patterns: `ubuntu-latest`, `actions/checkout@v4`, `permissions: contents: read`.
- `.build/` is gitignored. Goreleaser uses `dist/` by default — also needs gitignoring.
- There are no git tags yet.

### Kong `VersionFlag` usage pattern

```go
// In the CLI struct:
type CLI struct {
    Globals `kong:"embed"`
    Version kong.VersionFlag `help:"Print version." short:"V"`
    // ...subcommands...
}

// In main.go:
kong.Parse(&c,
    kong.Vars{"version": version.String()},
    // ...other options...
)
```

When the user runs `fat-controller --version` or `fat-controller -V`, kong prints the version string and exits with code 0.

### Goreleaser conventions

- Config file: `.goreleaser.yaml` at repo root.
- Goreleaser sets ldflags automatically via `.Version`, `.Commit`, `.Date` template vars.
- Binary name, main path, and archive format are configured in the YAML.
- `goreleaser release` is run in CI; `goreleaser build --snapshot` for local testing.

### Files you'll create or modify

| File | Action |
|------|--------|
| `internal/version/version.go` | **Create** — version variables + `String()` formatter |
| `internal/version/version_test.go` | **Create** — tests |
| `internal/cli/cli.go` | **Modify** — add `Version kong.VersionFlag` to `CLI` struct |
| `internal/cli/cli_test.go` | **Modify** — update help printer test (new `--version` flag) |
| `cmd/fat-controller/main.go` | **Modify** — add `kong.Vars{"version": ...}` |
| `.goreleaser.yaml` | **Create** — goreleaser config |
| `.github/workflows/release.yml` | **Create** — tag-triggered release workflow |
| `.config/mise/config.toml` | **Modify** — inject version via ldflags in build task |
| `.gitignore` | **Modify** — add `dist/` |

### Hazards

- Kong's `VersionFlag` uses `BeforeReset`, which runs before command parsing. This means `--version` works even without a subcommand.
- The `TestColorHelpPrinter_AllLeafCommands` test traces all leaf commands. Adding `Version` to `CLI` adds a `--version` global flag — this should not break existing tests since it's a flag, not a command. But verify.
- Goreleaser expects the `dist/` directory to be clean. It must be gitignored.
- The release workflow needs `contents: write` permission to create GitHub Releases and upload assets.
- `mise run build` should inject version from `git describe --tags --always --dirty` for local dev builds. If no tags exist yet, it falls back to the commit hash (e.g., `dev-abc1234`).

---

## Task 1: Create version package

**Files:**

- Create: `internal/version/version.go`
- Create: `internal/version/version_test.go`

**Step 1: Write the failing tests**

Create `internal/version/version_test.go`:

```go
package version_test

import (
	"strings"
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/version"
)

func TestString_Defaults(t *testing.T) {
	// With no ldflags, defaults should be used.
	got := version.String()
	if got == "" {
		t.Fatal("String() should not be empty")
	}
	if !strings.Contains(got, "dev") {
		t.Errorf("default version should contain 'dev', got %q", got)
	}
}

func TestString_Format(t *testing.T) {
	// The output should be a single line with version, commit, date.
	got := version.String()
	// Should not contain newlines.
	if strings.Contains(got, "\n") {
		t.Errorf("String() should be single line, got %q", got)
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/version/... -v`
Expected: FAIL — package doesn't exist.

**Step 3: Write the implementation**

Create `internal/version/version.go`:

```go
// Package version holds build-time version information set via ldflags.
//
// These variables are populated by goreleaser or the local build task:
//
//	go build -ldflags "-X github.com/hamishmorgan/fat-controller/internal/version.version=v1.0.0 ..."
package version

import "fmt"

// Set by ldflags at build time.
var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

// String returns a human-readable version string.
func String() string {
	return fmt.Sprintf("%s (commit %s, built %s)", version, commit, date)
}

// Version returns the semantic version (e.g. "v1.0.0" or "dev").
func Version() string {
	return version
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/version/... -v`
Expected: all PASS.

**Step 5: Commit**

```bash
git add internal/version/version.go internal/version/version_test.go
git commit -m "feat(version): add build-time version package with ldflags support"
```

---

## Task 2: Wire version into CLI

**Files:**

- Modify: `internal/cli/cli.go`
- Modify: `cmd/fat-controller/main.go`

**Step 1: Add `Version` field to `CLI` struct**

In `internal/cli/cli.go`, add to the `CLI` struct after the `Globals` embed:

```go
type CLI struct {
	Globals `kong:"embed"`

	Version kong.VersionFlag `help:"Print version." short:"V"`

	// Subcommand groups
	Auth        AuthCmd        `cmd:"" help:"Manage authentication."`
	Config      ConfigCmd      `cmd:"" name:"config" help:"Declarative configuration management."`
	Project     ProjectCmd     `cmd:"" help:"Manage projects."`
	Environment EnvironmentCmd `cmd:"" help:"Manage environments."`
	Workspace   WorkspaceCmd   `cmd:"" help:"Manage workspaces."`
}
```

This requires adding `"github.com/alecthomas/kong"` to the import block in `cli.go`.

**Step 2: Pass version to kong in `main.go`**

In `cmd/fat-controller/main.go`, add the import and version variable:

```go
import (
	"fmt"
	"os"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/charmbracelet/lipgloss"
	"github.com/hamishmorgan/fat-controller/internal/cli"
	"github.com/hamishmorgan/fat-controller/internal/version"
	"github.com/muesli/termenv"
)
```

Update `kong.Parse` to include the version:

```go
	ctx := kong.Parse(&c,
		kong.Name("fat-controller"),
		kong.Description("CLI for managing Railway projects. Pull live config, diff against desired state, apply the difference."),
		kong.Vars{"version": version.String()},
		kong.UsageOnError(),
		kong.Help(cli.ColorHelpPrinter),
	)
```

**Step 3: Run tests to verify nothing breaks**

Run: `go test ./internal/cli/... -v`
Expected: all PASS (the `VersionFlag` is a global flag, not a command — it doesn't affect leaf command enumeration).

**Step 4: Build and smoke test**

Run:

```bash
mise run build
.build/fat-controller --version
```

Expected: output like `dev (commit unknown, built unknown)`.

Run:

```bash
.build/fat-controller -V
```

Expected: same output (short flag).

**Step 5: Commit**

```bash
git add internal/cli/cli.go cmd/fat-controller/main.go
git commit -m "feat(cli): add --version flag wired to build-time version info"
```

---

## Task 3: Update mise build task to inject version

**Files:**

- Modify: `.config/mise/config.toml`

**Step 1: Update the build task**

Replace the `[tasks.build]` section in `.config/mise/config.toml`:

```toml
[tasks.build]
description = "Build Go binary"
run = """
[ -f go.mod ] || exit 0
mkdir -p .build
VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS="-s -w -X github.com/hamishmorgan/fat-controller/internal/version.version=${VERSION} -X github.com/hamishmorgan/fat-controller/internal/version.commit=${COMMIT} -X github.com/hamishmorgan/fat-controller/internal/version.date=${DATE}"
go build -ldflags "${LDFLAGS}" -o .build/fat-controller ./cmd/fat-controller
"""
```

**Step 2: Build and verify**

Run:

```bash
mise run build
.build/fat-controller --version
```

Expected: output like `e8bc652 (commit e8bc652, built 2026-03-02T21:00:00Z)` (commit hash since no tags exist yet).

**Step 3: Commit**

```bash
git add .config/mise/config.toml
git commit -m "build: inject version info via ldflags in mise build task"
```

---

## Task 4: Add goreleaser configuration

**Files:**

- Create: `.goreleaser.yaml`
- Modify: `.gitignore`

**Step 1: Add `dist/` to `.gitignore`**

Append to `.gitignore` after the `.build/` line:

```text
dist/
```

**Step 2: Create `.goreleaser.yaml`**

Create `.goreleaser.yaml` at repo root:

```yaml
# yaml-language-server: $schema=https://goreleaser.com/static/schema.json
version: 2

before:
  hooks:
    - go mod tidy

builds:
  - main: ./cmd/fat-controller
    binary: fat-controller
    env:
      - CGO_ENABLED=0
    goos:
      - linux
      - darwin
      - windows
    goarch:
      - amd64
      - arm64
    ldflags:
      - -s -w
      - -X github.com/hamishmorgan/fat-controller/internal/version.version={{.Version}}
      - -X github.com/hamishmorgan/fat-controller/internal/version.commit={{.Commit}}
      - -X github.com/hamishmorgan/fat-controller/internal/version.date={{.Date}}

archives:
  - formats:
      - tar.gz
    format_overrides:
      - goos: windows
        formats:
          - zip
    name_template: >-
      {{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}

checksum:
  name_template: "checksums.txt"

changelog:
  sort: asc
  filters:
    exclude:
      - "^docs:"
      - "^chore:"
      - "^test:"
      - "^ci:"

release:
  github:
    owner: hamishmorgan
    name: fat-controller
```

**Step 3: Verify the config locally**

Run:

```bash
go install github.com/goreleaser/goreleaser/v2@latest
goreleaser check
```

Expected: no errors.

Optionally, test a snapshot build:

```bash
goreleaser build --snapshot --clean
```

Expected: binaries in `dist/` for all platform/arch combinations.

**Step 4: Commit**

```bash
git add .goreleaser.yaml .gitignore
git commit -m "build: add goreleaser config for multi-platform releases"
```

---

## Task 5: Add tag-triggered release workflow

**Files:**

- Create: `.github/workflows/release.yml`

**Step 1: Create the workflow**

Create `.github/workflows/release.yml`:

```yaml
name: Release

on:
  push:
    tags:
      - "v*"

permissions:
  contents: write

jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - uses: jdx/mise-action@v2

      - uses: goreleaser/goreleaser-action@v6
        with:
          version: "~> v2"
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

**Step 2: Validate with actionlint**

Run:

```bash
actionlint
```

Expected: no errors.

**Step 3: Commit**

```bash
git add .github/workflows/release.yml
git commit -m "ci: add tag-triggered release workflow with goreleaser"
```

---

## Task 6: Final verification

**Step 1: Run the full check suite**

Run:

```bash
mise run check
```

Expected: all linters pass, all tests pass, build succeeds.

**Step 2: Verify version injection end-to-end**

Run:

```bash
mise run build
.build/fat-controller --version
.build/fat-controller -V
```

Expected: version string with commit hash and date.

**Step 3: Verify help output includes version flag**

Run:

```bash
.build/fat-controller --help
```

Expected: `--version` / `-V` flag visible in global flags section.

**Step 4: Commit any lint fixes**

If `mise run check` required any fixes:

```bash
git add -A
git commit -m "chore: fix lint issues from versioning implementation"
```

---

## Post-implementation: Creating a release

Once this is all merged, create the first release:

```bash
git tag v0.1.0
git push origin v0.1.0
```

This triggers the release workflow, which:

1. Checks out the tag with full history
2. Installs Go via mise
3. Runs goreleaser, which:
   - Cross-compiles for linux/darwin/windows × amd64/arm64
   - Injects `v0.1.0`, commit hash, and date via ldflags
   - Creates archives (`.tar.gz` for unix, `.zip` for windows)
   - Generates `checksums.txt`
   - Creates a GitHub Release with changelog and attached binaries

### Future enhancements (not in scope)

- **Homebrew tap** — goreleaser can auto-generate Homebrew formulas. Requires a separate tap repo.
- **Docker images** — goreleaser supports Docker builds. Add when containerized deployment is needed.
- **Changelog generation** — consider `git-cliff` or similar for structured changelogs.
- **Release candidates** — goreleaser supports `v1.0.0-rc.1` tags with prerelease flag.
