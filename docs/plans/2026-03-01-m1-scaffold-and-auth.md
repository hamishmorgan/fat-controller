# M1: Scaffold + Auth — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Bootstrap the fat-controller CLI with kong (struct-based CLI), XDG-compliant config/token storage, and a working `auth login/logout/status` flow using Railway's OAuth 2.0 + PKCE.

**Architecture:** Kong struct-based CLI with two subcommand groups (`auth`, `config` stubs). Global flags (token, project, environment, output, etc.) on root struct, passed to commands via a `Globals` type. Token storage uses OS keyring (go-keyring) with XDG file fallback. OAuth uses Railway's dynamic client registration + PKCE authorization code flow. Config stubs are wired up but return "not yet implemented" — implementation is M3+.

**Tech Stack:** Go, alecthomas/kong, adrg/xdg, zalando/go-keyring

---

## Background: Railway OAuth 2.0

Railway exposes a standard OAuth 2.0 + OIDC system. Key details:

- **OIDC discovery:** `https://backboard.railway.com/oauth/.well-known/openid-configuration`
- **Authorization endpoint:** `https://backboard.railway.com/oauth/auth`
- **Token endpoint:** `https://backboard.railway.com/oauth/token`
- **Dynamic registration:** `POST https://backboard.railway.com/oauth/register`
- **Userinfo:** `https://backboard.railway.com/oauth/me`
- **GraphQL:** `https://backboard.railway.com/graphql/v2`
- **Only `code` response type**, only `S256` PKCE, supports `none` auth method (public/native clients)
- **Scopes we need:** `openid email profile offline_access`
- **Access token TTL:** 1 hour. Refresh tokens rotate — always store the latest.
- **Native clients:** No client secret. `client_id` goes in POST body, not in header.
- **Redirect URI:** Must use `http://127.0.0.1:<port>/callback` (not `localhost`)
- **`prompt=consent`** is required to receive a refresh token with `offline_access`

## Background: Token Resolution Order

1. `--token` flag — highest priority, one-off use
2. `RAILWAY_API_TOKEN` env var — account/workspace-scoped, `Authorization: Bearer` header
3. `RAILWAY_TOKEN` env var — project-scoped, `Project-Access-Token` header
4. OS keyring / fallback file — stored OAuth credentials from `auth login`

Project access tokens use `Project-Access-Token` header. Account-level tokens use `Authorization: Bearer`.

## Background: Kong CLI Framework

Kong is a struct-based CLI parser. Key patterns:

- Root CLI struct: global flags as fields, subcommands as `cmd:""` tagged struct fields
- Leaf commands implement `Run(globals *Globals) error`
- `ctx.Run(&globals)` dispatches to the selected command's `Run()` method
- `env:"VAR"` tag binds a flag to an environment variable
- Slice fields (`[]string`) create repeatable flags
- Boolean flags: `--flag` sets true, `--flag=false` sets false, `negatable:""` creates `--no-flag`
- Field name `DryRun` becomes `--dry-run` on the CLI

---

### Task 1: Go module + dependencies

**Files:**

- Create: `go.mod`
- Create: `go.sum`

**Step 1: Initialize Go module**

Run:

```bash
go mod init github.com/hamishmorgan/fat-controller
```

Expected: `go.mod` created.

**Step 2: Add dependencies**

Run:

```bash
go get github.com/alecthomas/kong@latest
go get github.com/adrg/xdg@latest
go get github.com/zalando/go-keyring@latest
```

**Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "Initialize Go module with CLI and auth dependencies"
```

---

### Task 2: Kong root CLI struct + main.go

**Files:**

- Create: `main.go`
- Create: `cmd/cli.go`

This task sets up the CLI skeleton with global flags and empty command
groups. All global flags from the settings table in `docs/COMMANDS.md` are
defined here. Subcommand structs are stubs (no `Run()` methods yet).

**Step 1: Write cmd/cli.go**

`cmd/cli.go`:

```go
package cmd

import "github.com/alecthomas/kong"

// Globals holds values that are available to every command's Run() method.
type Globals struct {
	Token       string
	Project     string
	Environment string
	Output      string
	Color       string
	Timeout     string
	Confirm     bool
	DryRun      bool
	ShowSecrets bool
	SkipDeploys bool
	FailFast    bool
	Config      []string
	Service     string
	Full        bool
	Verbose     bool
	Quiet       bool
}

// CLI is the root struct for the kong CLI parser.
// Global flags are defined here; subcommand groups are nested structs.
type CLI struct {
	// Global flags
	Token       string   `help:"Auth token (overrides all other auth)." env:"RAILWAY_TOKEN"`
	Project     string   `help:"Project ID or name." env:"FAT_CONTROLLER_PROJECT"`
	Environment string   `help:"Environment name." env:"FAT_CONTROLLER_ENVIRONMENT"`
	Output      string   `help:"Output format: text, json, toml." enum:"text,json,toml" default:"text" short:"o" env:"FAT_CONTROLLER_OUTPUT"`
	Color       string   `help:"Color mode: auto, always, never." enum:"auto,always,never" default:"auto" env:"FAT_CONTROLLER_COLOR"`
	Timeout     string   `help:"API request timeout." default:"30s" env:"FAT_CONTROLLER_TIMEOUT"`
	Confirm     bool     `help:"Auto-execute mutations (skip confirmation)." env:"FAT_CONTROLLER_CONFIRM"`
	DryRun      bool     `help:"Force preview of mutations." name:"dry-run" env:"FAT_CONTROLLER_DRY_RUN"`
	ConfigFiles []string `help:"Railway config file paths. Repeatable." name:"config" short:"c" env:"FAT_CONTROLLER_CONFIG" sep:"none"`
	Service     string   `help:"Scope to a single service." env:"FAT_CONTROLLER_SERVICE"`
	SkipDeploys bool     `help:"Don't trigger redeployments." name:"skip-deploys" env:"FAT_CONTROLLER_SKIP_DEPLOYS"`
	FailFast    bool     `help:"Stop on first error during apply." name:"fail-fast" env:"FAT_CONTROLLER_FAIL_FAST"`
	ShowSecrets bool     `help:"Show secret values instead of masking." name:"show-secrets" env:"FAT_CONTROLLER_SHOW_SECRETS"`
	Full        bool     `help:"Include IDs and read-only fields (get only)."`
	Verbose     bool     `help:"Debug output (HTTP requests, timing)." short:"v"`
	Quiet       bool     `help:"Suppress informational output." short:"q"`

	// Subcommand groups
	Auth   AuthCmd   `cmd:"" help:"Manage authentication."`
	Config ConfigCmd `cmd:"" name:"config" help:"Declarative configuration management."`
}

// AuthCmd is the `auth` command group.
type AuthCmd struct {
	Login  AuthLoginCmd  `cmd:"" help:"Log in to Railway via browser-based OAuth."`
	Logout AuthLogoutCmd `cmd:"" help:"Clear stored credentials."`
	Status AuthStatusCmd `cmd:"" help:"Show current authentication status."`
}

// AuthLoginCmd implements `auth login`.
type AuthLoginCmd struct{}

// AuthLogoutCmd implements `auth logout`.
type AuthLogoutCmd struct{}

// AuthStatusCmd implements `auth status`.
type AuthStatusCmd struct{}

// ConfigCmd is the `config` command group.
type ConfigCmd struct {
	Get      ConfigGetCmd      `cmd:"" help:"Fetch live config from Railway."`
	Set      ConfigSetCmd      `cmd:"" help:"Set a single value by dot-path."`
	Delete   ConfigDeleteCmd   `cmd:"" help:"Delete a single value by dot-path."`
	Diff     ConfigDiffCmd     `cmd:"" help:"Compare local config against live state."`
	Apply    ConfigApplyCmd    `cmd:"" help:"Push configuration changes to Railway."`
	Validate ConfigValidateCmd `cmd:"" help:"Check config file for warnings (no API calls)."`
}

