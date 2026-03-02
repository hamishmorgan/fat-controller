# M2: Schema + GQL Client — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Set up genqlient code generation against the Railway GraphQL schema, build a refresh-aware authenticated HTTP transport, and verify the pipeline works end-to-end with a `projectToken` query.

**Architecture:** Apollo Rover introspects the Railway schema into `internal/railway/schema.graphql` (checked in). genqlient generates typed Go functions from `.graphql` operation files. An authenticated `http.RoundTripper` wraps requests with the correct auth header (Bearer vs Project-Access-Token) based on `ResolvedAuth`, and transparently refreshes expired OAuth tokens. A `Client` struct in `internal/railway/client.go` ties it all together.

**Tech Stack:** Go, Khan/genqlient, suessflorian/gqlfetch (indirect via genqlient), apollo-rover (mise tool, introspection only), existing internal/auth package

---

## Background: genqlient

[genqlient](https://github.com/Khan/genqlient) generates typed Go functions from `.graphql` operation files against a GraphQL schema. Key concepts:

- **`genqlient.yaml`** — config file at repo root. Points to schema, operation globs, output file.
- **`schema.graphql`** — the Railway API schema in SDL format. Fetched via introspection, checked into git.
- **`operations.graphql`** — queries and mutations we use. Each becomes a typed Go function.
- **`generated.go`** — genqlient output. Checked into git (CI doesn't need a Railway token).
- **`//go:generate go run github.com/Khan/genqlient`** — runs code generation.
- Generated functions have signature: `func queryName(ctx context.Context, client graphql.Client, args...) (*queryNameResponse, error)`
- `graphql.Client` is created via `graphql.NewClient(url, httpClient)` where `httpClient` has a custom `RoundTripper` for auth.
- Fragments in `.graphql` files create shared Go types across queries/mutations.

### genqlient annotations

In `.graphql` files, `# @genqlient(...)` directives customize code generation:

- `omitempty: true` — send null for zero-value fields in input objects
- `pointer: true` — use `*Type` in Go
- `for: "InputType.field"` — apply directive to a specific field of an input type

### The Railway Terraform provider pattern

The [community Terraform provider](https://github.com/terraform-community-providers/terraform-provider-railway) uses genqlient with the same Railway API. Their pattern:

- One `.graphql` file per resource, co-located with `.go` implementation
- Fragments for reusing response shapes across CRUD operations
- `authedTransport` wraps `http.RoundTripper` to inject auth header
- Custom scalar bindings for `DateTime` → `time.Time`, `JSON` → `map[string]interface{}`

## Background: Token Refresh

Railway access tokens have a 1-hour TTL. Refresh tokens rotate — when you use one, Railway issues a new access token AND a new refresh token. The old refresh token is invalidated.

The refresh-aware transport must:

1. Attach the current access token to every request
2. If a request returns 401, attempt a token refresh
3. If refresh succeeds, save the new tokens (both access + refresh) and retry the original request
4. If refresh fails, return the error (user needs to `auth login` again)

This only applies to stored OAuth tokens. Flag/env-var tokens are never refreshed.

## Background: Auth Header Resolution

The existing `auth.ResolvedAuth` struct (from `internal/auth/resolver.go`) contains:

```go
type ResolvedAuth struct {
    Token       string
    HeaderName  string    // "Authorization" or "Project-Access-Token"
    HeaderValue string    // "Bearer <token>" or "<token>"
    Source      string    // "flag", "env:RAILWAY_API_TOKEN", "env:RAILWAY_TOKEN", "stored"
}
```

The transport uses `HeaderName` and `HeaderValue` directly — no need to know which token type it is.

## Background: Project Structure After M2

```text
fat-controller/
├── genqlient.yaml                    # genqlient config (points to internal/railway/)
├── main.go
├── cmd/cli.go
├── internal/
│   ├── auth/                         # Existing — OAuth, keyring, token resolution
│   │   ├── resolver.go               # ResolvedAuth (used by transport)
│   │   ├── oauth.go                  # OAuthClient.RefreshToken (used by transport)
│   │   ├── store.go                  # TokenStore (used by transport for saving refreshed tokens)
│   │   └── ...
│   ├── platform/                     # Existing — XDG paths
│   └── railway/                      # NEW — GQL client
│       ├── schema.graphql            # Introspected Railway schema (checked in)
│       ├── operations.graphql        # Queries + mutations (starts with projectToken)
│       ├── generated.go              # genqlient output (checked in)
│       ├── client.go                 # Client struct, NewClient(), authed transport
│       ├── client_test.go            # Tests for client + transport
│       └── generate.go              # //go:generate directive
```

---

### Task 1: Add apollo-rover to mise and create introspection task

**Files:**

- Modify: `.config/mise/config.toml`

**Step 1: Add apollo-rover to mise tools**

Open `.config/mise/config.toml` and add `apollo-rover` to the `[tools]` section:

```toml
[tools]
go = "latest"
golangci-lint = "latest"
"npm:markdownlint-cli2" = "latest"
taplo = "latest"
actionlint = "latest"
apollo-rover = "latest"
```

**Step 2: Add introspection task**

Add a new task section to `.config/mise/config.toml`, after the GitHub Actions section:

```toml
# ---------------------------------------------------------------------------
# GraphQL Schema
# ---------------------------------------------------------------------------

[tasks."schema:introspect"]
description = "Fetch Railway GraphQL schema via introspection (requires RAILWAY_API_TOKEN)"
run = """
rover graph introspect https://backboard.railway.com/graphql/v2 \
  --header "Authorization: Bearer ${RAILWAY_API_TOKEN:?Set RAILWAY_API_TOKEN to introspect the schema}" \
  --output internal/railway/schema.graphql
"""
```

**Step 3: Install the new tool**

Run:

```bash
mise install
```

Expected: `apollo-rover` installed successfully.

**Step 4: Verify rover is available**

Run:

```bash
rover --version
```

Expected: Version output (e.g., `Rover 0.28.x`).

**Step 5: Commit**

```bash
git add .config/mise/config.toml
git commit -m "Add apollo-rover and schema introspection task to mise"
```

---

### Task 2: Introspect the Railway schema

This task requires a valid `RAILWAY_API_TOKEN`. Get one from the Railway dashboard (Account Settings → Tokens → Create Token).

**Files:**

- Create: `internal/railway/schema.graphql`

**Step 1: Create the railway directory**

Run:

```bash
mkdir -p internal/railway
```

**Step 2: Run introspection**

Run:

```bash
RAILWAY_API_TOKEN=<your-token> mise run schema:introspect
```

Expected: `internal/railway/schema.graphql` created with the full Railway API schema in SDL format. The file will be large (thousands of lines).

**Step 3: Verify the schema looks correct**

Run:

```bash
head -30 internal/railway/schema.graphql
```

Expected: SDL content starting with `type Query {` or `schema {` and containing Railway types like `Project`, `Service`, `Environment`, etc.

**Step 4: Verify key types exist**

Run:

```bash
grep -c 'type ' internal/railway/schema.graphql
```

Expected: Dozens of type definitions. Also spot-check:

```bash
grep 'projectToken' internal/railway/schema.graphql
grep 'variableCollectionUpsert' internal/railway/schema.graphql
grep 'serviceInstance' internal/railway/schema.graphql
```

Expected: All three should match — these are the queries/mutations we'll use in M3-M5.

**Step 5: Commit**

```bash
git add internal/railway/schema.graphql
git commit -m "Add Railway GraphQL schema via introspection"
```

---

### Task 3: Add genqlient dependency and configuration

**Files:**

- Modify: `go.mod`, `go.sum`
- Create: `genqlient.yaml`
- Create: `internal/railway/generate.go`

**Step 1: Add genqlient dependency**

Run:

```bash
go get github.com/Khan/genqlient@latest
```

**Step 2: Create the tools file to prevent go mod tidy from pruning genqlient**

Create `internal/railway/generate.go`:

```go
package railway

//go:generate go run github.com/Khan/genqlient
```

This file serves two purposes: (1) prevents `go mod tidy` from removing genqlient (since it's imported via `go run`), and (2) wires up `go generate`.

**Step 3: Create genqlient.yaml at repo root**

Create `genqlient.yaml`:

```yaml
schema: internal/railway/schema.graphql
operations:
  - internal/railway/operations.graphql
generated: internal/railway/generated.go
optional: pointer
bindings:
  DateTime:
    type: time.Time
  JSON:
    type: map[string]interface{}
  EnvironmentVariables:
    type: map[string]interface{}
  DeploymentMeta:
    type: map[string]interface{}
  PluginType:
    type: string
```

**Key config choices:**

- `optional: pointer` — nullable GraphQL fields become Go pointers. This is safer than `value` (which silently zero-fills nulls) and matches Railway's schema where many fields are optional.
- `bindings` — maps Railway's custom scalars to Go types. These 5 bindings match what the Terraform provider uses. `DateTime` → `time.Time` is standard. `JSON`, `EnvironmentVariables`, and `DeploymentMeta` are opaque JSON blobs. `PluginType` is a string enum.

**Step 4: Run `go mod tidy`**

Run:

```bash
go mod tidy
```

Expected: `go.mod` and `go.sum` updated with genqlient and its dependencies.

**Step 5: Commit**

```bash
git add genqlient.yaml internal/railway/generate.go go.mod go.sum
git commit -m "Add genqlient config and code generation setup"
```

---

### Task 4: Write the projectToken operation and generate code

**Files:**

- Create: `internal/railway/operations.graphql`
- Create: `internal/railway/generated.go` (via genqlient)

**Step 1: Create the operations file with projectToken query**

Create `internal/railway/operations.graphql`:

```graphql
# projectToken resolves a project-scoped token to its project and environment IDs.
# Used by auth status and as a basic connectivity check.
query projectToken {
  projectToken {
    projectId
    environmentId
  }
}
```

This is the simplest useful query — it takes no arguments (the token is in the header) and returns the project + environment IDs that the token is scoped to. Only works with `RAILWAY_TOKEN` (project-scoped tokens), not account-level tokens.

**Step 2: Run genqlient code generation**

Run:

```bash
go generate ./internal/railway/
```

Expected: `internal/railway/generated.go` created. If you see errors about missing types or scalars, check that the `bindings` in `genqlient.yaml` cover all custom scalars used in the schema.

**Step 3: Verify the generated code compiles**

Run:

```bash
go build ./internal/railway/
```

Expected: Build succeeds with no errors.

**Step 4: Inspect the generated function**

Open `internal/railway/generated.go` and look for the `projectToken` function. It should look approximately like:

```go
func projectToken(
    ctx_ context.Context,
    client_ graphql.Client,
) (*projectTokenResponse, error)
```

And the response type:

```go
type projectTokenResponse struct {
    ProjectToken projectTokenProjectToken `json:"projectToken"`
}

type projectTokenProjectToken struct {
    ProjectId     string `json:"projectId"`
    EnvironmentId string `json:"environmentId"`
}
```

**Step 5: Commit**

```bash
git add internal/railway/operations.graphql internal/railway/generated.go
git commit -m "Add projectToken query and generate typed GQL code"
```

---

### Task 5: Build the authenticated HTTP transport

This is the core of M2 — an `http.RoundTripper` that:

1. Injects the correct auth header on every request
2. Transparently refreshes expired OAuth tokens on 401

**Files:**

- Create: `internal/railway/transport.go`
- Create: `internal/railway/transport_test.go`

**Step 1: Write the failing tests**

Create `internal/railway/transport_test.go`:

```go
package railway_test

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/auth"
	"github.com/hamishmorgan/fat-controller/internal/railway"
)

func TestAuthTransport_InjectsHeader(t *testing.T) {
	var gotHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	resolved := &auth.ResolvedAuth{
		Token:       "test-token",
		HeaderName:  "Authorization",
		HeaderValue: "Bearer test-token",
		Source:      "flag",
	}
	transport := railway.NewAuthTransport(resolved, nil, nil)
	client := &http.Client{Transport: transport}

	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if gotHeader != "Bearer test-token" {
		t.Errorf("Authorization header = %q, want %q", gotHeader, "Bearer test-token")
	}
}

func TestAuthTransport_ProjectAccessTokenHeader(t *testing.T) {
	var gotHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("Project-Access-Token")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	resolved := &auth.ResolvedAuth{
		Token:       "proj-token",
		HeaderName:  "Project-Access-Token",
		HeaderValue: "proj-token",
		Source:      "env:RAILWAY_TOKEN",
	}
	transport := railway.NewAuthTransport(resolved, nil, nil)
	client := &http.Client{Transport: transport}

	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if gotHeader != "proj-token" {
		t.Errorf("Project-Access-Token header = %q, want %q", gotHeader, "proj-token")
	}
}

func TestAuthTransport_NoRefreshForNonStoredTokens(t *testing.T) {
	// First request returns 401. Transport should NOT attempt refresh
	// because the token source is "flag", not "stored".
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	resolved := &auth.ResolvedAuth{
		Token:       "flag-token",
		HeaderName:  "Authorization",
		HeaderValue: "Bearer flag-token",
		Source:      "flag",
	}
	transport := railway.NewAuthTransport(resolved, nil, nil)
	client := &http.Client{Transport: transport}

	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	// Should return 401 directly — no refresh attempted.
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("StatusCode = %d, want 401", resp.StatusCode)
	}
}
```

**Step 2: Run the tests to verify they fail**

Run:

```bash
go test ./internal/railway/ -run TestAuthTransport -v
```

Expected: FAIL — `railway.NewAuthTransport` does not exist yet.

**Step 3: Write the transport implementation**

Create `internal/railway/transport.go`:

```go
package railway

import (
	"net/http"
	"sync"

	"github.com/hamishmorgan/fat-controller/internal/auth"
)

// Refresher abstracts token refresh so transport doesn't depend on OAuthClient directly.
// In production, this wraps OAuthClient.RefreshToken. In tests, it can be a fake.
type Refresher interface {
	Refresh(clientID, refreshToken string) (*auth.TokenResponse, error)
}

// TokenSaver abstracts saving refreshed tokens. In production, this is a TokenStore.
type TokenSaver interface {
	Save(tokens *auth.StoredTokens) error
}

// AuthTransport is an http.RoundTripper that injects auth headers and
// transparently refreshes expired OAuth tokens.
type AuthTransport struct {
	mu       sync.Mutex
	resolved *auth.ResolvedAuth
	store    TokenSaver
	refresh  Refresher
	base     http.RoundTripper
}

// NewAuthTransport creates a transport that injects auth headers from resolved.
// If store and refresh are non-nil AND the token source is "stored", the
// transport will attempt a token refresh on 401 responses.
func NewAuthTransport(resolved *auth.ResolvedAuth, store TokenSaver, refresh Refresher) *AuthTransport {
	return &AuthTransport{
		resolved: resolved,
		store:    store,
		refresh:  refresh,
		base:     http.DefaultTransport,
	}
}

// RoundTrip implements http.RoundTripper.
func (t *AuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.mu.Lock()
	headerName := t.resolved.HeaderName
	headerValue := t.resolved.HeaderValue
	t.mu.Unlock()

	// Clone the request to avoid mutating the original.
	clone := req.Clone(req.Context())
	clone.Header.Set(headerName, headerValue)

	resp, err := t.base.RoundTrip(clone)
	if err != nil {
		return nil, err
	}

	// Only attempt refresh for stored OAuth tokens on 401.
	if resp.StatusCode == http.StatusUnauthorized && t.canRefresh() {
		refreshed, refreshErr := t.tryRefresh()
		if refreshErr != nil {
			// Refresh failed — return the original 401 response.
			return resp, nil
		}

		// Close the old response body before retrying.
		resp.Body.Close() //nolint:errcheck

		// Update resolved auth with new token.
		t.mu.Lock()
		t.resolved.Token = refreshed.AccessToken
		t.resolved.HeaderValue = "Bearer " + refreshed.AccessToken
		headerValue = t.resolved.HeaderValue
		t.mu.Unlock()

		// Retry the request with the new token.
		retry := req.Clone(req.Context())
		retry.Header.Set(headerName, headerValue)
		return t.base.RoundTrip(retry)
	}

	return resp, nil
}

// canRefresh returns true if this transport has the components needed
// for a token refresh (stored OAuth token + refresh capability).
func (t *AuthTransport) canRefresh() bool {
	return t.resolved.Source == "stored" && t.refresh != nil && t.store != nil
}

// tryRefresh attempts to refresh the stored OAuth token.
// On success, saves the new tokens and returns the response.
func (t *AuthTransport) tryRefresh() (*auth.TokenResponse, error) {
	t.mu.Lock()
	// We need the client ID and refresh token from the store.
	// The resolved auth only has the access token — we need the full stored tokens.
	t.mu.Unlock()

	// Load stored tokens to get client ID and refresh token.
	// Note: we pass through to the Refresher which has the client ID context.
	// For now, we'll need the stored tokens.
	return nil, nil // placeholder — completed in step 4
}
```

Wait — the `tryRefresh` function needs the stored tokens (client ID + refresh token). The transport needs access to the `TokenStore` for loading, not just saving. Let me revise.

Actually, let me restructure. The transport needs:

- The current `ResolvedAuth` (for the header)
- On 401: the stored `clientID` + `refreshToken` to call the refresh endpoint

The cleanest approach: the transport holds a reference to the `TokenStore` (which can both Load and Save) and the `Refresher`. On 401, it loads stored tokens, calls refresh, saves new tokens, updates the resolved auth.

Replace the `transport.go` content with:

```go
package railway

import (
	"net/http"
	"sync"

	"github.com/hamishmorgan/fat-controller/internal/auth"
)

// Refresher abstracts token refresh so transport doesn't depend on OAuthClient directly.
type Refresher interface {
	Refresh(clientID, refreshToken string) (*auth.TokenResponse, error)
}

// AuthTransport is an http.RoundTripper that injects auth headers and
// transparently refreshes expired OAuth tokens.
type AuthTransport struct {
	mu       sync.Mutex
	resolved *auth.ResolvedAuth
	store    *auth.TokenStore
	refresh  Refresher
	base     http.RoundTripper
}

// NewAuthTransport creates a transport that injects auth headers from resolved.
// If store and refresh are non-nil AND the token source is "stored", the
// transport will attempt a token refresh on 401 responses.
func NewAuthTransport(resolved *auth.ResolvedAuth, store *auth.TokenStore, refresh Refresher) *AuthTransport {
	return &AuthTransport{
		resolved: resolved,
		store:    store,
		refresh:  refresh,
		base:     http.DefaultTransport,
	}
}

// RoundTrip implements http.RoundTripper.
func (t *AuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.mu.Lock()
	headerName := t.resolved.HeaderName
	headerValue := t.resolved.HeaderValue
	t.mu.Unlock()

	clone := req.Clone(req.Context())
	clone.Header.Set(headerName, headerValue)

	resp, err := t.base.RoundTrip(clone)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusUnauthorized && t.canRefresh() {
		newTokens, refreshErr := t.tryRefresh()
		if refreshErr != nil {
			return resp, nil
		}

		resp.Body.Close() //nolint:errcheck

		t.mu.Lock()
		t.resolved.Token = newTokens.AccessToken
		t.resolved.HeaderValue = "Bearer " + newTokens.AccessToken
		headerValue = t.resolved.HeaderValue
		t.mu.Unlock()

		retry := req.Clone(req.Context())
		retry.Header.Set(headerName, headerValue)
		return t.base.RoundTrip(retry)
	}

	return resp, nil
}

func (t *AuthTransport) canRefresh() bool {
	return t.resolved.Source == "stored" && t.refresh != nil && t.store != nil
}

func (t *AuthTransport) tryRefresh() (*auth.TokenResponse, error) {
	stored, err := t.store.Load()
	if err != nil {
		return nil, err
	}

	newTokens, err := t.refresh.Refresh(stored.ClientID, stored.RefreshToken)
	if err != nil {
		return nil, err
	}

	// Railway rotates refresh tokens — save the new pair.
	if err := t.store.Save(&auth.StoredTokens{
		AccessToken:  newTokens.AccessToken,
		RefreshToken: newTokens.RefreshToken,
		ClientID:     stored.ClientID,
	}); err != nil {
		return nil, err
	}

	return newTokens, nil
}
```

**Step 4: Run the tests**

Run:

```bash
go test ./internal/railway/ -run TestAuthTransport -v
```

Expected: All 3 tests pass.

**Step 5: Add the refresh test**

Add to `internal/railway/transport_test.go`:

```go
// fakeRefresher implements railway.Refresher for testing.
type fakeRefresher struct {
	called    atomic.Bool
	returnTok *auth.TokenResponse
	returnErr error
}

func (f *fakeRefresher) Refresh(clientID, refreshToken string) (*auth.TokenResponse, error) {
	f.called.Store(true)
	return f.returnTok, f.returnErr
}

func TestAuthTransport_RefreshesOnUnauthorized(t *testing.T) {
	keyring.MockInit()

	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := requestCount.Add(1)
		if n == 1 {
			// First request: return 401 (token expired).
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		// Second request (after refresh): check new token and return 200.
		if got := r.Header.Get("Authorization"); got != "Bearer refreshed-token" {
			t.Errorf("retry Authorization = %q, want %q", got, "Bearer refreshed-token")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	store := auth.NewTokenStore(
		auth.WithKeyringService("test-refresh"),
		auth.WithFallbackPath(filepath.Join(t.TempDir(), "auth.json")),
	)
	// Pre-populate stored tokens so the transport can load them for refresh.
	if err := store.Save(&auth.StoredTokens{
		AccessToken:  "expired-token",
		RefreshToken: "valid-refresh-token",
		ClientID:     "client-123",
	}); err != nil {
		t.Fatal(err)
	}

	resolved := &auth.ResolvedAuth{
		Token:       "expired-token",
		HeaderName:  "Authorization",
		HeaderValue: "Bearer expired-token",
		Source:      "stored",
	}

	refresher := &fakeRefresher{
		returnTok: &auth.TokenResponse{
			AccessToken:  "refreshed-token",
			RefreshToken: "new-refresh-token",
		},
	}

	transport := railway.NewAuthTransport(resolved, store, refresher)
	client := &http.Client{Transport: transport}

	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want 200", resp.StatusCode)
	}
	if !refresher.called.Load() {
		t.Error("refresher should have been called")
	}
	if requestCount.Load() != 2 {
		t.Errorf("request count = %d, want 2", requestCount.Load())
	}

	// Verify new tokens were saved.
	saved, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if saved.AccessToken != "refreshed-token" {
		t.Errorf("saved AccessToken = %q, want %q", saved.AccessToken, "refreshed-token")
	}
	if saved.RefreshToken != "new-refresh-token" {
		t.Errorf("saved RefreshToken = %q, want %q", saved.RefreshToken, "new-refresh-token")
	}
}

func TestAuthTransport_RefreshFailsReturnsOriginal401(t *testing.T) {
	keyring.MockInit()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	store := auth.NewTokenStore(
		auth.WithKeyringService("test-refresh-fail"),
		auth.WithFallbackPath(filepath.Join(t.TempDir(), "auth.json")),
	)
	if err := store.Save(&auth.StoredTokens{
		AccessToken:  "expired-token",
		RefreshToken: "bad-refresh-token",
		ClientID:     "client-123",
	}); err != nil {
		t.Fatal(err)
	}

	resolved := &auth.ResolvedAuth{
		Token:       "expired-token",
		HeaderName:  "Authorization",
		HeaderValue: "Bearer expired-token",
		Source:      "stored",
	}

	refresher := &fakeRefresher{
		returnErr: fmt.Errorf("refresh token revoked"),
	}

	transport := railway.NewAuthTransport(resolved, store, refresher)
	client := &http.Client{Transport: transport}

	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	// Should return original 401 since refresh failed.
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("StatusCode = %d, want 401", resp.StatusCode)
	}
}
```

Also add the necessary imports at the top of `transport_test.go`:

```go
import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/auth"
	"github.com/hamishmorgan/fat-controller/internal/railway"
	"github.com/zalando/go-keyring"
)
```

**Step 6: Run all transport tests**

Run:

```bash
go test ./internal/railway/ -run TestAuthTransport -v
```

Expected: All 5 tests pass.

**Step 7: Commit**

```bash
git add internal/railway/transport.go internal/railway/transport_test.go
git commit -m "Add authenticated HTTP transport with token refresh on 401"
```

---

### Task 6: Create the OAuthRefresher adapter

The transport uses the `Refresher` interface. We need a production implementation that wraps `auth.OAuthClient.RefreshToken`.

**Files:**

- Create: `internal/railway/refresher.go`
- Create: `internal/railway/refresher_test.go`

**Step 1: Write the failing test**

Create `internal/railway/refresher_test.go`:

```go
package railway_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/auth"
	"github.com/hamishmorgan/fat-controller/internal/railway"
)

func TestOAuthRefresher_Refresh(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.Form.Get("grant_type") != "refresh_token" {
			t.Errorf("grant_type = %q", r.Form.Get("grant_type"))
		}
		if r.Form.Get("client_id") != "client-abc" {
			t.Errorf("client_id = %q", r.Form.Get("client_id"))
		}
		if r.Form.Get("refresh_token") != "refresh-xyz" {
			t.Errorf("refresh_token = %q", r.Form.Get("refresh_token"))
		}
		if err := json.NewEncoder(w).Encode(auth.TokenResponse{
			AccessToken:  "new-access",
			RefreshToken: "new-refresh",
			ExpiresIn:    3600,
		}); err != nil {
			t.Fatal(err)
		}
	}))
	defer server.Close()

	oauth := &auth.OAuthClient{
		TokenEndpoint: server.URL,
	}
	refresher := railway.NewOAuthRefresher(oauth)

	tok, err := refresher.Refresh("client-abc", "refresh-xyz")
	if err != nil {
		t.Fatal(err)
	}
	if tok.AccessToken != "new-access" {
		t.Errorf("AccessToken = %q", tok.AccessToken)
	}
	if tok.RefreshToken != "new-refresh" {
		t.Errorf("RefreshToken = %q", tok.RefreshToken)
	}
}
```

**Step 2: Run the test to verify it fails**

Run:

```bash
go test ./internal/railway/ -run TestOAuthRefresher -v
```

Expected: FAIL — `railway.NewOAuthRefresher` does not exist.

**Step 3: Write the implementation**

Create `internal/railway/refresher.go`:

```go
package railway

import "github.com/hamishmorgan/fat-controller/internal/auth"

// OAuthRefresher implements Refresher by delegating to auth.OAuthClient.
type OAuthRefresher struct {
	oauth *auth.OAuthClient
}

// NewOAuthRefresher creates a Refresher that uses the given OAuthClient.
func NewOAuthRefresher(oauth *auth.OAuthClient) *OAuthRefresher {
	return &OAuthRefresher{oauth: oauth}
}

// Refresh exchanges a refresh token for new tokens via the OAuth token endpoint.
func (r *OAuthRefresher) Refresh(clientID, refreshToken string) (*auth.TokenResponse, error) {
	return r.oauth.RefreshToken(clientID, refreshToken)
}
```

**Step 4: Run the test**

Run:

```bash
go test ./internal/railway/ -run TestOAuthRefresher -v
```

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/railway/refresher.go internal/railway/refresher_test.go
git commit -m "Add OAuthRefresher adapter for token refresh in transport"
```

---

### Task 7: Build the Client struct and NewClient constructor

**Files:**

- Create: `internal/railway/client.go`
- Create: `internal/railway/client_test.go`

**Step 1: Write the failing test**

Create `internal/railway/client_test.go`:

```go
package railway_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/auth"
	"github.com/hamishmorgan/fat-controller/internal/railway"
	"github.com/zalando/go-keyring"
)

func TestNewClient_WithFlagToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer my-flag-token" {
			t.Errorf("Authorization = %q, want %q", got, "Bearer my-flag-token")
		}
		w.Header().Set("Content-Type", "application/json")
		// Return a valid GraphQL response.
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"projectToken": map[string]interface{}{
					"projectId":     "proj-123",
					"environmentId": "env-456",
				},
			},
		}); err != nil {
			t.Fatal(err)
		}
	}))
	defer server.Close()

	keyring.MockInit()

	resolved := &auth.ResolvedAuth{
		Token:       "my-flag-token",
		HeaderName:  "Authorization",
		HeaderValue: "Bearer my-flag-token",
		Source:      "flag",
	}
	store := auth.NewTokenStore(
		auth.WithKeyringService("test-client"),
		auth.WithFallbackPath(filepath.Join(t.TempDir(), "auth.json")),
	)
	oauth := &auth.OAuthClient{TokenEndpoint: "http://unused"}

	client := railway.NewClient(server.URL, resolved, store, oauth)

	if client == nil {
		t.Fatal("NewClient returned nil")
	}
}
```

**Step 2: Run test to verify it fails**

Run:

```bash
go test ./internal/railway/ -run TestNewClient -v
```

Expected: FAIL — `railway.NewClient` does not exist.

**Step 3: Write the implementation**

Create `internal/railway/client.go`:

```go
package railway

