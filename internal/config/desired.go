package config

// Override records that a variable was overridden by a later config file.
type Override struct {
	Path   string // dot-path e.g. "api.variables.PORT"
	Source string // e.g. "extra.toml"
}

// ContextBlock identifies a workspace or project by name and optional ID.
type ContextBlock struct {
	Name string `toml:"name"`
	ID   string `toml:"id,omitempty"`
}

// ToolSettings holds tool behavior configuration (how fat-controller
// behaves, not what it manages).
type ToolSettings struct {
	APITimeout         string   `toml:"api_timeout,omitempty"`
	LogLevel           string   `toml:"log_level,omitempty"`
	OutputFormat       string   `toml:"output_format,omitempty"`
	OutputColor        string   `toml:"output_color,omitempty"`
	Prompt             string   `toml:"prompt,omitempty"`
	Deploy             string   `toml:"deploy,omitempty"`
	ShowSecrets        *bool    `toml:"show_secrets,omitempty"`
	FailFast           *bool    `toml:"fail_fast,omitempty"`
	AllowCreate        *bool    `toml:"allow_create,omitempty"`
	AllowUpdate        *bool    `toml:"allow_update,omitempty"`
	AllowDelete        *bool    `toml:"allow_delete,omitempty"`
	EnvFile            any      `toml:"env_file,omitempty"` // string or []string
	SensitiveKeywords  []string `toml:"sensitive_keywords,omitempty"`
	SensitiveAllowlist []string `toml:"sensitive_allowlist,omitempty"`
	SuppressWarnings   []string `toml:"suppress_warnings,omitempty"`
}

// DesiredDeploy holds deploy settings for a service.
// Pointer fields: nil = "not specified, don't touch".
type DesiredDeploy struct {
	// Source
	Repo                *string              `toml:"repo,omitempty"`
	Image               *string              `toml:"image,omitempty"`
	Branch              *string              `toml:"branch,omitempty"`
	RegistryCredentials *RegistryCredentials `toml:"registry_credentials,omitempty"`

	// Build
	Builder        *string  `toml:"builder,omitempty"`
	BuildCommand   *string  `toml:"build_command,omitempty"`
	DockerfilePath *string  `toml:"dockerfile_path,omitempty"`
	RootDirectory  *string  `toml:"root_directory,omitempty"`
	NixpacksPlan   any      `toml:"nixpacks_plan,omitempty"` // inline table, passed as-is
	WatchPatterns  []string `toml:"watch_patterns,omitempty"`

	// Run
	StartCommand     *string `toml:"start_command,omitempty"`
	PreDeployCommand any     `toml:"pre_deploy_command,omitempty"` // string or []string
	CronSchedule     *string `toml:"cron_schedule,omitempty"`

	// Health
	HealthcheckPath         *string `toml:"healthcheck_path,omitempty"`
	HealthcheckTimeout      *int    `toml:"healthcheck_timeout,omitempty"`
	RestartPolicy           *string `toml:"restart_policy,omitempty"`
	RestartPolicyMaxRetries *int    `toml:"restart_policy_max_retries,omitempty"`

	// Deploy strategy
	DrainingSeconds  *int  `toml:"draining_seconds,omitempty"`
	OverlapSeconds   *int  `toml:"overlap_seconds,omitempty"`
	SleepApplication *bool `toml:"sleep_application,omitempty"`

	// Placement (single-region shorthand)
	NumReplicas *int    `toml:"num_replicas,omitempty"`
	Region      *string `toml:"region,omitempty"`

	// Networking
	IPv6Egress *bool `toml:"ipv6_egress,omitempty"`
}

// RegistryCredentials holds Docker registry auth.
type RegistryCredentials struct {
	Username string `toml:"username"`
	Password string `toml:"password"`
}

// DesiredResources holds resource limits for a service.
type DesiredResources struct {
	VCPUs    *float64 `toml:"vcpus,omitempty"`
	MemoryGB *float64 `toml:"memory_gb,omitempty"`
}

// DomainConfig holds settings for a custom or service domain.
type DomainConfig struct {
	Port   *int `toml:"port,omitempty"`
	Delete bool `toml:"delete,omitempty"`
}

// VolumeConfig holds settings for an attached or unattached volume.
type VolumeConfig struct {
	Mount  string `toml:"mount"`
	Region string `toml:"region,omitempty"`
	Delete bool   `toml:"delete,omitempty"`
}

// TriggerConfig holds deployment trigger settings.
type TriggerConfig struct {
	Branch        string `toml:"branch"`
	Repository    string `toml:"repository"`
	Provider      string `toml:"provider,omitempty"`
	CheckSuites   *bool  `toml:"check_suites,omitempty"`
	RootDirectory string `toml:"root_directory,omitempty"`
}

// DesiredService declares a service within the environment.
type DesiredService struct {
	Name       string                  `toml:"name"`
	ID         string                  `toml:"id,omitempty"`
	Icon       string                  `toml:"icon,omitempty"`
	Delete     bool                    `toml:"delete,omitempty"`
	Variables  map[string]string       `toml:"variables,omitempty"`
	Deploy     *DesiredDeploy          `toml:"deploy,omitempty"`
	Resources  *DesiredResources       `toml:"resources,omitempty"`
	Scale      map[string]int          `toml:"scale,omitempty"`
	Domains    map[string]DomainConfig `toml:"domains,omitempty"`
	Volumes    map[string]VolumeConfig `toml:"volumes,omitempty"`
	TCPProxies []int                   `toml:"tcp_proxies,omitempty"`
	Network    *bool                   `toml:"network,omitempty"`
	Triggers   []TriggerConfig         `toml:"triggers,omitempty"`
	Egress     []string                `toml:"egress,omitempty"`
}

// DesiredConfig is the top-level config — one file, one environment.
type DesiredConfig struct {
	Name      string                  `toml:"name,omitempty"`
	ID        string                  `toml:"id,omitempty"`
	Variables map[string]string       `toml:"variables,omitempty"`
	Volumes   map[string]VolumeConfig `toml:"volumes,omitempty"`
	Buckets   []string                `toml:"buckets,omitempty"`
	Tool      *ToolSettings           `toml:"tool,omitempty"`
	Workspace *ContextBlock           `toml:"workspace,omitempty"`
	Project   *ContextBlock           `toml:"project,omitempty"`
	Services  []*DesiredService       `toml:"service,omitempty"`

	Overrides []Override `toml:"-"` // populated by LoadConfigs, checked by Validate
}
