package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/apply"
	"github.com/hamishmorgan/fat-controller/internal/auth"
	"github.com/hamishmorgan/fat-controller/internal/cli"
	"github.com/hamishmorgan/fat-controller/internal/config"
	"github.com/hamishmorgan/fat-controller/internal/railway"
	"github.com/zalando/go-keyring"
)

// ---------------------------------------------------------------------------
// Fixture constants — shared across all E2E subtests.
// ---------------------------------------------------------------------------

const (
	fixtureWorkspaceID   = "ws-1"
	fixtureWorkspaceName = "Acme"
	fixtureProjectID     = "proj-1"
	fixtureProjectName   = "my-app"
	fixtureEnvironmentID = "env-1"
	fixtureEnvironment   = "production"
	fixtureServiceAPIID  = "svc-api"
	fixtureServiceAPI    = "api"
	fixtureServiceWorkID = "svc-worker"
	fixtureServiceWork   = "worker"
)

// ---------------------------------------------------------------------------
// Mock GraphQL server
// ---------------------------------------------------------------------------

// mockGraphQLServer is an httptest.Server that responds to genqlient-generated
// GraphQL operations with canned Railway API responses. It also records
// mutation calls so tests can assert what was sent.
type mockGraphQLServer struct {
	t *testing.T

	// options set before the server is started (not guarded by mu).
	workspaces []mockWorkspace

	mu       sync.Mutex
	url      string // set once by newMockGraphQLServer
	upserts  []recordedUpsert
	deletes  []recordedDelete
	settings []recordedSettingsUpdate
	limits   []recordedLimitsUpdate
	lastAuth string
}

type mockWorkspace struct {
	ID   string
	Name string
}

type recordedUpsert struct {
	ProjectID     string
	EnvironmentID string
	ServiceID     *string
	Name          string
	Value         string
	SkipDeploys   *bool
}

type recordedDelete struct {
	ProjectID     string
	EnvironmentID string
	ServiceID     *string
	Name          string
}

type recordedSettingsUpdate struct {
	ServiceID string
	Input     map[string]interface{}
}

type recordedLimitsUpdate struct {
	EnvironmentID string
	ServiceID     string
	VCPUs         *float64
	MemoryGB      *float64
}

// mockServerOption configures a mockGraphQLServer before starting.
type mockServerOption func(*mockGraphQLServer)

// withWorkspaces sets multiple workspaces so the ApiToken query returns an
// ambiguous list (triggers the non-TTY error path).
func withWorkspaces(ws ...mockWorkspace) mockServerOption {
	return func(m *mockGraphQLServer) { m.workspaces = ws }
}

// newMockGraphQLServer creates a mock server with canned responses and
// starts an httptest.Server. The server is registered for cleanup via
// t.Cleanup so callers don't need to defer Close.
func newMockGraphQLServer(t *testing.T, opts ...mockServerOption) *mockGraphQLServer {
	t.Helper()
	srv := &mockGraphQLServer{t: t}
	for _, opt := range opts {
		opt(srv)
	}
	server := httptest.NewServer(http.HandlerFunc(srv.handle))
	t.Cleanup(server.Close)

	// Stash the URL so tests can access it via the mock.
	srv.mu.Lock()
	srv.url = server.URL
	srv.mu.Unlock()

	return srv
}

// URL returns the mock server's base URL. Safe to call concurrently.
func (m *mockGraphQLServer) URL() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.url
}

// snapshot returns a point-in-time copy of recorded mutations.
func (m *mockGraphQLServer) snapshot() mockSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	return mockSnapshot{
		Upserts:  append([]recordedUpsert(nil), m.upserts...),
		Deletes:  append([]recordedDelete(nil), m.deletes...),
		Settings: append([]recordedSettingsUpdate(nil), m.settings...),
		Limits:   append([]recordedLimitsUpdate(nil), m.limits...),
		LastAuth: m.lastAuth,
	}
}

type mockSnapshot struct {
	Upserts  []recordedUpsert
	Deletes  []recordedDelete
	Settings []recordedSettingsUpdate
	Limits   []recordedLimitsUpdate
	LastAuth string
}

