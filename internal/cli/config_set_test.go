package cli_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/cli"
)

func TestRunConfigSet(t *testing.T) {
	tests := []struct {
		name        string
		globals     *cli.Globals
		path        string
		value       string
		mutatorErr  error  // injected error for the mutator
		wantErr     string // substring of expected error; empty means no error
		wantDryRun  bool   // expect "dry run" in output
		wantCalled  bool   // expect the mutator to be invoked
		wantService string // expected service arg
		wantKey     string // expected key arg
		wantValue   string // expected value arg
	}{
		{
			name:       "dry-run by default",
			globals:    &cli.Globals{},
			path:       "api.variables.PORT",
			value:      "8080",
			wantDryRun: true,
		},
		{
			name:       "dry-run flag overrides confirm",
			globals:    &cli.Globals{Confirm: true, DryRun: true},
			path:       "api.variables.PORT",
			value:      "8080",
			wantDryRun: true,
		},
		{
			name:        "executes with confirm",
			globals:     &cli.Globals{Confirm: true},
			path:        "api.variables.PORT",
			value:       "8080",
			wantCalled:  true,
			wantService: "api",
			wantKey:     "PORT",
			wantValue:   "8080",
		},
		{
			name:    "rejects non-variable path",
			globals: &cli.Globals{Confirm: true},
			path:    "api.resources.vcpus",
			value:   "1",
			wantErr: "variables",
		},
		{
			name:    "rejects path without key",
			globals: &cli.Globals{Confirm: true},
			path:    "api.variables",
			value:   "1",
			wantErr: "variables",
		},
		{
			name:       "propagates setter error",
			globals:    &cli.Globals{Confirm: true},
			path:       "api.variables.PORT",
			value:      "8080",
			mutatorErr: errors.New("mutation failed"),
			wantErr:    "mutation failed",
			wantCalled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &recordingMutator{err: tt.mutatorErr}
			var buf bytes.Buffer

			err := cli.RunConfigSet(context.Background(), tt.globals, tt.path, tt.value, m, &buf)

			// Check error expectation.
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got: %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Check dry-run output.
			if tt.wantDryRun {
				if !strings.Contains(buf.String(), "dry run") {
					t.Errorf("expected dry run message, got: %s", buf.String())
				}
				if m.called {
					t.Error("mutator should not be called in dry-run mode")
				}
				return
			}

			// Check mutation was invoked with correct args.
			if tt.wantCalled && !m.called {
				t.Fatal("expected mutator to be called")
			}
			if !tt.wantCalled && m.called {
				t.Fatal("mutator should not have been called")
			}
			if m.service != tt.wantService {
				t.Errorf("service: got %q, want %q", m.service, tt.wantService)
			}
			if m.key != tt.wantKey {
				t.Errorf("key: got %q, want %q", m.key, tt.wantKey)
			}
			if m.value != tt.wantValue {
				t.Errorf("value: got %q, want %q", m.value, tt.wantValue)
			}
		})
	}
}
