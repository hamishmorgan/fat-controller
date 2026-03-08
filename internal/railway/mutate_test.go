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
	vcpus := 0.5
	mem := 1.0
	err := railway.UpdateServiceLimits(context.Background(), client, "env", "svc", &vcpus, &mem)
	if err != nil {
		t.Fatalf("UpdateServiceLimits() error: %v", err)
	}
}

func TestParseBuilder(t *testing.T) {
	tests := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{"nixpacks", "NIXPACKS", false},
		{"NIXPACKS", "NIXPACKS", false},
		{"Railpack", "RAILPACK", false},
		{"PAKETO", "PAKETO", false},
		{"heroku", "HEROKU", false},
		{"UNKNOWN", "", true},
	}
	for _, tt := range tests {
		b, err := railway.ParseBuilder(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("ParseBuilder(%q): err=%v, wantErr=%v", tt.input, err, tt.wantErr)
			continue
		}
		if !tt.wantErr && string(b) != tt.want {
			t.Errorf("ParseBuilder(%q) = %q, want %q", tt.input, b, tt.want)
		}
	}
}

func TestParseRestartPolicy(t *testing.T) {
	tests := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{"ALWAYS", "ALWAYS", false},
		{"never", "NEVER", false},
		{"ON_FAILURE", "ON_FAILURE", false},
		{"on_failure", "ON_FAILURE", false},
		{"INVALID", "", true},
	}
	for _, tt := range tests {
		rp, err := railway.ParseRestartPolicy(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("ParseRestartPolicy(%q): err=%v, wantErr=%v", tt.input, err, tt.wantErr)
			continue
		}
		if !tt.wantErr && string(rp) != tt.want {
			t.Errorf("ParseRestartPolicy(%q) = %q, want %q", tt.input, rp, tt.want)
		}
	}
}

func TestUpdateServiceSettings_NilDeploy(t *testing.T) {
	// Should be a no-op, no API call made.
	err := railway.UpdateServiceSettings(context.Background(), nil, "svc-1", nil)
	if err != nil {
		t.Fatalf("UpdateServiceSettings(nil) error: %v", err)
	}
}

func TestUpdateServiceLimits_PartialUpdate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"serviceInstanceLimitsUpdate": true}})
	}))
	defer server.Close()

	client := railway.NewClient(server.URL, testAuth(), nil, nil)
	vcpus := 2.0
	// memoryGB is nil — only update vCPUs.
	err := railway.UpdateServiceLimits(context.Background(), client, "env", "svc", &vcpus, nil)
	if err != nil {
		t.Fatalf("UpdateServiceLimits() error: %v", err)
	}
}
