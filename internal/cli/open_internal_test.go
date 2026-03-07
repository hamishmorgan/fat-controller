package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/BurntSushi/toml"
)

func TestRunOpen_Print_JSON(t *testing.T) {
	globals := &Globals{Output: "json"}

	var buf bytes.Buffer
	if err := RunOpen(context.Background(), globals, "proj-1", "env-1", true, func(string) error {
		return errors.New("should not be called")
	}, &buf, &buf); err != nil {
		t.Fatalf("RunOpen() error: %v", err)
	}

	var payload OpenOutput
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, buf.String())
	}
	if payload.URL == "" {
		t.Fatalf("url is empty")
	}
	if payload.Opened {
		t.Fatalf("opened = true, want false")
	}
}

func TestRunOpen_Open_TOML(t *testing.T) {
	globals := &Globals{Output: "toml"}
	called := false

	var buf bytes.Buffer
	if err := RunOpen(context.Background(), globals, "proj-1", "env-1", false, func(string) error {
		called = true
		return nil
	}, &buf, &buf); err != nil {
		t.Fatalf("RunOpen() error: %v", err)
	}
	if !called {
		t.Fatalf("openFn was not called")
	}

	var payload OpenOutput
	if err := toml.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatalf("output is not valid TOML: %v\n%s", err, buf.String())
	}
	if !payload.Opened {
		t.Fatalf("opened = false, want true")
	}
}
