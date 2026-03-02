package prompt

import "testing"

func TestPickItem_AutoSelectsSingleOption(t *testing.T) {
	items := []Item{{Name: "production", ID: "env-1"}}
	got, err := pickItem("environment", items, false)
	if err != nil {
		t.Fatalf("pickItem() error: %v", err)
	}
	if got != "env-1" {
		t.Errorf("got %q, want %q", got, "env-1")
	}
}

func TestPickItem_ErrorsOnEmptyList(t *testing.T) {
	_, err := pickItem("project", nil, false)
	if err == nil {
		t.Fatal("expected error for empty list")
	}
}

func TestPickItem_NonInteractiveMultipleOptions(t *testing.T) {
	items := []Item{
		{Name: "staging", ID: "env-1"},
		{Name: "production", ID: "env-2"},
	}
	_, err := pickItem("environment", items, false)
	if err == nil {
		t.Fatal("expected error when multiple options and non-interactive")
	}
}
