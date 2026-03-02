package prompt_test

import (
	"strings"
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/prompt"
)

func TestPickProject_AutoSelectsSingleOption(t *testing.T) {
	items := []prompt.Item{{Name: "my-app", ID: "proj-1"}}
	got, err := prompt.PickProject(items, false)
	if err != nil {
		t.Fatalf("PickProject() error: %v", err)
	}
	if got != "proj-1" {
		t.Errorf("got %q, want %q", got, "proj-1")
	}
}

func TestPickEnvironment_AutoSelectsSingleOption(t *testing.T) {
	items := []prompt.Item{{Name: "production", ID: "env-1"}}
	got, err := prompt.PickEnvironment(items, false)
	if err != nil {
		t.Fatalf("PickEnvironment() error: %v", err)
	}
	if got != "env-1" {
		t.Errorf("got %q, want %q", got, "env-1")
	}
}

func TestPickProject_ErrorsOnEmptyList(t *testing.T) {
	_, err := prompt.PickProject(nil, false)
	if err == nil {
		t.Fatal("expected error for empty list")
	}
	if !strings.Contains(err.Error(), "no projects") {
		t.Errorf("expected 'no projects' message, got: %v", err)
	}
}

func TestPickEnvironment_NonInteractiveMultipleOptions(t *testing.T) {
	items := []prompt.Item{
		{Name: "staging", ID: "env-1"},
		{Name: "production", ID: "env-2"},
	}
	_, err := prompt.PickEnvironment(items, false)
	if err == nil {
		t.Fatal("expected error when multiple options and non-interactive")
	}
	// Error should list available options and suggest --environment flag.
	if !strings.Contains(err.Error(), "staging") {
		t.Errorf("expected error to list options, got: %v", err)
	}
	if !strings.Contains(err.Error(), "--environment") {
		t.Errorf("expected error to suggest flag, got: %v", err)
	}
}

func TestPickWorkspace_AutoSelectsSingleOption(t *testing.T) {
	items := []prompt.Item{{Name: "my-team", ID: "ws-1"}}
	got, err := prompt.PickWorkspace(items, false)
	if err != nil {
		t.Fatalf("PickWorkspace() error: %v", err)
	}
	if got != "ws-1" {
		t.Errorf("got %q, want %q", got, "ws-1")
	}
}
