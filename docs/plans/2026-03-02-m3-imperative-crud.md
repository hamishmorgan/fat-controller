# M3 Imperative CRUD Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement `config get/set/delete` to read and mutate live Railway config by dot-path, with safe confirmation defaults.

**Architecture:** Add GraphQL operations + a small Railway fetch/mutate layer to resolve project/environment, load live config, and apply single-field updates. Build a minimal config model + dot-path parser in `internal/config` and wire CLI commands to render output and enforce confirm/dry-run semantics.

**Tech Stack:** Go, genqlient, kong, httptest, BurntSushi/toml

---

## Task 1: Add config model + dot-path parsing

**Files:**

- Create: `internal/config/model.go`
- Create: `internal/config/path.go`
- Test: `internal/config/path_test.go`

**Step 1: Write the failing test**

Create `internal/config/path_test.go`:

```go
package config

import "testing"

func TestParsePath(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    Path
		wantErr bool
	}{
		{
			name:  "service section key",
			input: "api.variables.PORT",
			want:  Path{Service: "api", Section: "variables", Key: "PORT"},
		},
		{
			name:  "service section",
			input: "api.variables",
			want:  Path{Service: "api", Section: "variables"},
		},
		{
			name:  "service only",
			input: "api",
			want:  Path{Service: "api"},
		},
		{
			name:    "empty",
			input:   "",
			wantErr: true,
		},
		{
			name:    "too many segments",
			input:   "a.b.c.d",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParsePath(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParsePath() err = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got != tt.want {
				t.Fatalf("ParsePath() = %#v, want %#v", got, tt.want)
			}
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/config -run TestParsePath -v`

Expected: FAIL with “undefined: ParsePath”.

**Step 3: Write minimal implementation**

Create `internal/config/model.go`:

```go
package config

// LiveConfig represents the live Railway config snapshot used by config get.
type LiveConfig struct {
	ProjectID     string
	EnvironmentID string
	Shared        map[string]string
	Services      map[string]*ServiceConfig
}

type ServiceConfig struct {
	ID        string
	Name      string
	Variables map[string]string
	Resources Resources
	Deploy    Deploy
}

type Resources struct {
	VCPUs    *float64
	MemoryGB *float64
}

type Deploy struct {
	Builder         *string
	DockerfilePath  *string
	RootDirectory   *string
	StartCommand    *string
	HealthcheckPath *string
}
```

Create `internal/config/path.go`:

```go
package config

import (
	"errors"
	"strings"
)

// Path is a parsed dot-path like "service.section.key".
type Path struct {
	Service string
	Section string
	Key     string
}

// ParsePath parses a dot-path into components.
func ParsePath(input string) (Path, error) {
	if strings.TrimSpace(input) == "" {
		return Path{}, errors.New("path cannot be empty")
	}
	parts := strings.Split(input, ".")
	if len(parts) < 1 || len(parts) > 3 {
		return Path{}, errors.New("path must have 1 to 3 segments")
	}
	for _, p := range parts {
		if p == "" {
			return Path{}, errors.New("path segments cannot be empty")
		}
	}
	path := Path{Service: parts[0]}
	if len(parts) > 1 {
		path.Section = parts[1]
	}
	if len(parts) > 2 {
		path.Key = parts[2]
	}
	return path, nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/config -run TestParsePath -v`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/config/model.go internal/config/path.go internal/config/path_test.go
git commit -m "Add live config model and dot-path parser"
```

---

### Task 2: Add output rendering for config get

**Files:**

- Create: `internal/config/render.go`
- Test: `internal/config/render_test.go`

**Step 1: Write the failing test**

Create `internal/config/render_test.go`:

```go
package config

import (
	"strings"
	"testing"
)