// Config subcommand stubs — implemented in M3+.
type ConfigGetCmd struct {
	Path string `arg:"" optional:"" help:"Dot-path to fetch (e.g. api.variables.PORT). Omit for all."`
}

type ConfigSetCmd struct {
	Path  string `arg:"" required:"" help:"Dot-path to set (e.g. api.variables.PORT)."`
	Value string `arg:"" required:"" help:"Value to set."`
}

type ConfigDeleteCmd struct {
	Path string `arg:"" required:"" help:"Dot-path to delete (e.g. api.variables.OLD)."`
}

type ConfigDiffCmd struct{}
type ConfigApplyCmd struct{}
type ConfigValidateCmd struct{}
```

**Step 2: Write main.go**

`main.go`:

```go
package main

import (
	"fmt"
	"os"

	"github.com/alecthomas/kong"
	"github.com/hamishmorgan/fat-controller/cmd"
)

func main() {
	var cli cmd.CLI
	ctx := kong.Parse(&cli,
		kong.Name("fat-controller"),
		kong.Description("CLI for managing Railway projects. Pull live config, diff against desired state, apply the difference."),
		kong.UsageOnError(),
	)

	globals := &cmd.Globals{
		Token:       cli.Token,
		Project:     cli.Project,
		Environment: cli.Environment,
		Output:      cli.Output,
		Color:       cli.Color,
		Timeout:     cli.Timeout,
		Confirm:     cli.Confirm,
		DryRun:      cli.DryRun,
		ShowSecrets: cli.ShowSecrets,
		SkipDeploys: cli.SkipDeploys,
		FailFast:    cli.FailFast,
		Config:      cli.ConfigFiles,
		Service:     cli.Service,
		Full:        cli.Full,
		Verbose:     cli.Verbose,
		Quiet:       cli.Quiet,
	}

	if err := ctx.Run(globals); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
```

**Step 3: Add stub Run() methods for all leaf commands**

Still in `cmd/cli.go`, add at the bottom:

```go
func (c *AuthLoginCmd) Run(globals *Globals) error {
	fmt.Println("auth login: not yet implemented")
	return nil
}

func (c *AuthLogoutCmd) Run(globals *Globals) error {
	fmt.Println("auth logout: not yet implemented")
	return nil
}

func (c *AuthStatusCmd) Run(globals *Globals) error {
	fmt.Println("auth status: not yet implemented")
	return nil
}

func (c *ConfigGetCmd) Run(globals *Globals) error {
	fmt.Println("config get: not yet implemented")
	return nil
}

func (c *ConfigSetCmd) Run(globals *Globals) error {
	fmt.Println("config set: not yet implemented")
	return nil
}

func (c *ConfigDeleteCmd) Run(globals *Globals) error {
	fmt.Println("config delete: not yet implemented")
	return nil
}

func (c *ConfigDiffCmd) Run(globals *Globals) error {
	fmt.Println("config diff: not yet implemented")
	return nil
}

func (c *ConfigApplyCmd) Run(globals *Globals) error {
	fmt.Println("config apply: not yet implemented")
	return nil
}

func (c *ConfigValidateCmd) Run(globals *Globals) error {
	fmt.Println("config validate: not yet implemented")
	return nil
}
```

Note: also add `"fmt"` to the imports in `cmd/cli.go`.

**Step 4: Verify it builds and runs**

Run:

```bash
go build -o fat-controller . && ./fat-controller --help
```

Expected: Help output showing global flags and `auth`, `config` subcommand groups.

Run:

```bash
./fat-controller auth --help
```

Expected: Help showing `login`, `logout`, `status` subcommands.

Run:

```bash
./fat-controller config --help
```

Expected: Help showing `get`, `set`, `delete`, `diff`, `apply`, `validate` subcommands.

Run:

```bash
./fat-controller auth login
```

Expected: "auth login: not yet implemented"

Run:

```bash
./fat-controller config get
```

Expected: "config get: not yet implemented"

**Step 5: Run mise check**

Run:

```bash
mise run check
```

Expected: All checks pass (golangci-lint may flag unused params — suppress with `//nolint` or fix).

**Step 6: Commit**

```bash
git add main.go cmd/cli.go
git commit -m "Add kong CLI skeleton with global flags and command stubs"
```

---

### Task 3: XDG paths module

**Files:**

- Create: `internal/platform/paths.go`
- Create: `internal/platform/paths_test.go`

This module wraps `adrg/xdg` to provide app-specific paths. All other
packages use this instead of calling xdg directly — single place to
change the app name.

**Step 1: Write the test**

`internal/platform/paths_test.go`:

```go
package platform_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/platform"
)

func TestConfigDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	dir := platform.ConfigDir()
	want := filepath.Join(tmp, "fat-controller")
	if dir != want {
		t.Errorf("ConfigDir() = %q, want %q", dir, want)
	}
}

func TestAuthFilePath(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	path := platform.AuthFilePath()
	want := filepath.Join(tmp, "fat-controller", "auth.json")
	if path != want {
		t.Errorf("AuthFilePath() = %q, want %q", path, want)
	}
}

func TestConfigFilePath(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	path := platform.ConfigFilePath()
	want := filepath.Join(tmp, "fat-controller", "config.toml")
	if path != want {
		t.Errorf("ConfigFilePath() = %q, want %q", path, want)
	}
}

func TestEnsureConfigDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	dir, err := platform.EnsureConfigDir()
	if err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("directory not created: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("%q is not a directory", dir)
	}
}
```

**Step 2: Run tests to verify they fail**

Run:

```bash
go test ./internal/platform/ -v
```

Expected: Compilation error — package doesn't exist yet.

**Step 3: Write the implementation**

`internal/platform/paths.go`:

```go
package platform

import (
	"os"
	"path/filepath"

	"github.com/adrg/xdg"
)

const appName = "fat-controller"

// ConfigDir returns the path to the app's config directory.
// Does NOT create the directory.
func ConfigDir() string {
	return filepath.Join(xdg.ConfigHome, appName)
}

// AuthFilePath returns the path to the auth token fallback file.
// Does NOT create the file or its parent directory.
func AuthFilePath() string {
	return filepath.Join(xdg.ConfigHome, appName, "auth.json")
}

// ConfigFilePath returns the path to the user config file.
// Does NOT create the file or its parent directory.
func ConfigFilePath() string {
	return filepath.Join(xdg.ConfigHome, appName, "config.toml")
}

// EnsureConfigDir creates the config directory if it doesn't exist
// and returns its path.
func EnsureConfigDir() (string, error) {
	dir := ConfigDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}
```

**Step 4: Run tests to verify they pass**

Run:

```bash
go test ./internal/platform/ -v
```

Expected: All 4 tests pass.

**Note:** `adrg/xdg` reads `XDG_CONFIG_HOME` at init time and caches it
in `xdg.ConfigHome`. The `t.Setenv` call sets the env var before the test
function runs, but xdg may have already cached the value from process
start. If tests fail for this reason, change the implementation to read
`os.Getenv("XDG_CONFIG_HOME")` directly with a fallback to
`~/.config` instead of using the xdg library variable. Verify and adjust
if needed.

**Step 5: Commit**

```bash
git add internal/platform/
git commit -m "Add XDG-compliant path helpers for config and auth files"
```

---

### Task 4: Token store — keyring with file fallback

**Files:**

- Create: `internal/auth/store.go`
- Create: `internal/auth/store_test.go`

This module handles persisting and retrieving OAuth tokens. It tries the
OS keyring first, falls back to a JSON file.

**Step 1: Write the test**

`internal/auth/store_test.go`:

```go
package auth_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/auth"
	"github.com/zalando/go-keyring"
)

func TestTokenStore_SaveAndLoad_Keyring(t *testing.T) {
	keyring.MockInit()

	store := auth.NewTokenStore(
		auth.WithKeyringService("fat-controller-test"),
		auth.WithFallbackPath(filepath.Join(t.TempDir(), "auth.json")),
	)

	tokens := &auth.StoredTokens{
		AccessToken:  "access-123",
		RefreshToken: "refresh-456",
		ClientID:     "client-789",
	}

	if err := store.Save(tokens); err != nil {
		t.Fatal(err)
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}

	if loaded.AccessToken != tokens.AccessToken {
		t.Errorf("AccessToken = %q, want %q", loaded.AccessToken, tokens.AccessToken)
	}
	if loaded.RefreshToken != tokens.RefreshToken {
		t.Errorf("RefreshToken = %q, want %q", loaded.RefreshToken, tokens.RefreshToken)
	}
	if loaded.ClientID != tokens.ClientID {
		t.Errorf("ClientID = %q, want %q", loaded.ClientID, tokens.ClientID)
	}
}

func TestTokenStore_SaveAndLoad_FileFallback(t *testing.T) {
	keyring.MockInitWithError(os.ErrPermission)

	fallbackPath := filepath.Join(t.TempDir(), "auth.json")
	store := auth.NewTokenStore(
		auth.WithKeyringService("fat-controller-test"),
		auth.WithFallbackPath(fallbackPath),
	)

	tokens := &auth.StoredTokens{
		AccessToken:  "access-abc",
		RefreshToken: "refresh-def",
		ClientID:     "client-ghi",
	}

	if err := store.Save(tokens); err != nil {
		t.Fatal(err)
	}

	// Verify file was created with correct permissions.
	info, err := os.Stat(fallbackPath)
	if err != nil {
		t.Fatalf("fallback file not created: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("file permissions = %o, want 0600", perm)
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}

	if loaded.AccessToken != tokens.AccessToken {
		t.Errorf("AccessToken = %q, want %q", loaded.AccessToken, tokens.AccessToken)
	}
}

func TestTokenStore_Delete_Keyring(t *testing.T) {
	keyring.MockInit()

	store := auth.NewTokenStore(
		auth.WithKeyringService("fat-controller-test"),
		auth.WithFallbackPath(filepath.Join(t.TempDir(), "auth.json")),
	)

	tokens := &auth.StoredTokens{
		AccessToken: "access-123",
		ClientID:    "client-789",
	}
	if err := store.Save(tokens); err != nil {
		t.Fatal(err)
	}

	if err := store.Delete(); err != nil {
		t.Fatal(err)
	}

	_, err := store.Load()
	if err != auth.ErrNoStoredTokens {
		t.Errorf("expected ErrNoStoredTokens, got %v", err)
	}
}

func TestTokenStore_Delete_FileFallback(t *testing.T) {
	keyring.MockInitWithError(os.ErrPermission)

	fallbackPath := filepath.Join(t.TempDir(), "auth.json")
	store := auth.NewTokenStore(
		auth.WithKeyringService("fat-controller-test"),
		auth.WithFallbackPath(fallbackPath),
	)

	tokens := &auth.StoredTokens{
		AccessToken: "access-abc",
		ClientID:    "client-ghi",
	}
	if err := store.Save(tokens); err != nil {
		t.Fatal(err)
	}

	if err := store.Delete(); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(fallbackPath); !os.IsNotExist(err) {
		t.Errorf("fallback file should be deleted")
	}
}

func TestTokenStore_Load_Empty(t *testing.T) {
	keyring.MockInit()

	store := auth.NewTokenStore(
		auth.WithKeyringService("fat-controller-test"),
		auth.WithFallbackPath(filepath.Join(t.TempDir(), "auth.json")),
	)

	_, err := store.Load()
	if err != auth.ErrNoStoredTokens {
		t.Errorf("expected ErrNoStoredTokens, got %v", err)
	}
}
```

**Step 2: Run tests to verify they fail**

Run:

```bash
go test ./internal/auth/ -v
```

Expected: Compilation error — types don't exist yet.

**Step 3: Write the implementation**

`internal/auth/store.go`:

```go
package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/zalando/go-keyring"
)

// ErrNoStoredTokens is returned when no tokens are found in keyring or file.
var ErrNoStoredTokens = errors.New("no stored tokens found")

// StoredTokens holds the persisted OAuth tokens and client registration.
type StoredTokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ClientID     string `json:"client_id"`
}

// TokenStore handles persisting OAuth tokens to OS keyring with file fallback.
type TokenStore struct {
	keyringService string
	keyringUser    string
	fallbackPath   string
}

// TokenStoreOption configures a TokenStore.
type TokenStoreOption func(*TokenStore)

// WithKeyringService sets a custom keyring service name (useful for tests).
func WithKeyringService(service string) TokenStoreOption {
	return func(s *TokenStore) { s.keyringService = service }
}

// WithFallbackPath sets a custom fallback file path (useful for tests).
func WithFallbackPath(path string) TokenStoreOption {
	return func(s *TokenStore) { s.fallbackPath = path }
}

// NewTokenStore creates a TokenStore with default settings.
// Pass options to override keyring service or fallback path.
func NewTokenStore(opts ...TokenStoreOption) *TokenStore {
	s := &TokenStore{
		keyringService: "fat-controller",
		keyringUser:    "oauth-token",
		// fallbackPath should be set by caller or via option.
		// Default empty — Load/Save will skip file fallback if unset.
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Save persists tokens. Tries keyring first, falls back to file.
func (s *TokenStore) Save(tokens *StoredTokens) error {
	data, err := json.Marshal(tokens)
	if err != nil {
		return fmt.Errorf("marshalling tokens: %w", err)
	}

	if err := keyring.Set(s.keyringService, s.keyringUser, string(data)); err != nil {
		// Keyring unavailable — fall back to file.
		return s.saveToFile(data)
	}
	return nil
}

// Load retrieves stored tokens. Tries keyring first, then file.
// Returns ErrNoStoredTokens if nothing is stored anywhere.
func (s *TokenStore) Load() (*StoredTokens, error) {
	// Try keyring.
	data, err := keyring.Get(s.keyringService, s.keyringUser)
	if err == nil {
		var tokens StoredTokens
		if err := json.Unmarshal([]byte(data), &tokens); err != nil {
			return nil, fmt.Errorf("unmarshalling keyring data: %w", err)
		}
		return &tokens, nil
	}

	// Keyring miss — try file fallback.
	return s.loadFromFile()
}

// Delete removes stored tokens from both keyring and file.
func (s *TokenStore) Delete() error {
	// Delete from keyring (ignore ErrNotFound).
	if err := keyring.Delete(s.keyringService, s.keyringUser); err != nil && !errors.Is(err, keyring.ErrNotFound) {
		// Keyring error — not fatal, continue to file cleanup.
	}

	// Delete file if it exists.
	if s.fallbackPath != "" {
		if err := os.Remove(s.fallbackPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("removing fallback file: %w", err)
		}
	}
	return nil
}

func (s *TokenStore) saveToFile(data []byte) error {
	if s.fallbackPath == "" {
		return fmt.Errorf("keyring unavailable and no fallback path configured")
	}

	dir := filepath.Dir(s.fallbackPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	if err := os.WriteFile(s.fallbackPath, data, 0o600); err != nil {
		return fmt.Errorf("writing fallback file: %w", err)
	}
	return nil
}

func (s *TokenStore) loadFromFile() (*StoredTokens, error) {
	if s.fallbackPath == "" {
		return nil, ErrNoStoredTokens
	}

	data, err := os.ReadFile(s.fallbackPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNoStoredTokens
		}
		return nil, fmt.Errorf("reading fallback file: %w", err)
	}

	var tokens StoredTokens
	if err := json.Unmarshal(data, &tokens); err != nil {
		return nil, fmt.Errorf("unmarshalling fallback file: %w", err)
	}
	return &tokens, nil
}
```

**Step 4: Run tests to verify they pass**

Run:

```bash
go test ./internal/auth/ -v
```

Expected: All 5 tests pass.

**Step 5: Commit**

```bash
git add internal/auth/
git commit -m "Add token store with OS keyring and JSON file fallback"
```

---

### Task 5: Token resolver — flag > env vars > keyring/file

**Files:**

- Create: `internal/auth/resolver.go`
- Create: `internal/auth/resolver_test.go`

This module determines which token to use and what auth header to send,
following the 4-level precedence: `--token` flag > `RAILWAY_API_TOKEN` >
`RAILWAY_TOKEN` > stored OAuth credentials.

**Step 1: Write the test**

`internal/auth/resolver_test.go`:

```go
package auth_test

import (
	"path/filepath"
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/auth"
	"github.com/zalando/go-keyring"
)

func TestResolveAuth_FlagTakesPrecedence(t *testing.T) {
	keyring.MockInit()

	store := auth.NewTokenStore(
		auth.WithKeyringService("fat-controller-test"),
		auth.WithFallbackPath(filepath.Join(t.TempDir(), "auth.json")),
	)
	// Store an OAuth token.
	store.Save(&auth.StoredTokens{AccessToken: "stored-token"})
	// Set env vars too.
	t.Setenv("RAILWAY_API_TOKEN", "api-token")
	t.Setenv("RAILWAY_TOKEN", "project-token")

	// Flag should win.
	resolved, err := auth.ResolveAuth("flag-token", store)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Token != "flag-token" {
		t.Errorf("Token = %q, want %q", resolved.Token, "flag-token")
	}
	if resolved.HeaderName != "Authorization" {
		t.Errorf("HeaderName = %q, want %q", resolved.HeaderName, "Authorization")
	}
	if resolved.Source != "flag" {
		t.Errorf("Source = %q, want %q", resolved.Source, "flag")
	}
}

func TestResolveAuth_APITokenEnvVar(t *testing.T) {
	keyring.MockInit()

	store := auth.NewTokenStore(
		auth.WithKeyringService("fat-controller-test"),
		auth.WithFallbackPath(filepath.Join(t.TempDir(), "auth.json")),
	)
	t.Setenv("RAILWAY_API_TOKEN", "api-token")
	t.Setenv("RAILWAY_TOKEN", "project-token")

	resolved, err := auth.ResolveAuth("", store)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Token != "api-token" {
		t.Errorf("Token = %q, want %q", resolved.Token, "api-token")
	}
	if resolved.HeaderName != "Authorization" {
		t.Errorf("HeaderName = %q, want %q", resolved.HeaderName, "Authorization")
	}
	if resolved.Source != "env:RAILWAY_API_TOKEN" {
		t.Errorf("Source = %q, want %q", resolved.Source, "env:RAILWAY_API_TOKEN")
	}
}

func TestResolveAuth_ProjectTokenEnvVar(t *testing.T) {
	keyring.MockInit()

	store := auth.NewTokenStore(
		auth.WithKeyringService("fat-controller-test"),
		auth.WithFallbackPath(filepath.Join(t.TempDir(), "auth.json")),
	)
	t.Setenv("RAILWAY_API_TOKEN", "")
	t.Setenv("RAILWAY_TOKEN", "project-token")

	resolved, err := auth.ResolveAuth("", store)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Token != "project-token" {
		t.Errorf("Token = %q, want %q", resolved.Token, "project-token")
	}
	if resolved.HeaderName != "Project-Access-Token" {
		t.Errorf("HeaderName = %q, want %q", resolved.HeaderName, "Project-Access-Token")
	}
	if resolved.Source != "env:RAILWAY_TOKEN" {
		t.Errorf("Source = %q, want %q", resolved.Source, "env:RAILWAY_TOKEN")
	}
}

func TestResolveAuth_FallsBackToStore(t *testing.T) {
	keyring.MockInit()

	store := auth.NewTokenStore(
		auth.WithKeyringService("fat-controller-test"),
		auth.WithFallbackPath(filepath.Join(t.TempDir(), "auth.json")),
	)
	store.Save(&auth.StoredTokens{AccessToken: "stored-token"})

	t.Setenv("RAILWAY_API_TOKEN", "")
	t.Setenv("RAILWAY_TOKEN", "")

	resolved, err := auth.ResolveAuth("", store)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Token != "stored-token" {
		t.Errorf("Token = %q, want %q", resolved.Token, "stored-token")
	}
	if resolved.HeaderName != "Authorization" {
		t.Errorf("HeaderName = %q, want %q", resolved.HeaderName, "Authorization")
	}
	if resolved.Source != "stored" {
		t.Errorf("Source = %q, want %q", resolved.Source, "stored")
	}
}

func TestResolveAuth_NothingAvailable(t *testing.T) {
	keyring.MockInit()

	store := auth.NewTokenStore(
		auth.WithKeyringService("fat-controller-test"),
		auth.WithFallbackPath(filepath.Join(t.TempDir(), "auth.json")),
	)

	t.Setenv("RAILWAY_API_TOKEN", "")
	t.Setenv("RAILWAY_TOKEN", "")

	_, err := auth.ResolveAuth("", store)
	if err != auth.ErrNotAuthenticated {
		t.Errorf("expected ErrNotAuthenticated, got %v", err)
	}
}
```

**Step 2: Run tests to verify they fail**

Run:

```bash
go test ./internal/auth/ -v -run TestResolveAuth
```

Expected: Compilation error — `ResolveAuth` doesn't exist.

**Step 3: Write the implementation**

`internal/auth/resolver.go`:

```go
package auth

import (
	"errors"
	"fmt"
	"os"
)

// ErrNotAuthenticated is returned when no token is available from any source.
var ErrNotAuthenticated = errors.New("not authenticated: run 'fat-controller auth login' or set RAILWAY_TOKEN")

// ResolvedAuth contains the resolved token and the HTTP header to use.
type ResolvedAuth struct {
	Token       string
	HeaderName  string
	HeaderValue string
	Source      string // "flag", "env:RAILWAY_API_TOKEN", "env:RAILWAY_TOKEN", "stored"
}

// ResolveAuth determines the active auth token using the precedence:
//  1. flagToken (from --token flag)
//  2. RAILWAY_API_TOKEN env var (account/workspace-scoped)
//  3. RAILWAY_TOKEN env var (project-scoped)
//  4. Stored OAuth token (from keyring or file)
func ResolveAuth(flagToken string, store *TokenStore) (*ResolvedAuth, error) {
	// 1. --token flag
	if flagToken != "" {
		return &ResolvedAuth{
			Token:       flagToken,
			HeaderName:  "Authorization",
			HeaderValue: "Bearer " + flagToken,
			Source:      "flag",
		}, nil
	}

	// 2. RAILWAY_API_TOKEN env var
	if token := os.Getenv("RAILWAY_API_TOKEN"); token != "" {
		return &ResolvedAuth{
			Token:       token,
			HeaderName:  "Authorization",
			HeaderValue: "Bearer " + token,
			Source:      "env:RAILWAY_API_TOKEN",
		}, nil
	}

	// 3. RAILWAY_TOKEN env var (project-scoped)
	if token := os.Getenv("RAILWAY_TOKEN"); token != "" {
		return &ResolvedAuth{
			Token:       token,
			HeaderName:  "Project-Access-Token",
			HeaderValue: token,
			Source:      "env:RAILWAY_TOKEN",
		}, nil
	}

	// 4. Stored OAuth token
	tokens, err := store.Load()
	if err != nil {
		if errors.Is(err, ErrNoStoredTokens) {
			return nil, ErrNotAuthenticated
		}
		return nil, fmt.Errorf("loading stored tokens: %w", err)
	}

	if tokens.AccessToken == "" {
		return nil, ErrNotAuthenticated
	}

	return &ResolvedAuth{
		Token:       tokens.AccessToken,
		HeaderName:  "Authorization",
		HeaderValue: "Bearer " + tokens.AccessToken,
		Source:      "stored",
	}, nil
}
```

**Step 4: Run tests to verify they pass**

Run:

```bash
go test ./internal/auth/ -v -run TestResolveAuth
```

Expected: All 5 tests pass.

**Step 5: Commit**

```bash
git add internal/auth/resolver.go internal/auth/resolver_test.go
git commit -m "Add token resolver with flag > env vars > keyring/file precedence"
```

---

### Task 6: PKCE helpers

**Files:**

- Create: `internal/auth/pkce.go`
- Create: `internal/auth/pkce_test.go`

**Step 1: Write the test**

`internal/auth/pkce_test.go`:

```go
package auth_test

import (
	"crypto/sha256"
	"encoding/base64"
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/auth"
)

func TestGenerateCodeVerifier(t *testing.T) {
	v1, err := auth.GenerateCodeVerifier()
	if err != nil {
		t.Fatal(err)
	}

	// Must be at least 43 characters (RFC 7636).
	if len(v1) < 43 {
		t.Errorf("verifier too short: %d chars", len(v1))
	}

	// Must be different each time.
	v2, err := auth.GenerateCodeVerifier()
	if err != nil {
		t.Fatal(err)
	}
	if v1 == v2 {
		t.Error("two verifiers should not be identical")
	}
}

func TestCodeChallenge(t *testing.T) {
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"

	challenge := auth.CodeChallenge(verifier)

	// Manually compute expected value.
	h := sha256.Sum256([]byte(verifier))
	want := base64.RawURLEncoding.EncodeToString(h[:])

	if challenge != want {
		t.Errorf("CodeChallenge() = %q, want %q", challenge, want)
	}
}

func TestGenerateState(t *testing.T) {
	s1, err := auth.GenerateState()
	if err != nil {
		t.Fatal(err)
	}
	if len(s1) == 0 {
		t.Error("state should not be empty")
	}

	s2, err := auth.GenerateState()
	if err != nil {
		t.Fatal(err)
	}
	if s1 == s2 {
		t.Error("two states should not be identical")
	}
}
```

**Step 2: Run tests to verify they fail**

Run:

```bash
go test ./internal/auth/ -v -run "TestGenerate|TestCode"
```

Expected: Compilation error.

**Step 3: Write the implementation**

`internal/auth/pkce.go`:

```go
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
)

// GenerateCodeVerifier creates a cryptographically random PKCE code verifier.
// Returns a 43-character base64url-encoded string (32 random bytes).
func GenerateCodeVerifier() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// CodeChallenge computes the S256 PKCE code challenge for a verifier.
func CodeChallenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

// GenerateState creates a random state parameter for CSRF protection.
func GenerateState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
```

**Step 4: Run tests**

Run:

```bash
go test ./internal/auth/ -v -run "TestGenerate|TestCode"
```

Expected: All 3 tests pass.

**Step 5: Commit**

```bash
git add internal/auth/pkce.go internal/auth/pkce_test.go
git commit -m "Add PKCE code verifier, challenge, and state helpers"
```

---

### Task 7: OAuth client — registration, auth URL, token exchange

**Files:**

- Create: `internal/auth/oauth.go`
- Create: `internal/auth/oauth_test.go`

This is the core OAuth client. It handles dynamic client registration,
building the authorization URL, exchanging codes for tokens, and refreshing.
Tests use `httptest.NewServer` to mock Railway's endpoints.

**Step 1: Write the test**

`internal/auth/oauth_test.go`:

```go
package auth_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/auth"
)

func TestOAuthClient_RegisterClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected application/json content type")
		}

		var req auth.RegistrationRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if req.ClientName != "Fat Controller CLI" {
			t.Errorf("ClientName = %q", req.ClientName)
		}
		if req.TokenEndpointAuthMethod != "none" {
			t.Errorf("TokenEndpointAuthMethod = %q", req.TokenEndpointAuthMethod)
		}
		if req.ApplicationType != "native" {
			t.Errorf("ApplicationType = %q", req.ApplicationType)
		}

		json.NewEncoder(w).Encode(auth.RegistrationResponse{
			ClientID:   "test-client-id",
			ClientName: "Fat Controller CLI",
		})
	}))
	defer server.Close()

	client := &auth.OAuthClient{
		RegistrationURL: server.URL,
	}
	resp, err := client.RegisterClient("http://127.0.0.1:12345/callback")
	if err != nil {
		t.Fatal(err)
	}
	if resp.ClientID != "test-client-id" {
		t.Errorf("ClientID = %q", resp.ClientID)
	}
}

func TestOAuthClient_AuthorizationURL(t *testing.T) {
	client := &auth.OAuthClient{
		AuthEndpoint: "https://example.com/oauth/auth",
	}

	authURL := client.AuthorizationURL("client-123", "http://127.0.0.1:8080/callback", "state-abc", "challenge-xyz")

	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatal(err)
	}

	tests := map[string]string{
		"response_type":         "code",
		"client_id":             "client-123",
		"redirect_uri":          "http://127.0.0.1:8080/callback",
		"state":                 "state-abc",
		"code_challenge":        "challenge-xyz",
		"code_challenge_method": "S256",
		"prompt":                "consent",
	}
	for key, want := range tests {
		got := parsed.Query().Get(key)
		if got != want {
			t.Errorf("param %q = %q, want %q", key, got, want)
		}
	}

	// Verify scope contains required values.
	scope := parsed.Query().Get("scope")
	for _, required := range []string{"openid", "offline_access"} {
		if !strings.Contains(scope, required) {
			t.Errorf("scope missing %q, got %q", required, scope)
		}
	}
}

func TestOAuthClient_ExchangeCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.Form.Get("grant_type") != "authorization_code" {
			t.Errorf("grant_type = %q", r.Form.Get("grant_type"))
		}
		if r.Form.Get("code") != "auth-code-123" {
			t.Errorf("code = %q", r.Form.Get("code"))
		}
		if r.Form.Get("code_verifier") != "verifier-abc" {
			t.Errorf("code_verifier = %q", r.Form.Get("code_verifier"))
		}
		// Native client: client_id in body, no secret.
		if r.Form.Get("client_id") != "client-123" {
			t.Errorf("client_id = %q", r.Form.Get("client_id"))
		}

		json.NewEncoder(w).Encode(auth.TokenResponse{
			AccessToken:  "new-access-token",
			RefreshToken: "new-refresh-token",
			ExpiresIn:    3600,
			TokenType:    "Bearer",
		})
	}))
	defer server.Close()

	client := &auth.OAuthClient{TokenEndpoint: server.URL}

	resp, err := client.ExchangeCode("client-123", "auth-code-123", "http://127.0.0.1:8080/callback", "verifier-abc")
	if err != nil {
		t.Fatal(err)
	}
	if resp.AccessToken != "new-access-token" {
		t.Errorf("AccessToken = %q", resp.AccessToken)
	}
	if resp.RefreshToken != "new-refresh-token" {
		t.Errorf("RefreshToken = %q", resp.RefreshToken)
	}
}

func TestOAuthClient_RefreshToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.Form.Get("grant_type") != "refresh_token" {
			t.Errorf("grant_type = %q", r.Form.Get("grant_type"))
		}
		if r.Form.Get("refresh_token") != "old-refresh" {
			t.Errorf("refresh_token = %q", r.Form.Get("refresh_token"))
		}
		if r.Form.Get("client_id") != "client-123" {
			t.Errorf("client_id = %q", r.Form.Get("client_id"))
		}

		json.NewEncoder(w).Encode(auth.TokenResponse{
			AccessToken:  "refreshed-access",
			RefreshToken: "rotated-refresh",
			ExpiresIn:    3600,
		})
	}))
	defer server.Close()

	client := &auth.OAuthClient{TokenEndpoint: server.URL}

	resp, err := client.RefreshToken("client-123", "old-refresh")
	if err != nil {
		t.Fatal(err)
	}
	if resp.AccessToken != "refreshed-access" {
		t.Errorf("AccessToken = %q", resp.AccessToken)
	}
	if resp.RefreshToken != "rotated-refresh" {
		t.Errorf("RefreshToken = %q", resp.RefreshToken)
	}
}
```

**Step 2: Run tests to verify they fail**

Run:

```bash
go test ./internal/auth/ -v -run TestOAuthClient
```

Expected: Compilation error.

**Step 3: Write the implementation**

`internal/auth/oauth.go`:

```go
package auth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

const (
	DefaultAuthEndpoint    = "https://backboard.railway.com/oauth/auth"
	DefaultTokenEndpoint   = "https://backboard.railway.com/oauth/token"
	DefaultRegistrationURL = "https://backboard.railway.com/oauth/register"
	DefaultUserinfoURL     = "https://backboard.railway.com/oauth/me"
	DefaultGraphQLURL      = "https://backboard.railway.com/graphql/v2"

	defaultScope = "openid email profile offline_access"
)

// OAuthClient handles Railway OAuth 2.0 operations.
// Endpoints are configurable for testing.
type OAuthClient struct {
	AuthEndpoint    string
	TokenEndpoint   string
	RegistrationURL string
	UserinfoURL     string

	HTTPClient *http.Client
}

// NewOAuthClient creates an OAuthClient with Railway's production endpoints.
func NewOAuthClient() *OAuthClient {
	return &OAuthClient{
		AuthEndpoint:    DefaultAuthEndpoint,
		TokenEndpoint:   DefaultTokenEndpoint,
		RegistrationURL: DefaultRegistrationURL,
		UserinfoURL:     DefaultUserinfoURL,
		HTTPClient:      http.DefaultClient,
	}
}

// RegistrationRequest is the body for dynamic client registration (RFC 7591).
type RegistrationRequest struct {
	ClientName              string   `json:"client_name"`
	RedirectURIs            []string `json:"redirect_uris"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
	ApplicationType         string   `json:"application_type"`
}