import (
	"net/http"

	"github.com/Khan/genqlient/graphql"
	"github.com/hamishmorgan/fat-controller/internal/auth"
)

// Client wraps the genqlient GraphQL client with Railway-specific auth.
type Client struct {
	gql graphql.Client
}

// NewClient creates a Railway GraphQL client with authenticated transport.
// The transport injects the correct auth header and handles token refresh
// for stored OAuth tokens.
func NewClient(endpoint string, resolved *auth.ResolvedAuth, store *auth.TokenStore, oauth *auth.OAuthClient) *Client {
	refresher := NewOAuthRefresher(oauth)
	transport := NewAuthTransport(resolved, store, refresher)

	httpClient := &http.Client{Transport: transport}
	gql := graphql.NewClient(endpoint, httpClient)

	return &Client{gql: gql}
}

// GQL returns the underlying genqlient client for making queries.
// Callers use the generated functions directly:
//
//	resp, err := projectToken(ctx, client.GQL())
func (c *Client) GQL() graphql.Client {
	return c.gql
}
```

**Step 4: Run the test**

Run:

```bash
go test ./internal/railway/ -run TestNewClient -v
```

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/railway/client.go internal/railway/client_test.go
git commit -m "Add Railway GraphQL client with authenticated transport"
```

---

### Task 8: Add `go generate` to mise tasks and verify the full pipeline

