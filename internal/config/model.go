package config

// LiveConfig represents the live Railway config snapshot used by config get.
type LiveConfig struct {
	ProjectID     string
	EnvironmentID string
	Variables     map[string]string
	Services      map[string]*ServiceConfig
}

// ServiceConfig holds a single service's configuration.
type ServiceConfig struct {
	ID        string
	Name      string
	Variables map[string]string
	Deploy    Deploy
	VCPUs     *float64 // live resource limit
	MemoryGB  *float64 // live resource limit
}

// Deploy holds service deploy/build settings.
type Deploy struct {
	Builder         string // Railway Builder enum: NIXPACKS, RAILPACK, etc.
	DockerfilePath  *string
	RootDirectory   *string
	StartCommand    *string
	HealthcheckPath *string
}
