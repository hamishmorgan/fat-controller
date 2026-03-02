# Secret Masking Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Automatically mask secret values in `config get` output using name-based keyword matching and Shannon entropy detection, with `--show-secrets` to override.

**Architecture:** Create `internal/config/mask.go` with a `Masker` struct that holds compiled regexes for sensitive keywords and allowlist patterns. The masker exposes a single `MaskValue(name, value string) string` method implementing the combined logic from `docs/SECRET-MASKING.md`. Wire it into `Render()` via a new `RenderOptions` struct that replaces the current `format string, full bool` parameters. All stdlib — no new dependencies.

**Tech Stack:** Go stdlib (`regexp`, `math`, `strings`, `unicode`)

---

## Context for the implementor

### How the codebase works today

- `internal/config/render.go` has `Render(cfg LiveConfig, format string, full bool) (string, error)` which outputs config in text/json/toml formats. Shared variables render under `[shared.variables]` (TOML/text) and `{"shared": {"variables": ...}}` (JSON). **No masking exists.** Values are written verbatim.
- `internal/cli/config_get.go:77` calls `config.Render(*cfg, globals.Output, globals.Full)` — this is the single integration point.
- `internal/cli/cli.go` has a `Globals` struct with `ShowSecrets bool` already defined but unused.
- `sensitive_keywords` and `sensitive_allowlist` are config-file-only settings (no CLI flag, no env var). Config file loading doesn't exist yet, so for now the masker uses hardcoded defaults. The API accepts custom lists so config loading can wire in later.
- All test files use **external test packages** (`package config_test`, `package cli_test`).
- Tests use **plain `testing.T`** — no testify.
- Run tests with `go test ./internal/config/... -v` or `mise run check` for full suite.
- **Known trade-off:** The keyword `PASS` with `(\b|_)` boundaries won't match mid-word (`COMPASS`), but will match segment boundaries like `BOARDING_PASS`. This is intentional — erring on the side of masking is safer than leaking secrets. Users can suppress false positives via the allowlist.

### The masking spec (`docs/SECRET-MASKING.md`)

1. If `--show-secrets` is set → show all values, no masking.
2. If value contains `${{` → show as-is (Railway reference template).
3. If name matches allowlist → show (false-positive suppression).
4. If name matches sensitive keyword → mask as `********`.
5. If value has high Shannon entropy (base64 > 4.5 or hex > 3.0, min 20 chars) → mask.
6. Otherwise → show.

Keyword matching uses `(\b|_)` boundary regex: `(?i)(\b|_)(PASSWORD|TOKEN|...)(\b|_)`.

### Files you'll touch

| File | Action |
|------|--------|
| `internal/config/mask.go` | **Create** — `Masker` struct, `MaskValue`, entropy, defaults |
| `internal/config/mask_test.go` | **Create** — comprehensive masking tests |
| `internal/config/render.go` | **Modify** — replace `format string, full bool` params with `RenderOptions`, apply masking |
| `internal/config/render_test.go` | **Modify** — update calls to use `RenderOptions` |
| `internal/cli/config_get.go` | **Modify** — pass `ShowSecrets` via `RenderOptions` |
| `internal/cli/config_get_test.go` | **Modify** — update calls, add masking integration tests |

### Hazards

- **`DSN` is a sensitive keyword.** The existing `TestRender_TOMLQuotesValues` test uses `"DSN"` as a variable name. After masking is wired in, this will break because the value gets masked. You must update that test to use `ShowSecrets: true` (or rename the variable to something non-sensitive).
- **Changing the `Render` signature breaks all callers.** Both `internal/cli/config_get.go` and all `render_test.go` tests call `Render`. Update them in the same task or tests won't compile.
- **The `base64Pattern` includes `-` and `_` (URL-safe base64).** This means slug-like strings like `my-app-prod-abc123def456` match the character set — but they score low on entropy (< 4.5) so they won't be falsely masked. Still, add a test for this.

---

## Task 1: Name-based keyword matching

Build the core `Masker` type with keyword and allowlist regex compilation. No entropy yet.

**Files:**

- Create: `internal/config/mask.go`
- Create: `internal/config/mask_test.go`

**Step 1: Write the failing test**

Create `internal/config/mask_test.go`:

```go
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
```

**Step 2: Run the test to verify it fails**

Run: `go test ./internal/config/... -run TestMaskValue -v`

Expected: FAIL — `undefined: config.NewMasker`.

**Step 3: Write minimal implementation**

