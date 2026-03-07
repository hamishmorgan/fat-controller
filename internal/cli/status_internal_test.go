package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/hamishmorgan/fat-controller/internal/railway"
)

func TestRunStatus_JSON(t *testing.T) {
	ctx := context.Background()
	globals := &Globals{Output: "json"}
	targets := []serviceTarget{{Name: "api", ID: "svc-1"}, {Name: "worker", ID: "svc-2"}}

	listLatest := func(ctx context.Context, environmentID, serviceID string) ([]railway.DeploymentInfo, error) {
		_ = ctx
		_ = environmentID
		if serviceID == "svc-2" {
			return nil, errors.New("list failed")
		}
		return []railway.DeploymentInfo{{ID: "dep-1", Status: railway.DeploymentStatusSuccess, CreatedAt: time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)}}, nil
	}

	var buf bytes.Buffer
	if err := RunStatus(ctx, globals, "env-1", targets, nil, listLatest, &buf, &buf); err != nil {
		t.Fatalf("RunStatus() error: %v", err)
	}

	var payload StatusOutput
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, buf.String())
	}
	if payload.EnvironmentID != "env-1" {
		t.Fatalf("environment_id = %q, want %q", payload.EnvironmentID, "env-1")
	}
	if len(payload.Statuses) != 2 {
		t.Fatalf("statuses len = %d, want 2", len(payload.Statuses))
	}
}

func TestRunStatus_TOML(t *testing.T) {
	ctx := context.Background()
	globals := &Globals{Output: "toml"}
	targets := []serviceTarget{{Name: "api", ID: "svc-1"}}

	listLatest := func(ctx context.Context, environmentID, serviceID string) ([]railway.DeploymentInfo, error) {
		_ = ctx
		_ = environmentID
		_ = serviceID
		return []railway.DeploymentInfo{}, nil
	}

	var buf bytes.Buffer
	if err := RunStatus(ctx, globals, "env-1", targets, nil, listLatest, &buf, &buf); err != nil {
		t.Fatalf("RunStatus() error: %v", err)
	}

	var payload StatusOutput
	if err := toml.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatalf("output is not valid TOML: %v\n%s", err, buf.String())
	}
	if payload.EnvironmentID != "env-1" {
		t.Fatalf("environment_id = %q, want %q", payload.EnvironmentID, "env-1")
	}
}
