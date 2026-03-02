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

func testAuth() *auth.ResolvedAuth {
	return &auth.ResolvedAuth{
		Token:       "test-token",
		HeaderName:  "Authorization",
		HeaderValue: "Bearer test-token",
		Source:      auth.SourceFlag,
	}
}

func TestVariableUpsert_SendsInput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"variableUpsert": true}})
	}))
	defer server.Close()

	client := railway.NewClient(server.URL, testAuth(), nil, nil)
	err := railway.UpsertVariable(context.Background(), client, "proj", "env", "svc", "PORT", "8080", true)
	if err != nil {
		t.Fatalf("UpsertVariable() error: %v", err)
	}
}

func TestDeleteVariable_SendsInput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"variableDelete": true}})
	}))
	defer server.Close()

	client := railway.NewClient(server.URL, testAuth(), nil, nil)
	err := railway.DeleteVariable(context.Background(), client, "proj", "env", "svc", "OLD_VAR")
	if err != nil {
		t.Fatalf("DeleteVariable() error: %v", err)
	}
}

func TestDeleteVariable_SharedScope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"variableDelete": true}})
	}))
	defer server.Close()

	client := railway.NewClient(server.URL, testAuth(), nil, nil)
	// Empty serviceID means shared scope.
	err := railway.DeleteVariable(context.Background(), client, "proj", "env", "", "SHARED_VAR")
	if err != nil {
		t.Fatalf("DeleteVariable() error: %v", err)
	}
}

func TestUpdateServiceLimits_Succeeds(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"serviceInstanceLimitsUpdate": true}})
	}))
	defer server.Close()

	client := railway.NewClient(server.URL, testAuth(), nil, nil)
	err := railway.UpdateServiceLimits(context.Background(), client, "env", "svc", 0.5, 1.0)
	if err != nil {
		t.Fatalf("UpdateServiceLimits() error: %v", err)
	}
}