Create `internal/config/mask.go`:

```go
package config

import (
	"regexp"
	"strings"
)

// MaskedValue is the replacement string for masked secrets.
const MaskedValue = "********"

// DefaultSensitiveKeywords are the name patterns that trigger masking.
// Matched with (\b|_) boundaries so KEY matches AUTH_KEY but not KEYBOARD.
var DefaultSensitiveKeywords = []string{
	// Passwords & passphrases
	"PASSWORD", "PASSWD", "PASS", "PWD",
	// Secrets & keys
	"SECRET", "PRIVATE_KEY", "SIGNING_KEY", "ENCRYPTION_KEY", "MASTER_KEY",
	"DEPLOY_KEY", "KEY",
	// API & access credentials
	"API_KEY", "APIKEY", "API_SECRET", "ACCESS_KEY", "AUTH_TOKEN", "AUTH_KEY",
	"CLIENT_SECRET", "SERVICE_KEY", "ACCOUNT_KEY",
	// Tokens
	"TOKEN",
	// Credentials
	"CREDENTIAL", "CREDS", "AUTH",
	// Certificates
	"CERT", "PEM", "PFX", "KEYSTORE", "STOREPASS",
	// Cryptographic material
	"HMAC", "SALT", "PEPPER", "NONCE", "SEED", "CIPHER",
	// Connection strings
	"CONNECTION_STRING", "DATABASE_URL", "REDIS_URL", "MONGODB_URI",
	"MYSQL_URL", "POSTGRES_URL", "DSN",
	// Webhooks & sessions
	"WEBHOOK_SECRET", "WEBHOOK_URL", "SESSION_SECRET", "COOKIE_SECRET",
	"JWT_SECRET",
}

// DefaultAllowlist suppresses false-positive matches from DefaultSensitiveKeywords.
var DefaultAllowlist = []string{
	// KEY — whole-segment matches that aren't secrets
	"PRIMARY_KEY", "FOREIGN_KEY", "SORT_KEY", "PARTITION_KEY", "PUBLIC_KEY",
	"KEY_ID", "KEY_NAME", "KEY_FILE", "KEY_LENGTH", "KEY_SIZE", "KEY_TYPE",
	"KEY_FORMAT", "KEY_VAULT_NAME",
	// TOKEN — metadata, not token values
	"TOKEN_URL", "TOKEN_ENDPOINT", "TOKEN_FILE", "TOKEN_TYPE", "TOKEN_EXPIRY",
	// CREDENTIAL — metadata
	"CREDENTIAL_ID", "CREDENTIALS_URL", "CREDENTIALS_ENDPOINT",
	// SECRET — metadata
	"SECRET_NAME", "SECRET_LENGTH", "SECRET_VERSION",
	// SEED — data seeding, not cryptographic seeds
	"SEED_DATA", "SEED_FILE",
}

// Masker determines whether variable values should be masked in output.
type Masker struct {
	sensitive *regexp.Regexp
	allowlist *regexp.Regexp
}

// NewMasker creates a Masker with the given keyword and allowlist patterns.
// Pass nil for either to use the defaults. Passing a non-nil slice
// (even empty) replaces the defaults entirely.
func NewMasker(keywords, allowlist []string) *Masker {
	if keywords == nil {
		keywords = DefaultSensitiveKeywords
	}
	if allowlist == nil {
		allowlist = DefaultAllowlist
	}
	return &Masker{
		sensitive: buildBoundaryRegex(keywords),
		allowlist: buildBoundaryRegex(allowlist),
	}
}

// buildBoundaryRegex compiles keywords into a single case-insensitive regex
// using (\b|_) as the boundary: (?i)(\b|_)(KW1|KW2|...)(\b|_).
// Returns nil if keywords is empty.
func buildBoundaryRegex(keywords []string) *regexp.Regexp {
	if len(keywords) == 0 {
		return nil
	}
	escaped := make([]string, len(keywords))
	for i, kw := range keywords {
		escaped[i] = regexp.QuoteMeta(kw)
	}
	pattern := `(?i)(\b|_)(` + strings.Join(escaped, "|") + `)(\b|_)`
	return regexp.MustCompile(pattern)
}

// MaskValue returns MaskedValue if the variable should be masked, or the
// original value if it should be shown. Implements the combined logic from
// docs/SECRET-MASKING.md (name-based layer only; entropy added in Task 2).
func (m *Masker) MaskValue(name, value string) string {
	// Railway reference templates are always shown.
	if strings.Contains(value, "${{") {
		return value
	}
	// Check allowlist first — suppresses false positives.
	if m.allowlist != nil && m.allowlist.MatchString(name) {
		return value
	}
	// Check sensitive keywords.
	if m.sensitive != nil && m.sensitive.MatchString(name) {
		return MaskedValue
	}
	return value
}
```