// RegistrationResponse is returned by the registration endpoint.
type RegistrationResponse struct {
	ClientID                string `json:"client_id"`
	ClientName              string `json:"client_name"`
	RegistrationAccessToken string `json:"registration_access_token"`
	RegistrationClientURI   string `json:"registration_client_uri"`
}

// TokenResponse is returned by the token endpoint.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	Scope        string `json:"scope"`
}

// RegisterClient performs dynamic client registration for a native (public) app.
func (c *OAuthClient) RegisterClient(redirectURI string) (*RegistrationResponse, error) {
	reqBody := RegistrationRequest{
		ClientName:              "Fat Controller CLI",
		RedirectURIs:            []string{redirectURI},
		TokenEndpointAuthMethod: "none",
		GrantTypes:              []string{"authorization_code", "refresh_token"},
		ResponseTypes:           []string{"code"},
		ApplicationType:         "native",
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshalling registration request: %w", err)
	}

	resp, err := c.httpClient().Post(c.RegistrationURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("registration request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("registration failed with status %d", resp.StatusCode)
	}

	var reg RegistrationResponse
	if err := json.NewDecoder(resp.Body).Decode(&reg); err != nil {
		return nil, fmt.Errorf("decoding registration response: %w", err)
	}
	return &reg, nil
}

// AuthorizationURL builds the URL the user should visit to authorize.
func (c *OAuthClient) AuthorizationURL(clientID, redirectURI, state, codeChallenge string) string {
	v := url.Values{
		"response_type":         {"code"},
		"client_id":             {clientID},
		"redirect_uri":          {redirectURI},
		"scope":                 {defaultScope},
		"state":                 {state},
		"code_challenge":        {codeChallenge},
		"code_challenge_method": {"S256"},
		"prompt":                {"consent"},
	}
	return c.AuthEndpoint + "?" + v.Encode()
}

// ExchangeCode exchanges an authorization code for tokens.
// Uses PKCE — no client secret (native client).
func (c *OAuthClient) ExchangeCode(clientID, code, redirectURI, codeVerifier string) (*TokenResponse, error) {
	data := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"client_id":     {clientID},
		"code_verifier": {codeVerifier},
	}

	resp, err := c.httpClient().PostForm(c.TokenEndpoint, data)
	if err != nil {
		return nil, fmt.Errorf("token exchange request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token exchange failed with status %d", resp.StatusCode)
	}

	var tok TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		return nil, fmt.Errorf("decoding token response: %w", err)
	}
	return &tok, nil
}

