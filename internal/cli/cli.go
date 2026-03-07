package cli

import (
	"context"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/alecthomas/kong"
)

// Globals holds values that are available to every command's Run() method.
// Kong tags are here so CLI can embed Globals directly.
// Command-specific flags live in mixin structs (ApiFlags, MutationFlags,
// ConfigFileFlags, and the resolution hierarchy
// ApiFlags → WorkspaceFlags → ProjectFlags → EnvironmentFlags → ServiceFlags)
// or directly on command structs — not here.
type Globals struct {
	Output  string `help:"Output format: text, json, toml." enum:"text,json,toml" default:"text" short:"o" env:"FAT_CONTROLLER_OUTPUT"`
	Color   string `help:"Color mode: auto, always, never." enum:"auto,always,never" default:"auto" env:"FAT_CONTROLLER_COLOR"`
	Verbose bool   `help:"Enable debug logging (config loading, auth, HTTP requests, apply operations)." short:"v"`
	Quiet   bool   `help:"Suppress informational and debug output (warnings and errors only)." short:"q"`

	// BaseCtx is the root context for all commands. Set by main() with
	// signal.NotifyContext so that SIGINT/SIGTERM cancels in-flight work.
	// Commands use this as the parent for TimeoutContext.
	BaseCtx context.Context `kong:"-"`
}

// Resolution flag hierarchy: each level embeds its parent so that a command
// only needs to embed one struct to get the full ancestry.
//
//	ApiFlags        (--token, --timeout)
//	  └── WorkspaceFlags   (+ --workspace)
//	        └── ProjectFlags      (+ --project)
//	              └── EnvironmentFlags   (+ --environment)
//	                    └── ServiceFlags        (+ --service)

// ApiFlags is embedded by commands that make API calls. It provides
// --token and --timeout. It is the base of the resolution hierarchy.
type ApiFlags struct {
	Token   string        `help:"Auth token (overrides all other auth). Env vars RAILWAY_API_TOKEN and RAILWAY_TOKEN are also supported — see docs/COMMANDS.md for precedence."`
	Timeout time.Duration `help:"API request timeout." default:"30s" env:"FAT_CONTROLLER_TIMEOUT"`
}

// TimeoutContext returns a context with the configured timeout applied.
// If Timeout is zero (or negative), it returns ctx and a no-op cancel func
// so callers always get a valid cancel to defer.
// A nil parent is treated as context.Background() for safety in tests.
func (a *ApiFlags) TimeoutContext(parent context.Context) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	if a.Timeout > 0 {
		return context.WithTimeout(parent, a.Timeout)
	}
	return parent, func() {}
}

// WorkspaceFlags is embedded by commands that need --workspace only.
type WorkspaceFlags struct {
	ApiFlags  `kong:"embed"`
	Workspace string `help:"Workspace ID or name." env:"FAT_CONTROLLER_WORKSPACE"`
}

// ProjectFlags is embedded by commands that need --workspace + --project.
type ProjectFlags struct {
	WorkspaceFlags `kong:"embed"`
	Project        string `help:"Project ID or name." env:"FAT_CONTROLLER_PROJECT"`
}

// EnvironmentFlags is embedded by commands that need --workspace + --project + --environment.
type EnvironmentFlags struct {
	ProjectFlags `kong:"embed"`
	Environment  string `help:"Environment name." env:"FAT_CONTROLLER_ENVIRONMENT"`
}

// ServiceFlags is embedded by commands that need the full resolution chain
// (--workspace + --project + --environment + --service).
type ServiceFlags struct {
	EnvironmentFlags `kong:"embed"`
	Service          string `help:"Scope to a single service." env:"FAT_CONTROLLER_SERVICE"`
}

// MutationFlags are embedded by commands that mutate state (set, delete, init, apply).
type MutationFlags struct {
	Yes    bool `help:"Answer yes to all confirmation prompts." short:"y" env:"FAT_CONTROLLER_YES"`
	DryRun bool `help:"Force preview of mutations." name:"dry-run" env:"FAT_CONTROLLER_DRY_RUN"`
}