**Step 4: Run the test to verify it passes**

Run: `go test ./internal/config/... -run TestMaskValue -v`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/config/mask.go internal/config/mask_test.go
git commit -m "Add name-based secret masking with keyword/allowlist matching"
```

---

## Task 2: Shannon entropy detection

Add Layer 2: entropy-based detection for values that pass name-based checks.

**Files:**

- Modify: `internal/config/mask.go`
- Modify: `internal/config/mask_test.go`

**Step 1: Write the failing tests**

Append to `internal/config/mask_test.go`:

```go
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
```

**Step 2: Run the tests to verify they fail**

Run: `go test ./internal/config/... -run "TestMaskValue_HighEntropy|TestMaskValue_Slug|TestShannonEntropy" -v`

Expected: FAIL — `undefined: config.ShannonEntropy`, and high-entropy values not masked.

**Step 3: Write minimal implementation**

Add to `internal/config/mask.go` (update the import block and add after the existing code):

```go
import (
	"math"
	"regexp"
	"strings"
	"unicode"
)

// Entropy detection thresholds (matching truffleHog / detect-secrets).
const (
	base64Threshold  = 4.5
	hexThreshold     = 3.0
	entropyMinLength = 20
)

// Character set patterns for entropy classification.
// base64Pattern includes URL-safe chars (_-). Slug-like strings
// ("my-app-prod-12345") match the charset but score well below the
// 4.5 entropy threshold, so they won't be falsely masked.
var (
	hexPattern    = regexp.MustCompile(`^[0-9a-fA-F]+$`)
	base64Pattern = regexp.MustCompile(`^[A-Za-z0-9+/=_\-]+$`)
)

// ShannonEntropy computes the Shannon entropy (bits per character) of s.
// Returns 0 for empty strings.
func ShannonEntropy(s string) float64 {
	if len(s) == 0 {
		return 0
	}
	freq := make(map[rune]int)
	for _, r := range s {
		freq[r]++
	}
	length := float64(len([]rune(s)))
	var entropy float64
	for _, count := range freq {
		p := float64(count) / length
		entropy -= p * math.Log2(p)
	}
	return entropy
}

// hasHighEntropy returns true if value looks like a random secret based
// on Shannon entropy thresholds for base64 and hex character sets.
func hasHighEntropy(value string) bool {
	v := strings.TrimSpace(value)
	if len(v) < entropyMinLength {
		return false
	}
	// Skip values with spaces — likely human text, not secrets.
	for _, r := range v {
		if unicode.IsSpace(r) {
			return false
		}
	}
	entropy := ShannonEntropy(v)
	if hexPattern.MatchString(v) && entropy > hexThreshold {
		return true
	}
	if base64Pattern.MatchString(v) && entropy > base64Threshold {
		return true
	}
	return false
}
```

Update `MaskValue` to add entropy check after name-based checks. Replace the existing method:

```go
// MaskValue returns MaskedValue if the variable should be masked, or the
// original value if it should be shown. Implements the combined logic from
// docs/SECRET-MASKING.md.
func (m *Masker) MaskValue(name, value string) string {
	// Railway reference templates are always shown.
	if strings.Contains(value, "${{") {
		return value
	}
	// Check allowlist first — suppresses false positives.
	if m.allowlist != nil && m.allowlist.MatchString(name) {
		return value
	}
	// Layer 1: name-based keyword matching.
	if m.sensitive != nil && m.sensitive.MatchString(name) {
		return MaskedValue
	}
	// Layer 2: entropy-based detection.
	if hasHighEntropy(value) {
		return MaskedValue
	}
	return value
}
```

**Step 4: Run the tests to verify they pass**

Run: `go test ./internal/config/... -run "TestMaskValue|TestShannonEntropy" -v`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/config/mask.go internal/config/mask_test.go
git commit -m "Add Shannon entropy detection for secret values"
```

---

## Task 3: Introduce RenderOptions and update existing tests

Replace `Render(cfg, format, full)` with `Render(cfg, opts)` using a new `RenderOptions` struct. No masking wiring yet — just the signature change.

