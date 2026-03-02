# M2: Schema + GQL Client — Implementation Plan

**Goal:** Set up genqlient code generation against the Railway GraphQL schema, build a refresh-aware authenticated HTTP transport, and verify the pipeline works end-to-end with a `projectToken` query.

**Architecture:** Apollo Rover introspects the Railway schema into
`internal/railway/schema.graphql` (checked in). genqlient generates typed
Go functions from `.graphql` operation files. An authenticated
`http.RoundTripper` wraps requests with the correct auth header (Bearer
vs Project-Access-Token) based on `ResolvedAuth`, and transparently
refreshes expired OAuth tokens. A `Client` struct in
`internal/railway/client.go` ties it all together.

**Tech Stack:** Go, Khan/genqlient, apollo-rover (mise tool, introspection
only), existing internal/auth package

---

## Background: genqlient

[genqlient](https://github.com/Khan/genqlient) generates typed Go functions from `.graphql` operation files against a GraphQL schema. Key concepts:

- **`.config/genqlient.yaml`** — config file. Points to schema, operation globs, output file.
- **`schema.graphql`** — the Railway API schema in SDL format. Fetched via introspection, checked into git.
- **`operations.graphql`** — queries and mutations we use. Each becomes a typed Go function.
- **`generated.go`** — genqlient output. Checked into git (CI doesn't need a Railway token).
- **`//go:generate go run github.com/Khan/genqlient -config ../../.config/genqlient.yaml`** — runs code generation.
- Generated function names preserve the query name casing from the
  `.graphql` file. A query named `ProjectToken` generates an exported
  `func ProjectToken(...)`. A query named `projectToken` generates an
  unexported `func projectToken(...)`.
- Signature: `func QueryName(ctx context.Context, client graphql.Client, args...) (*QueryNameResponse, error)`
- `graphql.Client` is created via `graphql.NewClient(endpoint, httpClient)`
  where `httpClient` satisfies the `Doer` interface (`*http.Client` works).
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
├── .config/
│   └── genqlient.yaml                # genqlient config (points to internal/railway/)
├── tools.go                          # //go:build tools — blank import keeps genqlient in go.mod
├── cmd/fat-controller/main.go        # Entry point
├── internal/
│   ├── cli/
│   │   ├── cli.go                    # CLI struct, kong wiring, config command stubs
│   │   ├── auth.go                   # Auth command Run() methods (login, logout, status)
│   │   └── help.go                   # Colored help printer
│   ├── auth/                         # Existing — OAuth, keyring, token resolution
│   │   ├── resolver.go               # ResolvedAuth (used by transport)
│   │   ├── oauth.go                  # OAuthClient.RefreshToken (used by transport)
│   │   ├── store.go                  # TokenStore (used by transport for saving refreshed tokens)
│   │   └── ...
│   ├── platform/                     # Existing — XDG paths
│   └── railway/                      # NEW — GQL client
│       ├── schema.graphql            # Introspected Railway schema (checked in)
│       ├── operations.graphql        # Queries + mutations (starts with ProjectToken)
│       ├── generated.go              # genqlient output (checked in)
│       ├── generate.go               # //go:generate directive
│       ├── client.go                 # Client struct, NewClient()
│       ├── client_test.go            # Tests for client + end-to-end
│       ├── transport.go              # AuthTransport — injects auth header, refreshes on 401
│       ├── transport_test.go         # Tests for transport (header injection, refresh, failure)
│       ├── refresher.go              # OAuthRefresher — adapts auth.OAuthClient to Refresher interface
│       └── refresher_test.go         # Tests for refresher
```

---

### Task 1: Add apollo-rover to mise and create introspection task

**Files:**

- Modify: `.config/mise/config.toml`

**Step 1: Add apollo-rover to mise tools**

Open `.config/mise/config.toml` and add `apollo-rover` to the `[tools]` section:

```toml
apollo-rover = "latest"
```

**Step 2: Add introspection task**

Add a new task section to `.config/mise/config.toml`, after the GitHub Actions section:

```toml
# ---------------------------------------------------------------------------
# GraphQL Schema
# ---------------------------------------------------------------------------

[tasks."schema:introspect"]
alias = ["introspect"]
description = "Fetch Railway GraphQL schema via introspection (requires RAILWAY_API_TOKEN)"
run = """
rover graph introspect https://backboard.railway.com/graphql/v2 \
  --header "Authorization:Bearer ${RAILWAY_API_TOKEN:?Set RAILWAY_API_TOKEN to introspect the schema}" \
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
export RAILWAY_API_TOKEN=<your-token>
mise run schema:introspect
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
- Create: `.config/genqlient.yaml`
- Create: `internal/railway/generate.go`
- Create: `tools.go`

**Step 1: Add genqlient dependency**

Run:

```bash
go get github.com/Khan/genqlient@latest
```

**Step 2: Create the generate directive file**

Create `internal/railway/generate.go`:

```go
package railway

//go:generate go run github.com/Khan/genqlient
```

This wires up `go generate` so that running `go generate ./internal/railway/` invokes genqlient.

**Note:** `//go:generate go run` does NOT count as a Go import. Without a real import somewhere, `go mod tidy` will prune genqlient from `go.mod`. That's what `tools.go` (next step) is for.

**Step 2b: Create the tools file to prevent `go mod tidy` from pruning genqlient**

Create `tools.go` at the repo root:

```go
//go:build tools

package tools

import _ "github.com/Khan/genqlient"
```

The `//go:build tools` tag ensures this file is never compiled into the binary, but the blank import keeps genqlient in the module graph so `go mod tidy` won't remove it.

**Step 3: Create genqlient.yaml in `.config/`**

Create `.config/genqlient.yaml`:

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

- `optional: pointer` — nullable GraphQL fields become Go pointers. This
  is safer than `value` (which silently zero-fills nulls) and matches
  Railway's schema where many fields are optional.
- `bindings` — maps Railway's custom scalars and enums to Go types. These
  5 bindings match what the Terraform provider uses. `DateTime` → `time.Time`
  is standard. `JSON`, `EnvironmentVariables`, and `DeploymentMeta` are
  opaque JSON blobs. `PluginType` is a GraphQL enum mapped to `string`.

**Step 4: Run `go mod tidy`**

Run:

```bash
go mod tidy
```

Expected: `go.mod` and `go.sum` updated with genqlient and its dependencies.

**Step 5: Commit**

```bash
git add .config/genqlient.yaml internal/railway/generate.go tools.go go.mod go.sum
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
# ProjectToken resolves a project-scoped token to its project and environment IDs.
# Used by auth status and as a basic connectivity check.
query ProjectToken {
  projectToken {
    projectId
    environmentId
  }
}
```

This is the simplest useful query — it takes no arguments (the token is in the header) and returns the project + environment IDs that the token is scoped to. Only works with `RAILWAY_TOKEN` (project-scoped tokens), not account-level tokens.

**Important:** The query name is capitalized (`ProjectToken`) so genqlient generates an exported Go function `func ProjectToken(...)`. This lets tests in the `railway_test` package call it directly.

**Step 2: Run genqlient code generation**

Run:

```bash
go generate ./internal/railway/
```

Expected: `internal/railway/generated.go` created. If you see errors about missing types or scalars, check that the `bindings` in `.config/genqlient.yaml` cover all custom scalars used in the schema.

**Step 3: Verify the generated code compiles**

Run:

```bash
go build ./internal/railway/
```

Expected: Build succeeds with no errors.

**Step 4: Inspect the generated function**

Open `internal/railway/generated.go` and look for the `ProjectToken` function. It should look approximately like:

```go
func ProjectToken(
    ctx_ context.Context,
    client_ graphql.Client,
) (*ProjectTokenResponse, error)
```

And the response type:

```go
type ProjectTokenResponse struct {
    ProjectToken ProjectTokenProjectToken `json:"projectToken"`
}

type ProjectTokenProjectToken struct {
    ProjectId     string `json:"projectId"`
    EnvironmentId string `json:"environmentId"`
}
```

Because the query name is capitalized, all generated types and functions are exported.

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

**Step 1: Write all the failing tests**

Create `internal/railway/transport_test.go`:

```go
package railway_test

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

Expected: All 5 tests pass.

**Step 5: Commit**

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
//	resp, err := ProjectToken(ctx, client.GQL())
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
alias = ["gen"]
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

Add `"context"` to the import block of `internal/railway/client_test.go`, then add the test function:

```go
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
git add internal/railway/client_test.go
git commit -m "Add end-to-end test for projectToken query via mock GQL server"
```

---

### Task 10: Update auth status to use refresh-aware client

Now that M2's refresh-aware transport exists, update `auth status` to use
it instead of calling `FetchUserInfo` with a bare HTTP client. This means
expired stored OAuth tokens get refreshed transparently on 401.

**Files:**

- Modify: `internal/cli/auth.go`

**Step 1: Update AuthStatusCmd.Run**

Replace the stored-token section in `internal/cli/auth.go`'s `AuthStatusCmd.Run` method. The current code does:

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

Replace it with code that wraps the OAuth client's HTTP client with the refresh-aware transport. Reuse the `store` variable already created earlier in the function:

```go
// For stored OAuth tokens, use the refresh-aware transport so
// expired tokens get refreshed transparently on 401.
oauth := auth.NewOAuthClient()
refresher := railway.NewOAuthRefresher(oauth)
transport := railway.NewAuthTransport(resolved, store, refresher)
oauth.HTTPClient = &http.Client{Transport: transport}

// Note: FetchUserInfo sets its own Authorization header, but the
// transport overwrites it. On 401, the transport refreshes and retries.
// Task 11 cleans this up by removing the token parameter entirely.
info, err := oauth.FetchUserInfo(resolved.Token)
if err != nil {
    fmt.Println("Authenticated (stored OAuth token).")
    fmt.Printf("Could not fetch user info: %v\n", err)
    fmt.Println("If your session expired, run 'fat-controller auth login' to re-authenticate.")
    return nil
}
```

Add the `railway` and `net/http` imports to `internal/cli/auth.go`:

```go
import (
    "fmt"
    "net/http"

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
git add internal/cli/auth.go
git commit -m "Use refresh-aware transport in auth status for transparent token refresh"
```

---

### Task 11: Refactor FetchUserInfo to rely on HTTPClient transport for auth

Currently `FetchUserInfo(accessToken string)` sets its own `Authorization` header, but Task 10 wraps the `HTTPClient` with a transport that also sets it. This double-auth is confusing — the transport wins (it overwrites the header), making the parameter misleading. Refactor to remove the parameter: `FetchUserInfo()` now relies entirely on the `HTTPClient`'s transport for auth.

**Files:**

- Modify: `internal/auth/userinfo.go`
- Modify: `internal/auth/userinfo_test.go`
- Modify: `internal/cli/auth.go`

**Step 1: Update the tests first**

Update `internal/auth/userinfo_test.go` to inject auth via the `HTTPClient`
instead of a parameter. Existing error-path tests (network error, JSON
decode error) should also be updated to remove the token parameter. The
key changes to the existing tests:

```go
package auth_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/auth"
)

// roundTripFunc lets us build a one-off RoundTripper from a function.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestFetchUserInfo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}

		if err := json.NewEncoder(w).Encode(auth.UserInfo{
			Sub:   "user_abc123",
			Email: "test@example.com",
			Name:  "Test User",
		}); err != nil {
			t.Fatal(err)
		}
	}))
	defer server.Close()

	// Inject auth via a simple transport that sets the Authorization header.
	client := &auth.OAuthClient{
		UserinfoURL: server.URL,
		HTTPClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				req.Header.Set("Authorization", "Bearer test-token")
				return http.DefaultTransport.RoundTrip(req)
			}),
		},
	}

	info, err := client.FetchUserInfo()
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

func TestFetchUserInfo_Unauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	client := &auth.OAuthClient{
		UserinfoURL: server.URL,
		HTTPClient:  http.DefaultClient,
	}

	_, err := client.FetchUserInfo()
	if err == nil {
		t.Fatal("expected error for 401 response")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error should mention status 401, got: %s", err)
	}
}
```

**Step 2: Run tests to verify they fail**

Run:

```bash
go test ./internal/auth/ -run TestFetchUserInfo -v
```

Expected: FAIL — `FetchUserInfo` still takes a parameter.

**Step 3: Update the implementation**

Replace `internal/auth/userinfo.go`'s `FetchUserInfo` method:

```go
// FetchUserInfo calls the OIDC userinfo endpoint.
// Auth is handled by the OAuthClient's HTTPClient transport —
// callers must set HTTPClient to a client with an auth-injecting transport.
func (c *OAuthClient) FetchUserInfo() (*UserInfo, error) {
	req, err := http.NewRequest(http.MethodGet, c.UserinfoURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("userinfo request failed: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

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

**Step 4: Update the caller in auth.go**

In `internal/cli/auth.go`, change:

```go
info, err := oauth.FetchUserInfo(resolved.Token)
```

to:

```go
info, err := oauth.FetchUserInfo()
```

The transport (set up in Task 10) already handles auth, so no token parameter is needed.

**Step 5: Run tests**

Run:

```bash
go test ./internal/auth/ -run TestFetchUserInfo -v
go build ./...
mise run check
```

Expected: All tests pass, build succeeds.

**Step 6: Commit**

```bash
git add internal/auth/userinfo.go internal/auth/userinfo_test.go internal/cli/auth.go
git commit -m "Refactor FetchUserInfo to rely on HTTPClient transport for auth"
```

---

### Task 12: Update docs/DECISIONS.md

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

### Task 13: Final verification

**Step 1: Run the full check suite**

Run:

```bash
mise run check
```

Expected: All linters pass, all tests pass (including race detector), build succeeds.

**Step 2: Verify test coverage**

Run:

```bash
mise run test:coverage
```

Expected: `internal/railway` coverage should be >85%. Transport, refresher, and client should all show covered functions. Overall project coverage should remain >85%.

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
go run ./cmd/fat-controller --help
go run ./cmd/fat-controller auth status
```

Expected: Help text shows all commands. Auth status works (shows "not authenticated" if no token stored, or shows user info if logged in).

**Step 6: Commit if there are any remaining changes**

```bash
git add -A
git commit -m "M2 complete: schema + GQL client with refresh-aware transport"
```
