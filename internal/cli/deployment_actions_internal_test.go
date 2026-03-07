package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/hamishmorgan/fat-controller/internal/railway"
)

func TestRunDeploymentAction_JSON(t *testing.T) {
	ctx := context.Background()
	globals := &Globals{Output: "json"}
	targets := []serviceTarget{{Name: "api", ID: "svc-1"}, {Name: "worker", ID: "svc-2"}}

	listLatest := func(ctx context.Context, environmentID, serviceID string) ([]railway.DeploymentInfo, error) {
		_ = ctx
		_ = environmentID
		if serviceID == "svc-2" {
			return nil, errors.New("list failed")
		}
		return []railway.DeploymentInfo{{ID: "dep-1"}}, nil
	}
	action := func(ctx context.Context, deploymentID string) (string, error) {
		_ = ctx
		if deploymentID != "dep-1" {
			return "", errors.New("unexpected deployment")
		}
		return "dep-new", nil
	}

	var buf bytes.Buffer
	if err := RunDeploymentAction(ctx, globals, "env-1", targets, "redeploy", listLatest, action, &buf, &buf); err != nil {
		t.Fatalf("RunDeploymentAction() error: %v", err)
	}

	var payload DeploymentActionOutput
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, buf.String())
	}
	if payload.Action != "redeploy" {
		t.Fatalf("action = %q, want %q", payload.Action, "redeploy")
	}
	if len(payload.Results) != 2 {
		t.Fatalf("results len = %d, want 2", len(payload.Results))
	}
}

func TestRunDeploymentAction_TOML(t *testing.T) {
	ctx := context.Background()
	globals := &Globals{Output: "toml"}
	targets := []serviceTarget{{Name: "api", ID: "svc-1"}}

	listLatest := func(ctx context.Context, environmentID, serviceID string) ([]railway.DeploymentInfo, error) {
		_ = ctx
		_ = environmentID
		_ = serviceID
		return []railway.DeploymentInfo{{ID: "dep-1"}}, nil
	}
	action := func(ctx context.Context, deploymentID string) (string, error) {
		_ = ctx
		_ = deploymentID
		return "", nil
	}

	var buf bytes.Buffer
	if err := RunDeploymentAction(ctx, globals, "env-1", targets, "restart", listLatest, action, &buf, &buf); err != nil {
		t.Fatalf("RunDeploymentAction() error: %v", err)
	}

	var payload DeploymentActionOutput
	if err := toml.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatalf("output is not valid TOML: %v\n%s", err, buf.String())
	}
	if payload.Action != "restart" {
		t.Fatalf("action = %q, want %q", payload.Action, "restart")
	}
}