// handle dispatches incoming GraphQL requests to canned responses.
// genqlient always sets OperationName; inferOperation is a fallback
// in case a future version omits it.
func (m *mockGraphQLServer) handle(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	m.lastAuth = r.Header.Get("Authorization")
	m.mu.Unlock()

	var req graphqlReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid graphql request", http.StatusBadRequest)
		m.t.Errorf("decode graphql request: %v", err)
		return
	}

	op := req.OperationName
	if op == "" {
		op = inferOperation(req.Query)
	}

	switch op {
	case "ApiToken":
		m.handleApiToken(w)
	case "Projects":
		m.handleProjects(w)
	case "Environments":
		m.handleEnvironments(w)
	case "ProjectServices":
		m.handleProjectServices(w)
	case "Variables":
		m.handleVariables(w, req)
	case "VariableUpsert":
		m.handleVariableUpsert(w, req)
	case "VariableDelete":
		m.handleVariableDelete(w, req)
	case "ServiceInstanceUpdate":
		m.handleServiceInstanceUpdate(w, req)
	case "ServiceInstanceLimitsUpdate":
		m.handleServiceInstanceLimitsUpdate(w, req)
	case "ProjectToken":
		m.handleProjectToken(w)
	default:
		http.Error(w, "unknown operation", http.StatusBadRequest)
		m.t.Errorf("unknown operation: %s", op)
	}
}

func (m *mockGraphQLServer) handleApiToken(w http.ResponseWriter) {
	workspaces := m.workspaces
	if len(workspaces) == 0 {
		workspaces = []mockWorkspace{{ID: fixtureWorkspaceID, Name: fixtureWorkspaceName}}
	}
	items := make([]map[string]any, 0, len(workspaces))
	for _, ws := range workspaces {
		items = append(items, map[string]any{"id": ws.ID, "name": ws.Name})
	}
	respondJSON(m.t, w, map[string]any{
		"data": map[string]any{
			"apiToken": map[string]any{"workspaces": items},
		},
	})
}

func (m *mockGraphQLServer) handleProjects(w http.ResponseWriter) {
	respondJSON(m.t, w, map[string]any{
		"data": map[string]any{
			"projects": map[string]any{
				"edges": []map[string]any{{
					"node": map[string]any{"id": fixtureProjectID, "name": fixtureProjectName},
				}},
			},
		},
	})
}

func (m *mockGraphQLServer) handleEnvironments(w http.ResponseWriter) {
	respondJSON(m.t, w, map[string]any{
		"data": map[string]any{
			"environments": map[string]any{
				"edges": []map[string]any{{
					"node": map[string]any{"id": fixtureEnvironmentID, "name": fixtureEnvironment},
				}},
			},
		},
	})
}

func (m *mockGraphQLServer) handleProjectServices(w http.ResponseWriter) {
	respondJSON(m.t, w, map[string]any{
		"data": map[string]any{
			"project": map[string]any{
				"services": map[string]any{
					"edges": []map[string]any{
						{"node": map[string]any{"id": fixtureServiceAPIID, "name": fixtureServiceAPI}},
						{"node": map[string]any{"id": fixtureServiceWorkID, "name": fixtureServiceWork}},
					},
				},
			},
		},
	})
}

func (m *mockGraphQLServer) handleVariables(w http.ResponseWriter, req graphqlReq) {
	serviceID, _ := req.Variables["serviceId"].(string)
	vars := map[string]any{"GLOBAL": "one"}
	if serviceID == fixtureServiceAPIID {
		vars = map[string]any{"PORT": "8080"}
	}
	if serviceID == fixtureServiceWorkID {
		vars = map[string]any{"QUEUE": "default"}
	}
	respondJSON(m.t, w, map[string]any{
		"data": map[string]any{"variables": vars},
	})
}

func (m *mockGraphQLServer) handleVariableUpsert(w http.ResponseWriter, req graphqlReq) {
	call, err := parseUpsertInput(req.Variables)
	if err != nil {
		http.Error(w, "invalid variable upsert", http.StatusBadRequest)
		m.t.Errorf("parse variable upsert: %v", err)
		return
	}
	m.mu.Lock()
	m.upserts = append(m.upserts, call)
	m.mu.Unlock()
	respondJSON(m.t, w, map[string]any{
		"data": map[string]any{"variableUpsert": true},
	})
}

