package config_test

import (
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/config"
)

func TestInterpolate_LocalEnvResolved(t *testing.T) {
	t.Setenv("MY_SECRET", "hunter2")
	cfg := &config.DesiredConfig{
		Variables: config.Variables{
			"KEY": "${MY_SECRET}",
		},
		Services: []*config.DesiredService{
			{Name: "api", Variables: config.Variables{
				"SECRET": "${MY_SECRET}",
			}},
		},
	}
	err := config.Interpolate(cfg, nil)
	if err != nil {
		t.Fatalf("Interpolate() error: %v", err)
	}
	if cfg.Variables["KEY"] != "hunter2" {
		t.Errorf("shared KEY = %q, want hunter2", cfg.Variables["KEY"])
	}
	if cfg.Services[0].Variables["SECRET"] != "hunter2" {
		t.Errorf("api SECRET = %q, want hunter2", cfg.Services[0].Variables["SECRET"])
	}
}

func TestInterpolate_RailwayRefUntouched(t *testing.T) {
	cfg := &config.DesiredConfig{
		Services: []*config.DesiredService{
			{Name: "api", Variables: config.Variables{
				"DB": "${{postgres.DATABASE_URL}}",
			}},
		},
	}
	err := config.Interpolate(cfg, nil)
	if err != nil {
		t.Fatalf("Interpolate() error: %v", err)
	}
	if cfg.Services[0].Variables["DB"] != "${{postgres.DATABASE_URL}}" {
		t.Errorf("Railway ref was modified: %q", cfg.Services[0].Variables["DB"])
	}
}

func TestInterpolate_MixedInSameValue(t *testing.T) {
	t.Setenv("HOST", "example.com")
	cfg := &config.DesiredConfig{
		Services: []*config.DesiredService{
			{Name: "api", Variables: config.Variables{
				"URL": "https://${HOST}/${{api.PATH}}",
			}},
		},
	}
	err := config.Interpolate(cfg, nil)
	if err != nil {
		t.Fatalf("Interpolate() error: %v", err)
	}
	want := "https://example.com/${{api.PATH}}"
	if cfg.Services[0].Variables["URL"] != want {
		t.Errorf("URL = %q, want %q", cfg.Services[0].Variables["URL"], want)
	}
}

func TestInterpolate_MissingEnvVar(t *testing.T) {
	// Ensure the var is definitely unset.
	t.Setenv("DEFINITELY_MISSING_FC_TEST", "")
	cfg := &config.DesiredConfig{
		Services: []*config.DesiredService{
			{Name: "api", Variables: config.Variables{
				"VAL": "${DEFINITELY_MISSING_FC_TEST_REALLY}",
			}},
		},
	}
	err := config.Interpolate(cfg, nil)
	if err == nil {
		t.Fatal("expected error for missing env var")
	}
}

func TestInterpolate_NoInterpolation(t *testing.T) {
	cfg := &config.DesiredConfig{
		Services: []*config.DesiredService{
			{Name: "api", Variables: config.Variables{
				"PLAIN": "hello world",
			}},
		},
	}
	err := config.Interpolate(cfg, nil)
	if err != nil {
		t.Fatalf("Interpolate() error: %v", err)
	}
	if cfg.Services[0].Variables["PLAIN"] != "hello world" {
		t.Error("plain value was modified")
	}
}

func TestInterpolate_EmptyStringPreserved(t *testing.T) {
	cfg := &config.DesiredConfig{
		Services: []*config.DesiredService{
			{Name: "api", Variables: config.Variables{
				"DEL": "",
			}},
		},
	}
	err := config.Interpolate(cfg, nil)
	if err != nil {
		t.Fatalf("Interpolate() error: %v", err)
	}
	if cfg.Services[0].Variables["DEL"] != "" {
		t.Error("empty string was modified")
	}
}

func TestInterpolate_MultipleVarsInOneValue(t *testing.T) {
	t.Setenv("USER_FC_TEST", "admin")
	t.Setenv("PASS_FC_TEST", "secret")
	cfg := &config.DesiredConfig{
		Services: []*config.DesiredService{
			{Name: "api", Variables: config.Variables{
				"CREDS": "${USER_FC_TEST}:${PASS_FC_TEST}",
			}},
		},
	}
	err := config.Interpolate(cfg, nil)
	if err != nil {
		t.Fatalf("Interpolate() error: %v", err)
	}
	if cfg.Services[0].Variables["CREDS"] != "admin:secret" {
		t.Errorf("CREDS = %q, want admin:secret", cfg.Services[0].Variables["CREDS"])
	}
}

func TestInterpolate_NilShared(t *testing.T) {
	cfg := &config.DesiredConfig{
		Services: []*config.DesiredService{},
	}
	err := config.Interpolate(cfg, nil)
	if err != nil {
		t.Fatalf("Interpolate() error: %v", err)
	}
}

func TestInterpolate_EnvFileBeforeProcessEnv(t *testing.T) {
	t.Setenv("MY_VAR", "from_process")
	envVars := map[string]string{"MY_VAR": "from_env_file"}
	cfg := &config.DesiredConfig{
		Services: []*config.DesiredService{
			{Name: "api", Variables: config.Variables{
				"VAL": "${MY_VAR}",
			}},
		},
	}
	err := config.Interpolate(cfg, envVars)
	if err != nil {
		t.Fatalf("Interpolate() error: %v", err)
	}
	if cfg.Services[0].Variables["VAL"] != "from_env_file" {
		t.Errorf("VAL = %q, want from_env_file (env file should take priority)", cfg.Services[0].Variables["VAL"])
	}
}

func TestInterpolate_FallsBackToProcessEnv(t *testing.T) {
	t.Setenv("PROCESS_ONLY", "from_process")
	envVars := map[string]string{"OTHER": "from_env_file"}
	cfg := &config.DesiredConfig{
		Services: []*config.DesiredService{
			{Name: "api", Variables: config.Variables{
				"VAL": "${PROCESS_ONLY}",
			}},
		},
	}
	err := config.Interpolate(cfg, envVars)
	if err != nil {
		t.Fatalf("Interpolate() error: %v", err)
	}
	if cfg.Services[0].Variables["VAL"] != "from_process" {
		t.Errorf("VAL = %q, want from_process", cfg.Services[0].Variables["VAL"])
	}
}

func TestInterpolate_RegistryCredentials(t *testing.T) {
	envVars := map[string]string{"REG_PASS": "secret123"}
	cfg := &config.DesiredConfig{
		Services: []*config.DesiredService{
			{Name: "api", Deploy: &config.DesiredDeploy{
				RegistryCredentials: &config.RegistryCredentials{
					Username: "deploy",
					Password: "${REG_PASS}",
				},
			}},
		},
	}
	err := config.Interpolate(cfg, envVars)
	if err != nil {
		t.Fatalf("Interpolate() error: %v", err)
	}
	if cfg.Services[0].Deploy.RegistryCredentials.Password != "secret123" {
		t.Errorf("Password = %q, want %q", cfg.Services[0].Deploy.RegistryCredentials.Password, "secret123")
	}
}
