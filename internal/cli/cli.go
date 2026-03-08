package cli

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/alecthomas/kong"
)

// Globals holds values that are available to every command's Run() method.
// Kong tags are here so CLI can embed Globals directly.
// Command-specific flags live in mixin structs (ApiFlags,
// ConfigFileFlags, and the resolution hierarchy
// ApiFlags → WorkspaceFlags → ProjectFlags → EnvironmentFlags → ServiceFlags)
// or directly on command structs — not here.
type Globals struct {
	Output  string `help:"Output format: text, json, toml, raw." enum:"text,json,toml,raw" default:"text" short:"o" env:"FAT_CONTROLLER_OUTPUT_FORMAT"`
	JSON    bool   `help:"Output as JSON (shorthand for --output=json)." name:"json"`
	TOML    bool   `help:"Output as TOML (shorthand for --output=toml)." name:"toml"`
	Raw     bool   `help:"Output bare value, no formatting (shorthand for --output=raw)." name:"raw"`
	Color   string `help:"Color mode: auto, always, never." enum:"auto,always,never" default:"auto" env:"FAT_CONTROLLER_OUTPUT_COLOR"`
	Verbose int    `help:"Increase log verbosity. Repeat for more detail (-v = debug, -vv = trace)." short:"v" type:"counter"`
	Quiet   int    `help:"Decrease log verbosity. Repeat for less output (-q = warn, -qq = error only)." short:"q" type:"counter"`
	EnvFile string `help:"Env file path for variable interpolation." name:"env-file" env:"FAT_CONTROLLER_ENV_FILE"`

	// BaseCtx is the root context for all commands. Set by main() with
	// signal.NotifyContext so that SIGINT/SIGTERM cancels in-flight work.
	// Commands use this as the parent for TimeoutContext.
	BaseCtx context.Context `kong:"-"`
}

// ResolveOutputFormat applies --json/--toml/--raw shorthand flags to the
// Output field. Shorthands take precedence over --output when set.
func (g *Globals) ResolveOutputFormat() {
	switch {
	case g.JSON:
		g.Output = "json"
	case g.TOML:
		g.Output = "toml"
	case g.Raw:
		g.Output = "raw"
	}
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
	Timeout time.Duration `help:"API request timeout." default:"30s" env:"FAT_CONTROLLER_API_TIMEOUT"`
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

// ConfigFileFlags are embedded by commands that read config files (diff, apply, validate, adopt).
type ConfigFileFlags struct {
	ConfigFile string `help:"Config file path. Disables upward walk — loads only this file." name:"config-file" short:"f" env:"FAT_CONTROLLER_CONFIG_FILE"`
}

// Logger returns a slog.Logger configured for the current verbosity level.
// Output goes to stderr with no timestamps for clean CLI output.
func (g *Globals) Logger() *slog.Logger {
	level := slog.LevelInfo
	if g.Verbose >= 2 {
		level = slog.LevelDebug - 4 // trace level
	} else if g.Verbose >= 1 {
		level = slog.LevelDebug
	} else if g.Quiet >= 2 {
		level = slog.LevelError
	} else if g.Quiet >= 1 {
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

	// Imperative commands
	Deploy   DeployCmd   `cmd:"" help:"Trigger a deployment for services."`
	Redeploy RedeployCmd `cmd:"" help:"Redeploy services from current image."`
	Restart  RestartCmd  `cmd:"" help:"Restart running deployments."`
	Rollback RollbackCmd `cmd:"" help:"Roll back services to previous deployment."`
	Stop     StopCmd     `cmd:"" help:"Cancel running deployments."`

	// Operational
	Logs   LogsCmd   `cmd:"" help:"Show service logs."`
	Status StatusCmd `cmd:"" help:"Show deployment status."`
	Open   OpenCmd   `cmd:"" help:"Open Railway dashboard in browser."`

	// Auth
	Auth AuthCmd `cmd:"" help:"Manage authentication."`

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
type AuthLogoutCmd struct {
	PromptFlags `kong:"embed"`
}

// AuthStatusCmd implements `auth status`.
type AuthStatusCmd struct {
	ApiFlags `kong:"embed"`
}

// ProjectListCmd, EnvironmentListCmd, WorkspaceListCmd are used by
// ListCmd in list.go to delegate `list workspaces/projects/environments`.

type ProjectListCmd struct {
	WorkspaceFlags `kong:"embed"`
}

type EnvironmentListCmd struct {
	ProjectFlags `kong:"embed"`
}

type WorkspaceListCmd struct {
	ApiFlags `kong:"embed"`
}