**Files:**

- Modify: `.config/mise/config.toml`

**Step 1: Add a generate task to mise**

Add to `.config/mise/config.toml`, in the Go section:

```toml
[tasks.generate]
description = "Run Go code generation (genqlient)"
run = """
[ -f go.mod ] || exit 0
go generate ./...
"""
```

**Step 2: Verify the full pipeline works**

Run:

```bash
mise run generate
```

Expected: No errors. `internal/railway/generated.go` is unchanged (already committed).

**Step 3: Run the full check suite**

Run:

```bash
mise run check
```

Expected: All linters pass, all tests pass, build succeeds.

**Step 4: Commit**

```bash
git add .config/mise/config.toml
git commit -m "Add go generate task to mise"
```

---

### Task 9: Integration test — projectToken query against mock server

This test proves the entire pipeline works end-to-end: genqlient-generated code → Client → AuthTransport → GraphQL server → typed response.

**Files:**

- Modify: `internal/railway/client_test.go`

**Step 1: Write the integration test**

Add to `internal/railway/client_test.go`:

```go
import "context"

func TestClient_ProjectToken_EndToEnd(t *testing.T) {
	keyring.MockInit()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify auth header is present.
		if r.Header.Get("Authorization") == "" {
			t.Error("missing Authorization header")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		// Verify it's a POST to the GraphQL endpoint.
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}

		// Return a valid projectToken response.
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"projectToken": map[string]interface{}{
					"projectId":     "proj-abc-123",
					"environmentId": "env-def-456",
				},
			},
		}); err != nil {
			t.Fatal(err)
		}
	}))
	defer server.Close()

	resolved := &auth.ResolvedAuth{
		Token:       "test-token",
		HeaderName:  "Authorization",
		HeaderValue: "Bearer test-token",
		Source:      "flag",
	}
	store := auth.NewTokenStore(
		auth.WithKeyringService("test-e2e"),
		auth.WithFallbackPath(filepath.Join(t.TempDir(), "auth.json")),
	)
	oauth := &auth.OAuthClient{TokenEndpoint: "http://unused"}

	client := railway.NewClient(server.URL, resolved, store, oauth)

	resp, err := railway.ProjectToken(context.Background(), client.GQL())
	if err != nil {
		t.Fatalf("ProjectToken() error: %v", err)
	}

	if resp.ProjectToken.ProjectId != "proj-abc-123" {
		t.Errorf("ProjectId = %q, want %q", resp.ProjectToken.ProjectId, "proj-abc-123")
	}
	if resp.ProjectToken.EnvironmentId != "env-def-456" {
		t.Errorf("EnvironmentId = %q, want %q", resp.ProjectToken.EnvironmentId, "env-def-456")
	}
}
```

