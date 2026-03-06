package config

import (
	"math"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
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

// IsSensitive returns true if the variable name matches a sensitive keyword
// (and is not on the allowlist). Unlike MaskValue, it does not check the value
// for entropy — it only uses name-based classification.
func (m *Masker) IsSensitive(name string) bool {
	if m.allowlist != nil && m.allowlist.MatchString(name) {
		return false
	}
	return m.sensitive != nil && m.sensitive.MatchString(name)
}

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
	base64Pattern = regexp.MustCompile(`^[A-Za-z0-9+/=_-]+$`)
)

// ShannonEntropy computes the Shannon entropy (bits per character) of s.
// Returns 0 for empty strings.
func ShannonEntropy(s string) float64 {
	if len(s) == 0 {
		return 0
	}
	freq := make(map[rune]int)
	var total int
	for _, r := range s {
		freq[r]++
		total++
	}
	length := float64(total)
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
	if utf8.RuneCountInString(v) < entropyMinLength {
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
