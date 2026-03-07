package config_test

import (
	"path/filepath"
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/config"
)

func TestParseEnvFile_BasicKeyValue(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	writeFile(t, path, "FOO=bar\nBAZ=qux\n")

	vars, err := config.ParseEnvFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if vars["FOO"] != "bar" {
		t.Errorf("FOO = %q, want bar", vars["FOO"])
	}
	if vars["BAZ"] != "qux" {
		t.Errorf("BAZ = %q, want qux", vars["BAZ"])
	}
}

func TestParseEnvFile_CommentsAndBlanks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	writeFile(t, path, "# comment\n\nFOO=bar\n  # another comment\nBAZ=qux\n")

	vars, err := config.ParseEnvFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(vars) != 2 {
		t.Errorf("len = %d, want 2", len(vars))
	}
}

func TestParseEnvFile_DoubleQuotedValue(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	writeFile(t, path, "FOO=\"hello world\"\n")

	vars, err := config.ParseEnvFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if vars["FOO"] != "hello world" {
		t.Errorf("FOO = %q, want %q", vars["FOO"], "hello world")
	}
}

func TestParseEnvFile_SingleQuotedValue(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	writeFile(t, path, "FOO='hello world'\n")

	vars, err := config.ParseEnvFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if vars["FOO"] != "hello world" {
		t.Errorf("FOO = %q, want %q", vars["FOO"], "hello world")
	}
}

func TestParseEnvFile_BareValue(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	writeFile(t, path, "FOO=hello\n")

	vars, err := config.ParseEnvFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if vars["FOO"] != "hello" {
		t.Errorf("FOO = %q, want hello", vars["FOO"])
	}
}

func TestParseEnvFile_ValueContainsEquals(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	writeFile(t, path, "URL=postgres://user:pass@host/db?opt=val\n")

	vars, err := config.ParseEnvFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if vars["URL"] != "postgres://user:pass@host/db?opt=val" {
		t.Errorf("URL = %q", vars["URL"])
	}
}

func TestParseEnvFile_EmptyValue(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	writeFile(t, path, "EMPTY=\n")

	vars, err := config.ParseEnvFile(path)
	if err != nil {
		t.Fatal(err)
	}
	val, ok := vars["EMPTY"]
	if !ok {
		t.Fatal("EMPTY key not found")
	}
	if val != "" {
		t.Errorf("EMPTY = %q, want empty string", val)
	}
}

func TestParseEnvFile_NonexistentFile(t *testing.T) {
	_, err := config.ParseEnvFile("/tmp/definitely-does-not-exist-fc-test.env")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestParseEnvFile_ExportPrefix(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	writeFile(t, path, "export FOO=bar\nexport BAZ=\"qux\"\n")

	vars, err := config.ParseEnvFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if vars["FOO"] != "bar" {
		t.Errorf("FOO = %q, want bar", vars["FOO"])
	}
	if vars["BAZ"] != "qux" {
		t.Errorf("BAZ = %q, want qux", vars["BAZ"])
	}
}