// RefreshToken exchanges a refresh token for a new access + refresh token pair.
// Important: Railway rotates refresh tokens. Always store the new one.
func (c *OAuthClient) RefreshToken(clientID, refreshToken string) (*TokenResponse, error) {
	data := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {clientID},
	}

	resp, err := c.httpClient().PostForm(c.TokenEndpoint, data)
	if err != nil {
		return nil, fmt.Errorf("refresh request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("refresh failed with status %d", resp.StatusCode)
	}

	var tok TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		return nil, fmt.Errorf("decoding refresh response: %w", err)
	}
	return &tok, nil
}

func (c *OAuthClient) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return http.DefaultClient
}
```

**Step 4: Run tests**

Run:

```bash
go test ./internal/auth/ -v -run TestOAuthClient
```

Expected: All 4 tests pass.

**Step 5: Commit**

```bash
git add internal/auth/oauth.go internal/auth/oauth_test.go
git commit -m "Add OAuth client with registration, auth URL, code exchange, and refresh"
```

---

### Task 8: Callback server

**Files:**

- Create: `internal/auth/callback.go`
- Create: `internal/auth/callback_test.go`

The local HTTP server that receives the OAuth redirect.

**Step 1: Write the test**

`internal/auth/callback_test.go`:

```go
package auth_test

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/hamishmorgan/fat-controller/internal/auth"
)

