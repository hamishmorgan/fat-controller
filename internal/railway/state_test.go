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

func TestFetchLiveConfig_IncludesSharedAndServiceVars(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var body struct{ Query string }
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch {
		case strings.Contains(body.Query, "project(id"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"project": map[string]any{
						"services": map[string]any{
							"edges": []map[string]any{{
								"node": map[string]any{"id": "svc-1", "name": "api"},
							}},
						},
					},
				},
			})
		case strings.Contains(body.Query, "variables("):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"variables": map[string]any{"FOO": "bar"},
				},
			})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{}})
		}
	}))
	defer server.Close()

	client := railway.NewClient(server.URL, &auth.ResolvedAuth{
		Token:       "test-token",
		HeaderName:  "Authorization",
		HeaderValue: "Bearer test-token",
		Source:      auth.SourceFlag,
	}, nil, nil)
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
}
