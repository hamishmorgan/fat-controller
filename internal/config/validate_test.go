package config_test

import (
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/config"
)

// --- helpers ---

func assertHasWarning(t *testing.T, warnings []config.Warning, code string) {
	t.Helper()
	for _, w := range warnings {
		if w.Code == code {
			return
		}
	}
	t.Errorf("expected warning %s, got %v", code, warningCodes(warnings))
}

func assertNoWarning(t *testing.T, warnings []config.Warning, code string) {
	t.Helper()
	for _, w := range warnings {
		if w.Code == code {
			t.Errorf("did not expect warning %s, but found: %s", code, w.Message)
			return
		}
	}
}

func warningCodes(warnings []config.Warning) []string {
	codes := make([]string, len(warnings))
	for i, w := range warnings {
		codes[i] = w.Code
	}
	return codes
}

// --- W003: Empty service block ---

func TestValidate_W003_EmptyServiceBlock(t *testing.T) {
	cfg := &config.DesiredConfig{
		Services: map[string]*config.DesiredService{
			"api": {}, // no variables, no resources, no deploy
		},
	}
	warnings := config.Validate(cfg, nil)
	assertHasWarning(t, warnings, "W003")
}

func TestValidate_W003_NotEmptyWhenVariablesPresent(t *testing.T) {
	cfg := &config.DesiredConfig{
		Services: map[string]*config.DesiredService{
			"api": {Variables: map[string]string{"PORT": "8080"}},
		},
	}
	warnings := config.Validate(cfg, nil)
	assertNoWarning(t, warnings, "W003")
}

func TestValidate_W003_NotEmptyWhenResourcesPresent(t *testing.T) {
	vcpus := 1.0
	cfg := &config.DesiredConfig{
		Services: map[string]*config.DesiredService{
			"api": {Resources: &config.DesiredResources{VCPUs: &vcpus}},
		},
	}
	warnings := config.Validate(cfg, nil)
	assertNoWarning(t, warnings, "W003")
}

func TestValidate_W003_NotEmptyWhenDeployPresent(t *testing.T) {
	builder := "nixpacks"
	cfg := &config.DesiredConfig{
		Services: map[string]*config.DesiredService{
			"api": {Deploy: &config.DesiredDeploy{Builder: &builder}},
		},
	}
	warnings := config.Validate(cfg, nil)
	assertNoWarning(t, warnings, "W003")
}

// --- W011: Suspicious ${word.word} syntax ---

func TestValidate_W011_SuspiciousRefSyntax(t *testing.T) {
	cfg := &config.DesiredConfig{
		Services: map[string]*config.DesiredService{
			"api": {Variables: map[string]string{
				"DB_URL": "${postgres.DATABASE_URL}",
			}},
		},
	}
	warnings := config.Validate(cfg, nil)
	assertHasWarning(t, warnings, "W011")
}

func TestValidate_W011_NoFalsePositiveForDoublebraces(t *testing.T) {
	cfg := &config.DesiredConfig{
		Services: map[string]*config.DesiredService{
			"api": {Variables: map[string]string{
				"DB_URL": "${{postgres.DATABASE_URL}}",
			}},
		},
	}
	warnings := config.Validate(cfg, nil)
	assertNoWarning(t, warnings, "W011")
}

func TestValidate_W011_NoFalsePositiveForSimpleEnvVar(t *testing.T) {
	cfg := &config.DesiredConfig{
		Services: map[string]*config.DesiredService{
			"api": {Variables: map[string]string{
				"KEY": "${SIMPLE_VAR}",
			}},
		},
	}
	warnings := config.Validate(cfg, nil)
	assertNoWarning(t, warnings, "W011")
}

// --- W012: Empty string = delete ---

func TestValidate_W012_EmptyStringDelete(t *testing.T) {
	cfg := &config.DesiredConfig{
		Services: map[string]*config.DesiredService{
			"api": {Variables: map[string]string{
				"OLD_VAR": "",
			}},
		},
	}
	warnings := config.Validate(cfg, nil)
	assertHasWarning(t, warnings, "W012")
}

func TestValidate_W012_NonEmptyStringNoWarning(t *testing.T) {
	cfg := &config.DesiredConfig{
		Services: map[string]*config.DesiredService{
			"api": {Variables: map[string]string{
				"PORT": "8080",
			}},
		},
	}
	warnings := config.Validate(cfg, nil)
	assertNoWarning(t, warnings, "W012")
}

// --- W020: Variable in both shared and service ---

func TestValidate_W020_SharedAndServiceConflict(t *testing.T) {
	cfg := &config.DesiredConfig{
		Shared: &config.DesiredVariables{Vars: map[string]string{
			"APP_ENV": "production",
		}},
		Services: map[string]*config.DesiredService{
			"api": {Variables: map[string]string{
				"APP_ENV": "staging",
			}},
		},
	}
	warnings := config.Validate(cfg, nil)
	assertHasWarning(t, warnings, "W020")
}

func TestValidate_W020_NoConflictWhenDifferentVars(t *testing.T) {
	cfg := &config.DesiredConfig{
		Shared: &config.DesiredVariables{Vars: map[string]string{
			"SHARED_KEY": "value",
		}},
		Services: map[string]*config.DesiredService{
			"api": {Variables: map[string]string{
				"SERVICE_KEY": "value",
			}},
		},
	}
	warnings := config.Validate(cfg, nil)
	assertNoWarning(t, warnings, "W020")
}