func TestCallbackServer_ReceivesCode(t *testing.T) {
	srv, err := auth.StartCallbackServer()
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Shutdown()

	if srv.Port == 0 {
		t.Fatal("port should be non-zero")
	}

	// Simulate browser redirect.
	go func() {
		url := fmt.Sprintf("http://127.0.0.1:%d/callback?code=test-auth-code&state=test-state", srv.Port)
		http.Get(url) //nolint:errcheck
	}()

	select {
	case result := <-srv.Result:
		if result.Code != "test-auth-code" {
			t.Errorf("Code = %q, want %q", result.Code, "test-auth-code")
		}
		if result.State != "test-state" {
			t.Errorf("State = %q, want %q", result.State, "test-state")
		}
		if result.Error != "" {
			t.Errorf("unexpected error: %s", result.Error)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for callback")
	}
}

func TestCallbackServer_ReceivesError(t *testing.T) {
	srv, err := auth.StartCallbackServer()
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Shutdown()

	go func() {
		url := fmt.Sprintf("http://127.0.0.1:%d/callback?error=access_denied&error_description=User+denied+access", srv.Port)
		http.Get(url) //nolint:errcheck
	}()

	select {
	case result := <-srv.Result:
		if result.Error != "access_denied" {
			t.Errorf("Error = %q, want %q", result.Error, "access_denied")
		}
		if result.ErrorDescription != "User denied access" {
			t.Errorf("ErrorDescription = %q", result.ErrorDescription)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for callback")
	}
}

func TestCallbackServer_RedirectURI(t *testing.T) {
	srv, err := auth.StartCallbackServer()
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Shutdown()

	uri := srv.RedirectURI()
	want := fmt.Sprintf("http://127.0.0.1:%d/callback", srv.Port)
	if uri != want {
		t.Errorf("RedirectURI() = %q, want %q", uri, want)
	}
}
```

**Step 2: Run tests to verify they fail**

Run:

```bash
go test ./internal/auth/ -v -run TestCallbackServer
```

Expected: Compilation error.

**Step 3: Write the implementation**

`internal/auth/callback.go`:

```go
package auth

import (
	"context"
	"fmt"
	"net"
	"net/http"
)

// CallbackResult holds the data received from the OAuth redirect.
type CallbackResult struct {
	Code             string
	State            string
	Error            string
	ErrorDescription string
}

// CallbackServer is a temporary local HTTP server for receiving OAuth redirects.
type CallbackServer struct {
	Port   int
	Result chan CallbackResult
	server *http.Server
}

// StartCallbackServer starts an HTTP server on a random available port.
// It listens for a single OAuth callback, sends the result on the Result channel,
// then the caller should Shutdown().
func StartCallbackServer() (*CallbackServer, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("starting callback listener: %w", err)
	}

	port := listener.Addr().(*net.TCPAddr).Port
	result := make(chan CallbackResult, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()

		if e := q.Get("error"); e != "" {
			result <- CallbackResult{
				Error:            e,
				ErrorDescription: q.Get("error_description"),
			}
			fmt.Fprint(w, "Authorization failed. You can close this tab.")
			return
		}

		result <- CallbackResult{
			Code:  q.Get("code"),
			State: q.Get("state"),
		}
		fmt.Fprint(w, "Authorization successful! You can close this tab.")
	})

	srv := &http.Server{Handler: mux}
	go srv.Serve(listener) //nolint:errcheck

	return &CallbackServer{
		Port:   port,
		Result: result,
		server: srv,
	}, nil
}

