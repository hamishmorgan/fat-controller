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
	"github.com/hamishmorgan/fat-controller/internal/prompt"
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

	// options — set before the server starts, never mutated after.
	url                  string // base URL of the httptest.Server
	workspaces           []mockWorkspace
	failUpsertAfter      int  // 0 = never fail; N = fail upserts after Nth
	failCollectionUpsert bool // if true, variableCollectionUpsert returns error
	failAll              bool // return GraphQL errors for all operations

	mu                sync.Mutex
	upserts           []recordedUpsert
	collectionUpserts []recordedCollectionUpsert
	deletes           []recordedDelete
	settings          []recordedSettingsUpdate
	limits            []recordedLimitsUpdate
	lastAuth          string
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

type recordedCollectionUpsert struct {
	ProjectID     string
	EnvironmentID string
	ServiceID     *string
	Variables     map[string]string
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

// withFailCollectionUpsert causes variableCollectionUpsert to return a
// GraphQL error. Useful for testing --fail-fast behavior with batch upserts.
func withFailCollectionUpsert() mockServerOption {
	return func(m *mockGraphQLServer) { m.failCollectionUpsert = true }
}

// withFailAllQueries causes all operations to return a GraphQL error.
func withFailAllQueries() mockServerOption {
	return func(m *mockGraphQLServer) { m.failAll = true }
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
	srv.url = server.URL
	return srv
}

// URL returns the mock server's base URL.
func (m *mockGraphQLServer) URL() string { return m.url }

// snapshot returns a point-in-time copy of recorded mutations.
func (m *mockGraphQLServer) snapshot() mockSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	return mockSnapshot{
		Upserts:           append([]recordedUpsert(nil), m.upserts...),
		CollectionUpserts: append([]recordedCollectionUpsert(nil), m.collectionUpserts...),
		Deletes:           append([]recordedDelete(nil), m.deletes...),
		Settings:          append([]recordedSettingsUpdate(nil), m.settings...),
		Limits:            append([]recordedLimitsUpdate(nil), m.limits...),
		LastAuth:          m.lastAuth,
	}
}