**Important:** This task changes the `Render` function signature, which breaks both `render_test.go` and `config_get.go`. Update both in this task so the codebase compiles.

**Files:**

- Modify: `internal/config/render.go`
- Modify: `internal/config/render_test.go`
- Modify: `internal/cli/config_get.go`

**Step 1: Update `render.go`**

Add `RenderOptions` and change the `Render` signature. Replace the current `Render` function:

```go
// RenderOptions controls how config is rendered.
type RenderOptions struct {
	Format      string   // "text", "json", "toml"
	Full        bool     // Include IDs and deploy settings
	ShowSecrets bool     // Show all values unmasked
	Keywords    []string // Custom sensitive keywords (nil = defaults)
	Allowlist   []string // Custom allowlist (nil = defaults)
}

// Render renders the live config in the requested output format.
// Variable values are masked by default unless ShowSecrets is true.
func Render(cfg LiveConfig, opts RenderOptions) (string, error) {
	switch opts.Format {
	case "json":
		buf, err := json.MarshalIndent(toJSONMap(cfg, opts.Full), "", "  ")
		if err != nil {
			return "", err
		}
		return string(buf), nil
	case "toml":
		return renderTOML(cfg, opts.Full), nil
	case "text", "":
		return renderText(cfg, opts.Full), nil
	default:
		return "", errors.New("unsupported output format")
	}
}
```

Note: masking is NOT wired in yet. That's Task 4. This task only changes the signature.

**Step 2: Update all `render_test.go` calls**

Replace every `config.Render(...)` call in `internal/config/render_test.go`. The pattern:

```go
// Old:
config.Render(cfg, "text", false)
config.Render(cfg, "text", true)
config.Render(cfg, "json", false)
config.Render(cfg, "json", true)
config.Render(cfg, "toml", false)
config.Render(cfg, "toml", true)
config.Render(cfg, "xml", false)

// New:
config.Render(cfg, config.RenderOptions{Format: "text"})
config.Render(cfg, config.RenderOptions{Format: "text", Full: true})
config.Render(cfg, config.RenderOptions{Format: "json"})
config.Render(cfg, config.RenderOptions{Format: "json", Full: true})
config.Render(cfg, config.RenderOptions{Format: "toml"})
config.Render(cfg, config.RenderOptions{Format: "toml", Full: true})
config.Render(cfg, config.RenderOptions{Format: "xml"})
```

Note: do NOT add `ShowSecrets: true` yet. Masking isn't wired in, so existing tests pass as-is.

**Step 3: Update `config_get.go`**

Change `internal/cli/config_get.go:77` from:

```go
output, err := config.Render(*cfg, globals.Output, globals.Full)
```

to:

```go
output, err := config.Render(*cfg, config.RenderOptions{
	Format:      globals.Output,
	Full:        globals.Full,
	ShowSecrets: globals.ShowSecrets,
})
```

**Step 4: Verify everything compiles and tests pass**

Run: `go test ./internal/config/... ./internal/cli/... -v`

Expected: PASS. No behavior change — just a signature change.

**Step 5: Commit**

```bash
git add internal/config/render.go internal/config/render_test.go internal/cli/config_get.go
git commit -m "Replace Render(format, full) with Render(RenderOptions)"
```

---

## Task 4: Wire masking into Render and add masking tests

Now wire the `Masker` into `Render` via `maskConfig`, and add tests for masking behavior across all output formats.

**Files:**

- Modify: `internal/config/render.go`
- Modify: `internal/config/render_test.go`

**Step 1: Write the failing masking tests**

Add to `internal/config/render_test.go`:

