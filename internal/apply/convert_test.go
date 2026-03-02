package apply_test

import (
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/apply"
	"github.com/hamishmorgan/fat-controller/internal/config"
	"github.com/hamishmorgan/fat-controller/internal/railway"
)

func TestToServiceInstanceUpdateInput_AllFields(t *testing.T) {
	builder := "NIXPACKS"
	start := "npm start"
	docker := "./Dockerfile"
	root := "/app"
	health := "/health"
	desired := &config.DesiredDeploy{
		Builder:         &builder,
		StartCommand:    &start,
		DockerfilePath:  &docker,
		RootDirectory:   &root,
		HealthcheckPath: &health,
	}

	input, err := apply.ToServiceInstanceUpdateInput(desired)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if input.Builder == nil || *input.Builder != railway.BuilderNixpacks {
		t.Errorf("Builder = %v, want NIXPACKS", input.Builder)
	}
	if input.StartCommand == nil || *input.StartCommand != "npm start" {
		t.Errorf("StartCommand = %v, want 'npm start'", input.StartCommand)
	}
	if input.DockerfilePath == nil || *input.DockerfilePath != "./Dockerfile" {
		t.Errorf("DockerfilePath = %v, want './Dockerfile'", input.DockerfilePath)
	}
	if input.RootDirectory == nil || *input.RootDirectory != "/app" {
		t.Errorf("RootDirectory = %v, want '/app'", input.RootDirectory)
	}
	if input.HealthcheckPath == nil || *input.HealthcheckPath != "/health" {
		t.Errorf("HealthcheckPath = %v, want '/health'", input.HealthcheckPath)
	}
}

func TestToServiceInstanceUpdateInput_BuilderCaseInsensitive(t *testing.T) {
	tests := []struct {
		input string
		want  railway.Builder
	}{
		{"nixpacks", railway.BuilderNixpacks},
		{"NIXPACKS", railway.BuilderNixpacks},
		{"Railpack", railway.BuilderRailpack},
		{"PAKETO", railway.BuilderPaketo},
		{"heroku", railway.BuilderHeroku},
	}
	for _, tt := range tests {
		desired := &config.DesiredDeploy{Builder: &tt.input}
		input, err := apply.ToServiceInstanceUpdateInput(desired)
		if err != nil {
			t.Errorf("builder %q: error: %v", tt.input, err)
			continue
		}
		if input.Builder == nil || *input.Builder != tt.want {
			t.Errorf("builder %q: got %v, want %v", tt.input, input.Builder, tt.want)
		}
	}
}

func TestToServiceInstanceUpdateInput_UnknownBuilderErrors(t *testing.T) {
	builder := "UNKNOWN"
	desired := &config.DesiredDeploy{Builder: &builder}
	_, err := apply.ToServiceInstanceUpdateInput(desired)
	if err == nil {
		t.Fatal("expected error for unknown builder")
	}
}

func TestToServiceInstanceUpdateInput_NilDesired(t *testing.T) {
	input, err := apply.ToServiceInstanceUpdateInput(nil)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	// All fields should be nil/zero.
	if input.Builder != nil {
		t.Error("expected nil Builder")
	}
}

func TestToServiceInstanceUpdateInput_PartialFields(t *testing.T) {
	start := "go run ."
	desired := &config.DesiredDeploy{StartCommand: &start}
	input, err := apply.ToServiceInstanceUpdateInput(desired)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if input.Builder != nil {
		t.Error("Builder should be nil when not specified")
	}
	if input.StartCommand == nil || *input.StartCommand != "go run ." {
		t.Errorf("StartCommand = %v, want 'go run .'", input.StartCommand)
	}
}
