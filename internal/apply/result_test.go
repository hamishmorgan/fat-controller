package apply_test

import (
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/apply"
)

func TestResult_Summary(t *testing.T) {
	r := &apply.Result{
		Applied: 3,
		Failed:  1,
	}
	got := r.Summary()
	if got != "applied=3 failed=1" {
		t.Fatalf("Summary() = %q, want %q", got, "applied=3 failed=1")
	}
}

func TestResult_Summary_Zero(t *testing.T) {
	r := &apply.Result{}
	got := r.Summary()
	if got != "applied=0 failed=0" {
		t.Fatalf("Summary() = %q, want %q", got, "applied=0 failed=0")
	}
}

func TestResult_Summary_Nil(t *testing.T) {
	var r *apply.Result
	got := r.Summary()
	if got != "applied=0 failed=0" {
		t.Fatalf("Summary() = %q, want %q", got, "applied=0 failed=0")
	}
}

func TestResult_HasFailures(t *testing.T) {
	r := &apply.Result{Applied: 1, Failed: 0}
	if r.HasFailures() {
		t.Error("HasFailures() should be false")
	}
	r.Failed = 1
	if !r.HasFailures() {
		t.Error("HasFailures() should be true")
	}
}
