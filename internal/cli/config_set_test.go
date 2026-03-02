package cli_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/cli"
)

type fakeSetter struct {
	called  bool
	service string
	key     string
	value   string
}

func (f *fakeSetter) SetVar(_ context.Context, service, key, value string) error {
	f.called = true
	f.service = service
	f.key = key
	f.value = value
	return nil
}

func TestConfigSet_DryRunByDefault(t *testing.T) {
	setter := &fakeSetter{}
	err := cli.RunConfigSet(context.Background(), &cli.Globals{}, "api.variables.PORT", "8080", setter)
	if err == nil {
		t.Fatal("expected dry run error")
	}
	if !strings.Contains(err.Error(), "dry run") {
		t.Errorf("expected dry run message, got: %v", err)
	}
	if setter.called {
		t.Fatal("setter should not be called in dry-run mode")
	}
}

func TestConfigSet_DryRunFlagOverridesConfirm(t *testing.T) {
	setter := &fakeSetter{}
	globals := &cli.Globals{Confirm: true, DryRun: true}
	err := cli.RunConfigSet(context.Background(), globals, "api.variables.PORT", "8080", setter)
	if err == nil {
		t.Fatal("expected dry run error when --dry-run is set")
	}
	if setter.called {
		t.Fatal("setter should not be called when --dry-run overrides --confirm")
	}
}

func TestConfigSet_ExecutesWithConfirm(t *testing.T) {
	setter := &fakeSetter{}
	globals := &cli.Globals{Confirm: true}
	err := cli.RunConfigSet(context.Background(), globals, "api.variables.PORT", "8080", setter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !setter.called {
		t.Fatal("expected setter to be called with --confirm")
	}
	if setter.service != "api" {
		t.Errorf("expected service=api, got %q", setter.service)
	}
	if setter.key != "PORT" {
		t.Errorf("expected key=PORT, got %q", setter.key)
	}
	if setter.value != "8080" {
		t.Errorf("expected value=8080, got %q", setter.value)
	}
}

func TestConfigSet_RejectsNonVariablePath(t *testing.T) {
	setter := &fakeSetter{}
	err := cli.RunConfigSet(context.Background(), &cli.Globals{Confirm: true}, "api.resources.vcpus", "1", setter)
	if err == nil {
		t.Fatal("expected error for non-variable path")
	}
	if !strings.Contains(err.Error(), "variables") {
		t.Errorf("expected error about variables, got: %v", err)
	}
	if setter.called {
		t.Fatal("setter should not be called for non-variable path")
	}
}

func TestConfigSet_RejectsPathWithoutKey(t *testing.T) {
	setter := &fakeSetter{}
	err := cli.RunConfigSet(context.Background(), &cli.Globals{Confirm: true}, "api.variables", "1", setter)
	if err == nil {
		t.Fatal("expected error for path without key")
	}
}

type failingSetter struct{}

func (f *failingSetter) SetVar(_ context.Context, _, _, _ string) error {
	return errors.New("mutation failed")
}

func TestConfigSet_PropagatesSetterError(t *testing.T) {
	setter := &failingSetter{}
	globals := &cli.Globals{Confirm: true}
	err := cli.RunConfigSet(context.Background(), globals, "api.variables.PORT", "8080", setter)
	if err == nil {
		t.Fatal("expected error from setter")
	}
	if !strings.Contains(err.Error(), "mutation failed") {
		t.Errorf("unexpected error: %v", err)
	}
}
