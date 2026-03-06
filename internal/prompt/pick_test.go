package prompt_test

import (
	"strings"
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/prompt"
)

func TestPickProject_AutoSelectsSingleOption(t *testing.T) {
	items := []prompt.Item{{Name: "my-app", ID: "proj-1"}}
	got, err := prompt.PickProject(items, false, prompt.PickOpts{})
	if err != nil {
		t.Fatalf("PickProject() error: %v", err)
	}
	if got != "proj-1" {
		t.Errorf("got %q, want %q", got, "proj-1")
	}
}

func TestPickEnvironment_AutoSelectsSingleOption(t *testing.T) {
	items := []prompt.Item{{Name: "production", ID: "env-1"}}
	got, err := prompt.PickEnvironment(items, false, prompt.PickOpts{})
	if err != nil {
		t.Fatalf("PickEnvironment() error: %v", err)
	}
	if got != "env-1" {
		t.Errorf("got %q, want %q", got, "env-1")
	}
}

func TestPickProject_ErrorsOnEmptyList(t *testing.T) {
	_, err := prompt.PickProject(nil, false, prompt.PickOpts{})
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
	_, err := prompt.PickEnvironment(items, false, prompt.PickOpts{})
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
	got, err := prompt.PickWorkspace(items, false, prompt.PickOpts{})
	if err != nil {
		t.Fatalf("PickWorkspace() error: %v", err)
	}
	if got != "ws-1" {
		t.Errorf("got %q, want %q", got, "ws-1")
	}
}

func TestPickProject_ForcePromptNonInteractiveAutoSelects(t *testing.T) {
	// ForcePrompt in non-interactive mode still auto-selects single item
	// (can't show picker without a TTY).
	items := []prompt.Item{{Name: "my-app", ID: "proj-1"}}
	got, err := prompt.PickProject(items, false, prompt.PickOpts{ForcePrompt: true})
	if err != nil {
		t.Fatalf("PickProject() error: %v", err)
	}
	if got != "proj-1" {
		t.Errorf("got %q, want %q", got, "proj-1")
	}
}

func TestPickServices_NonInteractiveReturnsAll(t *testing.T) {
	names := []string{"worker", "api", "web"}
	got, err := prompt.PickServices(names, false)
	if err != nil {
		t.Fatalf("PickServices() error: %v", err)
	}
	// Should return sorted.
	want := []string{"api", "web", "worker"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i, name := range want {
		if got[i] != name {
			t.Errorf("got[%d] = %q, want %q", i, got[i], name)
		}
	}
}

func TestPickServices_EmptyReturnsNil(t *testing.T) {
	got, err := prompt.PickServices(nil, false)
	if err != nil {
		t.Fatalf("PickServices() error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}
