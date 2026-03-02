package cli_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/cli"
)

type fakeDeleter struct {
	called  bool
	service string
	key     string
}

func (f *fakeDeleter) DeleteVar(_ context.Context, service, key string) error {
	f.called = true
	f.service = service
	f.key = key
	return nil
}

func TestConfigDelete_DryRunByDefault(t *testing.T) {
	deleter := &fakeDeleter{}
	err := cli.RunConfigDelete(context.Background(), &cli.Globals{}, "api.variables.OLD", deleter)
	if err == nil {
		t.Fatal("expected dry run error")
	}
	if !strings.Contains(err.Error(), "dry run") {
		t.Errorf("expected dry run message, got: %v", err)
	}
	if deleter.called {
		t.Fatal("deleter should not be called in dry-run mode")
	}
}

func TestConfigDelete_DryRunFlagOverridesConfirm(t *testing.T) {
	deleter := &fakeDeleter{}
	globals := &cli.Globals{Confirm: true, DryRun: true}
	err := cli.RunConfigDelete(context.Background(), globals, "api.variables.OLD", deleter)
	if err == nil {
		t.Fatal("expected dry run error when --dry-run is set")
	}
	if deleter.called {
		t.Fatal("deleter should not be called when --dry-run overrides --confirm")
	}
}

func TestConfigDelete_ExecutesWithConfirm(t *testing.T) {
	deleter := &fakeDeleter{}
	globals := &cli.Globals{Confirm: true}
	err := cli.RunConfigDelete(context.Background(), globals, "api.variables.OLD", deleter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !deleter.called {
		t.Fatal("expected deleter to be called with --confirm")
	}
	if deleter.service != "api" {
		t.Errorf("expected service=api, got %q", deleter.service)
	}
	if deleter.key != "OLD" {
		t.Errorf("expected key=OLD, got %q", deleter.key)
	}
}

func TestConfigDelete_RejectsNonVariablePath(t *testing.T) {
	deleter := &fakeDeleter{}
	err := cli.RunConfigDelete(context.Background(), &cli.Globals{Confirm: true}, "api.resources.vcpus", deleter)
	if err == nil {
		t.Fatal("expected error for non-variable path")
	}
	if !strings.Contains(err.Error(), "variables") {
		t.Errorf("expected error about variables, got: %v", err)
	}
	if deleter.called {
		t.Fatal("deleter should not be called for non-variable path")
	}
}

func TestConfigDelete_RejectsPathWithoutKey(t *testing.T) {
	deleter := &fakeDeleter{}
	err := cli.RunConfigDelete(context.Background(), &cli.Globals{Confirm: true}, "api.variables", deleter)
	if err == nil {
		t.Fatal("expected error for path without key")
	}
}

type failingDeleter struct{}

func (f *failingDeleter) DeleteVar(_ context.Context, _, _ string) error {
	return errors.New("delete failed")
}

func TestConfigDelete_PropagatesDeleterError(t *testing.T) {
	deleter := &failingDeleter{}
	globals := &cli.Globals{Confirm: true}
	err := cli.RunConfigDelete(context.Background(), globals, "api.variables.OLD", deleter)
	if err == nil {
		t.Fatal("expected error from deleter")
	}
	if !strings.Contains(err.Error(), "delete failed") {
		t.Errorf("unexpected error: %v", err)
	}
}