**Important note about the generated function:** genqlient generates `projectToken` (lowercase) as the function name, which is unexported. To call it from `_test.go` in `railway_test` package, you need to either:

(a) Export it by capitalizing the query name in the `.graphql` file: `query ProjectToken { ... }` → generates `func ProjectToken(...)`, OR
(b) Write the test in `package railway` (not `railway_test`)

**Choose option (a):** Update `internal/railway/operations.graphql` to use `ProjectToken` (capitalized):

```graphql
query ProjectToken {
  projectToken {
    projectId
    environmentId
  }
}
```

Then regenerate:

```bash
go generate ./internal/railway/
```

This generates an exported `func ProjectToken(ctx, client) (*ProjectTokenResponse, error)`.

**Step 2: Run the test**

Run:

```bash
go test ./internal/railway/ -run TestClient_ProjectToken -v
```

Expected: PASS. The test exercises the full chain: generated query function → Client.GQL() → AuthTransport → mock HTTP server → typed response.

**Step 3: Run the full suite**

Run:

```bash
mise run check
```

Expected: All checks pass.

**Step 4: Commit**

```bash
git add internal/railway/operations.graphql internal/railway/generated.go internal/railway/client_test.go
git commit -m "Add end-to-end test for projectToken query via mock GQL server"
```