type mockSnapshot struct {
	Upserts           []recordedUpsert
	CollectionUpserts []recordedCollectionUpsert
	Deletes           []recordedDelete
	Settings          []recordedSettingsUpdate
	Limits            []recordedLimitsUpdate
	LastAuth          string
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

	if m.failAll {
		respondJSON(m.t, w, map[string]any{
			"data":   nil,
			"errors": []map[string]any{{"message": "simulated server error"}},
		})
		return
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
	case "VariableCollectionUpsert":
		m.handleVariableCollectionUpsert(w, req)
	case "VariableUpsert":
		m.handleVariableUpsert(w, req)
	case "VariableDelete":
		m.handleVariableDelete(w, req)
	case "ServiceInstance":
		m.handleServiceInstance(w)
	case "ServiceInstanceUpdate":
		m.handleServiceInstanceUpdate(w, req)
	case "ServiceInstanceLimits":
		m.handleServiceInstanceLimits(w)
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

func (m *mockGraphQLServer) handleVariableCollectionUpsert(w http.ResponseWriter, req graphqlReq) {
	call, err := parseCollectionUpsertInput(req.Variables)
	if err != nil {
		http.Error(w, "invalid variable collection upsert", http.StatusBadRequest)
		m.t.Errorf("parse variable collection upsert: %v", err)
		return
	}
	m.mu.Lock()
	m.collectionUpserts = append(m.collectionUpserts, call)
	m.mu.Unlock()

	if m.failCollectionUpsert {
		respondJSON(m.t, w, map[string]any{
			"data":   nil,
			"errors": []map[string]any{{"message": "simulated collection upsert failure"}},
		})
		return
	}
	respondJSON(m.t, w, map[string]any{
		"data": map[string]any{"variableCollectionUpsert": true},
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
	count := len(m.upserts)
	m.mu.Unlock()

	if m.failUpsertAfter > 0 && count > m.failUpsertAfter {
		respondJSON(m.t, w, map[string]any{
			"data":   nil,
			"errors": []map[string]any{{"message": "simulated upsert failure"}},
		})
		return
	}
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

func (m *mockGraphQLServer) handleServiceInstance(w http.ResponseWriter) {
	respondJSON(m.t, w, map[string]any{
		"data": map[string]any{
			"serviceInstance": map[string]any{
				"builder":         "NIXPACKS",
				"dockerfilePath":  nil,
				"rootDirectory":   nil,
				"startCommand":    nil,
				"healthcheckPath": nil,
			},
		},
	})
}

func (m *mockGraphQLServer) handleServiceInstanceLimits(w http.ResponseWriter) {
	respondJSON(m.t, w, map[string]any{
		"data": map[string]any{
			"serviceInstanceLimits": map[string]any{
				"vCPUs":    8.0,
				"memoryGB": 8.0,
			},
		},
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
		{"mutation VariableCollectionUpsert", "VariableCollectionUpsert"},
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

func parseCollectionUpsertInput(vars map[string]interface{}) (recordedCollectionUpsert, error) {
	input, ok := vars["input"].(map[string]interface{})
	if !ok {
		return recordedCollectionUpsert{}, errors.New("missing input")
	}
	projectID, err := jsonString(input, "projectId")
	if err != nil {
		return recordedCollectionUpsert{}, err
	}
	environmentID, err := jsonString(input, "environmentId")
	if err != nil {
		return recordedCollectionUpsert{}, err
	}
	variables := make(map[string]string)
	if raw, ok := input["variables"].(map[string]interface{}); ok {
		for k, v := range raw {
			if s, ok := v.(string); ok {
				variables[k] = s
			}
		}
	}
	return recordedCollectionUpsert{
		ProjectID:     projectID,
		EnvironmentID: environmentID,
		ServiceID:     jsonOptionalString(input, "serviceId"),
		Variables:     variables,
		SkipDeploys:   jsonOptionalBool(input, "skipDeploys"),
	}, nil
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

// newTestFetcher creates a mock server + client + e2eFetcher in one call.
// Returns the mock (for snapshot assertions) and the fetcher.
func newTestFetcher(t *testing.T, opts ...mockServerOption) (*mockGraphQLServer, *e2eFetcher) {
	t.Helper()
	mock := newMockGraphQLServer(t, opts...)
	client := newTestClient(mock.URL())
	return mock, &e2eFetcher{client: client}
}

// newTestInitResolver creates an e2eInitResolver pointed at a mock server.
func newTestInitResolver(t *testing.T, opts ...mockServerOption) (*mockGraphQLServer, *e2eInitResolver) {
	t.Helper()
	mock := newMockGraphQLServer(t, opts...)
	client := newTestClient(mock.URL())
	return mock, &e2eInitResolver{client: client}
}

// newTestApplier creates a RailwayApplier pointed at the mock server with
// fixture project/environment IDs.
func newTestApplier(mock *mockGraphQLServer) *apply.RailwayApplier {
	client := newTestClient(mock.URL())
	return &apply.RailwayApplier{
		Client:        client,
		ProjectID:     fixtureProjectID,
		EnvironmentID: fixtureEnvironmentID,
	}
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

// e2eInitResolver implements initResolver for e2e tests, delegating to the
// Railway API pointed at the mock server.
type e2eInitResolver struct {
	client *railway.Client
}

func (e *e2eInitResolver) FetchWorkspaces(ctx context.Context) ([]prompt.Item, error) {
	resp, err := railway.ApiToken(ctx, e.client.GQL())
	if err != nil {
		return nil, err
	}
	items := make([]prompt.Item, len(resp.ApiToken.Workspaces))
	for i, ws := range resp.ApiToken.Workspaces {
		items[i] = prompt.Item{Name: ws.Name, ID: ws.Id}
	}
	return items, nil
}

func (e *e2eInitResolver) FetchProjects(ctx context.Context, workspaceID string) ([]prompt.Item, error) {
	resp, err := railway.Projects(ctx, e.client.GQL(), &workspaceID)
	if err != nil {
		return nil, err
	}
	items := make([]prompt.Item, len(resp.Projects.Edges))
	for i, edge := range resp.Projects.Edges {
		items[i] = prompt.Item{Name: edge.Node.Name, ID: edge.Node.Id}
	}
	return items, nil
}

func (e *e2eInitResolver) FetchEnvironments(ctx context.Context, projectID string) ([]prompt.Item, error) {
	resp, err := railway.Environments(ctx, e.client.GQL(), projectID)
	if err != nil {
		return nil, err
	}
	items := make([]prompt.Item, len(resp.Environments.Edges))
	for i, edge := range resp.Environments.Edges {
		items[i] = prompt.Item{Name: edge.Node.Name, ID: edge.Node.Id}
	}
	return items, nil
}

func (e *e2eInitResolver) FetchLiveState(ctx context.Context, projectID, environmentID string) (*config.LiveConfig, error) {
	return railway.FetchLiveConfig(ctx, e.client, projectID, environmentID, "")
}

// dedent strips the common leading whitespace from all non-empty lines
// in a multi-line string, so TOML literals can be indented with the
// surrounding test code.
func dedent(s string) string {
	lines := strings.Split(strings.TrimLeft(s, "\n"), "\n")
	min := len(s) // start with a value larger than any indent
	for _, l := range lines {
		trimmed := strings.TrimLeft(l, "\t ")
		if len(trimmed) > 0 {
			indent := len(l) - len(trimmed)
			if indent < min {
				min = indent
			}
		}
	}
	for i, l := range lines {
		if len(l) >= min {
			lines[i] = l[min:]
		}
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n"
}

// writeConfigTOML writes content to fat-controller.toml inside dir.
// The content is automatically dedented so callers can indent the
// TOML literal with the surrounding test code.
func writeConfigTOML(t *testing.T, dir, content string) {
	t.Helper()
	p := filepath.Join(dir, config.BaseConfigFile)
	if err := os.WriteFile(p, []byte(dedent(content)), 0o644); err != nil {
		t.Fatalf("write %s: %v", config.BaseConfigFile, err)
	}
}

// ---------------------------------------------------------------------------
// Tests — config init
// ---------------------------------------------------------------------------

// TestCLIE2E_MockedGraphQL exercises the CLI's core flows end-to-end
// against a mocked Railway GraphQL API (httptest server). No real
// credentials or network calls are required.
func TestCLIE2E_MockedGraphQL(t *testing.T) {
	t.Run("config init generates expected file", func(t *testing.T) {
		_, resolver := newTestInitResolver(t)

		dir := t.TempDir()
		var out bytes.Buffer
		if err := cli.RunConfigInit(context.Background(), dir, fixtureWorkspaceName, fixtureProjectName, fixtureEnvironment, resolver, false, false, true, &out); err != nil {
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

	t.Run("config init non-interactive shows preview when file exists", func(t *testing.T) {
		_, resolver := newTestInitResolver(t)
		dir := t.TempDir()
		writeConfigTOML(t, dir, `project = "existing"`)

		var out bytes.Buffer
		// Non-interactive without --yes shows preview, does not write.
		err := cli.RunConfigInit(context.Background(), dir, fixtureWorkspaceName, fixtureProjectName, fixtureEnvironment, resolver, false, false, false, &out)
		if err != nil {
			t.Fatalf("RunConfigInit() error: %v", err)
		}
		got := out.String()
		if !strings.Contains(got, "would write") {
			t.Errorf("expected preview output, got:\n%s", got)
		}
		if !strings.Contains(got, "use --yes") {
			t.Errorf("expected --yes suggestion, got:\n%s", got)
		}
		// Original file should be unchanged.
		content, _ := os.ReadFile(filepath.Join(dir, config.BaseConfigFile))
		if !strings.Contains(string(content), "existing") {
			t.Error("config file should not have been overwritten")
		}
	})

	t.Run("config init fails with ambiguous workspace in non-tty", func(t *testing.T) {
		_, resolver := newTestInitResolver(t, withWorkspaces(
			mockWorkspace{ID: fixtureWorkspaceID, Name: fixtureWorkspaceName},
			mockWorkspace{ID: "ws-2", Name: "Contoso"},
		))

		dir := t.TempDir()
		var out bytes.Buffer
		err := cli.RunConfigInit(context.Background(), dir, "", "", "", resolver, false, false, true, &out)
		if err == nil {
			t.Fatal("expected error for ambiguous workspace selection in non-tty")
		}
		if !strings.Contains(err.Error(), "multiple workspaces") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	// -----------------------------------------------------------------------
	// Tests — config get
	// -----------------------------------------------------------------------

	t.Run("config get returns live state", func(t *testing.T) {
		mock, fetcher := newTestFetcher(t)

		globals := &cli.Globals{
			Output: "text",
		}
		var out bytes.Buffer
		if err := cli.RunConfigGet(context.Background(), globals, "", fixtureProjectName, fixtureEnvironment, "", false, "", true, fetcher, &out); err != nil {
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

	t.Run("config get with dot-path filters to service", func(t *testing.T) {
		_, fetcher := newTestFetcher(t)

		globals := &cli.Globals{
			Output: "text",
		}
		var out bytes.Buffer
		// Dot-path "api.variables.PORT" should scope Fetch to the "api" service.
		if err := cli.RunConfigGet(context.Background(), globals, "", fixtureProjectName, fixtureEnvironment, "api.variables.PORT", false, "", true, fetcher, &out); err != nil {
			t.Fatalf("RunConfigGet() error: %v", err)
		}
		output := out.String()
		// Single-key lookup should output just the raw value.
		if strings.Contains(output, "QUEUE") {
			t.Errorf("dot-path should exclude other services, but worker QUEUE appeared:\n%s", output)
		}
		if !strings.Contains(output, "8080") {
			t.Errorf("expected raw value 8080 in output, got:\n%s", output)
		}
		if strings.Contains(output, "PORT") {
			t.Errorf("single-key lookup should output raw value, not key name:\n%s", output)
		}
	})

	t.Run("config get JSON output is valid JSON", func(t *testing.T) {
		_, fetcher := newTestFetcher(t)

		globals := &cli.Globals{
			Output: "json",
		}
		var out bytes.Buffer
		if err := cli.RunConfigGet(context.Background(), globals, "", fixtureProjectName, fixtureEnvironment, "", false, "", true, fetcher, &out); err != nil {
			t.Fatalf("RunConfigGet() error: %v", err)
		}

		var parsed map[string]any
		if err := json.Unmarshal(out.Bytes(), &parsed); err != nil {
			t.Fatalf("output is not valid JSON: %v\n%s", err, out.String())
		}
		if _, ok := parsed["shared"]; !ok {
			t.Errorf("expected 'shared' key in JSON, got keys: %v", keys(parsed))
		}
	})

	t.Run("config get TOML output", func(t *testing.T) {
		_, fetcher := newTestFetcher(t)

		globals := &cli.Globals{
			Output: "toml",
		}
		var out bytes.Buffer
		if err := cli.RunConfigGet(context.Background(), globals, "", fixtureProjectName, fixtureEnvironment, "", false, "", true, fetcher, &out); err != nil {
			t.Fatalf("RunConfigGet() error: %v", err)
		}
		output := out.String()
		if !strings.Contains(output, "[shared.variables]") {
			t.Errorf("expected TOML shared section, got:\n%s", output)
		}
		if !strings.Contains(output, `GLOBAL = "one"`) {
			t.Errorf("expected TOML quoted value, got:\n%s", output)
		}
	})

	t.Run("config get --full includes IDs", func(t *testing.T) {
		_, fetcher := newTestFetcher(t)

		globals := &cli.Globals{
			Output: "json",
		}
		var out bytes.Buffer
		if err := cli.RunConfigGet(context.Background(), globals, "", fixtureProjectName, fixtureEnvironment, "", true, "", true, fetcher, &out); err != nil {
			t.Fatalf("RunConfigGet() error: %v", err)
		}

		var parsed map[string]any
		if err := json.Unmarshal(out.Bytes(), &parsed); err != nil {
			t.Fatalf("output is not valid JSON: %v", err)
		}
		for _, want := range []string{"project_id", "environment_id"} {
			if _, ok := parsed[want]; !ok {
				t.Errorf("expected %q in --full JSON output, got keys: %v", want, keys(parsed))
			}
		}
	})

	// -----------------------------------------------------------------------
	// Tests — config apply
	// -----------------------------------------------------------------------

	t.Run("config apply with variables and deploy settings", func(t *testing.T) {
		mock, fetcher := newTestFetcher(t)
		applier := newTestApplier(mock)

		dir := t.TempDir()
		writeConfigTOML(t, dir, `
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

		globals := &cli.Globals{}
		var out bytes.Buffer
		if err := cli.RunConfigApply(context.Background(), globals, "", "", "", dir, nil, "", cli.ApplyOpts{Yes: true}, fetcher, applier, &out); err != nil {
			t.Fatalf("RunConfigApply() error: %v", err)
		}
		// Expect 4 applied operations:
		//   1. ServiceInstanceUpdate  (deploy settings: builder + start_command)
		//   2. ServiceInstanceLimitsUpdate (resources: vcpus + memory_gb)
		//   3. VariableUpsert (PORT 8080->9090)
		//   4. VariableUpsert (NEW created)
		if !strings.Contains(out.String(), "applied=4") {
			t.Errorf("expected applied=4 summary, got:\n%s", out.String())
		}

		snap := mock.snapshot()

		// Variable collection upserts (batched).
		if got := len(snap.CollectionUpserts); got != 1 {
			t.Fatalf("collection upserts: got %d, want 1", got)
		}
		cu := snap.CollectionUpserts[0]
		if cu.ProjectID != fixtureProjectID {
			t.Errorf("upsert projectId = %q, want %q", cu.ProjectID, fixtureProjectID)
		}
		if cu.EnvironmentID != fixtureEnvironmentID {
			t.Errorf("upsert environmentId = %q, want %q", cu.EnvironmentID, fixtureEnvironmentID)
		}
		if cu.ServiceID == nil || *cu.ServiceID != fixtureServiceAPIID {
			t.Errorf("upsert serviceId = %v, want %q", cu.ServiceID, fixtureServiceAPIID)
		}
		if len(cu.Variables) != 2 {
			t.Errorf("upsert variables count = %d, want 2", len(cu.Variables))
		}
		if cu.Variables["PORT"] != "9090" {
			t.Errorf("PORT = %q, want %q", cu.Variables["PORT"], "9090")
		}
		if cu.Variables["NEW"] != "hello" {
			t.Errorf("NEW = %q, want %q", cu.Variables["NEW"], "hello")
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

	t.Run("config apply dry-run sends no mutations", func(t *testing.T) {
		mock, fetcher := newTestFetcher(t)
		applier := newTestApplier(mock)

		dir := t.TempDir()
		writeConfigTOML(t, dir, `
			project = "`+fixtureProjectName+`"
			environment = "`+fixtureEnvironment+`"

			[api.variables]
			PORT = "9999"
		`)

		globals := &cli.Globals{}
		var out bytes.Buffer
		if err := cli.RunConfigApply(context.Background(), globals, "", "", "", dir, nil, "", cli.ApplyOpts{DryRun: true}, fetcher, applier, &out); err != nil {
			t.Fatalf("RunConfigApply() error: %v", err)
		}
		if !strings.Contains(out.String(), "dry run") {
			t.Errorf("expected dry-run message, got:\n%s", out.String())
		}

		snap := mock.snapshot()
		total := len(snap.Upserts) + len(snap.CollectionUpserts) + len(snap.Deletes) + len(snap.Settings) + len(snap.Limits)
		if total != 0 {
			t.Errorf("dry-run should send 0 mutations, got %d", total)
		}
	})

	t.Run("config apply reports no changes when config matches live", func(t *testing.T) {
		mock, fetcher := newTestFetcher(t)
		applier := newTestApplier(mock)

		dir := t.TempDir()
		// Config matches exactly what the mock returns.
		writeConfigTOML(t, dir, `
			project = "`+fixtureProjectName+`"
			environment = "`+fixtureEnvironment+`"

			[shared.variables]
			GLOBAL = "one"

			[api.variables]
			PORT = "8080"

			[worker.variables]
			QUEUE = "default"
		`)

		globals := &cli.Globals{}
		var out bytes.Buffer
		if err := cli.RunConfigApply(context.Background(), globals, "", "", "", dir, nil, "", cli.ApplyOpts{Yes: true}, fetcher, applier, &out); err != nil {
			t.Fatalf("RunConfigApply() error: %v", err)
		}
		if !strings.Contains(out.String(), "No changes") {
			t.Errorf("expected 'No changes' message, got:\n%s", out.String())
		}

		snap := mock.snapshot()
		total := len(snap.Upserts) + len(snap.CollectionUpserts) + len(snap.Deletes) + len(snap.Settings) + len(snap.Limits)
		if total != 0 {
			t.Errorf("expected zero mutations for no-change apply, got %d", total)
		}
	})

	t.Run("config apply with --service filter scopes to one service", func(t *testing.T) {
		mock, fetcher := newTestFetcher(t)
		applier := newTestApplier(mock)

		dir := t.TempDir()
		// Both services have changes, but --service=api should only apply api.
		writeConfigTOML(t, dir, `
			project = "`+fixtureProjectName+`"
			environment = "`+fixtureEnvironment+`"

			[api.variables]
			PORT = "9090"

			[worker.variables]
			QUEUE = "high"
		`)

		globals := &cli.Globals{}
		var out bytes.Buffer
		if err := cli.RunConfigApply(context.Background(), globals, "", "", "", dir, nil, "api", cli.ApplyOpts{Yes: true}, fetcher, applier, &out); err != nil {
			t.Fatalf("RunConfigApply() error: %v", err)
		}

		snap := mock.snapshot()
		if len(snap.CollectionUpserts) != 1 {
			t.Fatalf("expected 1 collection upsert (api only), got %d", len(snap.CollectionUpserts))
		}
		cu := snap.CollectionUpserts[0]
		if cu.Variables["PORT"] != "9090" {
			t.Errorf("upsert PORT = %q, want %q", cu.Variables["PORT"], "9090")
		}
		if cu.ServiceID == nil || *cu.ServiceID != fixtureServiceAPIID {
			t.Errorf("upsert serviceId = %v, want %q", cu.ServiceID, fixtureServiceAPIID)
		}
	})

	t.Run("config apply deletes variable with empty value", func(t *testing.T) {
		mock, fetcher := newTestFetcher(t)
		applier := newTestApplier(mock)

		dir := t.TempDir()
		// Empty string means "delete this variable".
		writeConfigTOML(t, dir, `
			project = "`+fixtureProjectName+`"
			environment = "`+fixtureEnvironment+`"

			[api.variables]
			PORT = ""
		`)

		globals := &cli.Globals{}
		var out bytes.Buffer
		if err := cli.RunConfigApply(context.Background(), globals, "", "", "", dir, nil, "", cli.ApplyOpts{Yes: true}, fetcher, applier, &out); err != nil {
			t.Fatalf("RunConfigApply() error: %v", err)
		}

		snap := mock.snapshot()
		if len(snap.CollectionUpserts) != 0 {
			t.Errorf("expected 0 collection upserts for delete, got %d", len(snap.CollectionUpserts))
		}
		if len(snap.Deletes) != 1 {
			t.Fatalf("expected 1 delete, got %d", len(snap.Deletes))
		}
		if snap.Deletes[0].Name != "PORT" {
			t.Errorf("delete name = %q, want PORT", snap.Deletes[0].Name)
		}
		if snap.Deletes[0].ServiceID == nil || *snap.Deletes[0].ServiceID != fixtureServiceAPIID {
			t.Errorf("delete serviceId = %v, want %q", snap.Deletes[0].ServiceID, fixtureServiceAPIID)
		}
	})

	t.Run("config apply --fail-fast stops on first error", func(t *testing.T) {
		// The mock fails all collection upserts.
		mock, fetcher := newTestFetcher(t, withFailCollectionUpsert())
		applier := newTestApplier(mock)

		dir := t.TempDir()
		// Two new shared variables -> one batch upsert. Batch will fail.
		writeConfigTOML(t, dir, `
			project = "`+fixtureProjectName+`"
			environment = "`+fixtureEnvironment+`"

			[shared.variables]
			GLOBAL = "one"
			ALPHA = "new-a"
			BRAVO = "new-b"
		`)

		globals := &cli.Globals{}
		var out bytes.Buffer
		err := cli.RunConfigApply(context.Background(), globals, "", "", "", dir, nil, "", cli.ApplyOpts{Yes: true, FailFast: true}, fetcher, applier, &out)
		if err == nil {
			t.Fatal("expected error from fail-fast on collection upsert failure")
		}

		snap := mock.snapshot()
		// With fail-fast + batching, 1 collection upsert attempted (fails).
		if len(snap.CollectionUpserts) != 1 {
			t.Errorf("expected 1 collection upsert attempted, got %d", len(snap.CollectionUpserts))
		}
	})

	t.Run("config apply --skip-deploys passes flag through", func(t *testing.T) {
		mock, fetcher := newTestFetcher(t)
		applier := newTestApplier(mock)

		dir := t.TempDir()
		writeConfigTOML(t, dir, `
			project = "`+fixtureProjectName+`"
			environment = "`+fixtureEnvironment+`"

			[api.variables]
			PORT = "9090"
		`)

		globals := &cli.Globals{}
		var out bytes.Buffer
		if err := cli.RunConfigApply(context.Background(), globals, "", "", "", dir, nil, "", cli.ApplyOpts{Yes: true, SkipDeploys: true}, fetcher, applier, &out); err != nil {
			t.Fatalf("RunConfigApply() error: %v", err)
		}

		snap := mock.snapshot()
		if len(snap.CollectionUpserts) != 1 {
			t.Fatalf("expected 1 collection upsert, got %d", len(snap.CollectionUpserts))
		}
		if snap.CollectionUpserts[0].SkipDeploys == nil || !*snap.CollectionUpserts[0].SkipDeploys {
			t.Errorf("skipDeploys = %v, want true", snap.CollectionUpserts[0].SkipDeploys)
		}
	})

	// -----------------------------------------------------------------------
	// Tests — error handling
	// -----------------------------------------------------------------------

	t.Run("resolve auth fails without credentials", func(t *testing.T) {
		// Cannot use t.Parallel: t.Setenv modifies process env.
		keyring.MockInit()
		t.Setenv("RAILWAY_TOKEN", "")
		t.Setenv("RAILWAY_API_TOKEN", "")

		store := auth.NewTokenStore(auth.WithFallbackPath(filepath.Join(t.TempDir(), "auth.json")))
		_, err := auth.ResolveAuth(context.Background(), "", store)
		if err == nil {
			t.Fatal("expected error when no auth is configured")
		}
		if !errors.Is(err, auth.ErrNotAuthenticated) {
			t.Errorf("error = %v, want %v", err, auth.ErrNotAuthenticated)
		}
	})

	t.Run("GraphQL error propagates to caller", func(t *testing.T) {
		_, fetcher := newTestFetcher(t, withFailAllQueries())

		globals := &cli.Globals{
			Output: "text",
		}
		var out bytes.Buffer
		err := cli.RunConfigGet(context.Background(), globals, "", fixtureProjectName, fixtureEnvironment, "", false, "", false, fetcher, &out)
		if err == nil {
			t.Fatal("expected error when GraphQL returns errors")
		}
	})

	t.Run("context cancellation stops apply", func(t *testing.T) {
		mock, fetcher := newTestFetcher(t)
		applier := newTestApplier(mock)

		dir := t.TempDir()
		writeConfigTOML(t, dir, `
			project = "`+fixtureProjectName+`"
			environment = "`+fixtureEnvironment+`"

			[api.variables]
			PORT = "9090"
		`)

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // cancel immediately

		globals := &cli.Globals{}
		var out bytes.Buffer
		err := cli.RunConfigApply(ctx, globals, "", "", "", dir, nil, "", cli.ApplyOpts{Yes: true}, fetcher, applier, &out)
		if err == nil {
			t.Fatal("expected error from cancelled context")
		}
	})
}

// keys returns the top-level keys of a map for diagnostic messages.
func keys(m map[string]any) []string {
	result := make([]string, 0, len(m))
	for k := range m {
		result = append(result, k)
	}
	return result
}