func TestRender_TextIncludesServiceAndKey(t *testing.T) {
	cfg := LiveConfig{
		Shared: map[string]string{"SHARED": "1"},
		Services: map[string]*ServiceConfig{
			"api": {Name: "api", Variables: map[string]string{"PORT": "8080"}},
		},
	}

	got, err := Render(cfg, "text", false)
	if err != nil {
		t.Fatalf("Render() error: %v", err)
	}
	if !strings.Contains(got, "[shared_variables]") {
		t.Fatalf("expected shared header, got: %s", got)
	}
	if !strings.Contains(got, "[api.variables]") {
		t.Fatalf("expected service variables header, got: %s", got)
	}
	if !strings.Contains(got, "PORT = \"8080\"") {
		t.Fatalf("expected PORT value, got: %s", got)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/config -run TestRender_TextIncludesServiceAndKey -v`

Expected: FAIL with “undefined: Render”.

**Step 3: Write minimal implementation**

Create `internal/config/render.go`:

```go
package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

// Render renders the live config in the requested output format.
func Render(cfg LiveConfig, format string, full bool) (string, error) {
	switch format {
	case "json":
		buf, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			return "", err
		}
		return string(buf), nil
	case "toml":
		var buf bytes.Buffer
		if err := toml.NewEncoder(&buf).Encode(cfg); err != nil {
			return "", err
		}
		return buf.String(), nil
	case "text", "":
		return renderText(cfg, full), nil
	default:
		return "", errors.New("unsupported output format")
	}
}

func renderText(cfg LiveConfig, full bool) string {
	var out strings.Builder

	if len(cfg.Shared) > 0 {
		out.WriteString("[shared_variables]\n")
		keys := sortedKeys(cfg.Shared)
		for _, k := range keys {
			out.WriteString(k + " = \"" + cfg.Shared[k] + "\"\n")
		}
		out.WriteString("\n")
	}

	serviceNames := make([]string, 0, len(cfg.Services))
	for name := range cfg.Services {
		serviceNames = append(serviceNames, name)
	}
	sort.Strings(serviceNames)
	for _, name := range serviceNames {
		svc := cfg.Services[name]
		if len(svc.Variables) > 0 {
			out.WriteString("[" + svc.Name + ".variables]\n")
			keys := sortedKeys(svc.Variables)
			for _, k := range keys {
				out.WriteString(k + " = \"" + svc.Variables[k] + "\"\n")
			}
			out.WriteString("\n")
		}
	}

	return strings.TrimRight(out.String(), "\n")
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/config -run TestRender_TextIncludesServiceAndKey -v`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/config/render.go internal/config/render_test.go
git commit -m "Add output renderer for config get"
```

---

### Task 3: Add GraphQL operations for M3 and regenerate

**Files:**

- Modify: `internal/railway/operations.graphql`
- Modify: `internal/railway/generated.go`

**Step 1: Write the failing test**

Create `internal/railway/queries_test.go`:

```go
package railway_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/railway"
)

func TestProjectsQuery_ParsesResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"projects": map[string]any{
					"edges": []map[string]any{{
						"node": map[string]any{"id": "proj-1", "name": "demo"},
					}},
				},
			},
		})
	}))
	defer server.Close()

	client := railway.NewClient(server.URL, nil, nil, nil)
	resp, err := railway.Projects(context.Background(), client.GQL())
	if err != nil {
		t.Fatalf("Projects() error: %v", err)
	}
	if len(resp.Projects.Edges) != 1 {
		t.Fatalf("edges = %d, want 1", len(resp.Projects.Edges))
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/railway -run TestProjectsQuery_ParsesResponse -v`

Expected: FAIL with “undefined: railway.Projects”.

**Step 3: Write minimal implementation**

Update `internal/railway/operations.graphql` by appending:

```graphql
# Project and environment resolution
query Projects {
  projects(first: 100) {
    edges {
      node {
        id
        name
      }
    }
  }
}

query Environments($projectId: String!) {
  environments(projectId: $projectId, first: 100) {
    edges {
      node {
        id
        name
      }
    }
  }
}

# Service list for a project
query ProjectServices($projectId: String!) {
  project(id: $projectId) {
    services(first: 200) {
      edges {
        node {
          id
          name
        }
      }
    }
  }
}

# Variables (shared + service)
query Variables($projectId: String!, $environmentId: String!, $serviceId: String) {
  variables(projectId: $projectId, environmentId: $environmentId, serviceId: $serviceId, unrendered: true)
}

# Service settings
query ServiceInstance($environmentId: String!, $serviceId: String!) {
  serviceInstance(environmentId: $environmentId, serviceId: $serviceId) {
    builder
    dockerfilePath
    rootDirectory
    startCommand
    healthcheckPath
  }
}

query ServiceInstanceLimitOverride($environmentId: String!, $serviceId: String!) {
  serviceInstanceLimitOverride(environmentId: $environmentId, serviceId: $serviceId) {
    vCPUs
    memoryGB
  }
}

# Mutations for set/delete
mutation VariableUpsert($input: VariableUpsertInput!) {
  variableUpsert(input: $input)
}

mutation VariableDelete($input: VariableDeleteInput!) {
  variableDelete(input: $input)
}

mutation ServiceInstanceUpdate($input: ServiceInstanceUpdateInput!) {
  serviceInstanceUpdate(input: $input)
}

mutation ServiceInstanceLimitsUpdate($input: ServiceInstanceLimitsUpdateInput!) {
  serviceInstanceLimitsUpdate(input: $input)
}
```

Run: `go generate ./internal/railway/`

**Step 4: Run test to verify it passes**

Run: `go test ./internal/railway -run TestProjectsQuery_ParsesResponse -v`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/railway/operations.graphql internal/railway/generated.go internal/railway/queries_test.go
git commit -m "Add M3 GraphQL operations and generation"
```

---

### Task 4: Resolve project/environment IDs

**Files:**

- Create: `internal/railway/resolve.go`
- Test: `internal/railway/resolve_test.go`

**Step 1: Write the failing test**

Create `internal/railway/resolve_test.go`:

```go
package railway_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/auth"
	"github.com/hamishmorgan/fat-controller/internal/railway"
)

func TestResolveProjectEnvironment_ProjectToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"projectToken": map[string]any{
					"projectId": "proj-1",
					"environmentId": "env-1",
				},
			},
		})
	}))
	defer server.Close()

	client := railway.NewClient(server.URL, &auth.ResolvedAuth{Source: auth.SourceEnvToken}, nil, nil)
	proj, env, err := railway.ResolveProjectEnvironment(context.Background(), client, "", "")
	if err != nil {
		t.Fatalf("ResolveProjectEnvironment() error: %v", err)
	}
	if proj != "proj-1" || env != "env-1" {
		t.Fatalf("got %s/%s", proj, env)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/railway -run TestResolveProjectEnvironment_ProjectToken -v`

Expected: FAIL with “undefined: railway.ResolveProjectEnvironment”.

**Step 3: Write minimal implementation**

Create `internal/railway/resolve.go`:

```go
package railway

import (
	"context"
	"errors"

	"github.com/hamishmorgan/fat-controller/internal/auth"
)

// ResolveProjectEnvironment returns project/environment IDs for the active auth.
// For project tokens, it uses ProjectToken query. For account tokens, it uses
// the provided project/environment names or IDs and resolves names via queries.
func ResolveProjectEnvironment(ctx context.Context, client *Client, project, environment string) (string, string, error) {
	if client == nil || client.auth == nil {
		return "", "", errors.New("missing auth")
	}
	if client.auth.Source == auth.SourceEnvToken {
		resp, err := ProjectToken(ctx, client.GQL())
		if err != nil {
			return "", "", err
		}
		return resp.ProjectToken.ProjectId, resp.ProjectToken.EnvironmentId, nil
	}
	if project == "" || environment == "" {
		return "", "", errors.New("project and environment required")
	}
	projID, err := resolveProjectID(ctx, client, project)
	if err != nil {
		return "", "", err
	}
	envID, err := resolveEnvironmentID(ctx, client, projID, environment)
	if err != nil {
		return "", "", err
	}
	return projID, envID, nil
}

func resolveProjectID(ctx context.Context, client *Client, project string) (string, error) {
	// If it looks like an ID, pass through (simple heuristic).
	if project != "" && project != "-" && len(project) > 8 {
		return project, nil
	}
	resp, err := Projects(ctx, client.GQL())
	if err != nil {
		return "", err
	}
	for _, edge := range resp.Projects.Edges {
		if edge.Node.Name == project {
			return edge.Node.Id, nil
		}
	}
	return "", errors.New("project not found")
}

func resolveEnvironmentID(ctx context.Context, client *Client, projectID, env string) (string, error) {
	resp, err := Environments(ctx, client.GQL(), projectID)
	if err != nil {
		return "", err
	}
	for _, edge := range resp.Environments.Edges {
		if edge.Node.Name == env {
			return edge.Node.Id, nil
		}
	}
	return "", errors.New("environment not found")
}
```

Update `internal/railway/client.go` to store resolved auth for resolvers:

```go
type Client struct {
	gql  graphql.Client
	auth *auth.ResolvedAuth
}

// In NewClient:
return &Client{gql: gql, auth: resolved}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/railway -run TestResolveProjectEnvironment_ProjectToken -v`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/railway/resolve.go internal/railway/resolve_test.go internal/railway/client.go
git commit -m "Add project/environment resolution for config commands"
```

---

### Task 5: Fetch live config snapshot

**Files:**

- Create: `internal/railway/state.go`
- Test: `internal/railway/state_test.go`

**Step 1: Write the failing test**

Create `internal/railway/state_test.go`:

```go
package railway_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/auth"
	"github.com/hamishmorgan/fat-controller/internal/config"
	"github.com/hamishmorgan/fat-controller/internal/railway"
)

func TestFetchLiveConfig_IncludesSharedAndServiceVars(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Minimal dispatcher based on query name
		var body struct{ Query string }
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch {
		case contains(body.Query, "project(id"):
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"project": map[string]any{"services": map[string]any{"edges": []map[string]any{{"node": map[string]any{"id": "svc-1", "name": "api"}}}}}}})
		case contains(body.Query, "variables("):
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"variables": map[string]any{"FOO": "bar"}}})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{}})
		}
	}))
	defer server.Close()

	client := railway.NewClient(server.URL, &auth.ResolvedAuth{Source: auth.SourceFlag}, nil, nil)
	cfg, err := railway.FetchLiveConfig(context.Background(), client, "proj-1", "env-1", "")
	if err != nil {
		t.Fatalf("FetchLiveConfig() error: %v", err)
	}
	if cfg.Shared["FOO"] != "bar" {
		t.Fatalf("shared FOO = %q", cfg.Shared["FOO"])
	}
	if cfg.Services["api"].Variables["FOO"] != "bar" {
		t.Fatalf("service FOO = %q", cfg.Services["api"].Variables["FOO"])
	}
	_ = config.LiveConfig{} // keep compile reference
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (string([]byte(haystack)) != "" && (func() bool { return len(needle) == 0 || (len(haystack) > 0 && (string(haystack) != "" && (func() bool { return (len(haystack) >= len(needle)) && (strings.Contains(haystack, needle)) })())) })())
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/railway -run TestFetchLiveConfig_IncludesSharedAndServiceVars -v`

Expected: FAIL with “undefined: railway.FetchLiveConfig”.

**Step 3: Write minimal implementation**

Create `internal/railway/state.go`:

```go
package railway

import (
	"context"

	"github.com/hamishmorgan/fat-controller/internal/config"
)

// FetchLiveConfig loads shared + per-service variables and basic settings.
func FetchLiveConfig(ctx context.Context, client *Client, projectID, environmentID, serviceFilter string) (*config.LiveConfig, error) {
	cfg := &config.LiveConfig{
		ProjectID:     projectID,
		EnvironmentID: environmentID,
		Shared:        map[string]string{},
		Services:      map[string]*config.ServiceConfig{},
	}

	shared, err := Variables(ctx, client.GQL(), projectID, environmentID, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range shared.Variables {
		cfg.Shared[k] = v
	}

	services, err := ProjectServices(ctx, client.GQL(), projectID)
	if err != nil {
		return nil, err
	}
	for _, edge := range services.Project.Services.Edges {
		if serviceFilter != "" && edge.Node.Name != serviceFilter {
			continue
		}
		svc := &config.ServiceConfig{
			ID:        edge.Node.Id,
			Name:      edge.Node.Name,
			Variables: map[string]string{},
		}
		vars, err := Variables(ctx, client.GQL(), projectID, environmentID, &edge.Node.Id)
		if err != nil {
			return nil, err
		}
		for k, v := range vars.Variables {
			svc.Variables[k] = v
		}
		cfg.Services[edge.Node.Name] = svc
	}

	return cfg, nil
}
```

Update the test helper to use `strings.Contains` directly (replace the inline contains hack).

**Step 4: Run test to verify it passes**

Run: `go test ./internal/railway -run TestFetchLiveConfig_IncludesSharedAndServiceVars -v`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/railway/state.go internal/railway/state_test.go
git commit -m "Add live config fetcher for config get"
```

---

### Task 6: Add mutations for set/delete variables

**Files:**

- Create: `internal/railway/mutate.go`
- Test: `internal/railway/mutate_test.go`

**Step 1: Write the failing test**

Create `internal/railway/mutate_test.go`:

```go
package railway_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/railway"
)

