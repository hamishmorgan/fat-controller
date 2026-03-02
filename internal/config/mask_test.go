package config_test

import (
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/config"
)

func TestMaskValue_MasksPasswordByName(t *testing.T) {
	m := config.NewMasker(nil, nil)
	got := m.MaskValue("DATABASE_PASSWORD", "hunter2")
	if got != "********" {
		t.Errorf("expected masked, got %q", got)
	}
}

func TestMaskValue_MasksTokenByName(t *testing.T) {
	m := config.NewMasker(nil, nil)
	got := m.MaskValue("AUTH_TOKEN", "abc123")
	if got != "********" {
		t.Errorf("expected masked, got %q", got)
	}
}

func TestMaskValue_MasksDSNByName(t *testing.T) {
	m := config.NewMasker(nil, nil)
	got := m.MaskValue("DSN", "postgres://user:pass@host/db")
	if got != "********" {
		t.Errorf("expected masked, got %q", got)
	}
}

func TestMaskValue_ShowsNonSensitiveName(t *testing.T) {
	m := config.NewMasker(nil, nil)
	got := m.MaskValue("APP_ENV", "production")
	if got != "production" {
		t.Errorf("expected unmasked, got %q", got)
	}
}

func TestMaskValue_BoundaryMatchNotMidWord(t *testing.T) {
	m := config.NewMasker(nil, nil)
	// KEYBOARD contains KEY mid-word — should NOT match.
	got := m.MaskValue("KEYBOARD_LAYOUT", "us")
	if got != "us" {
		t.Errorf("KEYBOARD should not trigger KEY match, got %q", got)
	}
}

func TestMaskValue_AllowlistSuppressesFalsePositive(t *testing.T) {
	m := config.NewMasker(nil, nil)
	// PRIMARY_KEY contains KEY at a boundary, but is in the allowlist.
	got := m.MaskValue("PRIMARY_KEY", "id")
	if got != "id" {
		t.Errorf("PRIMARY_KEY should be allowlisted, got %q", got)
	}
}

func TestMaskValue_RailwayReferenceShownAsIs(t *testing.T) {
	m := config.NewMasker(nil, nil)
	ref := "${{postgres.DATABASE_URL}}"
	got := m.MaskValue("DATABASE_URL", ref)
	if got != ref {
		t.Errorf("reference template should be shown, got %q", got)
	}
}

func TestMaskValue_CustomKeywordsReplaceDefaults(t *testing.T) {
	// Passing custom keywords replaces all defaults.
	// Passing nil allowlist keeps the default allowlist.
	m := config.NewMasker([]string{"CUSTOM_FIELD"}, nil)
	got := m.MaskValue("MY_CUSTOM_FIELD", "value")
	if got != "********" {
		t.Errorf("custom keyword should mask, got %q", got)
	}
	// Default keywords should no longer match.
	got = m.MaskValue("DATABASE_PASSWORD", "hunter2")
	if got != "hunter2" {
		t.Errorf("default keyword should not mask with custom keywords, got %q", got)
	}
}

func TestMaskValue_CustomAllowlist(t *testing.T) {
	m := config.NewMasker(nil, []string{"MY_SAFE_TOKEN"})
	got := m.MaskValue("MY_SAFE_TOKEN", "value")
	if got != "value" {
		t.Errorf("custom allowlist should suppress, got %q", got)
	}
}