// RedirectURI returns the redirect URI for this server.
func (s *CallbackServer) RedirectURI() string {
	return fmt.Sprintf("http://127.0.0.1:%d/callback", s.Port)
}

// Shutdown gracefully stops the callback server.
func (s *CallbackServer) Shutdown() {
	s.server.Shutdown(context.Background()) //nolint:errcheck
}
```

**Step 4: Run tests**

Run:

```bash
go test ./internal/auth/ -v -run TestCallbackServer
```

Expected: All 3 tests pass.

**Step 5: Commit**

```bash
git add internal/auth/callback.go internal/auth/callback_test.go
git commit -m "Add local callback server for OAuth redirect handling"
```

---

### Task 9: Userinfo fetcher

**Files:**

- Create: `internal/auth/userinfo.go`
- Create: `internal/auth/userinfo_test.go`

**Step 1: Write the test**

`internal/auth/userinfo_test.go`:

```go
package auth_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/auth"
)

func TestFetchUserInfo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}

		json.NewEncoder(w).Encode(auth.UserInfo{
			Sub:   "user_abc123",
			Email: "test@example.com",
			Name:  "Test User",
		})
	}))
	defer server.Close()

	client := &auth.OAuthClient{
		UserinfoURL: server.URL,
		HTTPClient:  http.DefaultClient,
	}

	info, err := client.FetchUserInfo("test-token")
	if err != nil {
		t.Fatal(err)
	}
	if info.Email != "test@example.com" {
		t.Errorf("Email = %q", info.Email)
	}
	if info.Name != "Test User" {
		t.Errorf("Name = %q", info.Name)
	}
}
```

**Step 2: Run test to verify it fails**

Run:

```bash
go test ./internal/auth/ -v -run TestFetchUserInfo
```

Expected: Compilation error.

**Step 3: Write the implementation**

`internal/auth/userinfo.go`:

```go
package auth

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// UserInfo represents the OIDC userinfo response.
type UserInfo struct {
	Sub     string `json:"sub"`
	Email   string `json:"email"`
	Name    string `json:"name"`
	Picture string `json:"picture"`
}