func (m *mockGraphQLServer) handleVariableDelete(w http.ResponseWriter, req graphqlReq) {
	call, err := parseDeleteInput(req.Variables)
	if err != nil {
		http.Error(w, "invalid variable delete", http.StatusBadRequest)
		m.t.Errorf("parse variable delete: %v", err)
		return
	}
	m.mu.Lock()
	m.deletes = append(m.deletes, call)
	m.mu.Unlock()
	respondJSON(m.t, w, map[string]any{
		"data": map[string]any{"variableDelete": true},
	})
}

func (m *mockGraphQLServer) handleServiceInstanceUpdate(w http.ResponseWriter, req graphqlReq) {
	call, err := parseSettingsInput(req.Variables)
	if err != nil {
		http.Error(w, "invalid service instance update", http.StatusBadRequest)
		m.t.Errorf("parse service instance update: %v", err)
		return
	}
	m.mu.Lock()
	m.settings = append(m.settings, call)
	m.mu.Unlock()
	respondJSON(m.t, w, map[string]any{
		"data": map[string]any{"serviceInstanceUpdate": true},
	})
}

func (m *mockGraphQLServer) handleServiceInstanceLimitsUpdate(w http.ResponseWriter, req graphqlReq) {
	call, err := parseLimitsInput(req.Variables)
	if err != nil {
		http.Error(w, "invalid service instance limits update", http.StatusBadRequest)
		m.t.Errorf("parse service instance limits update: %v", err)
		return
	}
	m.mu.Lock()
	m.limits = append(m.limits, call)
	m.mu.Unlock()
	respondJSON(m.t, w, map[string]any{
		"data": map[string]any{"serviceInstanceLimitsUpdate": true},
	})
}

func (m *mockGraphQLServer) handleProjectToken(w http.ResponseWriter) {
	respondJSON(m.t, w, map[string]any{
		"data": map[string]any{
			"projectToken": map[string]any{
				"projectId":     fixtureProjectID,
				"environmentId": fixtureEnvironmentID,
			},
		},
	})
}

// ---------------------------------------------------------------------------
// Request parsing helpers
// ---------------------------------------------------------------------------

type graphqlReq struct {
	Query         string                 `json:"query"`
	OperationName string                 `json:"operationName"`
	Variables     map[string]interface{} `json:"variables"`
}

// inferOperation is a fallback for when OperationName is empty.
// genqlient always sets OperationName, so this should rarely fire.
func inferOperation(query string) string {
	for _, pair := range []struct {
		needle string
		op     string
	}{
		{"mutation ServiceInstanceLimitsUpdate", "ServiceInstanceLimitsUpdate"},
		{"mutation ServiceInstanceUpdate", "ServiceInstanceUpdate"},
		{"mutation VariableUpsert", "VariableUpsert"},
		{"mutation VariableDelete", "VariableDelete"},
		{"query ApiToken", "ApiToken"},
		{"query Projects", "Projects"},
		{"query Environments", "Environments"},
		{"query ProjectServices", "ProjectServices"},
		{"query Variables", "Variables"},
		{"query ProjectToken", "ProjectToken"},
	} {
		if strings.Contains(query, pair.needle) {
			return pair.op
		}
	}
	return ""
}

func parseUpsertInput(vars map[string]interface{}) (recordedUpsert, error) {
	input, ok := vars["input"].(map[string]interface{})
	if !ok {
		return recordedUpsert{}, errors.New("missing input")
	}
	projectID, err := jsonString(input, "projectId")
	if err != nil {
		return recordedUpsert{}, err
	}
	environmentID, err := jsonString(input, "environmentId")
	if err != nil {
		return recordedUpsert{}, err
	}
	name, err := jsonString(input, "name")
	if err != nil {
		return recordedUpsert{}, err
	}
	value, err := jsonString(input, "value")
	if err != nil {
		return recordedUpsert{}, err
	}
	return recordedUpsert{
		ProjectID:     projectID,
		EnvironmentID: environmentID,
		ServiceID:     jsonOptionalString(input, "serviceId"),
		Name:          name,
		Value:         value,
		SkipDeploys:   jsonOptionalBool(input, "skipDeploys"),
	}, nil
}

