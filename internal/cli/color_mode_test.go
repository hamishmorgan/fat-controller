package cli

import "testing"

func TestResolveColorMode_NoColorWins(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("FORCE_COLOR", "1")
	mode := ResolveColorMode("")
	if mode != "never" {
		t.Fatalf("mode = %q, want never", mode)
	}
}

func TestResolveColorMode_ForceColor(t *testing.T) {
	t.Setenv("FORCE_COLOR", "1")
	mode := ResolveColorMode("")
	if mode != "always" {
		t.Fatalf("mode = %q, want always", mode)
	}
}

func TestResolveColorMode_ForceColorZeroIsIgnored(t *testing.T) {
	t.Setenv("FORCE_COLOR", "0")
	mode := ResolveColorMode("")
	if mode != "auto" {
		t.Fatalf("mode = %q, want auto", mode)
	}
}

func TestResolveColorMode_CLICOLOR0(t *testing.T) {
	t.Setenv("CLICOLOR", "0")
	mode := ResolveColorMode("")
	if mode != "never" {
		t.Fatalf("mode = %q, want never", mode)
	}
}

func TestResolveColorMode_CLICOLORForce(t *testing.T) {
	t.Setenv("CLICOLOR_FORCE", "1")
	mode := ResolveColorMode("")
	if mode != "always" {
		t.Fatalf("mode = %q, want always", mode)
	}
}

func TestResolveColorMode_TermDumb(t *testing.T) {
	t.Setenv("TERM", "dumb")
	mode := ResolveColorMode("")
	if mode != "never" {
		t.Fatalf("mode = %q, want never", mode)
	}
}

func TestResolveColorMode_ExplicitWins(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	mode := ResolveColorMode("always")
	if mode != "always" {
		t.Fatalf("mode = %q, want always", mode)
	}
}
