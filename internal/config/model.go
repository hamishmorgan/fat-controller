package config

// LiveConfig represents the live Railway config snapshot used by config get.
type LiveConfig struct {
	ProjectID     string
	EnvironmentID string
	Variables     map[string]string
	Services      map[string]*ServiceConfig
}

// LiveDomain represents a domain attached to a service.
type LiveDomain struct {
	ID         string
	Domain     string
	TargetPort *int
	IsService  bool   // true for service domains, false for custom domains
	Suffix     string // only for service domains
}

// LiveVolume represents a volume instance attached to a service.
type LiveVolume struct {
	ID        string
	VolumeID  string
	Name      string
	MountPath string
	Region    string
}

// LiveTCPProxy represents a TCP proxy for a service.
type LiveTCPProxy struct {
	ID              string
	ApplicationPort int
	ProxyPort       int
	Domain          string
}

// LiveTrigger represents a deployment trigger for a service.
type LiveTrigger struct {
	ID         string
	Branch     string
	Repository string
	Provider   string
}

// LiveEgressGateway represents an egress gateway for a service.
type LiveEgressGateway struct {
	Region string
	IPv4   string
}

// LiveNetworkEndpoint represents a private network endpoint for a service.
type LiveNetworkEndpoint struct {
	ID      string // publicId
	DNSName string
}

// ServiceConfig holds a single service's configuration.
type ServiceConfig struct {
	ID         string
	Name       string
	Icon       string
	Variables  map[string]string
	Deploy     Deploy
	VCPUs      *float64 // live resource limit
	MemoryGB   *float64 // live resource limit
	Domains    []LiveDomain
	Volumes    []LiveVolume
	TCPProxies []LiveTCPProxy
	Triggers   []LiveTrigger
	Egress     []LiveEgressGateway
	Network    *LiveNetworkEndpoint // nil = no private network
}

// Deploy holds service deploy/build settings.
type Deploy struct {
	// Source
	Builder string // Railway Builder enum: NIXPACKS, RAILPACK, etc.
	Repo    *string
	Image   *string

	// Build
	BuildCommand   *string
	DockerfilePath *string
	RootDirectory  *string
	WatchPatterns  []string

	// Pre-deploy
	PreDeployCommand []string // resolved from Railway's *map[string]interface{}

	// Run
	StartCommand *string
	CronSchedule *string

	// Health
	HealthcheckPath         *string
	HealthcheckTimeout      *int
	RestartPolicy           string // Railway RestartPolicyType enum
	RestartPolicyMaxRetries *int

	// Deploy strategy
	DrainingSeconds  *int
	OverlapSeconds   *int
	SleepApplication *bool

	// Placement
	NumReplicas *int
	Region      *string

	// Networking
	IPv6Egress *bool
}
