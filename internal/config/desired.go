package config

// DesiredConfig represents the desired state parsed from fat-controller.toml.
// It contains only the fields the user explicitly specified — omitted fields
// mean "don't touch".
type DesiredConfig struct {
	Shared   *DesiredVariables          // nil means no shared section in config
	Services map[string]*DesiredService // keyed by service name
}

// DesiredService holds one service's desired configuration.
type DesiredService struct {
	Variables map[string]string // nil means no [svc.variables] section
	Resources *DesiredResources // nil means no [svc.resources] section
	Deploy    *DesiredDeploy    // nil means no [svc.deploy] section
}

// DesiredVariables holds shared or per-service variables from config.
type DesiredVariables struct {
	Vars map[string]string
}

// DesiredResources holds resource limit overrides.
type DesiredResources struct {
	VCPUs    *float64 `toml:"vcpus"`
	MemoryGB *float64 `toml:"memory_gb"`
}

// DesiredDeploy holds deploy/build setting overrides.
// Pointer fields — nil means "not specified, don't touch".
type DesiredDeploy struct {
	Builder         *string `toml:"builder"`
	DockerfilePath  *string `toml:"dockerfile_path"`
	RootDirectory   *string `toml:"root_directory"`
	StartCommand    *string `toml:"start_command"`
	HealthcheckPath *string `toml:"healthcheck_path"`
}
