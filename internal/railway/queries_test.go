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

	client := railway.NewClient(server.URL, &auth.ResolvedAuth{
		Token:       "test-token",
		HeaderName:  "Authorization",
		HeaderValue: "Bearer test-token",
		Source:      auth.SourceFlag,
	}, nil, nil)
	resp, err := railway.Projects(context.Background(), client.GQL())
	if err != nil {
		t.Fatalf("Projects() error: %v", err)
	}
	if len(resp.Projects.Edges) != 1 {
		t.Fatalf("edges = %d, want 1", len(resp.Projects.Edges))
	}
}
