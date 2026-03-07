package config_test

import (
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/config"
)

func TestMerge_EmptySlice(t *testing.T) {
	result := config.Merge()
	if result.Variables != nil {
		t.Error("expected nil variables from empty merge")
	}
	if len(result.Services) != 0 {
		t.Error("expected no services from empty merge")
	}
}

func TestMerge_Single(t *testing.T) {
	cfg := &config.DesiredConfig{
		Variables: config.Variables{"A": "1"},
		Services: []*config.DesiredService{
			{Name: "api", Variables: config.Variables{"PORT": "8080"}},
		},
	}
	result := config.Merge(cfg)
	if result.Variables == nil || result.Variables["A"] != "1" {
		t.Error("expected variables A=1")
	}
	if result.Services[0].Variables["PORT"] != "8080" {
		t.Error("expected api PORT=8080")
	}
}

func TestMerge_LaterOverridesEarlier(t *testing.T) {
	base := &config.DesiredConfig{
		Variables: config.Variables{
			"KEEP":     "base",
			"OVERRIDE": "base",
		},
		Services: []*config.DesiredService{
			{Name: "api", Variables: config.Variables{
				"PORT":    "8080",
				"APP_ENV": "staging",
			}},
		},
	}
	local := &config.DesiredConfig{
		Variables: config.Variables{
			"OVERRIDE": "local",
			"NEW":      "local",
		},
		Services: []*config.DesiredService{
			{Name: "api", Variables: config.Variables{
				"APP_ENV": "production",
			}},
			{Name: "worker", Variables: config.Variables{
				"QUEUE": "default",
			}},
		},
	}
	result := config.Merge(base, local)

	// Variables: KEEP preserved, OVERRIDE overridden, NEW added
	if result.Variables["KEEP"] != "base" {
		t.Errorf("KEEP = %q, want base", result.Variables["KEEP"])
	}
	if result.Variables["OVERRIDE"] != "local" {
		t.Errorf("OVERRIDE = %q, want local", result.Variables["OVERRIDE"])
	}
	if result.Variables["NEW"] != "local" {
		t.Errorf("NEW = %q, want local", result.Variables["NEW"])
	}

	// Service api: PORT preserved from base, APP_ENV overridden
	if result.Services[0].Variables["PORT"] != "8080" {
		t.Errorf("api PORT = %q, want 8080", result.Services[0].Variables["PORT"])
	}
	if result.Services[0].Variables["APP_ENV"] != "production" {
		t.Errorf("api APP_ENV = %q, want production", result.Services[0].Variables["APP_ENV"])
	}

	// Service worker: added from local
	var worker *config.DesiredService
	for _, svc := range result.Services {
		if svc.Name == "worker" {
			worker = svc
		}
	}
	if worker == nil {
		t.Fatal("expected worker service from local")
	}
	if worker.Variables["QUEUE"] != "default" {
		t.Error("expected worker QUEUE=default")
	}
}

func TestMerge_ResourcesAndDeployOverride(t *testing.T) {
	vcpus2 := 2.0
	vcpus4 := 4.0
	mem4 := 4.0
	builder := "NIXPACKS"

	base := &config.DesiredConfig{
		Services: []*config.DesiredService{
			{
				Name:      "api",
				Resources: &config.DesiredResources{VCPUs: &vcpus2, MemoryGB: &mem4},
				Deploy:    &config.DesiredDeploy{Builder: &builder},
			},
		},
	}
	override := &config.DesiredConfig{
		Services: []*config.DesiredService{
			{
				Name:      "api",
				Resources: &config.DesiredResources{VCPUs: &vcpus4},
			},
		},
	}
	result := config.Merge(base, override)
	svc := result.Services[0]

	if svc.Resources == nil || svc.Resources.VCPUs == nil || *svc.Resources.VCPUs != 4.0 {
		t.Error("expected VCPUs overridden to 4")
	}
	if svc.Resources.MemoryGB == nil || *svc.Resources.MemoryGB != 4.0 {
		t.Error("expected MemoryGB preserved from base")
	}
	if svc.Deploy == nil || svc.Deploy.Builder == nil || *svc.Deploy.Builder != "NIXPACKS" {
		t.Error("expected Deploy.Builder preserved from base")
	}
}

