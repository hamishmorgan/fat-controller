# M1: Scaffold + Auth — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Bootstrap the fat-controller CLI with cobra, XDG-compliant config/token storage, and a working `auth login/logout/status` flow using Railway's OAuth 2.0 + PKCE.

**Architecture:** Cobra for CLI dispatch with two subcommand groups (`auth`, `config` stub). Token storage uses OS keyring (go-keyring) with XDG file fallback. OAuth uses Railway's dynamic client registration + PKCE authorization code flow. Tool config uses koanf for layered loading.

**Tech Stack:** Go, cobra, adrg/xdg, zalando/go-keyring, knadh/koanf, BurntSushi/toml

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

1. `RAILWAY_TOKEN` env var (project access token or account token) — for CI
2. OS keyring via go-keyring — primary interactive storage
3. Fallback file at `$XDG_CONFIG_HOME/fat-controller/auth.json` (mode 0600) — headless/SSH

Project access tokens use `Project-Access-Token` header. Account-level tokens use `Authorization: Bearer`.

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
Expected: `go.mod` created

**Step 2: Add dependencies**

Run:
```bash
go get github.com/spf13/cobra@latest
go get github.com/adrg/xdg@latest
go get github.com/zalando/go-keyring@latest
go get github.com/knadh/koanf/v2@latest
go get github.com/knadh/koanf/providers/file@latest
go get github.com/knadh/koanf/providers/env/v2@latest
go get github.com/knadh/koanf/providers/structs@latest
go get github.com/knadh/koanf/parsers/toml/v2@latest
go get github.com/BurntSushi/toml@latest
```

**Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "feat: initialize Go module with CLI and config dependencies"
```

---

### Task 2: Cobra root command + main.go

**Files:**
- Create: `main.go`
- Create: `cmd/root.go`

**Step 1: Write root command**

`cmd/root.go`:
```go
package cmd

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "fat-controller",
	Short: "CLI for managing Railway projects",
	Long:  "Fat Controller is a CLI for managing Railway projects. Pull live config, diff against a desired state, apply the difference.",
	SilenceUsage: true,
}

func Execute() error {
	return rootCmd.Execute()
}
```

**Step 2: Write main.go**

`main.go`:
```go
package main

import (
	"fmt"
	"os"

	"github.com/hamishmorgan/fat-controller/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
```

**Step 3: Verify it builds and runs**

Run:
```bash
go build -o fat-controller . && ./fat-controller
```
Expected: Help output showing "CLI for managing Railway projects"

Run:
```bash
go build -o fat-controller . && ./fat-controller --help
```
Expected: Same help output

**Step 4: Commit**

```bash
git add main.go cmd/root.go
git commit -m "feat: add cobra root command and main entrypoint"
```

---

### Task 3: Auth subcommand group with login/logout/status stubs

**Files:**
- Create: `cmd/auth/auth.go`
- Create: `cmd/auth/login.go`
- Create: `cmd/auth/logout.go`
- Create: `cmd/auth/status.go`
- Modify: `cmd/root.go`

**Step 1: Create the auth group command**

`cmd/auth/auth.go`:
```go
package auth

import "github.com/spf13/cobra"

// Cmd is the `fat-controller auth` parent command.
// It has no RunE — invoking it without a subcommand shows help.
var Cmd = &cobra.Command{
	Use:   "auth",
	Short: "Manage authentication",
}

func init() {
	Cmd.AddCommand(loginCmd)
	Cmd.AddCommand(logoutCmd)
	Cmd.AddCommand(statusCmd)
}
```

**Step 2: Create stub subcommands**

`cmd/auth/login.go`:
```go
package auth

import (
	"fmt"

	"github.com/spf13/cobra"
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Log in to Railway via browser-based OAuth",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("login: not yet implemented")
		return nil
	},
}
```

`cmd/auth/logout.go`:
```go
package auth

import (
	"fmt"

	"github.com/spf13/cobra"
)

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Clear stored credentials",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("logout: not yet implemented")
		return nil
	},
}
```

`cmd/auth/status.go`:
```go
package auth