// MergeFlags controls what a merge operation does.
type MergeFlags struct {
	Create bool `help:"Add entities that exist in source but not target." negatable:"" default:"true" env:"FAT_CONTROLLER_ALLOW_CREATE"`
	Update bool `help:"Overwrite entities that exist in both." negatable:"" default:"true" env:"FAT_CONTROLLER_ALLOW_UPDATE"`
	Delete bool `help:"Remove entities that exist in target but not source." negatable:"" default:"false" env:"FAT_CONTROLLER_ALLOW_DELETE"`
}

// PromptFlags controls interactive prompting behavior.
type PromptFlags struct {
	Ask bool `help:"Prompt for all parameters." short:"a" xor:"prompt"`
	Yes bool `help:"Skip all confirmation prompts." short:"y" xor:"prompt" env:"FAT_CONTROLLER_YES"`
}

// PromptMode returns the effective prompt mode.
func (f *PromptFlags) PromptMode() string {
	if f.Ask {
		return "all"
	}
	if f.Yes {
		return "none"
	}
	return "default"
}

// ConfigFileFlags are embedded by commands that read config files (diff, apply, validate).
type ConfigFileFlags struct {
	ConfigFiles []string `help:"Railway config file paths. Repeatable." name:"file" short:"f" env:"FAT_CONTROLLER_CONFIG" sep:"none"`
}

// Logger returns a slog.Logger configured for the current verbosity level.
// Output goes to stderr with no timestamps for clean CLI output.
func (g *Globals) Logger() *slog.Logger {
	level := slog.LevelInfo
	if g.Verbose {
		level = slog.LevelDebug
	} else if g.Quiet {
		level = slog.LevelWarn
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level:     level,
		AddSource: false,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// Remove timestamp for clean CLI output.
			if a.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return a
		},
	}))
}

// CLI is the root struct for the kong CLI parser.
// Global flags come from the embedded Globals; subcommand groups are nested structs.
type CLI struct {
	Globals `kong:"embed"`

	Version kong.VersionFlag `help:"Print version." short:"V"`

	// Core declarative commands
	Adopt    AdoptCmd    `cmd:"" help:"Pull live Railway state into config."`
	Diff     DiffCmd     `cmd:"" help:"Compare config against live Railway state."`
	Apply    ApplyCmd    `cmd:"" help:"Push config changes to Railway."`
	Validate ValidateCmd `cmd:"" help:"Check config for errors (offline)."`
	Show     ShowCmd     `cmd:"" help:"Display live Railway state."`
	New      NewCmd      `cmd:"" help:"Scaffold config entries."`

	// Discovery
	List ListCmd `cmd:"" help:"List Railway entities."`

	// Auth
	Auth AuthCmd `cmd:"" help:"Manage authentication."`

	// Legacy (hidden, deprecated — will be removed)
	Config      ConfigCmd      `cmd:"" name:"config" help:"Declarative configuration management." hidden:""`
	Project     ProjectCmd     `cmd:"" help:"Manage projects." hidden:""`
	Environment EnvironmentCmd `cmd:"" help:"Manage environments." hidden:""`
	Workspace   WorkspaceCmd   `cmd:"" help:"Manage workspaces." hidden:""`

	// Utility
	Completion CompletionCmd `cmd:"" help:"Generate shell completions." hidden:""`
}

// AuthCmd is the `auth` command group.
type AuthCmd struct {
	Login  AuthLoginCmd  `cmd:"" help:"Log in to Railway via browser-based OAuth."`
	Logout AuthLogoutCmd `cmd:"" help:"Clear stored credentials."`
	Status AuthStatusCmd `cmd:"" help:"Show current authentication status."`
}

// AuthLoginCmd implements `auth login`.
type AuthLoginCmd struct {
	ApiFlags `kong:"embed"`
}

// AuthLogoutCmd implements `auth logout`.
type AuthLogoutCmd struct{}

// AuthStatusCmd implements `auth status`.
type AuthStatusCmd struct {
	ApiFlags `kong:"embed"`
}

