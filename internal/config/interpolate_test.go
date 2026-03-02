package config_test

import (
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/config"
)

func TestInterpolate_LocalEnvResolved(t *testing.T) {
	t.Setenv("MY_SECRET", "hunter2")
	cfg := &config.DesiredConfig{
		Shared: &config.DesiredVariables{Vars: map[string]string{
			"KEY": "${MY_SECRET}",
		}},
		Services: map[string]*config.DesiredService{
			"api": {Variables: map[string]string{
				"SECRET": "${MY_SECRET}",
			}},
		},
	}
	err := config.Interpolate(cfg)
	if err != nil {
		t.Fatalf("Interpolate() error: %v", err)
	}
	if cfg.Shared.Vars["KEY"] != "hunter2" {
		t.Errorf("shared KEY = %q, want hunter2", cfg.Shared.Vars["KEY"])
	}
	if cfg.Services["api"].Variables["SECRET"] != "hunter2" {
		t.Errorf("api SECRET = %q, want hunter2", cfg.Services["api"].Variables["SECRET"])
	}
}

func TestInterpolate_RailwayRefUntouched(t *testing.T) {
	cfg := &config.DesiredConfig{
		Services: map[string]*config.DesiredService{
			"api": {Variables: map[string]string{
				"DB": "${{postgres.DATABASE_URL}}",
			}},
		},
	}
	err := config.Interpolate(cfg)
	if err != nil {
		t.Fatalf("Interpolate() error: %v", err)
	}
	if cfg.Services["api"].Variables["DB"] != "${{postgres.DATABASE_URL}}" {
		t.Errorf("Railway ref was modified: %q", cfg.Services["api"].Variables["DB"])
	}
}

func TestInterpolate_MixedInSameValue(t *testing.T) {
	t.Setenv("HOST", "example.com")
	cfg := &config.DesiredConfig{
		Services: map[string]*config.DesiredService{
			"api": {Variables: map[string]string{
				"URL": "https://${HOST}/${{api.PATH}}",
			}},
		},
	}
	err := config.Interpolate(cfg)
	if err != nil {
		t.Fatalf("Interpolate() error: %v", err)
	}
	want := "https://example.com/${{api.PATH}}"
	if cfg.Services["api"].Variables["URL"] != want {
		t.Errorf("URL = %q, want %q", cfg.Services["api"].Variables["URL"], want)
	}
}

func TestInterpolate_MissingEnvVar(t *testing.T) {
	// Ensure the var is definitely unset.
	t.Setenv("DEFINITELY_MISSING_FC_TEST", "")
	cfg := &config.DesiredConfig{
		Services: map[string]*config.DesiredService{
			"api": {Variables: map[string]string{
				"VAL": "${DEFINITELY_MISSING_FC_TEST_REALLY}",
			}},
		},
	}
	err := config.Interpolate(cfg)
	if err == nil {
		t.Fatal("expected error for missing env var")
	}
}

func TestInterpolate_NoInterpolation(t *testing.T) {
	cfg := &config.DesiredConfig{
		Services: map[string]*config.DesiredService{
			"api": {Variables: map[string]string{
				"PLAIN": "hello world",
			}},
		},
	}
	err := config.Interpolate(cfg)
	if err != nil {
		t.Fatalf("Interpolate() error: %v", err)
	}
	if cfg.Services["api"].Variables["PLAIN"] != "hello world" {
		t.Error("plain value was modified")
	}
}

func TestInterpolate_EmptyStringPreserved(t *testing.T) {
	cfg := &config.DesiredConfig{
		Services: map[string]*config.DesiredService{
			"api": {Variables: map[string]string{
				"DEL": "",
			}},
		},
	}
	err := config.Interpolate(cfg)
	if err != nil {
		t.Fatalf("Interpolate() error: %v", err)
	}
	if cfg.Services["api"].Variables["DEL"] != "" {
		t.Error("empty string was modified")
	}
}

func TestInterpolate_MultipleVarsInOneValue(t *testing.T) {
	t.Setenv("USER_FC_TEST", "admin")
	t.Setenv("PASS_FC_TEST", "secret")
	cfg := &config.DesiredConfig{
		Services: map[string]*config.DesiredService{
			"api": {Variables: map[string]string{
				"CREDS": "${USER_FC_TEST}:${PASS_FC_TEST}",
			}},
		},
	}
	err := config.Interpolate(cfg)
	if err != nil {
		t.Fatalf("Interpolate() error: %v", err)
	}
	if cfg.Services["api"].Variables["CREDS"] != "admin:secret" {
		t.Errorf("CREDS = %q, want admin:secret", cfg.Services["api"].Variables["CREDS"])
	}
}

func TestInterpolate_NilShared(t *testing.T) {
	cfg := &config.DesiredConfig{
		Services: map[string]*config.DesiredService{},
	}
	err := config.Interpolate(cfg)
	if err != nil {
		t.Fatalf("Interpolate() error: %v", err)
	}
}
