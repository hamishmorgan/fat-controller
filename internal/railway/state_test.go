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
		var body struct {
			Query         string `json:"query"`
			OperationName string `json:"operationName"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch {
		case body.OperationName == "ProjectServices" || strings.Contains(body.Query, "project(id"):
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
		case body.OperationName == "Variables" || strings.Contains(body.Query, "variables("):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"variables": map[string]any{"FOO": "bar"},
				},
			})
		case body.OperationName == "EnvironmentBulk":
			startCmd := "npm start"
			healthPath := "/health"
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"environment": map[string]any{
						"serviceInstances": map[string]any{
							"edges": []any{
								map[string]any{"node": map[string]any{
									"serviceId":               "svc-1",
									"builder":                 "NIXPACKS",
									"buildCommand":            nil,
									"startCommand":            startCmd,
									"dockerfilePath":          nil,
									"rootDirectory":           nil,
									"healthcheckPath":         healthPath,
									"healthcheckTimeout":      nil,
									"cronSchedule":            nil,
									"numReplicas":             nil,
									"region":                  nil,
									"restartPolicyType":       "ON_FAILURE",
									"restartPolicyMaxRetries": 0,
									"drainingSeconds":         nil,
									"overlapSeconds":          nil,
									"sleepApplication":        nil,
									"ipv6EgressEnabled":       nil,
									"watchPatterns":           []any{},
									"preDeployCommand":        nil,
									"source":                  nil,
									"domains": map[string]any{
										"customDomains":  []any{},
										"serviceDomains": []any{},
									},
								}},
							},
						},
						"deploymentTriggers": map[string]any{
							"edges": []any{},
						},
						"volumeInstances": map[string]any{
							"edges": []any{},
						},
					},
					"privateNetworks": []any{},
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
	cfg, err := railway.FetchLiveConfig(context.Background(), client, "proj-1", "env-1", nil, nil)
	if err != nil {
		t.Fatalf("FetchLiveConfig() error: %v", err)
	}
	if cfg.Variables["FOO"] != "bar" {
		t.Fatalf("shared FOO = %q", cfg.Variables["FOO"])
	}
	if cfg.Services["api"].Variables["FOO"] != "bar" {
		t.Fatalf("service FOO = %q", cfg.Services["api"].Variables["FOO"])
	}

	deploy := cfg.Services["api"].Deploy
	if deploy.Builder != "NIXPACKS" {
		t.Fatalf("deploy.Builder = %q, want NIXPACKS", deploy.Builder)
	}
	if deploy.StartCommand == nil || *deploy.StartCommand != "npm start" {
		t.Fatalf("deploy.StartCommand = %v, want 'npm start'", deploy.StartCommand)
	}
	if deploy.HealthcheckPath == nil || *deploy.HealthcheckPath != "/health" {
		t.Fatalf("deploy.HealthcheckPath = %v, want '/health'", deploy.HealthcheckPath)
	}
	if deploy.DockerfilePath != nil {
		t.Fatalf("deploy.DockerfilePath = %v, want nil", deploy.DockerfilePath)
	}
}