func parseDeleteInput(vars map[string]interface{}) (recordedDelete, error) {
	input, ok := vars["input"].(map[string]interface{})
	if !ok {
		return recordedDelete{}, errors.New("missing input")
	}
	projectID, err := jsonString(input, "projectId")
	if err != nil {
		return recordedDelete{}, err
	}
	environmentID, err := jsonString(input, "environmentId")
	if err != nil {
		return recordedDelete{}, err
	}
	name, err := jsonString(input, "name")
	if err != nil {
		return recordedDelete{}, err
	}
	return recordedDelete{
		ProjectID:     projectID,
		EnvironmentID: environmentID,
		ServiceID:     jsonOptionalString(input, "serviceId"),
		Name:          name,
	}, nil
}

func parseSettingsInput(vars map[string]interface{}) (recordedSettingsUpdate, error) {
	serviceID, err := jsonString(vars, "serviceId")
	if err != nil {
		return recordedSettingsUpdate{}, err
	}
	input, ok := vars["input"].(map[string]interface{})
	if !ok {
		return recordedSettingsUpdate{}, errors.New("missing input")
	}
	return recordedSettingsUpdate{ServiceID: serviceID, Input: input}, nil
}

func parseLimitsInput(vars map[string]interface{}) (recordedLimitsUpdate, error) {
	input, ok := vars["input"].(map[string]interface{})
	if !ok {
		return recordedLimitsUpdate{}, errors.New("missing input")
	}
	environmentID, err := jsonString(input, "environmentId")
	if err != nil {
		return recordedLimitsUpdate{}, err
	}
	serviceID, err := jsonString(input, "serviceId")
	if err != nil {
		return recordedLimitsUpdate{}, err
	}
	return recordedLimitsUpdate{
		EnvironmentID: environmentID,
		ServiceID:     serviceID,
		VCPUs:         jsonOptionalFloat(input, "vCPUs"),
		MemoryGB:      jsonOptionalFloat(input, "memoryGB"),
	}, nil
}

// ---------------------------------------------------------------------------
// JSON extraction helpers
// ---------------------------------------------------------------------------

func jsonString(m map[string]interface{}, key string) (string, error) {
	raw, ok := m[key]
	if !ok {
		return "", fmt.Errorf("missing %s", key)
	}
	s, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("%s: expected string, got %T", key, raw)
	}
	return s, nil
}

func jsonOptionalString(m map[string]interface{}, key string) *string {
	raw, ok := m[key]
	if !ok || raw == nil {
		return nil
	}
	s, ok := raw.(string)
	if !ok {
		return nil
	}
	return &s
}

func jsonOptionalBool(m map[string]interface{}, key string) *bool {
	raw, ok := m[key]
	if !ok || raw == nil {
		return nil
	}
	b, ok := raw.(bool)
	if !ok {
		return nil
	}
	return &b
}

func jsonOptionalFloat(m map[string]interface{}, key string) *float64 {
	raw, ok := m[key]
	if !ok || raw == nil {
		return nil
	}
	switch v := raw.(type) {
	case float64:
		return &v
	case json.Number:
		f, err := v.Float64()
		if err != nil {
			return nil
		}
		return &f
	default:
		return nil
	}
}