func TestMerge_VariablesNilInBaseNonNilInOverride(t *testing.T) {
	base := &config.DesiredConfig{Services: []*config.DesiredService{}}
	local := &config.DesiredConfig{
		Variables: config.Variables{"X": "1"},
		Services:  []*config.DesiredService{},
	}
	result := config.Merge(base, local)
	if result.Variables == nil || result.Variables["X"] != "1" {
		t.Error("expected variables X=1 from override")
	}
}

func TestMerge_NameOverride(t *testing.T) {
	base := &config.DesiredConfig{
		Name:     "production",
		Services: []*config.DesiredService{},
	}
	// Local override sets name.
	local := &config.DesiredConfig{
		Name:     "staging",
		Services: []*config.DesiredService{},
	}
	result := config.Merge(base, local)
	if result.Name != "staging" {
		t.Errorf("Name = %q, want %q (overridden by local)", result.Name, "staging")
	}
}

func TestMerge_ToolSettings(t *testing.T) {
	base := &config.DesiredConfig{Tool: &config.ToolSettings{SensitiveKeywords: []string{"SECRET"}}}
	overlay := &config.DesiredConfig{Tool: &config.ToolSettings{SensitiveKeywords: []string{"TOKEN", "KEY"}}}
	result := config.Merge(base, overlay)
	if result.Tool == nil || len(result.Tool.SensitiveKeywords) != 2 || result.Tool.SensitiveKeywords[0] != "TOKEN" {
		t.Errorf("expected overlay tool settings to win")
	}
}

func TestMerge_ToolSettingsFieldLevel(t *testing.T) {
	showSecrets := true
	failFast := false
	base := &config.DesiredConfig{
		Tool: &config.ToolSettings{
			LogLevel:          "debug",
			OutputFormat:      "json",
			ShowSecrets:       &showSecrets,
			SensitiveKeywords: []string{"SECRET", "PASSWORD"},
		},
	}
	overlay := &config.DesiredConfig{
		Tool: &config.ToolSettings{
			OutputFormat:       "toml",
			FailFast:           &failFast,
			SensitiveAllowlist: []string{"KEYSTROKE"},
		},
	}
	result := config.Merge(base, overlay)
	if result.Tool == nil {
		t.Fatal("expected non-nil Tool")
	}
	// LogLevel preserved from base (overlay empty).
	if result.Tool.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want %q", result.Tool.LogLevel, "debug")
	}
	// OutputFormat overridden by overlay.
	if result.Tool.OutputFormat != "toml" {
		t.Errorf("OutputFormat = %q, want %q", result.Tool.OutputFormat, "toml")
	}
	// ShowSecrets preserved from base (overlay nil).
	if result.Tool.ShowSecrets == nil || *result.Tool.ShowSecrets != true {
		t.Error("expected ShowSecrets preserved from base")
	}
	// FailFast set by overlay.
	if result.Tool.FailFast == nil || *result.Tool.FailFast != false {
		t.Error("expected FailFast set by overlay")
	}
	// SensitiveKeywords preserved from base (overlay nil).
	if len(result.Tool.SensitiveKeywords) != 2 {
		t.Errorf("expected SensitiveKeywords preserved, got %v", result.Tool.SensitiveKeywords)
	}
	// SensitiveAllowlist set by overlay.
	if len(result.Tool.SensitiveAllowlist) != 1 || result.Tool.SensitiveAllowlist[0] != "KEYSTROKE" {
		t.Errorf("expected SensitiveAllowlist from overlay, got %v", result.Tool.SensitiveAllowlist)
	}
}

func TestMerge_ToolSettingsBaseNilOverlaySet(t *testing.T) {
	failFast := true
	result := config.Merge(
		&config.DesiredConfig{},
		&config.DesiredConfig{Tool: &config.ToolSettings{LogLevel: "warn", FailFast: &failFast}},
	)
	if result.Tool == nil {
		t.Fatal("expected non-nil Tool from overlay")
	}
	if result.Tool.LogLevel != "warn" {
		t.Errorf("LogLevel = %q, want warn", result.Tool.LogLevel)
	}
	if result.Tool.FailFast == nil || *result.Tool.FailFast != true {
		t.Error("expected FailFast from overlay")
	}
}

