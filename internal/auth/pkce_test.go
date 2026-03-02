package auth_test

import (
	"crypto/sha256"
	"encoding/base64"
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/auth"
)

func TestGenerateCodeVerifier(t *testing.T) {
	v1, err := auth.GenerateCodeVerifier()
	if err != nil {
		t.Fatal(err)
	}

	// Must be at least 43 characters (RFC 7636).
	if len(v1) < 43 {
		t.Errorf("verifier too short: %d chars", len(v1))
	}

	// Must be different each time.
	v2, err := auth.GenerateCodeVerifier()
	if err != nil {
		t.Fatal(err)
	}
	if v1 == v2 {
		t.Error("two verifiers should not be identical")
	}
}

func TestCodeChallenge(t *testing.T) {
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"

	challenge := auth.CodeChallenge(verifier)

	// Manually compute expected value.
	h := sha256.Sum256([]byte(verifier))
	want := base64.RawURLEncoding.EncodeToString(h[:])

	if challenge != want {
		t.Errorf("CodeChallenge() = %q, want %q", challenge, want)
	}
}

func TestGenerateState(t *testing.T) {
	s1, err := auth.GenerateState()
	if err != nil {
		t.Fatal(err)
	}
	if len(s1) == 0 {
		t.Error("state should not be empty")
	}

	s2, err := auth.GenerateState()
	if err != nil {
		t.Fatal(err)
	}
	if s1 == s2 {
		t.Error("two states should not be identical")
	}
}