// ConfigCmd is the `config` command group.
type ConfigCmd struct {
	Init     ConfigInitCmd     `cmd:"" help:"Bootstrap a fat-controller.toml from live Railway state."`
	Get      ConfigGetCmd      `cmd:"" help:"Fetch live config from Railway."`
	Set      ConfigSetCmd      `cmd:"" help:"Set a single value by dot-path."`
	Delete   ConfigDeleteCmd   `cmd:"" help:"Delete a single value by dot-path."`
	Diff     ConfigDiffCmd     `cmd:"" help:"Compare local config against live state."`
	Apply    ConfigApplyCmd    `cmd:"" help:"Push configuration changes to Railway."`
	Validate ConfigValidateCmd `cmd:"" help:"Check config file for warnings (no API calls)."`
}

// ConfigGetCmd implements `config get`.
type ConfigGetCmd struct {
	ServiceFlags `kong:"embed"`
	Path         string    `arg:"" optional:"" help:"Dot-path to fetch (e.g. api.variables.PORT). Omit for all."`
	Full         bool      `help:"Include IDs and read-only fields."`
	ShowSecrets  bool      `help:"Show secret values instead of masking." name:"show-secrets" env:"FAT_CONTROLLER_SHOW_SECRETS"`
	output       io.Writer `kong:"-"`
}

// ConfigSetCmd implements `config set`.
type ConfigSetCmd struct {
	EnvironmentFlags `kong:"embed"`
	MutationFlags    `kong:"embed"`
	Path             string `arg:"" required:"" help:"Dot-path to set (e.g. api.variables.PORT)."`
	Value            string `arg:"" required:"" help:"Value to set."`
	SkipDeploys      bool   `help:"Don't trigger redeployments." name:"skip-deploys" env:"FAT_CONTROLLER_SKIP_DEPLOYS"`
}

// ConfigDeleteCmd implements `config delete`.
type ConfigDeleteCmd struct {
	EnvironmentFlags `kong:"embed"`
	MutationFlags    `kong:"embed"`
	Path             string `arg:"" required:"" help:"Dot-path to delete (e.g. api.variables.OLD)."`
}

// ConfigInitCmd implements `config init`.
type ConfigInitCmd struct {
	EnvironmentFlags `kong:"embed"`
	MutationFlags    `kong:"embed"`
}

// ConfigDiffCmd implements `config diff`.
type ConfigDiffCmd struct {
	ServiceFlags    `kong:"embed"`
	ConfigFileFlags `kong:"embed"`
	ShowSecrets     bool `help:"Show secret values instead of masking." name:"show-secrets" env:"FAT_CONTROLLER_SHOW_SECRETS"`
}

// ConfigApplyCmd implements `config apply`.
type ConfigApplyCmd struct {
	ServiceFlags    `kong:"embed"`
	MutationFlags   `kong:"embed"`
	ConfigFileFlags `kong:"embed"`
	ShowSecrets     bool `help:"Show secret values instead of masking." name:"show-secrets" env:"FAT_CONTROLLER_SHOW_SECRETS"`
	SkipDeploys     bool `help:"Don't trigger redeployments." name:"skip-deploys" env:"FAT_CONTROLLER_SKIP_DEPLOYS"`
	FailFast        bool `help:"Stop on first error during apply." name:"fail-fast" env:"FAT_CONTROLLER_FAIL_FAST"`
}

// ConfigValidateCmd implements `config validate`.
type ConfigValidateCmd struct {
	ConfigFileFlags `kong:"embed"`
}

type ProjectCmd struct {
	List ProjectListCmd `cmd:"" help:"List available projects."`
}

type ProjectListCmd struct {
	WorkspaceFlags `kong:"embed"`
}

type EnvironmentCmd struct {
	List EnvironmentListCmd `cmd:"" help:"List environments for a project."`
}

type EnvironmentListCmd struct {
	ProjectFlags `kong:"embed"`
}

type WorkspaceCmd struct {
	List WorkspaceListCmd `cmd:"" help:"List available workspaces."`
}

type WorkspaceListCmd struct {
	ApiFlags `kong:"embed"`
}

// Run methods:
// - ConfigInitCmd.Run   → config_init.go
// - ConfigGetCmd.Run    → config_get.go
// - ConfigSetCmd.Run    → config_set.go
// - ConfigDeleteCmd.Run → config_delete.go
// - ConfigDiffCmd.Run   → config_diff.go
// - ConfigApplyCmd.Run  → config_apply.go

// ConfigValidateCmd.Run → config_validate.go
