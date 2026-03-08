package cli_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/app"
	"github.com/hamishmorgan/fat-controller/internal/cli"
	"github.com/hamishmorgan/fat-controller/internal/config"
)

// fakeFetcher and capturingFetcher are defined in helpers_test.go.

func TestRunConfigGet_RendersText(t *testing.T) {
	fetcher := &fakeFetcher{
		cfg: &config.LiveConfig{
			ProjectID:     "proj-1",
			EnvironmentID: "env-1",
			Variables:     map[string]string{"FOO": "bar"},
			Services: map[string]*config.ServiceConfig{
				"api": {
					ID:        "svc-1",
					Name:      "api",
					Variables: map[string]string{"PORT": "8080"},
				},
			},
		},
	}

	var buf bytes.Buffer
	globals := &cli.Globals{Output: "text"}
	err := cli.RunConfigGet(context.Background(), globals, "", "", "", "", false, "", false, fetcher, &buf)
	if err != nil {
		t.Fatalf("RunConfigGet() error: %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "FOO") {
		t.Errorf("expected output to contain FOO, got:\n%s", got)
	}
	if !strings.Contains(got, "PORT") {
		t.Errorf("expected output to contain PORT, got:\n%s", got)
	}
}

func TestRunConfigGet_RendersJSON(t *testing.T) {
	fetcher := &fakeFetcher{
		cfg: &config.LiveConfig{
			ProjectID:     "proj-1",
			EnvironmentID: "env-1",
			Variables:     map[string]string{"DB": "postgres"},
			Services:      map[string]*config.ServiceConfig{},
		},
	}

	var buf bytes.Buffer
	globals := &cli.Globals{Output: "json"}
	err := cli.RunConfigGet(context.Background(), globals, "", "", "", "", false, "", false, fetcher, &buf)
	if err != nil {
		t.Fatalf("RunConfigGet() error: %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, `"DB"`) {
		t.Errorf("expected JSON output to contain DB key, got:\n%s", got)
	}
}

func TestRunConfigGet_RawRequiresScalar(t *testing.T) {
	fetcher := &fakeFetcher{
		cfg: &config.LiveConfig{
			Variables: map[string]string{"A": "1"},
		},
	}
	var buf bytes.Buffer
	globals := &cli.Globals{Output: "raw"}
	err := cli.RunConfigGet(context.Background(), globals, "", "", "", "", false, "", false, fetcher, &buf)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRunConfigGet_RawScalarOutputsBareValue(t *testing.T) {
	fetcher := &fakeFetcher{
		cfg: &config.LiveConfig{
			Variables: map[string]string{"A": "1"},
		},
	}
	var buf bytes.Buffer
	globals := &cli.Globals{Output: "raw"}
	err := cli.RunConfigGet(context.Background(), globals, "", "", "", "shared.variables.A", false, "", true, fetcher, &buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := buf.String(), "1\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestRunConfigGet_PathExtractsService(t *testing.T) {
	var fetchedService string
	fetcher := &fakeFetcher{
		cfg: &config.LiveConfig{
			ProjectID:     "proj-1",
			EnvironmentID: "env-1",
			Variables:     map[string]string{},
			Services: map[string]*config.ServiceConfig{
				"api": {
					Name:      "api",
					Variables: map[string]string{"PORT": "8080"},
				},
			},
		},
	}
	// Wrap to capture the service argument.
	wrapper := &serviceCaptureFetcher{inner: fetcher, captured: &fetchedService}

	var buf bytes.Buffer
	globals := &cli.Globals{Output: "text"}
	err := cli.RunConfigGet(context.Background(), globals, "", "", "", "api.variables.PORT", false, "", false, wrapper, &buf)
	if err != nil {
		t.Fatalf("RunConfigGet() error: %v", err)
	}
	if fetchedService != "api" {
		t.Errorf("expected service=api, got %q", fetchedService)
	}
}

// serviceCaptureFetcher wraps a fakeFetcher to capture the service arg passed to Fetch.
type serviceCaptureFetcher struct {
	inner    *fakeFetcher
	captured *string
}

func (s *serviceCaptureFetcher) Resolve(ctx context.Context, workspace, project, environment string) (*app.ResolvedIdentity, error) {
	return s.inner.Resolve(ctx, workspace, project, environment)
}

func (s *serviceCaptureFetcher) Fetch(ctx context.Context, projectID, environmentID string, services []string) (*config.LiveConfig, error) {
	if len(services) > 0 {
		*s.captured = services[0]
	}
	return s.inner.Fetch(ctx, projectID, environmentID, services)
}

func TestRunConfigGet_ResolveError(t *testing.T) {
	fetcher := &fakeFetcher{resolveErr: errors.New("no project")}
	var buf bytes.Buffer
	err := cli.RunConfigGet(context.Background(), &cli.Globals{}, "", "", "", "", false, "", false, fetcher, &buf)
	if err == nil {
		t.Fatal("expected error from resolve failure")
	}
	if !strings.Contains(err.Error(), "no project") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunConfigGet_FetchError(t *testing.T) {
	fetcher := &fakeFetcher{fetchErr: errors.New("api error")}
	var buf bytes.Buffer
	err := cli.RunConfigGet(context.Background(), &cli.Globals{}, "", "", "", "", false, "", false, fetcher, &buf)
	if err == nil {
		t.Fatal("expected error from fetch failure")
	}
}

func TestRunConfigGet_NilConfig(t *testing.T) {
	fetcher := &fakeFetcher{cfg: nil}
	var buf bytes.Buffer
	err := cli.RunConfigGet(context.Background(), &cli.Globals{}, "", "", "", "", false, "", false, fetcher, &buf)
	if err == nil {
		t.Fatal("expected error for nil config")
	}
}

func TestRunConfigGet_FiltersByPathSectionAndKey(t *testing.T) {
	fetcher := &fakeFetcher{
		cfg: &config.LiveConfig{
			Services: map[string]*config.ServiceConfig{
				"api": {
					Name: "api",
					Variables: map[string]string{
						"PORT":  "8080",
						"DEBUG": "false",
					},
				},
			},
		},
	}
	var buf bytes.Buffer
	globals := &cli.Globals{Output: "text"}
	err := cli.RunConfigGet(context.Background(), globals, "", "", "", "api.variables.PORT", false, "", true, fetcher, &buf)
	if err != nil {
		t.Fatal(err)
	}
	output := buf.String()
	if !strings.Contains(output, "8080") {
		t.Errorf("expected PORT value: %s", output)
	}
	if strings.Contains(output, "DEBUG") {
		t.Errorf("should not contain other variables: %s", output)
	}
}

func TestRunConfigGet_FiltersByPathSection(t *testing.T) {
	fetcher := &fakeFetcher{
		cfg: &config.LiveConfig{
			Variables: map[string]string{"GLOBAL": "yes"},
			Services: map[string]*config.ServiceConfig{
				"api": {
					Name: "api",
					Variables: map[string]string{
						"PORT":  "8080",
						"DEBUG": "false",
					},
				},
			},
		},
	}
	var buf bytes.Buffer
	globals := &cli.Globals{Output: "text"}
	err := cli.RunConfigGet(context.Background(), globals, "", "", "", "api.variables", false, "", true, fetcher, &buf)
	if err != nil {
		t.Fatal(err)
	}
	output := buf.String()
	if !strings.Contains(output, "PORT") {
		t.Errorf("expected PORT in section output: %s", output)
	}
	if !strings.Contains(output, "DEBUG") {
		t.Errorf("expected DEBUG in section output: %s", output)
	}
	if strings.Contains(output, "GLOBAL") {
		t.Errorf("should not contain shared variables: %s", output)
	}
}

func TestRunConfigGet_SharedVariablesKeyLookup(t *testing.T) {
	fetcher := &fakeFetcher{
		cfg: &config.LiveConfig{
			Variables: map[string]string{
				"GLOBAL": "yes",
			},
			Services: map[string]*config.ServiceConfig{},
		},
	}
	var buf bytes.Buffer
	globals := &cli.Globals{Output: "text"}
	err := cli.RunConfigGet(context.Background(), globals, "", "", "", "shared.variables.GLOBAL", false, "", true, fetcher, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(buf.String()); got != "yes" {
		t.Fatalf("expected shared variable value 'yes', got %q", got)
	}
}

func TestRunConfigGet_SharedVariablesSectionLookup(t *testing.T) {
	fetcher := &fakeFetcher{
		cfg: &config.LiveConfig{
			Variables: map[string]string{
				"A": "1",
				"B": "2",
			},
			Services: map[string]*config.ServiceConfig{
				"api": {Name: "api", Variables: map[string]string{"PORT": "8080"}},
			},
		},
	}
	var buf bytes.Buffer
	globals := &cli.Globals{Output: "text"}
	err := cli.RunConfigGet(context.Background(), globals, "", "", "", "shared.variables", false, "", true, fetcher, &buf)
	if err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "A") || !strings.Contains(out, "B") {
		t.Fatalf("expected shared section output to include A and B, got:\n%s", out)
	}
	if strings.Contains(out, "PORT") {
		t.Fatalf("expected shared section output to exclude service vars, got:\n%s", out)
	}
}

func TestRunConfigGet_MasksSecretsByDefault(t *testing.T) {
	fetcher := &fakeFetcher{
		cfg: &config.LiveConfig{
			ProjectID:     "proj-1",
			EnvironmentID: "env-1",
			Variables:     map[string]string{"DATABASE_PASSWORD": "hunter2"},
			Services:      map[string]*config.ServiceConfig{},
		},
	}
	var buf bytes.Buffer
	globals := &cli.Globals{Output: "text"}
	err := cli.RunConfigGet(context.Background(), globals, "", "", "", "", false, "", false, fetcher, &buf)
	if err != nil {
		t.Fatalf("RunConfigGet() error: %v", err)
	}
	got := buf.String()
	if strings.Contains(got, "hunter2") {
		t.Errorf("password should be masked by default, got:\n%s", got)
	}
	if !strings.Contains(got, "********") {
		t.Errorf("expected masked placeholder, got:\n%s", got)
	}
}

func TestRunConfigGet_ShowSecretsRevealsValues(t *testing.T) {
	fetcher := &fakeFetcher{
		cfg: &config.LiveConfig{
			ProjectID:     "proj-1",
			EnvironmentID: "env-1",
			Variables:     map[string]string{"DATABASE_PASSWORD": "hunter2"},
			Services:      map[string]*config.ServiceConfig{},
		},
	}
	var buf bytes.Buffer
	globals := &cli.Globals{Output: "text"}
	err := cli.RunConfigGet(context.Background(), globals, "", "", "", "", false, "", true, fetcher, &buf)
	if err != nil {
		t.Fatalf("RunConfigGet() error: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "hunter2") {
		t.Errorf("--show-secrets should reveal password, got:\n%s", got)
	}
}
