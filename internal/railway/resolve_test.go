package railway_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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
					"projectId":     "proj-1",
					"environmentId": "env-1",
				},
			},
		})
	}))
	defer server.Close()

	client := railway.NewClient(server.URL, &auth.ResolvedAuth{
		Token:       "project-token",
		HeaderName:  "Project-Access-Token",
		HeaderValue: "project-token",
		Source:      auth.SourceEnvToken,
	}, nil, nil)
	proj, env, err := railway.ResolveProjectEnvironment(context.Background(), client, "", "", "")
	if err != nil {
		t.Fatalf("ResolveProjectEnvironment() error: %v", err)
	}
	if proj != "proj-1" || env != "env-1" {
		t.Fatalf("got %s/%s", proj, env)
	}
}

func TestResolveProjectEnvironment_AutoSelectsSingleProject(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var body struct{ Query string }
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch {
		case strings.Contains(body.Query, "apiToken"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"apiToken": map[string]any{
						"workspaces": []map[string]any{{
							"id":   "ws-1",
							"name": "workspace",
						}},
					},
				},
			})
		case strings.Contains(body.Query, "projects("):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"projects": map[string]any{
						"edges": []map[string]any{{
							"node": map[string]any{"id": "proj-1", "name": "my-app"},
						}},
					},
				},
			})
		case strings.Contains(body.Query, "environments("):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"environments": map[string]any{
						"edges": []map[string]any{{
							"node": map[string]any{"id": "env-1", "name": "production"},
						}},
					},
				},
			})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{},
			})
		}
	}))
	defer server.Close()

	client := railway.NewClient(server.URL, &auth.ResolvedAuth{
		Source:      auth.SourceEnvAPIToken,
		HeaderName:  "Authorization",
		HeaderValue: "Bearer test",
	}, nil, nil)

	projID, envID, err := railway.ResolveProjectEnvironment(ctx, client, "", "", "")
	if err != nil {
		t.Fatalf("ResolveProjectEnvironment() error: %v", err)
	}
	if projID != "proj-1" {
		t.Errorf("projID = %q, want proj-1", projID)
	}
	if envID != "env-1" {
		t.Errorf("envID = %q, want env-1", envID)
	}
}

func TestResolveProjectEnvironment_ErrorsOnAmbiguousNonInteractive(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var body struct{ Query string }
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch {
		case strings.Contains(body.Query, "apiToken"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"apiToken": map[string]any{
						"workspaces": []map[string]any{{
							"id":   "ws-1",
							"name": "workspace",
						}},
					},
				},
			})
		case strings.Contains(body.Query, "projects("):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"projects": map[string]any{
						"edges": []map[string]any{
							{"node": map[string]any{"id": "proj-1", "name": "app-1"}},
							{"node": map[string]any{"id": "proj-2", "name": "app-2"}},
						},
					},
				},
			})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{},
			})
		}
	}))
	defer server.Close()

	client := railway.NewClient(server.URL, &auth.ResolvedAuth{
		Source:      auth.SourceEnvAPIToken,
		HeaderName:  "Authorization",
		HeaderValue: "Bearer test",
	}, nil, nil)

	_, _, err := railway.ResolveProjectEnvironment(ctx, client, "", "", "")
	if err == nil {
		t.Fatal("expected error for ambiguous project")
	}
	if !strings.Contains(err.Error(), "app-1") || !strings.Contains(err.Error(), "app-2") {
		t.Errorf("expected error to list projects, got: %v", err)
	}
}

func TestResolveProjectID_AutoSelectsSingleProject(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var body struct{ Query string }
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch {
		case strings.Contains(body.Query, "apiToken"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"apiToken": map[string]any{
						"workspaces": []map[string]any{{
							"id":   "ws-1",
							"name": "workspace",
						}},
					},
				},
			})
		case strings.Contains(body.Query, "projects("):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"projects": map[string]any{
						"edges": []map[string]any{{
							"node": map[string]any{"id": "proj-1", "name": "my-app"},
						}},
					},
				},
			})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{}})
		}
	}))
	defer server.Close()

	client := railway.NewClient(server.URL, &auth.ResolvedAuth{
		Source:      auth.SourceEnvAPIToken,
		HeaderName:  "Authorization",
		HeaderValue: "Bearer test",
	}, nil, nil)

	projID, err := railway.ResolveProjectID(ctx, client, "", "")
	if err != nil {
		t.Fatalf("ResolveProjectID() error: %v", err)
	}
	if projID != "proj-1" {
		t.Errorf("projID = %q, want proj-1", projID)
	}
}