---

### Task 10: Update auth status to use refresh-aware client

Now that M2's refresh-aware transport exists, update `auth status` to use it instead of calling `FetchUserInfo` directly. This means expired tokens get refreshed transparently.

**Files:**

- Modify: `cmd/cli.go`

**Step 1: Update AuthStatusCmd.Run**

Replace the stored-token section in `cmd/cli.go`'s `AuthStatusCmd.Run` method. The current code (lines 132-147) does:

```go
// For stored OAuth tokens, fetch user info.
// Note: if the access token is expired (>1hr), this will fail with a 401.
// M2 will add a refresh-aware HTTP client that handles this transparently.
// For now, we show a helpful message.
oauth := auth.NewOAuthClient()
info, err := oauth.FetchUserInfo(resolved.Token)
if err != nil {
    fmt.Println("Authenticated (stored OAuth token).")
    fmt.Printf("Could not fetch user info: %v\n", err)
    fmt.Println("If your session expired, run 'fat-controller auth login' to re-authenticate.")
    return nil
}
```

Replace it with code that creates a refresh-aware HTTP client:

```go
// For stored OAuth tokens, use the refresh-aware transport so
// expired tokens get refreshed transparently.
oauth := auth.NewOAuthClient()
store := auth.NewTokenStore(
    auth.WithFallbackPath(platform.AuthFilePath()),
)
refresher := railway.NewOAuthRefresher(oauth)
transport := railway.NewAuthTransport(resolved, store, refresher)
httpClient := &http.Client{Transport: transport}

// Use the transport-wrapped HTTP client for the userinfo request.
oauth.HTTPClient = httpClient
info, err := oauth.FetchUserInfo(resolved.Token)
if err != nil {
    fmt.Println("Authenticated (stored OAuth token).")
    fmt.Printf("Could not fetch user info: %v\n", err)
    fmt.Println("If your session expired, run 'fat-controller auth login' to re-authenticate.")
    return nil
}
```