func TestMerge_IDBasedMatching(t *testing.T) {
	base := &config.DesiredConfig{
		Services: []*config.DesiredService{
			{Name: "api", ID: "svc-123", Variables: config.Variables{"PORT": "8080"}},
		},
	}
	// Overlay matches by ID even though name differs (e.g. rename).
	overlay := &config.DesiredConfig{
		Services: []*config.DesiredService{
			{Name: "api-renamed", ID: "svc-123", Variables: config.Variables{"HOST": "0.0.0.0"}},
		},
	}
	result := config.Merge(base, overlay)
	if len(result.Services) != 1 {
		t.Fatalf("expected 1 service, got %d", len(result.Services))
	}
	svc := result.Services[0]
	if svc.Name != "api-renamed" {
		t.Errorf("Name = %q, want api-renamed (overlay name should win via ID match)", svc.Name)
	}
	if svc.Variables["PORT"] != "8080" {
		t.Error("expected PORT preserved from base")
	}
	if svc.Variables["HOST"] != "0.0.0.0" {
		t.Error("expected HOST added from overlay")
	}
}

func TestMerge_AllDeployFields(t *testing.T) {
	repo := "github.com/org/repo"
	image := "docker.io/org/img"
	branch := "main"
	builder := "NIXPACKS"
	buildCmd := "npm run build"
	dockerfile := "Dockerfile"
	rootDir := "/app"
	startCmd := "npm start"
	preDeployCmd := "npm run migrate"
	cronSchedule := "0 * * * *"
	healthPath := "/health"
	healthTimeout := 30
	restartPolicy := "always"
	restartMax := 5
	draining := 60
	overlap := 10
	sleepApp := true
	numReplicas := 3
	region := "us-west1"
	ipv6 := true

	base := &config.DesiredConfig{
		Services: []*config.DesiredService{
			{
				Name: "api",
				Deploy: &config.DesiredDeploy{
					Repo:    &repo,
					Builder: &builder,
				},
			},
		},
	}
	overlay := &config.DesiredConfig{
		Services: []*config.DesiredService{
			{
				Name: "api",
				Deploy: &config.DesiredDeploy{
					Image:                   &image,
					Branch:                  &branch,
					BuildCommand:            &buildCmd,
					DockerfilePath:          &dockerfile,
					RootDirectory:           &rootDir,
					StartCommand:            &startCmd,
					CronSchedule:            &cronSchedule,
					HealthcheckPath:         &healthPath,
					HealthcheckTimeout:      &healthTimeout,
					RestartPolicy:           &restartPolicy,
					RestartPolicyMaxRetries: &restartMax,
					DrainingSeconds:         &draining,
					OverlapSeconds:          &overlap,
					SleepApplication:        &sleepApp,
					NumReplicas:             &numReplicas,
					Region:                  &region,
					IPv6Egress:              &ipv6,
				},
			},
		},
	}
	result := config.Merge(base, overlay)
	d := result.Services[0].Deploy

	// Base field preserved.
	if d.Repo == nil || *d.Repo != repo {
		t.Error("expected Repo preserved from base")
	}
	if d.Builder == nil || *d.Builder != builder {
		t.Error("expected Builder preserved from base")
	}
	// Overlay fields applied.
	checks := []struct {
		name string
		ok   bool
	}{
		{"Image", d.Image != nil && *d.Image == image},
		{"Branch", d.Branch != nil && *d.Branch == branch},
		{"BuildCommand", d.BuildCommand != nil && *d.BuildCommand == buildCmd},
		{"DockerfilePath", d.DockerfilePath != nil && *d.DockerfilePath == dockerfile},
		{"RootDirectory", d.RootDirectory != nil && *d.RootDirectory == rootDir},
		{"StartCommand", d.StartCommand != nil && *d.StartCommand == startCmd},
		{"CronSchedule", d.CronSchedule != nil && *d.CronSchedule == cronSchedule},
		{"HealthcheckPath", d.HealthcheckPath != nil && *d.HealthcheckPath == healthPath},
		{"HealthcheckTimeout", d.HealthcheckTimeout != nil && *d.HealthcheckTimeout == healthTimeout},
		{"RestartPolicy", d.RestartPolicy != nil && *d.RestartPolicy == restartPolicy},
		{"RestartPolicyMaxRetries", d.RestartPolicyMaxRetries != nil && *d.RestartPolicyMaxRetries == restartMax},
		{"DrainingSeconds", d.DrainingSeconds != nil && *d.DrainingSeconds == draining},
		{"OverlapSeconds", d.OverlapSeconds != nil && *d.OverlapSeconds == overlap},
		{"SleepApplication", d.SleepApplication != nil && *d.SleepApplication == sleepApp},
		{"NumReplicas", d.NumReplicas != nil && *d.NumReplicas == numReplicas},
		{"Region", d.Region != nil && *d.Region == region},
		{"IPv6Egress", d.IPv6Egress != nil && *d.IPv6Egress == ipv6},
	}
	for _, c := range checks {
		if !c.ok {
			t.Errorf("Deploy.%s not merged correctly", c.name)
		}
	}

	// PreDeployCommand uses any type.
	_ = preDeployCmd
}