import (
	"fmt"

	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current authentication status",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("status: not yet implemented")
		return nil
	},
}
```

**Step 3: Wire auth group into root**

Add to `cmd/root.go`:
```go
import (
	"github.com/hamishmorgan/fat-controller/cmd/auth"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(auth.Cmd)
}
```

**Step 4: Verify the command tree works**

Run:
```bash
go build -o fat-controller . && ./fat-controller auth
```
Expected: Help showing login, logout, status subcommands

Run:
```bash
./fat-controller auth login
```
Expected: "login: not yet implemented"

Run:
```bash
./fat-controller auth logout
```
Expected: "logout: not yet implemented"

Run:
```bash
./fat-controller auth status
```
Expected: "status: not yet implemented"

**Step 5: Commit**

```bash
git add cmd/auth/ cmd/root.go
git commit -m "feat: add auth subcommand group with login/logout/status stubs"
```

---

### Task 4: Config subcommand group stubs

**Files:**
- Create: `cmd/config/config.go`
- Create: `cmd/config/pull.go`
- Create: `cmd/config/diff.go`
- Create: `cmd/config/apply.go`
- Modify: `cmd/root.go`

**Step 1: Create config group and stub subcommands**

`cmd/config/config.go`:
```go
package config

import "github.com/spf13/cobra"

// Cmd is the `fat-controller config` parent command.
var Cmd = &cobra.Command{
	Use:   "config",
	Short: "Declarative configuration management",
}

func init() {
	Cmd.AddCommand(pullCmd)
	Cmd.AddCommand(diffCmd)
	Cmd.AddCommand(applyCmd)
}
```

`cmd/config/pull.go`:
```go
package config

import (
	"fmt"

	"github.com/spf13/cobra"
)

var pullCmd = &cobra.Command{
	Use:   "pull",
	Short: "Fetch live state from Railway",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("config pull: not yet implemented")
		return nil
	},
}
```

`cmd/config/diff.go`:
```go
package config

import (
	"fmt"

	"github.com/spf13/cobra"
)

var diffCmd = &cobra.Command{
	Use:   "diff",
	Short: "Compare local config against live state",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("config diff: not yet implemented")
		return nil
	},
}
```

`cmd/config/apply.go`:
```go
package config

import (
	"fmt"

	"github.com/spf13/cobra"
)

var applyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Push configuration changes to Railway",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("config apply: not yet implemented")
		return nil
	},
}
```

**Step 2: Wire config group into root**

Update `cmd/root.go` to add:
```go
import (
	"github.com/hamishmorgan/fat-controller/cmd/auth"
	cmdconfig "github.com/hamishmorgan/fat-controller/cmd/config"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(auth.Cmd)
	rootCmd.AddCommand(cmdconfig.Cmd)
}
```

Note: import alias `cmdconfig` avoids collision with any `config` package.

**Step 3: Verify**

Run:
```bash
go build -o fat-controller . && ./fat-controller config
```
Expected: Help showing pull, diff, apply subcommands

Run:
```bash
./fat-controller config pull
```
Expected: "config pull: not yet implemented"

**Step 4: Commit**

```bash
git add cmd/config/ cmd/root.go
git commit -m "feat: add config subcommand group with pull/diff/apply stubs"
```

---

### Task 5: XDG paths module

**Files:**
- Create: `internal/platform/paths.go`
- Create: `internal/platform/paths_test.go`

This module wraps `adrg/xdg` to provide app-specific paths. All other
packages use this instead of calling xdg directly — single place to
change the app name.

**Step 1: Write the test**

`internal/platform/paths_test.go`:
```go
package platform

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	dir := ConfigDir()
	want := filepath.Join(tmp, "fat-controller")
	if dir != want {
		t.Errorf("ConfigDir() = %q, want %q", dir, want)
	}
}

func TestAuthFilePath(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	path := AuthFilePath()
	want := filepath.Join(tmp, "fat-controller", "auth.json")
	if path != want {
		t.Errorf("AuthFilePath() = %q, want %q", path, want)
	}
}

func TestConfigFilePath(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	path := ConfigFilePath()
	want := filepath.Join(tmp, "fat-controller", "config.toml")
	if path != want {
		t.Errorf("ConfigFilePath() = %q, want %q", path, want)
	}
}

func TestEnsureConfigDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	dir, err := EnsureConfigDir()
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
	if err := os.MkdirAll(dir, 0700); err != nil {
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

**Note:** `adrg/xdg` reads `XDG_CONFIG_HOME` at init time. The `t.Setenv`
approach sets the env var before the test function runs, but xdg may have
already cached the value. If tests fail because xdg cached the original
value, the paths module should use `xdg.ConfigHome` (which is a mutable
package-level variable that can be reassigned in tests) or we should
construct paths from the env var directly. Verify this works; if not,
adjust the implementation to read `os.Getenv("XDG_CONFIG_HOME")` with
a fallback to `~/.config` instead of using the xdg library for this
specific case.

**Step 5: Commit**

```bash
git add internal/platform/
git commit -m "feat: add XDG-compliant path helpers for config and auth files"
```

---

### Task 6: Token store — keyring with file fallback

**Files:**
- Create: `internal/auth/store.go`
- Create: `internal/auth/store_test.go`

This module handles persisting and retrieving OAuth tokens. It tries the
OS keyring first, falls back to a JSON file.

**Step 1: Write the test**

`internal/auth/store_test.go`:
```go
package auth

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/zalando/go-keyring"
)

func TestTokenStore_SaveAndLoad_Keyring(t *testing.T) {
	keyring.MockInit()

	store := &TokenStore{
		keyringService: "fat-controller-test",
		keyringUser:    "oauth-token",
		fallbackPath:   filepath.Join(t.TempDir(), "auth.json"),
	}

	tokens := &StoredTokens{
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
	// Simulate broken keyring
	keyring.MockInitWithError(errKeyringUnavailable)

	fallbackPath := filepath.Join(t.TempDir(), "auth.json")
	store := &TokenStore{
		keyringService: "fat-controller-test",
		keyringUser:    "oauth-token",
		fallbackPath:   fallbackPath,
	}

	tokens := &StoredTokens{
		AccessToken:  "access-abc",
		RefreshToken: "refresh-def",
		ClientID:     "client-ghi",
	}

	if err := store.Save(tokens); err != nil {
		t.Fatal(err)
	}

	// Verify file was created with correct permissions
	info, err := os.Stat(fallbackPath)
	if err != nil {
		t.Fatalf("fallback file not created: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
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

	store := &TokenStore{
		keyringService: "fat-controller-test",
		keyringUser:    "oauth-token",
		fallbackPath:   filepath.Join(t.TempDir(), "auth.json"),
	}

	tokens := &StoredTokens{
		AccessToken:  "access-123",
		RefreshToken: "refresh-456",
		ClientID:     "client-789",
	}
	store.Save(tokens)

	if err := store.Delete(); err != nil {
		t.Fatal(err)
	}

	_, err := store.Load()
	if err != ErrNoStoredTokens {
		t.Errorf("expected ErrNoStoredTokens, got %v", err)
	}
}

func TestTokenStore_Delete_FileFallback(t *testing.T) {
	keyring.MockInitWithError(errKeyringUnavailable)

	fallbackPath := filepath.Join(t.TempDir(), "auth.json")
	store := &TokenStore{
		keyringService: "fat-controller-test",
		keyringUser:    "oauth-token",
		fallbackPath:   fallbackPath,
	}

	tokens := &StoredTokens{
		AccessToken:  "access-abc",
		RefreshToken: "refresh-def",
		ClientID:     "client-ghi",
	}
	store.Save(tokens)

	if err := store.Delete(); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(fallbackPath); !os.IsNotExist(err) {
		t.Errorf("fallback file should be deleted")
	}
}

func TestTokenStore_Load_Empty(t *testing.T) {
	keyring.MockInit()

	store := &TokenStore{
		keyringService: "fat-controller-test",
		keyringUser:    "oauth-token",
		fallbackPath:   filepath.Join(t.TempDir(), "auth.json"),
	}

	_, err := store.Load()
	if err != ErrNoStoredTokens {
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

var (
	// ErrNoStoredTokens is returned when no tokens are found in keyring or file.
	ErrNoStoredTokens = errors.New("no stored tokens found")

	// errKeyringUnavailable is an internal sentinel for keyring failures.
	errKeyringUnavailable = errors.New("keyring unavailable")
)

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

// NewTokenStore creates a TokenStore with the default app-specific settings.
func NewTokenStore(fallbackPath string) *TokenStore {
	return &TokenStore{
		keyringService: "fat-controller",
		keyringUser:    "oauth-token",
		fallbackPath:   fallbackPath,
	}
}

// Save persists tokens. Tries keyring first, falls back to file.
func (s *TokenStore) Save(tokens *StoredTokens) error {
	data, err := json.Marshal(tokens)
	if err != nil {
		return fmt.Errorf("marshalling tokens: %w", err)
	}

	if err := keyring.Set(s.keyringService, s.keyringUser, string(data)); err != nil {
		// Keyring unavailable — fall back to file
		return s.saveToFile(data)
	}
	return nil
}

// Load retrieves stored tokens. Tries keyring first, then file.
// Returns ErrNoStoredTokens if nothing is stored anywhere.
func (s *TokenStore) Load() (*StoredTokens, error) {
	// Try keyring
	data, err := keyring.Get(s.keyringService, s.keyringUser)
	if err == nil {
		var tokens StoredTokens
		if err := json.Unmarshal([]byte(data), &tokens); err != nil {
			return nil, fmt.Errorf("unmarshalling keyring data: %w", err)
		}
		return &tokens, nil
	}

	if !errors.Is(err, keyring.ErrNotFound) {
		// Keyring error (not "not found") — try file fallback
	}

	// Try file
	return s.loadFromFile()
}

// Delete removes stored tokens from both keyring and file.
func (s *TokenStore) Delete() error {
	// Delete from keyring (ignore ErrNotFound)
	if err := keyring.Delete(s.keyringService, s.keyringUser); err != nil && !errors.Is(err, keyring.ErrNotFound) {
		// Keyring error — not fatal, continue to file cleanup
	}

	// Delete file if it exists
	if err := os.Remove(s.fallbackPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing fallback file: %w", err)
	}
	return nil
}

func (s *TokenStore) saveToFile(data []byte) error {
	dir := filepath.Dir(s.fallbackPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	if err := os.WriteFile(s.fallbackPath, data, 0600); err != nil {
		return fmt.Errorf("writing fallback file: %w", err)
	}
	return nil
}

func (s *TokenStore) loadFromFile() (*StoredTokens, error) {
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
git commit -m "feat: add token store with OS keyring and JSON file fallback"
```

---

### Task 7: Token resolver — env var > keyring > file

**Files:**
- Create: `internal/auth/resolver.go`
- Create: `internal/auth/resolver_test.go`

This module determines which token to use and what auth header to send.
Project access tokens use `Project-Access-Token`, account-level tokens
use `Authorization: Bearer`.

**Step 1: Write the test**

`internal/auth/resolver_test.go`:
```go
package auth

import (
	"path/filepath"
	"testing"

	"github.com/zalando/go-keyring"
)

func TestResolveToken_EnvVarTakesPrecedence(t *testing.T) {
	keyring.MockInit()

	store := &TokenStore{
		keyringService: "fat-controller-test",
		keyringUser:    "oauth-token",
		fallbackPath:   filepath.Join(t.TempDir(), "auth.json"),
	}
	// Store an OAuth token
	store.Save(&StoredTokens{AccessToken: "stored-token"})

	// Env var should win
	t.Setenv("RAILWAY_TOKEN", "env-token")

	resolved, err := ResolveAuth(store)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Token != "env-token" {
		t.Errorf("Token = %q, want %q", resolved.Token, "env-token")
	}
	if resolved.HeaderName != "Project-Access-Token" {
		t.Errorf("HeaderName = %q, want %q", resolved.HeaderName, "Project-Access-Token")
	}
}

func TestResolveToken_FallsBackToStore(t *testing.T) {
	keyring.MockInit()

	store := &TokenStore{
		keyringService: "fat-controller-test",
		keyringUser:    "oauth-token",
		fallbackPath:   filepath.Join(t.TempDir(), "auth.json"),
	}
	store.Save(&StoredTokens{AccessToken: "stored-token"})

	// No env var set
	t.Setenv("RAILWAY_TOKEN", "")

	resolved, err := ResolveAuth(store)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Token != "stored-token" {
		t.Errorf("Token = %q, want %q", resolved.Token, "stored-token")
	}
	if resolved.HeaderName != "Authorization" {
		t.Errorf("HeaderName = %q, want %q", resolved.HeaderName, "Authorization")
	}
	if resolved.HeaderValue != "Bearer stored-token" {
		t.Errorf("HeaderValue = %q, want %q", resolved.HeaderValue, "Bearer stored-token")
	}
}

func TestResolveToken_NothingAvailable(t *testing.T) {
	keyring.MockInit()

	store := &TokenStore{
		keyringService: "fat-controller-test",
		keyringUser:    "oauth-token",
		fallbackPath:   filepath.Join(t.TempDir(), "auth.json"),
	}

	t.Setenv("RAILWAY_TOKEN", "")

	_, err := ResolveAuth(store)
	if err != ErrNotAuthenticated {
		t.Errorf("expected ErrNotAuthenticated, got %v", err)
	}
}
```

**Step 2: Run tests to verify they fail**

Run:
```bash
go test ./internal/auth/ -v -run TestResolve
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

var ErrNotAuthenticated = errors.New("not authenticated: run 'fat-controller auth login' or set RAILWAY_TOKEN")

// ResolvedAuth contains the resolved token and the HTTP header to use.
type ResolvedAuth struct {
	Token       string
	HeaderName  string
	HeaderValue string
	Source      string // "env", "keyring", "file" — for diagnostics
}

// ResolveAuth determines the active auth token using the precedence:
// 1. RAILWAY_TOKEN env var (project access token assumed)
// 2. Stored OAuth token (from keyring or file)
func ResolveAuth(store *TokenStore) (*ResolvedAuth, error) {
	// 1. Environment variable
	if token := os.Getenv("RAILWAY_TOKEN"); token != "" {
		return &ResolvedAuth{
			Token:       token,
			HeaderName:  "Project-Access-Token",
			HeaderValue: token,
			Source:      "env",
		}, nil
	}

	// 2. Stored OAuth token
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
go test ./internal/auth/ -v -run TestResolve
```
Expected: All 3 tests pass.

**Step 5: Commit**

```bash
git add internal/auth/resolver.go internal/auth/resolver_test.go
git commit -m "feat: add token resolver with env var > keyring > file precedence"
```

---

### Task 8: PKCE helpers

**Files:**
- Create: `internal/auth/pkce.go`
- Create: `internal/auth/pkce_test.go`

**Step 1: Write the test**

`internal/auth/pkce_test.go`:
```go
package auth

import (
	"crypto/sha256"
	"encoding/base64"
	"testing"
)

func TestGenerateCodeVerifier(t *testing.T) {
	v1, err := GenerateCodeVerifier()
	if err != nil {
		t.Fatal(err)
	}

	// Must be at least 43 characters (RFC 7636)
	if len(v1) < 43 {
		t.Errorf("verifier too short: %d chars", len(v1))
	}

	// Must be different each time
	v2, err := GenerateCodeVerifier()
	if err != nil {
		t.Fatal(err)
	}
	if v1 == v2 {
		t.Error("two verifiers should not be identical")
	}
}

func TestCodeChallenge(t *testing.T) {
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"

	challenge := CodeChallenge(verifier)

	// Manually compute expected value
	h := sha256.Sum256([]byte(verifier))
	want := base64.RawURLEncoding.EncodeToString(h[:])

	if challenge != want {
		t.Errorf("CodeChallenge() = %q, want %q", challenge, want)
	}
}

func TestGenerateState(t *testing.T) {
	s1, err := GenerateState()
	if err != nil {
		t.Fatal(err)
	}
	if len(s1) == 0 {
		t.Error("state should not be empty")
	}

	s2, err := GenerateState()
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
git commit -m "feat: add PKCE code verifier, challenge, and state helpers"
```

---

### Task 9: OAuth client — registration, auth URL, token exchange

**Files:**
- Create: `internal/auth/oauth.go`
- Create: `internal/auth/oauth_test.go`

This is the core OAuth client. It handles dynamic client registration,
building the authorization URL, exchanging codes for tokens, and refreshing.
The actual HTTP calls use `net/http` — tests use `httptest.NewServer`.

**Step 1: Write the test**

`internal/auth/oauth_test.go`:
```go
package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestOAuthClient_RegisterClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected application/json content type")
		}

		var req RegistrationRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if req.ClientName != "Fat Controller CLI" {
			t.Errorf("ClientName = %q, want %q", req.ClientName, "Fat Controller CLI")
		}
		if req.TokenEndpointAuthMethod != "none" {
			t.Errorf("TokenEndpointAuthMethod = %q, want %q", req.TokenEndpointAuthMethod, "none")
		}
		if req.ApplicationType != "native" {
			t.Errorf("ApplicationType = %q, want %q", req.ApplicationType, "native")
		}

		json.NewEncoder(w).Encode(RegistrationResponse{
			ClientID:   "test-client-id",
			ClientName: "Fat Controller CLI",
		})
	}))
	defer server.Close()

	client := &OAuthClient{
		RegistrationURL: server.URL,
	}
	resp, err := client.RegisterClient("http://127.0.0.1:12345/callback")
	if err != nil {
		t.Fatal(err)
	}
	if resp.ClientID != "test-client-id" {
		t.Errorf("ClientID = %q, want %q", resp.ClientID, "test-client-id")
	}
}

func TestOAuthClient_AuthorizationURL(t *testing.T) {
	client := &OAuthClient{
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

	// Verify scope contains required values
	scope := parsed.Query().Get("scope")
	for _, required := range []string{"openid", "offline_access"} {
		found := false
		for _, s := range splitScope(scope) {
			if s == required {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("scope missing %q, got %q", required, scope)
		}
	}
}

func TestOAuthClient_ExchangeCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
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
		// Native client: client_id in body, no secret
		if r.Form.Get("client_id") != "client-123" {
			t.Errorf("client_id = %q", r.Form.Get("client_id"))
		}

		json.NewEncoder(w).Encode(TokenResponse{
			AccessToken:  "new-access-token",
			RefreshToken: "new-refresh-token",
			ExpiresIn:    3600,
			TokenType:    "Bearer",
		})
	}))
	defer server.Close()

	client := &OAuthClient{TokenEndpoint: server.URL}

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
		r.ParseForm()
		if r.Form.Get("grant_type") != "refresh_token" {
			t.Errorf("grant_type = %q", r.Form.Get("grant_type"))
		}
		if r.Form.Get("refresh_token") != "old-refresh" {
			t.Errorf("refresh_token = %q", r.Form.Get("refresh_token"))
		}
		if r.Form.Get("client_id") != "client-123" {
			t.Errorf("client_id = %q", r.Form.Get("client_id"))
		}

		json.NewEncoder(w).Encode(TokenResponse{
			AccessToken:  "refreshed-access",
			RefreshToken: "rotated-refresh",
			ExpiresIn:    3600,
		})
	}))
	defer server.Close()

	client := &OAuthClient{TokenEndpoint: server.URL}

	resp, err := client.RefreshToken("client-123", "old-refresh")
	if err != nil {
		t.Fatal(err)
	}
	if resp.AccessToken != "refreshed-access" {
		t.Errorf("AccessToken = %q", resp.AccessToken)
	}
	if resp.RefreshToken != "rotated-refresh" {
		t.Errorf("RefreshToken = %q, want rotated token", resp.RefreshToken)
	}
}

func splitScope(s string) []string {
	var result []string
	for _, part := range []byte(s) {
		_ = part
	}
	// Simple split by space
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ' ' {
			if i > start {
				result = append(result, s[start:i])
			}
			start = i + 1
		}
	}
	return result
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
git commit -m "feat: add OAuth client with registration, auth URL, code exchange, and refresh"
```

---

### Task 10: Callback server

**Files:**
- Create: `internal/auth/callback.go`
- Create: `internal/auth/callback_test.go`

The local HTTP server that receives the OAuth redirect.

**Step 1: Write the test**

`internal/auth/callback_test.go`:
```go
package auth

import (
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestCallbackServer_ReceivesCode(t *testing.T) {
	srv, err := StartCallbackServer()
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Shutdown()

	if srv.Port == 0 {
		t.Fatal("port should be non-zero")
	}

	// Simulate browser redirect
	go func() {
		url := fmt.Sprintf("http://127.0.0.1:%d/callback?code=test-auth-code&state=test-state", srv.Port)
		http.Get(url)
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
	srv, err := StartCallbackServer()
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Shutdown()

	go func() {
		url := fmt.Sprintf("http://127.0.0.1:%d/callback?error=access_denied&error_description=User+denied+access", srv.Port)
		http.Get(url)
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
	srv, err := StartCallbackServer()
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
	go srv.Serve(listener)

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
	s.server.Shutdown(context.Background())
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
git commit -m "feat: add local callback server for OAuth redirect handling"
```

---

### Task 11: Wire up `auth login` command

**Files:**
- Create: `internal/auth/login.go`
- Modify: `cmd/auth/login.go`

This orchestrates the full login flow: start callback server → register
client (if needed) → generate PKCE → open browser → wait for callback →
exchange code → store tokens.

**Step 1: Write the login orchestrator**

`internal/auth/login.go`:
```go
package auth

import (
	"fmt"
	"os/exec"
	"runtime"
)

// Login performs the full OAuth login flow:
// 1. Start callback server
// 2. Register client if no client ID stored
// 3. Generate PKCE verifier + state
// 4. Open browser to authorization URL
// 5. Wait for callback
// 6. Exchange code for tokens
// 7. Store tokens
func Login(oauth *OAuthClient, store *TokenStore) error {
	// Start callback server
	srv, err := StartCallbackServer()
	if err != nil {
		return fmt.Errorf("starting callback server: %w", err)
	}
	defer srv.Shutdown()

	redirectURI := srv.RedirectURI()

	// Check for existing client registration
	clientID, err := loadOrRegisterClient(oauth, store, redirectURI)
	if err != nil {
		return fmt.Errorf("client registration: %w", err)
	}

	// Generate PKCE
	verifier, err := GenerateCodeVerifier()
	if err != nil {
		return fmt.Errorf("generating code verifier: %w", err)
	}
	challenge := CodeChallenge(verifier)

	state, err := GenerateState()
	if err != nil {
		return fmt.Errorf("generating state: %w", err)
	}

	// Build authorization URL and open browser
	authURL := oauth.AuthorizationURL(clientID, redirectURI, state, challenge)
	fmt.Println("Opening browser to log in...")
	fmt.Printf("If the browser doesn't open, visit:\n%s\n\n", authURL)

	if err := openBrowser(authURL); err != nil {
		// Non-fatal — user can copy the URL
		fmt.Printf("Could not open browser: %v\n", err)
	}

	// Wait for callback
	fmt.Println("Waiting for authorization...")
	result := <-srv.Result

	if result.Error != "" {
		return fmt.Errorf("authorization failed: %s: %s", result.Error, result.ErrorDescription)
	}

	if result.State != state {
		return fmt.Errorf("state mismatch: possible CSRF attack")
	}

	// Exchange code for tokens
	tokenResp, err := oauth.ExchangeCode(clientID, result.Code, redirectURI, verifier)
	if err != nil {
		return fmt.Errorf("exchanging authorization code: %w", err)
	}

	// Store tokens
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
	// Check if we already have a client ID from a previous login
	existing, err := store.Load()
	if err == nil && existing.ClientID != "" {
		return existing.ClientID, nil
	}

	// Register a new client
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

**Step 2: Update the login command to call the orchestrator**

`cmd/auth/login.go`:
```go
package auth

import (
	internalauth "github.com/hamishmorgan/fat-controller/internal/auth"
	"github.com/hamishmorgan/fat-controller/internal/platform"
	"github.com/spf13/cobra"
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Log in to Railway via browser-based OAuth",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		oauth := internalauth.NewOAuthClient()
		store := internalauth.NewTokenStore(platform.AuthFilePath())
		return internalauth.Login(oauth, store)
	},
}
```

**Step 3: Verify it builds**

Run:
```bash
go build -o fat-controller .
```
Expected: Compiles without errors.

**Step 4: Commit**

```bash
git add internal/auth/login.go cmd/auth/login.go
git commit -m "feat: wire up auth login with full OAuth + PKCE flow"
```

---

### Task 12: Wire up `auth logout` command

**Files:**
- Modify: `cmd/auth/logout.go`

**Step 1: Update logout to clear tokens**

`cmd/auth/logout.go`:
```go
package auth

