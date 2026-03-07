package config_test

import (
	"os"
	"path/filepath"
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

// --- W002: Unknown key in service block ---

func TestValidate_W002_UnknownServiceKey(t *testing.T) {
	cfg := &config.DesiredConfig{
		Services: map[string]*config.DesiredService{
			"api": {
				Variables:   map[string]string{"PORT": "8080"},
				UnknownKeys: []string{"foo"},
			},
		},
	}
	warnings := config.Validate(cfg, nil)
	assertHasWarning(t, warnings, "W002")
	for _, w := range warnings {
		if w.Code == "W002" {
			if w.Path != "api.foo" {
				t.Errorf("W002 path = %q, want %q", w.Path, "api.foo")
			}
		}
	}
}

func TestValidate_W002_NoWarningWithKnownKeys(t *testing.T) {
	cfg := &config.DesiredConfig{
		Services: map[string]*config.DesiredService{
			"api": {
				Variables: map[string]string{"PORT": "8080"},
				// No unknown keys
			},
		},
	}
	warnings := config.Validate(cfg, nil)
	assertNoWarning(t, warnings, "W002")
}

// --- W021: Variable overridden by local file ---

func TestValidate_W021_VariableOverriddenByLocal(t *testing.T) {
	cfg := &config.DesiredConfig{
		Services: map[string]*config.DesiredService{
			"api": {Variables: map[string]string{"PORT": "4000"}},
		},
		Overrides: []config.Override{
			{Path: "api.variables.PORT", Source: "local override"},
		},
	}
	warnings := config.Validate(cfg, nil)
	assertHasWarning(t, warnings, "W021")
}

func TestValidate_W021_NoOverridesNoWarning(t *testing.T) {
	cfg := &config.DesiredConfig{
		Services: map[string]*config.DesiredService{
			"api": {Variables: map[string]string{"PORT": "4000"}},
		},
	}
	warnings := config.Validate(cfg, nil)
	assertNoWarning(t, warnings, "W021")
}

// --- W031: Invalid variable name characters ---

func TestValidate_W031_SpaceInVarName(t *testing.T) {
	cfg := &config.DesiredConfig{
		Services: map[string]*config.DesiredService{
			"api": {Variables: map[string]string{
				"MY VAR": "value",
			}},
		},
	}
	warnings := config.Validate(cfg, nil)
	assertHasWarning(t, warnings, "W031")
}

func TestValidate_W031_SpecialCharsInVarName(t *testing.T) {
	cfg := &config.DesiredConfig{
		Services: map[string]*config.DesiredService{
			"api": {Variables: map[string]string{
				"PORT@8080": "value",
			}},
		},
	}
	warnings := config.Validate(cfg, nil)
	assertHasWarning(t, warnings, "W031")
}

func TestValidate_W031_ValidNameNoWarning(t *testing.T) {
	cfg := &config.DesiredConfig{
		Services: map[string]*config.DesiredService{
			"api": {Variables: map[string]string{
				"MY_VAR_123": "value",
			}},
		},
	}
	warnings := config.Validate(cfg, nil)
	assertNoWarning(t, warnings, "W031")
}

// --- W050: Hardcoded secret in config ---

func TestValidate_W050_HardcodedSecret(t *testing.T) {
	cfg := &config.DesiredConfig{
		Services: map[string]*config.DesiredService{
			"api": {Variables: map[string]string{
				// ENCRYPTION_KEY matches sensitive keyword → masker returns MaskedValue
				// regardless of value content. Short value avoids gitleaks false positive.
				"ENCRYPTION_KEY": "my-secret-value",
			}},
		},
	}
	warnings := config.Validate(cfg, nil)
	assertHasWarning(t, warnings, "W050")
}

func TestValidate_W050_InterpolatedNoWarning(t *testing.T) {
	cfg := &config.DesiredConfig{
		Services: map[string]*config.DesiredService{
			"api": {Variables: map[string]string{
				"API_KEY": "${API_KEY}",
			}},
		},
	}
	warnings := config.Validate(cfg, nil)
	assertNoWarning(t, warnings, "W050")
}

func TestValidate_W050_NonSensitiveNameNoWarning(t *testing.T) {
	cfg := &config.DesiredConfig{
		Services: map[string]*config.DesiredService{
			"api": {Variables: map[string]string{
				"PORT": "8080",
			}},
		},
	}
	warnings := config.Validate(cfg, nil)
	assertNoWarning(t, warnings, "W050")
}

// --- W051: Local override file not gitignored ---

func TestValidateFiles_W051_LocalNotGitignored(t *testing.T) {
	dir := t.TempDir()
	// Create local config file but no .gitignore.
	if err := os.WriteFile(filepath.Join(dir, config.LocalConfigFile), []byte("# local"), 0o644); err != nil {
		t.Fatal(err)
	}
	warnings := config.ValidateFiles(dir)
	assertHasWarning(t, warnings, "W051")
}

func TestValidateFiles_W051_GitignoreContainsLocal(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, config.LocalConfigFile), []byte("# local"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(config.LocalConfigFile+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	warnings := config.ValidateFiles(dir)
	assertNoWarning(t, warnings, "W051")
}

func TestValidateFiles_W051_NoLocalFile(t *testing.T) {
	dir := t.TempDir()
	warnings := config.ValidateFiles(dir)
	assertNoWarning(t, warnings, "W051")
}

func TestValidateFiles_W051_GitignoreWithoutLocal(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, config.LocalConfigFile), []byte("# local"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("node_modules\n.env\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	warnings := config.ValidateFiles(dir)
	assertHasWarning(t, warnings, "W051")
}

// --- W060: Reference to unknown service ---

func TestValidate_W060_UnknownServiceRef(t *testing.T) {
	cfg := &config.DesiredConfig{
		Services: map[string]*config.DesiredService{
			"api": {Variables: map[string]string{
				"DB_URL": "${{postgres.DATABASE_URL}}",
			}},
		},
	}
	warnings := config.Validate(cfg, nil)
	assertHasWarning(t, warnings, "W060")
}

func TestValidate_W060_KnownServiceRefNoWarning(t *testing.T) {
	cfg := &config.DesiredConfig{
		Services: map[string]*config.DesiredService{
			"api": {Variables: map[string]string{
				"DB_URL": "${{postgres.DATABASE_URL}}",
			}},
			"postgres": {Variables: map[string]string{"PORT": "5432"}},
		},
	}
	warnings := config.Validate(cfg, nil)
	assertNoWarning(t, warnings, "W060")
}

func TestValidate_W060_SharedVarUnknownServiceRef(t *testing.T) {
	cfg := &config.DesiredConfig{
		Shared: &config.DesiredVariables{Vars: map[string]string{
			"DB_URL": "${{postgres.DATABASE_URL}}",
		}},
		Services: map[string]*config.DesiredService{
			"api": {Variables: map[string]string{"PORT": "8080"}},
		},
	}
	warnings := config.Validate(cfg, nil)
	assertHasWarning(t, warnings, "W060")
}

// --- W002 via parser integration ---

func TestParse_W002_UnknownServiceKey(t *testing.T) {
	data := []byte(`
[api]
foo = "bar"

[api.variables]
PORT = "8080"
`)
	cfg, err := config.Parse(data)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	svc := cfg.Services["api"]
	if len(svc.UnknownKeys) == 0 {
		t.Fatal("expected UnknownKeys to contain 'foo'")
	}
	found := false
	for _, k := range svc.UnknownKeys {
		if k == "foo" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'foo' in UnknownKeys, got %v", svc.UnknownKeys)
	}
}