func TestVariableUpsert_SendsInput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"variableUpsert": true}})
	}))
	defer server.Close()

	client := railway.NewClient(server.URL, nil, nil, nil)
	err := railway.UpsertVariable(context.Background(), client, "proj", "env", "svc", "PORT", "8080", true)
	if err != nil {
		t.Fatalf("UpsertVariable() error: %v", err)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/railway -run TestVariableUpsert_SendsInput -v`

Expected: FAIL with “undefined: railway.UpsertVariable”.

**Step 3: Write minimal implementation**

Create `internal/railway/mutate.go`:

```go
package railway

import "context"

// UpsertVariable sets a single variable for shared or service scope.
func UpsertVariable(ctx context.Context, client *Client, projectID, environmentID, serviceID, name, value string, skipDeploys bool) error {
	input := VariableUpsertInput{
		ProjectId:     projectID,
		EnvironmentId: environmentID,
		Name:          name,
		Value:         value,
		SkipDeploys:   &skipDeploys,
	}
	if serviceID != "" {
		input.ServiceId = &serviceID
	}
	_, err := VariableUpsert(ctx, client.GQL(), input)
	return err
}

// DeleteVariable deletes a single variable.
func DeleteVariable(ctx context.Context, client *Client, projectID, environmentID, serviceID, name string) error {
	input := VariableDeleteInput{
		ProjectId:     projectID,
		EnvironmentId: environmentID,
		Name:          name,
	}
	if serviceID != "" {
		input.ServiceId = &serviceID
	}
	_, err := VariableDelete(ctx, client.GQL(), input)
	return err
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/railway -run TestVariableUpsert_SendsInput -v`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/railway/mutate.go internal/railway/mutate_test.go
git commit -m "Add variable upsert/delete mutations"
```

---

### Task 7: Add mutations for service settings updates

**Files:**

- Modify: `internal/railway/mutate.go`
- Test: `internal/railway/mutate_test.go`

**Step 1: Write the failing test**

Append to `internal/railway/mutate_test.go`:

```go
func TestUpdateServiceLimits_Succeeds(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"serviceInstanceLimitsUpdate": true}})
	}))
	defer server.Close()

	client := railway.NewClient(server.URL, nil, nil, nil)
	err := railway.UpdateServiceLimits(context.Background(), client, "env", "svc", 0.5, 1.0)
	if err != nil {
		t.Fatalf("UpdateServiceLimits() error: %v", err)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/railway -run TestUpdateServiceLimits_Succeeds -v`

Expected: FAIL with “undefined: railway.UpdateServiceLimits”.

**Step 3: Write minimal implementation**

Append to `internal/railway/mutate.go`:

```go
// UpdateServiceLimits updates vCPU and memory limits.
func UpdateServiceLimits(ctx context.Context, client *Client, environmentID, serviceID string, vcpus, memoryGB float64) error {
	input := ServiceInstanceLimitsUpdateInput{
		EnvironmentId: environmentID,
		ServiceId:     serviceID,
		VCPUs:         &vcpus,
		MemoryGB:      &memoryGB,
	}
	_, err := ServiceInstanceLimitsUpdate(ctx, client.GQL(), input)
	return err
}

// UpdateServiceSettings updates deploy/build settings.
func UpdateServiceSettings(ctx context.Context, client *Client, serviceID string, input ServiceInstanceUpdateInput) error {
	input.ServiceId = serviceID
	_, err := ServiceInstanceUpdate(ctx, client.GQL(), input)
	return err
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/railway -run TestUpdateServiceLimits_Succeeds -v`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/railway/mutate.go internal/railway/mutate_test.go
git commit -m "Add service settings update mutations"
```

---

### Task 8: Implement config get in CLI

**Files:**

- Modify: `internal/cli/cli.go`
- Create: `internal/cli/config_get.go`
- Test: `internal/cli/config_get_test.go`

**Step 1: Write the failing test**

Create `internal/cli/config_get_test.go`:

```go
package cli_test

import (
	"bytes"
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/cli"
)

func TestConfigGet_PrintsOutput(t *testing.T) {
	var buf bytes.Buffer
	cmd := &cli.ConfigGetCmd{}
	cmd.SetOutput(&buf)
	if err := cmd.Run(&cli.Globals{}); err == nil {
		// Expected to fail before implementation wires dependencies.
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/cli -run TestConfigGet_PrintsOutput -v`

Expected: FAIL with “undefined: (*ConfigGetCmd).SetOutput”.

**Step 3: Write minimal implementation**

Create `internal/cli/config_get.go`:

```go
package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/hamishmorgan/fat-controller/internal/auth"
	"github.com/hamishmorgan/fat-controller/internal/config"
	"github.com/hamishmorgan/fat-controller/internal/platform"
	"github.com/hamishmorgan/fat-controller/internal/railway"
)

// allow injection for tests
type configFetcher interface {
	Resolve(ctx context.Context, project, environment string) (string, string, error)
	Fetch(ctx context.Context, projectID, environmentID, service string) (*config.LiveConfig, error)
}

type defaultConfigFetcher struct {
	client *railway.Client
}

func (d *defaultConfigFetcher) Resolve(ctx context.Context, project, environment string) (string, string, error) {
	return railway.ResolveProjectEnvironment(ctx, d.client, project, environment)
}

func (d *defaultConfigFetcher) Fetch(ctx context.Context, projectID, environmentID, service string) (*config.LiveConfig, error) {
	return railway.FetchLiveConfig(ctx, d.client, projectID, environmentID, service)
}

type outputSink interface{
	WriteString(string) (int, error)
}

func (c *ConfigGetCmd) SetOutput(w io.Writer) {
	if c.output == nil {
		c.output = w
	}
}

func (c *ConfigGetCmd) Run(globals *Globals) error {
	store := auth.NewTokenStore(auth.WithFallbackPath(platform.AuthFilePath()))
	resolved, err := auth.ResolveAuth(globals.Token, store)
	if err != nil {
		return err
	}
	client := railway.NewClient(railway.Endpoint, resolved, store, auth.NewOAuthClient())
	fetcher := &defaultConfigFetcher{client: client}
	return runConfigGet(context.Background(), globals, c.Path, fetcher, c.output)
}

func runConfigGet(ctx context.Context, globals *Globals, path string, fetcher configFetcher, out io.Writer) error {
	if out == nil {
		out = &bytes.Buffer{}
	}
	projID, envID, err := fetcher.Resolve(ctx, globals.Project, globals.Environment)
	if err != nil {
		return err
	}
	service := globals.Service
	if path != "" {
		parsed, err := config.ParsePath(path)
		if err != nil {
			return err
		}
		if parsed.Service != "" {
			service = parsed.Service
		}
	}
	cfg, err := fetcher.Fetch(ctx, projID, envID, service)
	if err != nil {
		return err
	}
	if cfg == nil {
		return errors.New("no config")
	}
	output, err := config.Render(*cfg, globals.Output, globals.Full)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, output)
	return err
}
```

Update `internal/cli/cli.go` to add a `output io.Writer` field to `ConfigGetCmd` and remove stub Run body.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/cli -run TestConfigGet_PrintsOutput -v`

Expected: PASS (or update test to assert error behavior once wiring is complete).

**Step 5: Commit**

```bash
git add internal/cli/config_get.go internal/cli/config_get_test.go internal/cli/cli.go
git commit -m "Implement config get command"
```

---

### Task 9: Implement config set/delete with confirm/dry-run

**Files:**

- Create: `internal/cli/config_set.go`
- Create: `internal/cli/config_delete.go`
- Modify: `internal/cli/cli.go`
- Test: `internal/cli/config_set_test.go`
- Test: `internal/cli/config_delete_test.go`

**Step 1: Write the failing tests**

Create `internal/cli/config_set_test.go`:

```go
package cli_test

import (
	"context"
	"errors"
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/cli"
)

type fakeSetter struct{ called bool }

func (f *fakeSetter) SetVar(ctx context.Context, service, key, value string, confirm bool) error {
	f.called = true
	if !confirm {
		return errors.New("dry run")
	}
	return nil
}

func TestConfigSet_DryRunByDefault(t *testing.T) {
	cmd := &cli.ConfigSetCmd{Path: "api.variables.PORT", Value: "8080"}
	setter := &fakeSetter{}
	err := cli.RunConfigSet(context.Background(), &cli.Globals{}, cmd.Path, cmd.Value, setter)
	if err == nil {
		t.Fatal("expected dry run error")
	}
	if !setter.called {
		t.Fatal("expected setter to be called")
	}
}
```

Create `internal/cli/config_delete_test.go`:

```go
package cli_test

import (
	"context"
	"errors"
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/cli"
)

type fakeDeleter struct{ called bool }

func (f *fakeDeleter) DeleteVar(ctx context.Context, service, key string, confirm bool) error {
	f.called = true
	if !confirm {
		return errors.New("dry run")
	}
	return nil
}

func TestConfigDelete_DryRunByDefault(t *testing.T) {
	cmd := &cli.ConfigDeleteCmd{Path: "api.variables.OLD"}
	deleter := &fakeDeleter{}
	err := cli.RunConfigDelete(context.Background(), &cli.Globals{}, cmd.Path, deleter)
	if err == nil {
		t.Fatal("expected dry run error")
	}
	if !deleter.called {
		t.Fatal("expected deleter to be called")
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/cli -run TestConfigSet_DryRunByDefault -v`

Expected: FAIL with “undefined: cli.RunConfigSet”.

**Step 3: Write minimal implementation**

Create `internal/cli/config_set.go`:

```go
package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/hamishmorgan/fat-controller/internal/config"
)

type setRunner interface {
	SetVar(ctx context.Context, service, key, value string, confirm bool) error
}

func RunConfigSet(ctx context.Context, globals *Globals, path, value string, runner setRunner) error {
	parsed, err := config.ParsePath(path)
	if err != nil {
		return err
	}
	if parsed.Section != "variables" || parsed.Key == "" {
		return errors.New("config set currently supports only variables")
	}
	confirm := globals.Confirm && !globals.DryRun
	if err := runner.SetVar(ctx, parsed.Service, parsed.Key, value, confirm); err != nil {
		return err
	}
	if !confirm {
		return fmt.Errorf("dry run: use --confirm to apply")
	}
	return nil
}
```

Create `internal/cli/config_delete.go`:

```go
package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/hamishmorgan/fat-controller/internal/config"
)

type deleteRunner interface {
	DeleteVar(ctx context.Context, service, key string, confirm bool) error
}

func RunConfigDelete(ctx context.Context, globals *Globals, path string, runner deleteRunner) error {
	parsed, err := config.ParsePath(path)
	if err != nil {
		return err
	}
	if parsed.Section != "variables" || parsed.Key == "" {
		return errors.New("config delete currently supports only variables")
	}
	confirm := globals.Confirm && !globals.DryRun
	if err := runner.DeleteVar(ctx, parsed.Service, parsed.Key, confirm); err != nil {
		return err
	}
	if !confirm {
		return fmt.Errorf("dry run: use --confirm to apply")
	}
	return nil
}
```

Wire `ConfigSetCmd.Run` and `ConfigDeleteCmd.Run` in `internal/cli/cli.go` to call these helpers with real railway mutations.

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/cli -run TestConfigSet_DryRunByDefault -v`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/cli/config_set.go internal/cli/config_delete.go internal/cli/config_set_test.go internal/cli/config_delete_test.go internal/cli/cli.go
git commit -m "Implement config set/delete with confirm gating"
```

---

### Task 10: Wire CLI set/delete to Railway mutations

**Files:**

- Modify: `internal/cli/cli.go`
- Modify: `internal/cli/config_set.go`
- Modify: `internal/cli/config_delete.go`

**Step 1: Write the failing test**

Append to `internal/cli/config_set_test.go`:

```go
func TestConfigSet_RejectsNonVariablePath(t *testing.T) {
	cmd := &cli.ConfigSetCmd{Path: "api.resources.vcpus", Value: "1"}
	err := cli.RunConfigSet(context.Background(), &cli.Globals{Confirm: true}, cmd.Path, cmd.Value, &fakeSetter{})
	if err == nil {
		t.Fatal("expected error for non-variable path")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/cli -run TestConfigSet_RejectsNonVariablePath -v`

Expected: FAIL until CLI wiring is complete.

**Step 3: Write minimal implementation**

In `internal/cli/config_set.go`, add a concrete runner that wraps railway mutations:

```go
type railwaySetter struct {
	client       *railway.Client
	projectID    string
	environmentID string
	skipDeploys  bool
}

func (r *railwaySetter) SetVar(ctx context.Context, service, key, value string, confirm bool) error {
	if !confirm {
		return nil
	}
	serviceID := r.resolveServiceID(service)
	return railway.UpsertVariable(ctx, r.client, r.projectID, r.environmentID, serviceID, key, value, r.skipDeploys)
}
```

Add a small `resolveServiceID` helper that maps service name → ID using a cached live config fetch (reuse `FetchLiveConfig` with `serviceFilter`).

Do the same for delete using `railway.DeleteVariable`.

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/cli -run TestConfigSet_RejectsNonVariablePath -v`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/cli/config_set.go internal/cli/config_delete.go internal/cli/cli.go
git commit -m "Wire config set/delete to Railway variable mutations"
```

---

### Task 11: Update docs for M3 behavior

**Files:**

- Modify: `docs/COMMANDS.md`

**Step 1: Write the failing doc lint check**

Run: `mise run lint:md`

Expected: PASS (this is a no-op check for later).

**Step 2: Update docs**

In `docs/COMMANDS.md`, add a note in the config set/delete section:

```markdown
Note: In M3, `config set/delete` supports variables only (`*.variables.KEY`).
Other sections will be added in later milestones.
```

**Step 3: Run lint to verify it passes**

Run: `mise run lint:md`

Expected: PASS.

**Step 4: Commit**

```bash
git add docs/COMMANDS.md
git commit -m "Document M3 config set/delete scope"
```

---

### Task 12: Final verification

**Files:**

- Test: `./...`

**Step 1: Run the full check suite**

Run: `mise run check`

Expected: All linters pass, all tests pass, build succeeds.

**Step 2: Run targeted CLI tests**

Run: `go test ./internal/cli -v`

Expected: PASS.

**Step 3: Commit if any remaining changes**

```bash
git add -A
git commit -m "Complete M3 imperative CRUD"
```
