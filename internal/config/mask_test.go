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

func TestMaskValue_HighEntropyBase64Masked(t *testing.T) {
	m := config.NewMasker(nil, nil)
	// A random-looking base64 string (>20 chars, high entropy).
	got := m.MaskValue("SETTING_X", "xK9mZpQ7wL3nR8vY2jT6bA5cD4eF1gH")
	if got != "********" {
		t.Errorf("high entropy base64 should be masked, got %q", got)
	}
}

func TestMaskValue_HighEntropyHexMasked(t *testing.T) {
	m := config.NewMasker(nil, nil)
	// A random hex string (>20 chars, high entropy).
	got := m.MaskValue("BUILD_THING", "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6")
	if got != "********" {
		t.Errorf("high entropy hex should be masked, got %q", got)
	}
}

func TestMaskValue_ShortStringNotMasked(t *testing.T) {
	m := config.NewMasker(nil, nil)
	// Short string — below 20 char minimum for entropy check.
	got := m.MaskValue("BUILD_HASH", "abc123")
	if got != "abc123" {
		t.Errorf("short string should not trigger entropy mask, got %q", got)
	}
}

func TestMaskValue_LowEntropyLongStringNotMasked(t *testing.T) {
	m := config.NewMasker(nil, nil)
	// Long but low-entropy (repeated pattern).
	got := m.MaskValue("APP_DESCRIPTION", "aaaaaaaaaaaaaaaaaaaaaaaaa")
	if got != "aaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Errorf("low entropy string should not be masked, got %q", got)
	}
}

func TestMaskValue_URLNotMasked(t *testing.T) {
	m := config.NewMasker(nil, nil)
	// URLs contain :// and / which fail the base64/hex charset check.
	got := m.MaskValue("APP_URL", "https://my-app.railway.app/api/v1/health")
	if got != "https://my-app.railway.app/api/v1/health" {
		t.Errorf("URL should not trigger entropy mask, got %q", got)
	}
}

func TestMaskValue_SlugNotMasked(t *testing.T) {
	m := config.NewMasker(nil, nil)
	// Slug-like strings match base64 charset but have low entropy.
	got := m.MaskValue("APP_SLUG", "my-app-production-deploy-v2")
	if got != "my-app-production-deploy-v2" {
		t.Errorf("slug should not trigger entropy mask, got %q", got)
	}
}

func TestShannonEntropy_RandomBase64(t *testing.T) {
	// "xK9mZpQ7wL3nR8vY2jT6bA5cD4eF1gH" has high entropy.
	e := config.ShannonEntropy("xK9mZpQ7wL3nR8vY2jT6bA5cD4eF1gH")
	if e < 4.0 {
		t.Errorf("expected high entropy, got %.2f", e)
	}
}

func TestShannonEntropy_RepeatedChars(t *testing.T) {
	e := config.ShannonEntropy("aaaaaaaaaa")
	if e != 0 {
		t.Errorf("expected zero entropy, got %.2f", e)
	}
}

func TestShannonEntropy_EmptyString(t *testing.T) {
	e := config.ShannonEntropy("")
	if e != 0 {
		t.Errorf("expected zero entropy for empty string, got %.2f", e)
	}
}

func TestMaskValue_EmptyKeywordsDisablesNameMatching(t *testing.T) {
	// Passing an empty (non-nil) slice disables all keyword matching.
	m := config.NewMasker([]string{}, nil)
	got := m.MaskValue("DATABASE_PASSWORD", "hunter2")
	if got != "hunter2" {
		t.Errorf("empty keywords should disable name masking, got %q", got)
	}
}

func TestMaskValue_AllowlistSuppressesEntropyToo(t *testing.T) {
	// An allowlisted name should suppress both name-based AND entropy-based
	// masking, since the user explicitly marked it as non-secret.
	m := config.NewMasker(nil, []string{"BUILD_KEY"})
	got := m.MaskValue("BUILD_KEY", "xK9mZpQ7wL3nR8vY2jT6bA5cD4eF1gH") // gitleaks:allow test fixture
	if got == "********" {
		t.Errorf("allowlisted name should not be entropy-masked, got %q", got)
	}
}

func TestMaskValue_BoardingPassMasked(t *testing.T) {
	// BOARDING_PASS matches PASS at a segment boundary — intentionally
	// errs on the side of masking (see SECRET-MASKING.md).
	m := config.NewMasker(nil, nil)
	got := m.MaskValue("BOARDING_PASS", "ABC123")
	if got != "********" {
		t.Errorf("BOARDING_PASS should trigger PASS match, got %q", got)
	}
}

func TestIsSensitive_MatchesSensitiveNames(t *testing.T) {
	m := config.NewMasker(nil, nil)
	sensitive := []string{"DATABASE_URL", "API_KEY", "SESSION_SECRET", "AUTH_TOKEN", "STRIPE_API_KEY"}
	for _, name := range sensitive {
		if !m.IsSensitive(name) {
			t.Errorf("IsSensitive(%q) = false, want true", name)
		}
	}
}

func TestIsSensitive_IgnoresNonSensitiveNames(t *testing.T) {
	m := config.NewMasker(nil, nil)
	nonSensitive := []string{"PORT", "APP_NAME", "HOST", "LOG_LEVEL", "QUEUE"}
	for _, name := range nonSensitive {
		if m.IsSensitive(name) {
			t.Errorf("IsSensitive(%q) = true, want false", name)
		}
	}
}

func TestIsSensitive_RespectsAllowlist(t *testing.T) {
	m := config.NewMasker(nil, nil)
	// PRIMARY_KEY is on the allowlist — should not be sensitive.
	if m.IsSensitive("PRIMARY_KEY") {
		t.Error("IsSensitive(PRIMARY_KEY) = true, want false (allowlisted)")
	}
}