// FetchUserInfo calls the OIDC userinfo endpoint.
func (c *OAuthClient) FetchUserInfo(accessToken string) (*UserInfo, error) {
	req, err := http.NewRequest(http.MethodGet, c.UserinfoURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("userinfo request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("userinfo failed with status %d", resp.StatusCode)
	}

	var info UserInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("decoding userinfo: %w", err)
	}
	return &info, nil
}
```

**Step 4: Run test**

Run:

```bash
go test ./internal/auth/ -v -run TestFetchUserInfo
```

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/auth/userinfo.go internal/auth/userinfo_test.go
git commit -m "Add userinfo fetcher for auth status display"
```

---

### Task 10: Login orchestrator

**Files:**

- Create: `internal/auth/login.go`

This function orchestrates the full login flow: start callback server →
register client (if needed) → generate PKCE → open browser → wait for
callback → exchange code → store tokens.

No test for this — it's pure orchestration of already-tested components,
and it opens a browser + waits for user interaction. Integration testing
happens manually.

**Step 1: Write the implementation**

`internal/auth/login.go`:

```go
package auth

import (
	"fmt"
	"os/exec"
	"runtime"
)

// Login performs the full OAuth login flow:
//  1. Start callback server
//  2. Register client if no client ID stored
//  3. Generate PKCE verifier + state
//  4. Open browser to authorization URL
//  5. Wait for callback
//  6. Exchange code for tokens
//  7. Store tokens
func Login(oauth *OAuthClient, store *TokenStore) error {
	// Start callback server.
	srv, err := StartCallbackServer()
	if err != nil {
		return fmt.Errorf("starting callback server: %w", err)
	}
	defer srv.Shutdown()

	redirectURI := srv.RedirectURI()

	// Check for existing client registration.
	clientID, err := loadOrRegisterClient(oauth, store, redirectURI)
	if err != nil {
		return fmt.Errorf("client registration: %w", err)
	}

	// Generate PKCE.
	verifier, err := GenerateCodeVerifier()
	if err != nil {
		return fmt.Errorf("generating code verifier: %w", err)
	}
	challenge := CodeChallenge(verifier)

	state, err := GenerateState()
	if err != nil {
		return fmt.Errorf("generating state: %w", err)
	}

	// Build authorization URL and open browser.
	authURL := oauth.AuthorizationURL(clientID, redirectURI, state, challenge)
	fmt.Println("Opening browser to log in...")
	fmt.Printf("If the browser doesn't open, visit:\n%s\n\n", authURL)

	if err := openBrowser(authURL); err != nil {
		// Non-fatal — user can copy the URL.
		fmt.Printf("Could not open browser: %v\n", err)
	}

	// Wait for callback.
	fmt.Println("Waiting for authorization...")
	result := <-srv.Result

	if result.Error != "" {
		return fmt.Errorf("authorization failed: %s: %s", result.Error, result.ErrorDescription)
	}

	if result.State != state {
		return fmt.Errorf("state mismatch: possible CSRF attack")
	}

	// Exchange code for tokens.
	tokenResp, err := oauth.ExchangeCode(clientID, result.Code, redirectURI, verifier)
	if err != nil {
		return fmt.Errorf("exchanging authorization code: %w", err)
	}

	// Store tokens.
	if err := store.Save(&StoredTokens{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ClientID:     clientID,
	}); err != nil {
		return fmt.Errorf("storing tokens: %w", err)
	}

	fmt.Println("Login successful!")
	return nil
}

// loadOrRegisterClient returns a client ID, registering a new client if needed.
func loadOrRegisterClient(oauth *OAuthClient, store *TokenStore, redirectURI string) (string, error) {
	// Check if we already have a client ID from a previous login.
	existing, err := store.Load()
	if err == nil && existing.ClientID != "" {
		return existing.ClientID, nil
	}

	// Register a new client.
	reg, err := oauth.RegisterClient(redirectURI)
	if err != nil {
		return "", err
	}
	return reg.ClientID, nil
}

// openBrowser opens the given URL in the user's default browser.
func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default: // linux, freebsd, etc.
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}
```

