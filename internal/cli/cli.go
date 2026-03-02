package cli

import (
	"fmt"
	"io"
	"time"
)

// Globals holds values that are available to every command's Run() method.
// Kong tags are here so CLI can embed Globals directly.
type Globals struct {
	Token       string        `help:"Auth token (overrides all other auth). Env vars RAILWAY_API_TOKEN and RAILWAY_TOKEN are also supported — see docs/COMMANDS.md for precedence."`
	Workspace   string        `help:"Workspace ID or name." env:"FAT_CONTROLLER_WORKSPACE"`
	Project     string        `help:"Project ID or name." env:"FAT_CONTROLLER_PROJECT"`
	Environment string        `help:"Environment name." env:"FAT_CONTROLLER_ENVIRONMENT"`
	Output      string        `help:"Output format: text, json, toml." enum:"text,json,toml" default:"text" short:"o" env:"FAT_CONTROLLER_OUTPUT"`
	Color       string        `help:"Color mode: auto, always, never." enum:"auto,always,never" default:"auto" env:"FAT_CONTROLLER_COLOR"`
	Timeout     time.Duration `help:"API request timeout." default:"30s" env:"FAT_CONTROLLER_TIMEOUT"`
	Confirm     bool          `help:"Auto-execute mutations (skip confirmation)." env:"FAT_CONTROLLER_CONFIRM"`
	DryRun      bool          `help:"Force preview of mutations." name:"dry-run" env:"FAT_CONTROLLER_DRY_RUN"`
	ConfigFiles []string      `help:"Railway config file paths. Repeatable." name:"config" short:"c" env:"FAT_CONTROLLER_CONFIG" sep:"none"`
	Service     string        `help:"Scope to a single service." env:"FAT_CONTROLLER_SERVICE"`
	SkipDeploys bool          `help:"Don't trigger redeployments." name:"skip-deploys" env:"FAT_CONTROLLER_SKIP_DEPLOYS"`
	FailFast    bool          `help:"Stop on first error during apply." name:"fail-fast" env:"FAT_CONTROLLER_FAIL_FAST"`
	ShowSecrets bool          `help:"Show secret values instead of masking." name:"show-secrets" env:"FAT_CONTROLLER_SHOW_SECRETS"`
	Full        bool          `help:"Include IDs and read-only fields (get only)."`
	Verbose     bool          `help:"Debug output (HTTP requests, timing)." short:"v"`
	Quiet       bool          `help:"Suppress informational output." short:"q"`
}

// CLI is the root struct for the kong CLI parser.
// Global flags come from the embedded Globals; subcommand groups are nested structs.
type CLI struct {
	Globals `kong:"embed"`

	// Subcommand groups
	Auth        AuthCmd        `cmd:"" help:"Manage authentication."`
	Config      ConfigCmd      `cmd:"" name:"config" help:"Declarative configuration management."`
	Project     ProjectCmd     `cmd:"" help:"Manage projects."`
	Environment EnvironmentCmd `cmd:"" help:"Manage environments."`
	Workspace   WorkspaceCmd   `cmd:"" help:"Manage workspaces."`
}

// AuthCmd is the `auth` command group.
type AuthCmd struct {
	Login  AuthLoginCmd  `cmd:"" help:"Log in to Railway via browser-based OAuth."`
	Logout AuthLogoutCmd `cmd:"" help:"Clear stored credentials."`
	Status AuthStatusCmd `cmd:"" help:"Show current authentication status."`
}

// AuthLoginCmd implements `auth login`.
type AuthLoginCmd struct{}

// AuthLogoutCmd implements `auth logout`.
type AuthLogoutCmd struct{}

// AuthStatusCmd implements `auth status`.
type AuthStatusCmd struct{}

// ConfigCmd is the `config` command group.
type ConfigCmd struct {
	Get      ConfigGetCmd      `cmd:"" help:"Fetch live config from Railway."`
	Set      ConfigSetCmd      `cmd:"" help:"Set a single value by dot-path."`
	Delete   ConfigDeleteCmd   `cmd:"" help:"Delete a single value by dot-path."`
	Diff     ConfigDiffCmd     `cmd:"" help:"Compare local config against live state."`
	Apply    ConfigApplyCmd    `cmd:"" help:"Push configuration changes to Railway."`
	Validate ConfigValidateCmd `cmd:"" help:"Check config file for warnings (no API calls)."`
}

// ConfigGetCmd implements `config get`.
type ConfigGetCmd struct {
	Path   string    `arg:"" optional:"" help:"Dot-path to fetch (e.g. api.variables.PORT). Omit for all."`
	output io.Writer `kong:"-"`
}

// ConfigSetCmd implements `config set`.
type ConfigSetCmd struct {
	Path  string `arg:"" required:"" help:"Dot-path to set (e.g. api.variables.PORT)."`
	Value string `arg:"" required:"" help:"Value to set."`
}

// ConfigDeleteCmd implements `config delete`.
type ConfigDeleteCmd struct {
	Path string `arg:"" required:"" help:"Dot-path to delete (e.g. api.variables.OLD)."`
}

type ConfigDiffCmd struct{}
type ConfigApplyCmd struct{}
type ConfigValidateCmd struct{}

type ProjectCmd struct {
	List ProjectListCmd `cmd:"" help:"List available projects."`
}

type ProjectListCmd struct{}

type EnvironmentCmd struct {
	List EnvironmentListCmd `cmd:"" help:"List environments for a project."`
}

type EnvironmentListCmd struct{}

type WorkspaceCmd struct {
	List WorkspaceListCmd `cmd:"" help:"List available workspaces."`
}

type WorkspaceListCmd struct{}

// Run methods:
// - ConfigGetCmd.Run    → config_get.go
// - ConfigSetCmd.Run    → config_set.go
// - ConfigDeleteCmd.Run → config_delete.go

func (c *ConfigDiffCmd) Run(globals *Globals) error {
	fmt.Println("config diff: not yet implemented")
	return nil
}

func (c *ConfigApplyCmd) Run(globals *Globals) error {
	fmt.Println("config apply: not yet implemented")
	return nil
}

func (c *ConfigValidateCmd) Run(globals *Globals) error {
	fmt.Println("config validate: not yet implemented")
	return nil
}
