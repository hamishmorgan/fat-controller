package cmd

import (
	"fmt"

	"github.com/hamishmorgan/fat-controller/internal/auth"
	"github.com/hamishmorgan/fat-controller/internal/platform"
)

// Globals holds values that are available to every command's Run() method.
type Globals struct {
	Token       string
	Project     string
	Environment string
	Output      string
	Color       string
	Timeout     string
	Confirm     bool
	DryRun      bool
	ShowSecrets bool
	SkipDeploys bool
	FailFast    bool
	Config      []string
	Service     string
	Full        bool
	Verbose     bool
	Quiet       bool
}

// CLI is the root struct for the kong CLI parser.
// Global flags are defined here; subcommand groups are nested structs.
type CLI struct {
	// Global flags
	Token       string   `help:"Auth token (overrides all other auth). Env vars RAILWAY_API_TOKEN and RAILWAY_TOKEN are also supported — see docs/COMMANDS.md for precedence."`
	Project     string   `help:"Project ID or name." env:"FAT_CONTROLLER_PROJECT"`
	Environment string   `help:"Environment name." env:"FAT_CONTROLLER_ENVIRONMENT"`
	Output      string   `help:"Output format: text, json, toml." enum:"text,json,toml" default:"text" short:"o" env:"FAT_CONTROLLER_OUTPUT"`
	Color       string   `help:"Color mode: auto, always, never." enum:"auto,always,never" default:"auto" env:"FAT_CONTROLLER_COLOR"`
	Timeout     string   `help:"API request timeout." default:"30s" env:"FAT_CONTROLLER_TIMEOUT"`
	Confirm     bool     `help:"Auto-execute mutations (skip confirmation)." env:"FAT_CONTROLLER_CONFIRM"`
	DryRun      bool     `help:"Force preview of mutations." name:"dry-run" env:"FAT_CONTROLLER_DRY_RUN"`
	ConfigFiles []string `help:"Railway config file paths. Repeatable." name:"config" short:"c" env:"FAT_CONTROLLER_CONFIG" sep:"none"`
	Service     string   `help:"Scope to a single service." env:"FAT_CONTROLLER_SERVICE"`
	SkipDeploys bool     `help:"Don't trigger redeployments." name:"skip-deploys" env:"FAT_CONTROLLER_SKIP_DEPLOYS"`
	FailFast    bool     `help:"Stop on first error during apply." name:"fail-fast" env:"FAT_CONTROLLER_FAIL_FAST"`
	ShowSecrets bool     `help:"Show secret values instead of masking." name:"show-secrets" env:"FAT_CONTROLLER_SHOW_SECRETS"`
	Full        bool     `help:"Include IDs and read-only fields (get only)."`
	Verbose     bool     `help:"Debug output (HTTP requests, timing)." short:"v"`
	Quiet       bool     `help:"Suppress informational output." short:"q"`

	// Subcommand groups
	Auth   AuthCmd   `cmd:"" help:"Manage authentication."`
	Config ConfigCmd `cmd:"" name:"config" help:"Declarative configuration management."`
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

// Config subcommand stubs — implemented in M3+.
type ConfigGetCmd struct {
	Path string `arg:"" optional:"" help:"Dot-path to fetch (e.g. api.variables.PORT). Omit for all."`
}

type ConfigSetCmd struct {
	Path  string `arg:"" required:"" help:"Dot-path to set (e.g. api.variables.PORT)."`
	Value string `arg:"" required:"" help:"Value to set."`
}

type ConfigDeleteCmd struct {
	Path string `arg:"" required:"" help:"Dot-path to delete (e.g. api.variables.OLD)."`
}

type ConfigDiffCmd struct{}
type ConfigApplyCmd struct{}
type ConfigValidateCmd struct{}

func (c *AuthLoginCmd) Run(globals *Globals) error {
	oauth := auth.NewOAuthClient()
	store := auth.NewTokenStore(
		auth.WithFallbackPath(platform.AuthFilePath()),
	)
	return auth.Login(oauth, store, auth.OpenBrowser)
}

func (c *AuthLogoutCmd) Run(globals *Globals) error {
	store := auth.NewTokenStore(
		auth.WithFallbackPath(platform.AuthFilePath()),
	)
	if err := store.Delete(); err != nil {
		return fmt.Errorf("clearing credentials: %w", err)
	}
	fmt.Println("Logged out successfully.")
	return nil
}

func (c *AuthStatusCmd) Run(globals *Globals) error {
	store := auth.NewTokenStore(
		auth.WithFallbackPath(platform.AuthFilePath()),
	)

	resolved, err := auth.ResolveAuth(globals.Token, store)
	if err != nil {
		fmt.Println("Not authenticated.")
		fmt.Println("Run 'fat-controller auth login' or set RAILWAY_TOKEN.")
		return nil
	}

	fmt.Printf("Authenticated via: %s\n", resolved.Source)

	if resolved.Source == "env:RAILWAY_TOKEN" {
		fmt.Println("Using RAILWAY_TOKEN environment variable (project access token).")
		return nil
	}
	if resolved.Source == "env:RAILWAY_API_TOKEN" {
		fmt.Println("Using RAILWAY_API_TOKEN environment variable (account/workspace token).")
		return nil
	}
	if resolved.Source == "flag" {
		fmt.Println("Using --token flag.")
		return nil
	}

	// For stored OAuth tokens, fetch user info.
	// Note: if the access token is expired (>1hr), this will fail with a 401.
	// M2 will add a refresh-aware HTTP client that handles this transparently.
	// For now, we show a helpful message.
	oauth := auth.NewOAuthClient()
	info, err := oauth.FetchUserInfo(resolved.Token)
	if err != nil {
		fmt.Println("Authenticated (stored OAuth token).")
		fmt.Printf("Could not fetch user info: %v\n", err)
		fmt.Println("If your session expired, run 'fat-controller auth login' to re-authenticate.")
		return nil
	}

	fmt.Printf("User: %s\n", info.Name)
	fmt.Printf("Email: %s\n", info.Email)
	return nil
}

func (c *ConfigGetCmd) Run(globals *Globals) error {
	fmt.Println("config get: not yet implemented")
	return nil
}

func (c *ConfigSetCmd) Run(globals *Globals) error {
	fmt.Println("config set: not yet implemented")
	return nil
}

func (c *ConfigDeleteCmd) Run(globals *Globals) error {
	fmt.Println("config delete: not yet implemented")
	return nil
}

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
