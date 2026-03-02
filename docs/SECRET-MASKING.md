# Secret Masking

Variables are automatically masked in output when they appear to contain
secrets. Detection uses two layers: **name-based** and **entropy-based**.

## Layer 1: Name-based detection

Keywords are compiled into a single case-insensitive regex using
`(\b|_)` as the boundary: e.g. `AUTH, WEBHOOK_URL` becomes
`(?i)(\b|_)(AUTH|WEBHOOK_URL)(\b|_)`. This matches at underscores and
string edges but not mid-word (`AUTH_TOKEN` matches, `AUTHORIZE` does
not). A false-positive **allowlist** uses the same pattern to suppress
known non-secret matches.

**Default sensitive keywords**:

```text
# Passwords & passphrases
PASSWORD, PASSWD, PASS, PWD

# Secrets & keys
SECRET, PRIVATE_KEY, SIGNING_KEY, ENCRYPTION_KEY, MASTER_KEY,
DEPLOY_KEY, KEY

# API & access credentials
API_KEY, APIKEY, API_SECRET, ACCESS_KEY, AUTH_TOKEN, AUTH_KEY,
CLIENT_SECRET, SERVICE_KEY, ACCOUNT_KEY

# Tokens
TOKEN

# Credentials
CREDENTIAL, CREDS, AUTH

# Certificates
CERT, PEM, PFX, KEYSTORE, STOREPASS

# Cryptographic material
HMAC, SALT, PEPPER, NONCE, SEED, CIPHER

# Connection strings (often embed credentials)
CONNECTION_STRING, DATABASE_URL, REDIS_URL, MONGODB_URI,
MYSQL_URL, POSTGRES_URL, DSN

# Webhooks & sessions
WEBHOOK_SECRET, WEBHOOK_URL, SESSION_SECRET, COOKIE_SECRET,
JWT_SECRET
```

**Default false-positive allowlist** (same boundary regex, checked first):

```text
# KEY — whole-segment matches that aren't secrets
PRIMARY_KEY, FOREIGN_KEY, SORT_KEY, PARTITION_KEY, PUBLIC_KEY,
KEY_ID, KEY_NAME, KEY_FILE, KEY_LENGTH, KEY_SIZE, KEY_TYPE,
KEY_FORMAT, KEY_VAULT_NAME

# TOKEN — metadata, not token values
TOKEN_URL, TOKEN_ENDPOINT, TOKEN_FILE, TOKEN_TYPE, TOKEN_EXPIRY

# CREDENTIAL — metadata
CREDENTIAL_ID, CREDENTIALS_URL, CREDENTIALS_ENDPOINT

# SECRET — metadata
SECRET_NAME, SECRET_LENGTH, SECRET_VERSION

# SEED — data seeding, not cryptographic seeds
SEED_DATA, SEED_FILE
```

Note: with `(\b|_)` boundaries, mid-word matches like `KEYBOARD`,
`AUTHORIZE`, `PASSENGER` are already excluded — no allowlist entry
needed.

Both lists are configurable. Setting `sensitive_keywords` or
`sensitive_allowlist` in config replaces the respective defaults.

## Layer 2: Entropy-based detection

Values that pass name-based checks are tested for high Shannon entropy,
which indicates random/generated strings typical of API keys and tokens.
Uses the same thresholds as truffleHog and Yelp's detect-secrets:

| Charset | Characters | Threshold | Min length |
|---------|-----------|-----------|------------|
| Base64 | `A-Za-z0-9+/=` | > 4.5 bits/char | 20 chars |
| Hex | `0-9a-fA-F` | > 3.0 bits/char | 20 chars |

The Shannon entropy formula: `H = -Σ p(x) * log₂(p(x))` where `p(x)` is
the frequency of character `x` in the string. Random strings approach the
theoretical maximum for their charset; structured strings (English words,
URLs, paths) score much lower.

This catches secrets with non-obvious names like
`SETTING_X = "sk_live_4eC39HqL..."` that name-based detection would miss.

## Combined masking logic

1. The tool always fetches the unrendered value from Railway (needed to
   detect `${{}}` references and compute diffs correctly).
2. If the value contains `${{` — it's a Railway reference template.
   **Show as-is** regardless of name or entropy.
3. If the name matches a sensitive pattern — **mask as `********`**.
4. If the value has high entropy (base64 > 4.5 or hex > 3.0, min 20
   chars) — **mask as `********`**.
5. Otherwise — **show**.
6. `--show-secrets` overrides all masking.

**Examples:**

```text
DATABASE_PASSWORD = "********"              # masked (name matches PASSWORD)
DATABASE_URL = "${{postgres.DATABASE_URL}}" # shown (reference template)
APP_ENV = "production"                      # shown (no name match, low entropy)
SETTING_X = "********"                      # masked (high entropy value)
BUILD_HASH = "abc123"                       # shown (too short for entropy check)
```

**Custom keywords and allowlist** (each replaces its defaults entirely):

```toml
# .fat-controller.toml
sensitive_keywords = ["SECRET", "TOKEN", "PASSWORD", "MY_CUSTOM_FIELD"]
sensitive_allowlist = ["KEYSTROKE", "MY_SAFE_TOKEN_NAME"]
```