**Step 2: Verify it compiles**

Run:

```bash
go build ./internal/auth/
```

Expected: Compiles without errors.

**Step 3: Commit**

```bash
git add internal/auth/login.go
git commit -m "Add login orchestrator for full OAuth + PKCE flow"
```

---

### Task 11: Wire up auth commands to real implementations

**Files:**

- Modify: `cmd/cli.go`

Replace the stub `Run()` methods for `AuthLoginCmd`, `AuthLogoutCmd`, and
`AuthStatusCmd` with real implementations that call the `internal/auth`
and `internal/platform` packages.

**Step 1: Update the Run() methods**

Replace the three auth stub methods in `cmd/cli.go`:

```go
func (c *AuthLoginCmd) Run(globals *Globals) error {
	oauth := auth.NewOAuthClient()
	store := auth.NewTokenStore(
		auth.WithFallbackPath(platform.AuthFilePath()),
	)
	return auth.Login(oauth, store)
}

func (c *AuthLogoutCmd) Run(globals *Globals) error {
	store := auth.NewTokenStore(
		auth.WithFallbackPath(platform.AuthFilePath()),
	)
	if err := store.Delete(); err != nil {
		return fmt.Errorf("clearing credentials: %w", err)
	}
	fmt.Println("Logged out successfully.")
	return nil
}

func (c *AuthStatusCmd) Run(globals *Globals) error {
	store := auth.NewTokenStore(
		auth.WithFallbackPath(platform.AuthFilePath()),
	)

	resolved, err := auth.ResolveAuth(globals.Token, store)
	if err != nil {
		fmt.Println("Not authenticated.")
		fmt.Println("Run 'fat-controller auth login' or set RAILWAY_TOKEN.")
		return nil
	}

	fmt.Printf("Authenticated via: %s\n", resolved.Source)

	if resolved.Source == "env:RAILWAY_TOKEN" {
		fmt.Println("Using RAILWAY_TOKEN environment variable (project access token).")
		return nil
	}
	if resolved.Source == "env:RAILWAY_API_TOKEN" {
		fmt.Println("Using RAILWAY_API_TOKEN environment variable (account/workspace token).")
		return nil
	}
	if resolved.Source == "flag" {
		fmt.Println("Using --token flag.")
		return nil
	}

	// For stored OAuth tokens, fetch user info.
	oauth := auth.NewOAuthClient()
	info, err := oauth.FetchUserInfo(resolved.Token)
	if err != nil {
		fmt.Printf("Authenticated (could not fetch user info: %v)\n", err)
		return nil
	}

	fmt.Printf("User: %s\n", info.Name)
	fmt.Printf("Email: %s\n", info.Email)
	return nil
}
```

Update the imports at the top of `cmd/cli.go` to include:

```go
import (
	"fmt"

	"github.com/alecthomas/kong"
	"github.com/hamishmorgan/fat-controller/internal/auth"
	"github.com/hamishmorgan/fat-controller/internal/platform"
)
```

Note: the `kong` import may show as unused at this point — it was already
imported for the type definitions. Remove it if the linter complains
(it's only needed if `kong` types are referenced in the structs, which
they aren't in the current design).

**Step 2: Verify it builds**

Run:

```bash
go build -o fat-controller .
```

Expected: Compiles without errors.

**Step 3: Run mise check**

Run:

```bash
mise run check
```

Expected: All checks pass.

**Step 4: Commit**

```bash
git add cmd/cli.go
git commit -m "Wire up auth login/logout/status with real implementations"
```

---

### Task 12: Run full test suite and smoke test

**Step 1: Run all tests**

Run:

```bash
go test ./... -v
```

Expected: All tests pass.

**Step 2: Run mise check (linter + build)**

Run:

```bash
mise run check
```

Expected: All checks pass.

**Step 3: Smoke test the CLI**

Run:

```bash
go build -o fat-controller .
./fat-controller --help
./fat-controller auth --help
./fat-controller auth login --help
./fat-controller auth status
./fat-controller auth logout
./fat-controller config --help
./fat-controller config get --help
./fat-controller config diff
```

Expected:

- `--help` shows global flags and command groups
- `auth --help` shows login, logout, status
- `auth status` shows "Not authenticated" (since we haven't logged in)
- `auth logout` shows "Logged out successfully."
- `config --help` shows get, set, delete, diff, apply, validate
- `config diff` shows "config diff: not yet implemented"

**Step 4: Commit any fixes**

If any issues were found, fix and commit:

```bash
git add -A
git commit -m "Fix issues found during final verification"
```

---

## Summary

After completing all tasks, the project will have:

| Component | Location |
|-----------|----------|
| CLI entrypoint | `main.go` |
| CLI struct + commands | `cmd/cli.go` |
| XDG path helpers | `internal/platform/paths.go` |
| Token store (keyring + file) | `internal/auth/store.go` |
| Token resolver (flag > env > keyring/file) | `internal/auth/resolver.go` |
| PKCE helpers | `internal/auth/pkce.go` |
| OAuth client | `internal/auth/oauth.go` |
| Callback server | `internal/auth/callback.go` |
| Login orchestrator | `internal/auth/login.go` |
| Userinfo fetcher | `internal/auth/userinfo.go` |

**Not included in M1** (deferred to M2+):

- genqlient / GraphQL schema
- koanf config loading (no tool config to load yet — auth doesn't need it)
- BurntSushi/toml (no config parsing yet)
- `config get/set/delete/diff/apply/validate` implementations

**Testing approach:**

- Unit tests with `go-keyring` `MockInit()` / `MockInitWithError()` for token store
- `httptest.NewServer` for OAuth endpoint mocking
- `t.TempDir()` + `t.Setenv("XDG_CONFIG_HOME", ...)` for file path tests
- Login orchestrator tested manually (browser interaction)

**Key differences from old plan (cobra → kong):**

- All commands in a single `cmd/cli.go` file (kong's struct nesting means no separate files per command group)
- No `cmd/root.go`, no `init()` wiring — kong discovers commands from struct tags
- `Globals` struct passed to `Run()` methods instead of cobra's `cmd.Flags()` API
- `--token` flag resolves through `Globals`, not just `RAILWAY_TOKEN` env var
- Token resolver has 4 levels (flag > `RAILWAY_API_TOKEN` > `RAILWAY_TOKEN` > stored) instead of 2
- Config command stubs include all 6 subcommands (get/set/delete/diff/apply/validate) not just 3 (pull/diff/apply)
- `TokenStore` uses functional options pattern for test configurability