```go
func TestRender_MasksSecretsByDefault(t *testing.T) {
	cfg := config.LiveConfig{
		Shared: map[string]string{"DATABASE_PASSWORD": "hunter2"},
		Services: map[string]*config.ServiceConfig{
			"api": {Name: "api", Variables: map[string]string{
				"AUTH_TOKEN": "secret123",
				"APP_ENV":    "production",
			}},
		},
	}
	got, err := config.Render(cfg, config.RenderOptions{Format: "text"})
	if err != nil {
		t.Fatalf("Render() error: %v", err)
	}
	if !strings.Contains(got, "********") {
		t.Errorf("expected masked values, got:\n%s", got)
	}
	if !strings.Contains(got, "production") {
		t.Errorf("expected non-secret shown, got:\n%s", got)
	}
	if strings.Contains(got, "hunter2") {
		t.Errorf("password should be masked, got:\n%s", got)
	}
	if strings.Contains(got, "secret123") {
		t.Errorf("token should be masked, got:\n%s", got)
	}
}

func TestRender_ShowSecretsOverridesMasking(t *testing.T) {
	cfg := config.LiveConfig{
		Shared: map[string]string{"DATABASE_PASSWORD": "hunter2"},
	}
	got, err := config.Render(cfg, config.RenderOptions{
		Format:      "text",
		ShowSecrets: true,
	})
	if err != nil {
		t.Fatalf("Render() error: %v", err)
	}
	if !strings.Contains(got, "hunter2") {
		t.Errorf("--show-secrets should show password, got:\n%s", got)
	}
}

func TestRender_MaskingWorksInJSON(t *testing.T) {
	cfg := config.LiveConfig{
		Shared: map[string]string{"API_KEY": "fakekeyfakekeyfakekey"},
	}
	got, err := config.Render(cfg, config.RenderOptions{Format: "json"})
	if err != nil {
		t.Fatalf("Render() error: %v", err)
	}
	if strings.Contains(got, "fakekeyfakekeyfakekey") {
		t.Errorf("API key should be masked in JSON, got:\n%s", got)
	}
}

func TestRender_MaskingWorksInTOML(t *testing.T) {
	cfg := config.LiveConfig{
		Shared: map[string]string{"API_KEY": "fakekeyfakekeyfakekey"},
	}
	got, err := config.Render(cfg, config.RenderOptions{Format: "toml"})
	if err != nil {
		t.Fatalf("Render() error: %v", err)
	}
	if strings.Contains(got, "fakekeyfakekeyfakekey") {
		t.Errorf("API key should be masked in TOML, got:\n%s", got)
	}
}

func TestRender_ReferenceTemplateNotMasked(t *testing.T) {
	cfg := config.LiveConfig{
		Shared: map[string]string{
			"DATABASE_URL": "${{postgres.DATABASE_URL}}",
		},
	}
	got, err := config.Render(cfg, config.RenderOptions{Format: "text"})
	if err != nil {
		t.Fatalf("Render() error: %v", err)
	}
	if !strings.Contains(got, "${{postgres.DATABASE_URL}}") {
		t.Errorf("reference template should not be masked, got:\n%s", got)
	}
}
```

**Step 2: Run the new tests to verify they fail**

Run: `go test ./internal/config/... -run "TestRender_Masks|TestRender_ShowSecrets|TestRender_Reference" -v`

Expected: FAIL — masking not yet wired in, so `hunter2` appears unmasked.

**Step 3: Wire masking into Render**

Update `Render` in `internal/config/render.go`:

```go
func Render(cfg LiveConfig, opts RenderOptions) (string, error) {
	var masker *Masker
	if !opts.ShowSecrets {
		masker = NewMasker(opts.Keywords, opts.Allowlist)
	}
	masked := maskConfig(cfg, masker)

	switch opts.Format {
	case "json":
		buf, err := json.MarshalIndent(toJSONMap(masked, opts.Full), "", "  ")
		if err != nil {
			return "", err
		}
		return string(buf), nil
	case "toml":
		return renderTOML(masked, opts.Full), nil
	case "text", "":
		return renderText(masked, opts.Full), nil
	default:
		return "", errors.New("unsupported output format")
	}
}
```

Add `maskConfig` and `maskVars` helper functions (add these after the `Render` function):

```go
// maskConfig returns a copy of cfg with variable values masked.
// If masker is nil (ShowSecrets mode), returns cfg unchanged.
func maskConfig(cfg LiveConfig, masker *Masker) LiveConfig {
	if masker == nil {
		return cfg
	}
	out := LiveConfig{
		ProjectID:     cfg.ProjectID,
		EnvironmentID: cfg.EnvironmentID,
		Shared:        maskVars(cfg.Shared, masker),
		Services:      make(map[string]*ServiceConfig, len(cfg.Services)),
	}
	for name, svc := range cfg.Services {
		out.Services[name] = &ServiceConfig{
			ID:        svc.ID,
			Name:      svc.Name,
			Variables: maskVars(svc.Variables, masker),
			Deploy:    svc.Deploy,
		}
	}
	return out
}

// maskVars returns a new map with values masked as needed.
func maskVars(vars map[string]string, masker *Masker) map[string]string {
	if len(vars) == 0 {
		return vars
	}
	out := make(map[string]string, len(vars))
	for k, v := range vars {
		out[k] = masker.MaskValue(k, v)
	}
	return out
}
```