Add the `railway` import to `cmd/cli.go`:

```go
import (
    "fmt"
    "net/http"
    "time"

    "github.com/hamishmorgan/fat-controller/internal/auth"
    "github.com/hamishmorgan/fat-controller/internal/platform"
    "github.com/hamishmorgan/fat-controller/internal/railway"
)
```

**Step 2: Verify it compiles**

Run:

```bash
go build ./...
```

Expected: Build succeeds.

**Step 3: Run all tests**

Run:

```bash
mise run check
```

Expected: All checks pass.

**Step 4: Commit**

```bash
git add cmd/cli.go
git commit -m "Use refresh-aware transport in auth status for transparent token refresh"
```

---

### Task 11: Update docs/DECISIONS.md

**Files:**

- Modify: `docs/DECISIONS.md`

**Step 1: Update the token refresh decision**

Replace the "Token refresh: deferred to M2" section with:

```markdown
## Token refresh: transparent via HTTP transport

The authenticated HTTP transport (`internal/railway/transport.go`)
transparently refreshes expired OAuth tokens. On a 401 response, it:

1. Loads stored tokens (client ID + refresh token) from the token store
2. Calls the OAuth token endpoint to refresh
3. Saves the new token pair (Railway rotates refresh tokens)
4. Retries the original request with the new access token

This only applies to stored OAuth tokens (source = "stored"). Tokens
from `--token` flag or environment variables are never refreshed — a 401
is returned directly.

The transport is used by both the Railway GraphQL client and the
`auth status` userinfo call, so all commands benefit from transparent
refresh.
```

