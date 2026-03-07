package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/BurntSushi/toml"
)

func TestRunDeploy_JSON(t *testing.T) {
	targets := []serviceTarget{{Name: "api", ID: "svc-1"}, {Name: "worker", ID: "svc-2"}}
	globals := &Globals{Output: "json"}

	deployFn := func(ctx context.Context, environmentID, serviceID string) (string, error) {
		_ = ctx
		_ = environmentID
		if serviceID == "svc-1" {
			return "dep-1", nil
		}
		return "", errors.New("boom")
	}

	var buf bytes.Buffer
	if err := RunDeploy(context.Background(), globals, "env-1", targets, deployFn, &buf, &buf); err != nil {
		t.Fatalf("RunDeploy() error: %v", err)
	}

	var payload DeployOutput
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, buf.String())
	}
	if payload.Action != "deploy" {
		t.Fatalf("action = %q, want %q", payload.Action, "deploy")
	}
	if payload.EnvironmentID != "env-1" {
		t.Fatalf("environment_id = %q, want %q", payload.EnvironmentID, "env-1")
	}
	if len(payload.Results) != 2 {
		t.Fatalf("results len = %d, want 2", len(payload.Results))
	}
}

func TestRunDeploy_TOML(t *testing.T) {
	targets := []serviceTarget{{Name: "api", ID: "svc-1"}}
	globals := &Globals{Output: "toml"}

	deployFn := func(ctx context.Context, environmentID, serviceID string) (string, error) {
		_ = ctx
		_ = environmentID
		_ = serviceID
		return "dep-1", nil
	}

	var buf bytes.Buffer
	if err := RunDeploy(context.Background(), globals, "env-1", targets, deployFn, &buf, &buf); err != nil {
		t.Fatalf("RunDeploy() error: %v", err)
	}

	var payload DeployOutput
	if err := toml.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatalf("output is not valid TOML: %v\n%s", err, buf.String())
	}
	if payload.EnvironmentID != "env-1" {
		t.Fatalf("environment_id = %q, want %q", payload.EnvironmentID, "env-1")
	}
}
