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
	proj, env, err := railway.ResolveProjectEnvironment(context.Background(), client, "", "")
	if err != nil {
		t.Fatalf("ResolveProjectEnvironment() error: %v", err)
	}
	if proj != "proj-1" || env != "env-1" {
		t.Fatalf("got %s/%s", proj, env)
	}
}