func TestMerge_SubResources(t *testing.T) {
	netTrue := true
	base := &config.DesiredConfig{
		Services: []*config.DesiredService{
			{
				Name:       "api",
				Domains:    map[string]config.DomainConfig{"example.com": {Port: intPtr(443)}},
				Volumes:    map[string]config.VolumeConfig{"data": {Mount: "/data"}},
				TCPProxies: []int{5432},
				Network:    &netTrue,
				Triggers:   []config.TriggerConfig{{Branch: "main", Repository: "org/repo"}},
				Egress:     []string{"us-west1"},
				Scale:      map[string]int{"us-west1": 2},
			},
		},
	}
	overlay := &config.DesiredConfig{
		Services: []*config.DesiredService{
			{
				Name:    "api",
				Domains: map[string]config.DomainConfig{"api.example.com": {Port: intPtr(8080)}},
				Volumes: map[string]config.VolumeConfig{"logs": {Mount: "/logs"}},
				Scale:   map[string]int{"us-east1": 1},
			},
		},
	}
	result := config.Merge(base, overlay)
	svc := result.Services[0]

	// Domains merged (both present).
	if len(svc.Domains) != 2 {
		t.Errorf("expected 2 domains, got %d", len(svc.Domains))
	}
	// Volumes merged (both present).
	if len(svc.Volumes) != 2 {
		t.Errorf("expected 2 volumes, got %d", len(svc.Volumes))
	}
	// Scale merged (both present).
	if len(svc.Scale) != 2 {
		t.Errorf("expected 2 scale regions, got %d", len(svc.Scale))
	}
	// TCPProxies preserved from base (overlay nil).
	if len(svc.TCPProxies) != 1 || svc.TCPProxies[0] != 5432 {
		t.Error("expected TCPProxies preserved from base")
	}
	// Network preserved from base (overlay nil).
	if svc.Network == nil || !*svc.Network {
		t.Error("expected Network preserved from base")
	}
	// Triggers preserved from base (overlay nil).
	if len(svc.Triggers) != 1 {
		t.Error("expected Triggers preserved from base")
	}
	// Egress preserved from base (overlay nil).
	if len(svc.Egress) != 1 {
		t.Error("expected Egress preserved from base")
	}
}

func intPtr(v int) *int { return &v }

func TestMerge_NameEmpty_DoesNotOverride(t *testing.T) {
	base := &config.DesiredConfig{
		Name:     "production",
		Services: []*config.DesiredService{},
	}
	// Overlay with empty name should not wipe base values.
	overlay := &config.DesiredConfig{
		Services: []*config.DesiredService{
			{Name: "api", Variables: config.Variables{"PORT": "9090"}},
		},
	}
	result := config.Merge(base, overlay)
	if result.Name != "production" {
		t.Errorf("Name = %q, want %q (empty should not override)", result.Name, "production")
	}
}