// --- W030: Lowercase variable name ---

func TestValidate_W030_LowercaseVarName(t *testing.T) {
	cfg := &config.DesiredConfig{
		Services: map[string]*config.DesiredService{
			"api": {Variables: map[string]string{
				"myVar": "value",
			}},
		},
	}
	warnings := config.Validate(cfg, nil)
	assertHasWarning(t, warnings, "W030")
}

func TestValidate_W030_UppercaseNoWarning(t *testing.T) {
	cfg := &config.DesiredConfig{
		Services: map[string]*config.DesiredService{
			"api": {Variables: map[string]string{
				"MY_VAR": "value",
			}},
		},
	}
	warnings := config.Validate(cfg, nil)
	assertNoWarning(t, warnings, "W030")
}

func TestValidate_W030_SharedLowercaseVarName(t *testing.T) {
	cfg := &config.DesiredConfig{
		Shared: &config.DesiredVariables{Vars: map[string]string{
			"badName": "value",
		}},
		// Need at least one service or shared var to avoid W041 dominating.
	}
	warnings := config.Validate(cfg, nil)
	assertHasWarning(t, warnings, "W030")
}

// --- W040: Unknown service name ---

func TestValidate_W040_UnknownServiceName(t *testing.T) {
	cfg := &config.DesiredConfig{
		Services: map[string]*config.DesiredService{
			"api":     {Variables: map[string]string{"PORT": "8080"}},
			"unknown": {Variables: map[string]string{"PORT": "9090"}},
		},
	}
	liveServices := []string{"api", "worker"}
	warnings := config.Validate(cfg, liveServices)
	assertHasWarning(t, warnings, "W040")

	// Verify the warning is about "unknown", not "api".
	for _, w := range warnings {
		if w.Code == "W040" {
			if w.Path != "unknown" {
				t.Errorf("W040 path = %q, want %q", w.Path, "unknown")
			}
		}
	}
}

func TestValidate_W040_NoWarningWhenAllKnown(t *testing.T) {
	cfg := &config.DesiredConfig{
		Services: map[string]*config.DesiredService{
			"api": {Variables: map[string]string{"PORT": "8080"}},
		},
	}
	liveServices := []string{"api", "worker"}
	warnings := config.Validate(cfg, liveServices)
	assertNoWarning(t, warnings, "W040")
}

func TestValidate_W040_SkippedWhenNilLiveServices(t *testing.T) {
	cfg := &config.DesiredConfig{
		Services: map[string]*config.DesiredService{
			"nonexistent": {Variables: map[string]string{"PORT": "8080"}},
		},
	}
	warnings := config.Validate(cfg, nil)
	assertNoWarning(t, warnings, "W040")
}

// --- W041: Nothing actionable ---

func TestValidate_W041_NothingActionable(t *testing.T) {
	cfg := &config.DesiredConfig{
		Services: map[string]*config.DesiredService{},
	}
	warnings := config.Validate(cfg, nil)
	assertHasWarning(t, warnings, "W041")
}

func TestValidate_W041_NilSharedNilServices(t *testing.T) {
	cfg := &config.DesiredConfig{}
	warnings := config.Validate(cfg, nil)
	assertHasWarning(t, warnings, "W041")
}

func TestValidate_W041_NoWarningWithServices(t *testing.T) {
	cfg := &config.DesiredConfig{
		Services: map[string]*config.DesiredService{
			"api": {Variables: map[string]string{"PORT": "8080"}},
		},
	}
	warnings := config.Validate(cfg, nil)
	assertNoWarning(t, warnings, "W041")
}

func TestValidate_W041_NoWarningWithSharedOnly(t *testing.T) {
	cfg := &config.DesiredConfig{
		Shared: &config.DesiredVariables{Vars: map[string]string{
			"APP_ENV": "production",
		}},
	}
	warnings := config.Validate(cfg, nil)
	assertNoWarning(t, warnings, "W041")
}

// --- Suppressed warnings ---

func TestValidate_SuppressedWarnings(t *testing.T) {
	cfg := &config.DesiredConfig{
		SuppressWarnings: []string{"W012", "W030"},
		Services: map[string]*config.DesiredService{
			"api": {Variables: map[string]string{
				"myVar":   "",   // would trigger W030 + W012
				"MY_VAR2": "ok", // clean
			}},
		},
	}
	warnings := config.Validate(cfg, nil)
	assertNoWarning(t, warnings, "W012")
	assertNoWarning(t, warnings, "W030")
}

func TestValidate_SuppressedWarnings_OthersStillEmitted(t *testing.T) {
	cfg := &config.DesiredConfig{
		SuppressWarnings: []string{"W030"},
		Services: map[string]*config.DesiredService{
			"api": {Variables: map[string]string{
				"myVar": "", // W030 suppressed, W012 still emitted
			}},
		},
	}
	warnings := config.Validate(cfg, nil)
	assertNoWarning(t, warnings, "W030")
	assertHasWarning(t, warnings, "W012")
}
