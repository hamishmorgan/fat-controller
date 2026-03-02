package config

import (
	"strings"
	"testing"
)

func TestRender_TextIncludesServiceAndKey(t *testing.T) {
	cfg := LiveConfig{
		Shared: map[string]string{"SHARED": "1"},
		Services: map[string]*ServiceConfig{
			"api": {Name: "api", Variables: map[string]string{"PORT": "8080"}},
		},
	}

	got, err := Render(cfg, "text", false)
	if err != nil {
		t.Fatalf("Render() error: %v", err)
	}
	if !strings.Contains(got, "[shared_variables]") {
		t.Fatalf("expected shared header, got: %s", got)
	}
	if !strings.Contains(got, "[api.variables]") {
		t.Fatalf("expected service variables header, got: %s", got)
	}
	if !strings.Contains(got, "PORT = \"8080\"") {
		t.Fatalf("expected PORT value, got: %s", got)
	}
}