**Step 4: Fix broken existing tests**

Now that masking is active by default, existing tests that use sensitive variable names will break. Specifically, `TestRender_TOMLQuotesValues` uses `"DSN"` which is a sensitive keyword.

Update that test to use `ShowSecrets: true`:

```go
func TestRender_TOMLQuotesValues(t *testing.T) {
	cfg := config.LiveConfig{
		Shared: map[string]string{"CONNECTION_INFO": `host="db" port=5432`},
	}
	got, err := config.Render(cfg, config.RenderOptions{Format: "toml", ShowSecrets: true})
	// ...
}
```

Or rename the variable from `DSN` to `CONNECTION_INFO` (not a keyword). Either works — renaming is cleaner because it tests TOML quoting independently of masking.

Also audit all other existing `render_test.go` tests for sensitive names:

- `sampleConfig()` uses `SHARED` and `PORT` — not sensitive. Safe.
- `TestRender_JSONIncludesVariables` uses `PORT` — safe.
- `TestRender_EmptyConfig` uses no variables — safe.
- `TestRender_TextFullOmitsDeployWhenEmpty` uses `X` — safe.

**Step 5: Run all tests**

Run: `go test ./internal/config/... -v`

Expected: PASS.

**Step 6: Commit**

```bash
git add internal/config/render.go internal/config/render_test.go
git commit -m "Wire secret masking into Render — mask by default, ShowSecrets to override"
```

---

## Task 5: CLI integration tests

Add integration tests that verify masking flows through from `RunConfigGet` via `Globals.ShowSecrets`.

**Files:**

- Modify: `internal/cli/config_get_test.go`

**Step 1: Write the integration tests**

Add to `internal/cli/config_get_test.go`:

```go
func TestRunConfigGet_MasksSecretsByDefault(t *testing.T) {
	fetcher := &fakeFetcher{
		cfg: &config.LiveConfig{
			ProjectID:     "proj-1",
			EnvironmentID: "env-1",
			Shared:        map[string]string{"DATABASE_PASSWORD": "hunter2"},
			Services:      map[string]*config.ServiceConfig{},
		},
	}
	var buf bytes.Buffer
	globals := &cli.Globals{Output: "text"}
	err := cli.RunConfigGet(context.Background(), globals, "", fetcher, &buf)
	if err != nil {
		t.Fatalf("RunConfigGet() error: %v", err)
	}
	got := buf.String()
	if strings.Contains(got, "hunter2") {
		t.Errorf("password should be masked by default, got:\n%s", got)
	}
	if !strings.Contains(got, "********") {
		t.Errorf("expected masked placeholder, got:\n%s", got)
	}
}

func TestRunConfigGet_ShowSecretsRevealsValues(t *testing.T) {
	fetcher := &fakeFetcher{
		cfg: &config.LiveConfig{
			ProjectID:     "proj-1",
			EnvironmentID: "env-1",
			Shared:        map[string]string{"DATABASE_PASSWORD": "hunter2"},
			Services:      map[string]*config.ServiceConfig{},
		},
	}
	var buf bytes.Buffer
	globals := &cli.Globals{Output: "text", ShowSecrets: true}
	err := cli.RunConfigGet(context.Background(), globals, "", fetcher, &buf)
	if err != nil {
		t.Fatalf("RunConfigGet() error: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "hunter2") {
		t.Errorf("--show-secrets should reveal password, got:\n%s", got)
	}
}
```

**Step 2: Run the tests**

Run: `go test ./internal/cli/... -run "TestRunConfigGet_Masks|TestRunConfigGet_ShowSecrets" -v`

Expected: PASS (since `config_get.go` already passes `ShowSecrets` via `RenderOptions` from Task 3).

**Step 3: Verify existing CLI tests still pass**

The existing tests use variable names `FOO`, `PORT`, `DB` — none are sensitive keywords. Confirm:

Run: `go test ./internal/cli/... -v`

Expected: PASS.

**Step 4: Commit**

```bash
git add internal/cli/config_get_test.go
git commit -m "Add CLI integration tests for secret masking"
```

---

## Task 6: Final verification

Run the full suite to confirm nothing is broken.

**Step 1: Full check**

Run: `mise run check`

Expected: All linters pass, all tests pass, build succeeds.

**Step 2: If any remaining uncommitted changes**

```bash
git add -A
git commit -m "Complete secret masking implementation"
```
