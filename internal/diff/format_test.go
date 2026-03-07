package diff_test

import (
	"strings"
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/diff"
)

func TestFormat_Empty(t *testing.T) {
	result := &diff.Result{
		Services: map[string]*diff.SectionDiff{},
	}
	got := diff.Format(result, false)
	if got != "No changes." {
		t.Errorf("expected 'No changes.', got %q", got)
	}
}

func TestFormat_CreateVariable(t *testing.T) {
	result := &diff.Result{
		Services: map[string]*diff.SectionDiff{
			"api": {
				Variables: []diff.Change{
					{Key: "NEW", Action: diff.ActionCreate, DesiredValue: "value"},
				},
			},
		},
	}
	got := diff.Format(result, false)
	if !strings.Contains(got, "api") {
		t.Errorf("expected service name in output:\n%s", got)
	}
	if !strings.Contains(got, "NEW") {
		t.Errorf("expected variable name in output:\n%s", got)
	}
	if !strings.Contains(got, "+") || !strings.Contains(got, "value") {
		t.Errorf("expected + and value in create output:\n%s", got)
	}
}

func TestFormat_UpdateVariable(t *testing.T) {
	result := &diff.Result{
		Services: map[string]*diff.SectionDiff{
			"api": {
				Variables: []diff.Change{
					{Key: "PORT", Action: diff.ActionUpdate, LiveValue: "8080", DesiredValue: "9090"},
				},
			},
		},
	}
	got := diff.Format(result, false)
	if !strings.Contains(got, "8080") || !strings.Contains(got, "9090") {
		t.Errorf("expected old and new values in output:\n%s", got)
	}
}

func TestFormat_DeleteVariable(t *testing.T) {
	result := &diff.Result{
		Services: map[string]*diff.SectionDiff{
			"api": {
				Variables: []diff.Change{
					{Key: "OLD", Action: diff.ActionDelete, LiveValue: "bye"},
				},
			},
		},
	}
	got := diff.Format(result, false)
	if !strings.Contains(got, "OLD") {
		t.Errorf("expected variable name in output:\n%s", got)
	}
	if !strings.Contains(got, "-") {
		t.Errorf("expected - in delete output:\n%s", got)
	}
}

func TestFormat_SharedVariables(t *testing.T) {
	result := &diff.Result{
		Shared: &diff.SectionDiff{
			Variables: []diff.Change{
				{Key: "GLOBAL", Action: diff.ActionCreate, DesiredValue: "yes"},
			},
		},
		Services: map[string]*diff.SectionDiff{},
	}
	got := diff.Format(result, false)
	if !strings.Contains(got, "shared") {
		t.Errorf("expected 'shared' in output:\n%s", got)
	}
}

func TestFormat_Settings(t *testing.T) {
	result := &diff.Result{
		Services: map[string]*diff.SectionDiff{
			"api": {
				Settings: []diff.Change{
					{Key: "builder", Action: diff.ActionUpdate, LiveValue: "NIXPACKS", DesiredValue: "RAILPACK"},
				},
			},
		},
	}
	got := diff.Format(result, false)
	if !strings.Contains(got, "builder") {
		t.Errorf("expected 'builder' in output:\n%s", got)
	}
}

func TestFormat_MasksSecrets(t *testing.T) {
	result := &diff.Result{
		Services: map[string]*diff.SectionDiff{
			"api": {
				Variables: []diff.Change{
					{Key: "DATABASE_PASSWORD", Action: diff.ActionUpdate, LiveValue: "hunter2", DesiredValue: "newpass"},
				},
			},
		},
	}
	// showSecrets=false: live value should be masked.
	got := diff.Format(result, false)
	if strings.Contains(got, "hunter2") {
		t.Errorf("live secret should be masked:\n%s", got)
	}
	// Desired value should be masked too.
	if strings.Contains(got, "newpass") {
		t.Errorf("desired secret should be masked:\n%s", got)
	}
}

func TestFormat_ShowSecretsRevealsLiveValues(t *testing.T) {
	result := &diff.Result{
		Services: map[string]*diff.SectionDiff{
			"api": {
				Variables: []diff.Change{
					{Key: "DATABASE_PASSWORD", Action: diff.ActionUpdate, LiveValue: "hunter2", DesiredValue: "newpass"},
				},
			},
		},
	}
	got := diff.Format(result, true)
	if !strings.Contains(got, "hunter2") {
		t.Errorf("--show-secrets should reveal live value:\n%s", got)
	}
	if !strings.Contains(got, "newpass") {
		t.Errorf("--show-secrets should reveal desired value:\n%s", got)
	}
}

func TestFormat_MultipleServices(t *testing.T) {
	result := &diff.Result{
		Services: map[string]*diff.SectionDiff{
			"api": {
				Variables: []diff.Change{
					{Key: "PORT", Action: diff.ActionCreate, DesiredValue: "8080"},
				},
			},
			"worker": {
				Variables: []diff.Change{
					{Key: "QUEUE", Action: diff.ActionCreate, DesiredValue: "default"},
				},
			},
		},
	}
	got := diff.Format(result, false)
	if !strings.Contains(got, "api") || !strings.Contains(got, "worker") {
		t.Errorf("expected both service names:\n%s", got)
	}
}

func TestFormat_SummaryLine(t *testing.T) {
	result := &diff.Result{
		Services: map[string]*diff.SectionDiff{
			"api": {
				Variables: []diff.Change{
					{Key: "A", Action: diff.ActionCreate, DesiredValue: "1"},
					{Key: "B", Action: diff.ActionUpdate, LiveValue: "x", DesiredValue: "y"},
					{Key: "C", Action: diff.ActionDelete, LiveValue: "z"},
				},
			},
		},
	}
	got := diff.Format(result, false)
	// Should contain a summary line with counts.
	if !strings.Contains(got, "1 create") {
		t.Errorf("expected create count in summary:\n%s", got)
	}
	if !strings.Contains(got, "1 update") {
		t.Errorf("expected update count in summary:\n%s", got)
	}
	if !strings.Contains(got, "1 delete") {
		t.Errorf("expected delete count in summary:\n%s", got)
	}
}
