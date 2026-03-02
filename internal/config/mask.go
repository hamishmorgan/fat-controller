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
