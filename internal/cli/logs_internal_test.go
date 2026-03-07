package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/hamishmorgan/fat-controller/internal/railway"
)

func TestRunLogsEnvironment_JSON(t *testing.T) {
	entries := []railway.LogEntry{{Timestamp: "2026-03-07T12:00:00Z", Severity: ptrString("info"), Message: "hello"}}
	lines := 10
	globals := &Globals{Output: "json"}

	var buf bytes.Buffer
	if err := RunLogsEnvironment(globals, "env-1", &lines, entries, &buf); err != nil {
		t.Fatalf("RunLogsEnvironment() error: %v", err)
	}

	var payload struct {
		EnvironmentID string `json:"environment_id"`
		Results       []struct {
			Scope string `json:"scope"`
			Build bool   `json:"build"`
			Lines *int   `json:"lines"`
		} `json:"results"`
	}
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, buf.String())
	}
	if payload.EnvironmentID != "env-1" {
		t.Fatalf("environment_id = %q, want %q", payload.EnvironmentID, "env-1")
	}
	if len(payload.Results) != 1 {
		t.Fatalf("results len = %d, want 1", len(payload.Results))
	}
	if payload.Results[0].Scope != "environment" {
		t.Fatalf("scope = %q, want %q", payload.Results[0].Scope, "environment")
	}
	if payload.Results[0].Build {
		t.Fatalf("build = true, want false")
	}
	if payload.Results[0].Lines == nil || *payload.Results[0].Lines != lines {
		t.Fatalf("lines = %v, want %d", payload.Results[0].Lines, lines)
	}
}

func TestRunLogsServices_TOML(t *testing.T) {
	ctx := context.Background()
	lines := 50
	globals := &Globals{Output: "toml"}

	targets := []serviceTarget{{Name: "api", ID: "svc-1"}}
	listLatest := func(ctx context.Context, environmentID, serviceID string) ([]railway.DeploymentInfo, error) {
		_ = ctx
		_ = environmentID
		_ = serviceID
		return []railway.DeploymentInfo{{ID: "dep-1"}}, nil
	}
	fetch := func(ctx context.Context, deploymentID string, build bool) ([]railway.LogEntry, error) {
		_ = ctx
		_ = deploymentID
		_ = build
		return []railway.LogEntry{{Timestamp: "2026-03-07T12:00:00Z", Message: "ok"}}, nil
	}

	var buf bytes.Buffer
	if err := RunLogsServices(ctx, globals, "env-1", targets, false, &lines, listLatest, fetch, &buf, &buf); err != nil {
		t.Fatalf("RunLogsServices() error: %v", err)
	}

	var payload LogsOutput
	if err := toml.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatalf("output is not valid TOML: %v\n%s", err, buf.String())
	}
	if payload.EnvironmentID != "env-1" {
		t.Fatalf("environment_id = %q, want %q", payload.EnvironmentID, "env-1")
	}
	if len(payload.Results) != 1 {
		t.Fatalf("results len = %d, want 1", len(payload.Results))
	}
	if payload.Results[0].Service != "api" {
		t.Fatalf("service = %q, want %q", payload.Results[0].Service, "api")
	}
	if payload.Results[0].Lines == nil || *payload.Results[0].Lines != lines {
		t.Fatalf("lines = %v, want %d", payload.Results[0].Lines, lines)
	}
}

func ptrString(s string) *string { return &s }