**Step 2: Run lint**

Run:

```bash
mise run lint:md
```

Expected: No errors.

**Step 3: Commit**

```bash
git add docs/DECISIONS.md
git commit -m "Update token refresh decision: implemented in M2 via HTTP transport"
```

---

### Task 12: Final verification

**Step 1: Run the full check suite**

Run:

```bash
mise run check
```

Expected: All linters pass, all tests pass (including race detector), build succeeds.

**Step 2: Verify test coverage**

Run:

```bash
go test -coverprofile=/tmp/cover.out ./internal/... && go tool cover -func=/tmp/cover.out
```

Expected: `internal/railway` coverage should be reasonable (>70%). Transport, refresher, and client should all show covered functions.

**Step 3: Verify the generated code is committed**

Run:

```bash
git status
```

Expected: Clean working tree. `internal/railway/generated.go` should be tracked.

**Step 4: Run go vet**

Run:

```bash
go vet ./...
```

Expected: No issues.

**Step 5: Smoke test the CLI**

Run:

```bash
go run . --help
go run . auth status
```

Expected: Help text shows all commands. Auth status works (shows "not authenticated" if no token stored, or shows user info if logged in).

**Step 6: Commit if there are any remaining changes**

```bash
git add -A
git commit -m "M2 complete: schema + GQL client with refresh-aware transport"
```