func respondJSON(t *testing.T, w http.ResponseWriter, payload any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		t.Errorf("encode response: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// newTestClient creates a Railway client pointing at the mock server with
// a flag-sourced Bearer token.
func newTestClient(url string) *railway.Client {
	return railway.NewClient(url, &auth.ResolvedAuth{
		Token:       "test-token",
		HeaderName:  "Authorization",
		HeaderValue: "Bearer test-token",
		Source:      auth.SourceFlag,
	}, nil, nil)
}

// e2eFetcher delegates Resolve and Fetch to the real railway package,
// but pointed at a mock httptest server.
type e2eFetcher struct {
	client *railway.Client
}

func (e *e2eFetcher) Resolve(ctx context.Context, workspace, project, environment string) (string, string, error) {
	return railway.ResolveProjectEnvironment(ctx, e.client, workspace, project, environment)
}

func (e *e2eFetcher) Fetch(ctx context.Context, projectID, environmentID, service string) (*config.LiveConfig, error) {
	return railway.FetchLiveConfig(ctx, e.client, projectID, environmentID, service)
}

// writeTOML writes content to fat-controller.toml inside dir.
func writeTOML(t *testing.T, dir, content string) {
	t.Helper()
	p := filepath.Join(dir, config.BaseConfigFile)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", config.BaseConfigFile, err)
	}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestCLIE2E_MockedGraphQL exercises the CLI's core flows end-to-end
// against a mocked Railway GraphQL API (httptest server). No real
// credentials or network calls are required.
func TestCLIE2E_MockedGraphQL(t *testing.T) {
	t.Run("config init generates expected file", func(t *testing.T) {
		mock := newMockGraphQLServer(t)
		client := newTestClient(mock.URL())
		fetcher := &e2eFetcher{client: client}

		dir := t.TempDir()
		var out bytes.Buffer
		if err := cli.RunConfigInit(context.Background(), dir, fixtureProjectName, fixtureEnvironment, fetcher, &out); err != nil {
			t.Fatalf("RunConfigInit() error: %v", err)
		}

		content, err := os.ReadFile(filepath.Join(dir, config.BaseConfigFile))
		if err != nil {
			t.Fatalf("read %s: %v", config.BaseConfigFile, err)
		}
		got := string(content)

		for _, want := range []string{
			`project = "` + fixtureProjectName + `"`,
			`environment = "` + fixtureEnvironment + `"`,
			"[shared.variables]",
			"[api.variables]",
		} {
			if !strings.Contains(got, want) {
				t.Errorf("expected %q in config file, got:\n%s", want, got)
			}
		}
	})

	t.Run("config get returns live state", func(t *testing.T) {
		mock := newMockGraphQLServer(t)
		client := newTestClient(mock.URL())
		fetcher := &e2eFetcher{client: client}

		globals := &cli.Globals{
			Project:     fixtureProjectName,
			Environment: fixtureEnvironment,
			Output:      "text",
			ShowSecrets: true, // so values are not masked
		}
		var out bytes.Buffer
		if err := cli.RunConfigGet(context.Background(), globals, "", fetcher, &out); err != nil {
			t.Fatalf("RunConfigGet() error: %v", err)
		}
		output := out.String()

		for _, want := range []string{"GLOBAL = one", "PORT = 8080", "QUEUE = default"} {
			if !strings.Contains(output, want) {
				t.Errorf("expected %q in output, got:\n%s", want, output)
			}
		}

		snap := mock.snapshot()
		if snap.LastAuth != "Bearer test-token" {
			t.Errorf("Authorization = %q, want %q", snap.LastAuth, "Bearer test-token")
		}
	})

	t.Run("config apply with variables and deploy settings", func(t *testing.T) {
		mock := newMockGraphQLServer(t)
		client := newTestClient(mock.URL())
		fetcher := &e2eFetcher{client: client}
		applier := &apply.RailwayApplier{
			Client:        client,
			ProjectID:     fixtureProjectID,
			EnvironmentID: fixtureEnvironmentID,
		}

		dir := t.TempDir()
		writeTOML(t, dir, `
project = "`+fixtureProjectName+`"
environment = "`+fixtureEnvironment+`"

[api.variables]
PORT = "9090"
NEW = "hello"

[api.deploy]
builder = "NIXPACKS"
start_command = "./start"

[api.resources]
vcpus = 1
memory_gb = 2
`)

		globals := &cli.Globals{Confirm: true}
		var out bytes.Buffer
		if err := cli.RunConfigApply(context.Background(), globals, dir, nil, fetcher, applier, &out); err != nil {
			t.Fatalf("RunConfigApply() error: %v", err)
		}
		// Expect 4 applied operations:
		//   1. ServiceInstanceUpdate  (deploy settings: builder + start_command)
		//   2. ServiceInstanceLimitsUpdate (resources: vcpus + memory_gb)
		//   3. VariableUpsert (PORT 8080→9090)
		//   4. VariableUpsert (NEW created)
		if !strings.Contains(out.String(), "applied=4") {
			t.Errorf("expected applied=4 summary, got:\n%s", out.String())
		}

		snap := mock.snapshot()

		// Variable upserts.
		if got := len(snap.Upserts); got != 2 {
			t.Fatalf("upserts: got %d, want 2", got)
		}
		for _, u := range snap.Upserts {
			if u.ProjectID != fixtureProjectID {
				t.Errorf("upsert projectId = %q, want %q", u.ProjectID, fixtureProjectID)
			}
			if u.EnvironmentID != fixtureEnvironmentID {
				t.Errorf("upsert environmentId = %q, want %q", u.EnvironmentID, fixtureEnvironmentID)
			}
			if u.ServiceID == nil || *u.ServiceID != fixtureServiceAPIID {
				t.Errorf("upsert serviceId = %v, want %q", u.ServiceID, fixtureServiceAPIID)
			}
		}

		// Deploy settings.
		if got := len(snap.Settings); got != 1 {
			t.Fatalf("serviceInstanceUpdate: got %d, want 1", got)
		}
		if snap.Settings[0].ServiceID != fixtureServiceAPIID {
			t.Errorf("settings serviceId = %q, want %q", snap.Settings[0].ServiceID, fixtureServiceAPIID)
		}
		if b, ok := snap.Settings[0].Input["builder"].(string); !ok || b != "NIXPACKS" {
			t.Errorf("builder = %v, want %q", snap.Settings[0].Input["builder"], "NIXPACKS")
		}
		if sc, ok := snap.Settings[0].Input["startCommand"].(string); !ok || sc != "./start" {
			t.Errorf("startCommand = %v, want %q", snap.Settings[0].Input["startCommand"], "./start")
		}

		// Resource limits.
		if got := len(snap.Limits); got != 1 {
			t.Fatalf("serviceInstanceLimitsUpdate: got %d, want 1", got)
		}
		lim := snap.Limits[0]
		if lim.EnvironmentID != fixtureEnvironmentID {
			t.Errorf("limits environmentId = %q, want %q", lim.EnvironmentID, fixtureEnvironmentID)
		}
		if lim.ServiceID != fixtureServiceAPIID {
			t.Errorf("limits serviceId = %q, want %q", lim.ServiceID, fixtureServiceAPIID)
		}
		if lim.VCPUs == nil || *lim.VCPUs != 1 {
			t.Errorf("vCPUs = %v, want 1", lim.VCPUs)
		}
		if lim.MemoryGB == nil || *lim.MemoryGB != 2 {
			t.Errorf("memoryGB = %v, want 2", lim.MemoryGB)
		}
	})

	t.Run("resolve auth fails without credentials", func(t *testing.T) {
		// Cannot use t.Parallel here: t.Setenv modifies process env.
		keyring.MockInit()
		t.Setenv("RAILWAY_TOKEN", "")
		t.Setenv("RAILWAY_API_TOKEN", "")

		store := auth.NewTokenStore(auth.WithFallbackPath(filepath.Join(t.TempDir(), "auth.json")))
		_, err := auth.ResolveAuth("", store)
		if err == nil {
			t.Fatal("expected error when no auth is configured")
		}
		if !errors.Is(err, auth.ErrNotAuthenticated) {
			t.Errorf("error = %v, want %v", err, auth.ErrNotAuthenticated)
		}
	})

	t.Run("config init fails with ambiguous workspace in non-tty", func(t *testing.T) {
		mock := newMockGraphQLServer(t, withWorkspaces(
			mockWorkspace{ID: fixtureWorkspaceID, Name: fixtureWorkspaceName},
			mockWorkspace{ID: "ws-2", Name: "Contoso"},
		))
		client := newTestClient(mock.URL())
		fetcher := &e2eFetcher{client: client}

		dir := t.TempDir()
		var out bytes.Buffer
		err := cli.RunConfigInit(context.Background(), dir, "", "", fetcher, &out)
		if err == nil {
			t.Fatal("expected error for ambiguous workspace selection in non-tty")
		}
		if !strings.Contains(err.Error(), "multiple workspaces") {
			t.Errorf("unexpected error: %v", err)
		}
	})
}