import (
	"fmt"

	internalauth "github.com/hamishmorgan/fat-controller/internal/auth"
	"github.com/hamishmorgan/fat-controller/internal/platform"
	"github.com/spf13/cobra"
)

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Clear stored credentials",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		store := internalauth.NewTokenStore(platform.AuthFilePath())
		if err := store.Delete(); err != nil {
			return fmt.Errorf("clearing credentials: %w", err)
		}
		fmt.Println("Logged out successfully.")
		return nil
	},
}
```

**Step 2: Verify it builds**

Run:
```bash
go build -o fat-controller .
```
Expected: Compiles without errors.

**Step 3: Commit**

```bash
git add cmd/auth/logout.go
git commit -m "feat: wire up auth logout to clear stored tokens"
```

---

### Task 13: Wire up `auth status` command

**Files:**
- Create: `internal/auth/userinfo.go`
- Create: `internal/auth/userinfo_test.go`
- Modify: `cmd/auth/status.go`

**Step 1: Write the test for userinfo**

`internal/auth/userinfo_test.go`:
```go
package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchUserInfo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}

		json.NewEncoder(w).Encode(UserInfo{
			Sub:   "user_abc123",
			Email: "test@example.com",
			Name:  "Test User",
		})
	}))
	defer server.Close()

	client := &OAuthClient{
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
	req, err := http.NewRequest("GET", c.UserinfoURL, nil)
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

**Step 5: Update the status command**

`cmd/auth/status.go`:
```go
package auth

import (
	"fmt"

	internalauth "github.com/hamishmorgan/fat-controller/internal/auth"
	"github.com/hamishmorgan/fat-controller/internal/platform"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current authentication status",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		store := internalauth.NewTokenStore(platform.AuthFilePath())

		resolved, err := internalauth.ResolveAuth(store)
		if err != nil {
			fmt.Println("Not authenticated.")
			fmt.Println("Run 'fat-controller auth login' or set RAILWAY_TOKEN.")
			return nil
		}

		fmt.Printf("Authenticated via: %s\n", resolved.Source)

		if resolved.Source == "env" {
			fmt.Println("Using RAILWAY_TOKEN environment variable (project access token).")
			return nil
		}

		// For OAuth tokens, fetch user info
		oauth := internalauth.NewOAuthClient()
		info, err := oauth.FetchUserInfo(resolved.Token)
		if err != nil {
			fmt.Printf("Authenticated (could not fetch user info: %v)\n", err)
			return nil
		}

		fmt.Printf("User: %s\n", info.Name)
		fmt.Printf("Email: %s\n", info.Email)
		return nil
	},
}
```

**Step 6: Verify it builds**

Run:
```bash
go build -o fat-controller .
```
Expected: Compiles without errors.

**Step 7: Commit**

```bash
git add internal/auth/userinfo.go internal/auth/userinfo_test.go cmd/auth/status.go
git commit -m "feat: wire up auth status with userinfo display"
```

---

### Task 14: Run full test suite and verify build

**Step 1: Run all tests**

Run:
```bash
go test ./... -v
```
Expected: All tests pass.

**Step 2: Run vet and check for issues**

Run:
```bash
go vet ./...
```
Expected: No issues.

**Step 3: Build the binary**

Run:
```bash
go build -o fat-controller .
```
Expected: Clean build.

**Step 4: Smoke test the CLI**

Run:
```bash
./fat-controller --help
./fat-controller auth --help
./fat-controller auth login --help
./fat-controller auth status
./fat-controller auth logout
./fat-controller config --help
./fat-controller config pull
```
Expected: Each shows appropriate help or stub output.

**Step 5: Commit any fixes**

If any issues were found, fix and commit:
```bash
git add -A
git commit -m "fix: address issues found during final verification"
```

---

## Summary

After completing all tasks, the project will have:

| Component | Location |
|-----------|----------|
| CLI entrypoint | `main.go` |
| Root command | `cmd/root.go` |
| Auth commands | `cmd/auth/{auth,login,logout,status}.go` |
| Config command stubs | `cmd/config/{config,pull,diff,apply}.go` |
| XDG path helpers | `internal/platform/paths.go` |
| Token store (keyring + file) | `internal/auth/store.go` |
| Token resolver (env > keyring > file) | `internal/auth/resolver.go` |
| PKCE helpers | `internal/auth/pkce.go` |
| OAuth client | `internal/auth/oauth.go` |
| Callback server | `internal/auth/callback.go` |
| Login orchestrator | `internal/auth/login.go` |
| Userinfo fetcher | `internal/auth/userinfo.go` |

**Not included in M1** (deferred to M2+):
- genqlient / GraphQL schema
- koanf config loading (no tool config to load yet — auth doesn't need it)
- `config pull/diff/apply` implementations

**Testing approach:**
- Unit tests with `go-keyring` `MockInit()` for token store
- `httptest.NewServer` for OAuth endpoint mocking
- `t.TempDir()` + `t.Setenv("XDG_CONFIG_HOME", ...)` for file path tests
