package config

// LiveConfig represents the live Railway config snapshot used by config get.
type LiveConfig struct {
	ProjectID     string
	EnvironmentID string
	Shared        map[string]string
	Services      map[string]*ServiceConfig
}

// ServiceConfig holds a single service's configuration.
type ServiceConfig struct {
	ID        string
	Name      string
	Variables map[string]string
	Deploy    Deploy
}

// Deploy holds service deploy/build settings.
type Deploy struct {
	Builder         string // Railway Builder enum: NIXPACKS, RAILPACK, etc.
	DockerfilePath  *string
	RootDirectory   *string
	StartCommand    *string
	HealthcheckPath *string
}
