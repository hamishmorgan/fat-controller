package cli_test

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/zalando/go-keyring"

	"github.com/hamishmorgan/fat-controller/internal/auth"
	"github.com/hamishmorgan/fat-controller/internal/cli"
)

// ---------- Config stub tests ----------
// Note: ConfigGetCmd, ConfigSetCmd, ConfigDeleteCmd tests moved to
// config_get_test.go, config_set_test.go, config_delete_test.go respectively.

func TestConfigApplyCmd_Run(t *testing.T) {
	cmd := &cli.ConfigApplyCmd{}
	if err := cmd.Run(&cli.Globals{}); err != nil {
		t.Fatalf("ConfigApplyCmd.Run() returned error: %v", err)
	}
}

func TestConfigValidateCmd_Run(t *testing.T) {
	cmd := &cli.ConfigValidateCmd{}
	if err := cmd.Run(&cli.Globals{}); err != nil {
		t.Fatalf("ConfigValidateCmd.Run() returned error: %v", err)
	}
}

// ---------- Config stubs print expected text ----------

func TestConfigStubs_OutputMessages(t *testing.T) {
	tests := []struct {
		name string
		run  func(*cli.Globals) error
		want string
	}{
		{"Apply", (&cli.ConfigApplyCmd{}).Run, "config apply: not yet implemented"},
		{"Validate", (&cli.ConfigValidateCmd{}).Run, "config validate: not yet implemented"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			old := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			err := tt.run(&cli.Globals{})

			_ = w.Close()
			os.Stdout = old

			if err != nil {
				t.Fatalf("Run() returned error: %v", err)
			}

			var buf bytes.Buffer
			_, _ = buf.ReadFrom(r)
			got := strings.TrimSpace(buf.String())
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// ---------- Auth tests ----------

// clearAuthEnv unsets all auth-related env vars for the duration of the test.
func clearAuthEnv(t *testing.T) {
	t.Helper()
	t.Setenv("RAILWAY_TOKEN", "")
	t.Setenv("RAILWAY_API_TOKEN", "")
}

func TestAuthLogoutCmd_Run(t *testing.T) {
	keyring.MockInit()
	cmd := &cli.AuthLogoutCmd{}
	if err := cmd.Run(&cli.Globals{}); err != nil {
		t.Fatalf("AuthLogoutCmd.Run() returned error: %v", err)
	}
}

func TestAuthStatusCmd_NotAuthenticated(t *testing.T) {
	keyring.MockInit()
	clearAuthEnv(t)

	cmd := &cli.AuthStatusCmd{}
	if err := cmd.Run(&cli.Globals{}); err != nil {
		t.Fatalf("AuthStatusCmd.Run() returned error: %v", err)
	}
}

func TestAuthStatusCmd_ViaFlagToken(t *testing.T) {
	keyring.MockInit()
	clearAuthEnv(t)

	cmd := &cli.AuthStatusCmd{}
	globals := &cli.Globals{Token: "test-flag-token"}
	if err := cmd.Run(globals); err != nil {
		t.Fatalf("AuthStatusCmd.Run() returned error: %v", err)
	}
}

func TestAuthStatusCmd_ViaRailwayToken(t *testing.T) {
	keyring.MockInit()
	clearAuthEnv(t)
	t.Setenv("RAILWAY_TOKEN", "test-project-token")

	cmd := &cli.AuthStatusCmd{}
	if err := cmd.Run(&cli.Globals{}); err != nil {
		t.Fatalf("AuthStatusCmd.Run() returned error: %v", err)
	}
}

func TestAuthStatusCmd_ViaRailwayAPIToken(t *testing.T) {
	keyring.MockInit()
	clearAuthEnv(t)
	t.Setenv("RAILWAY_API_TOKEN", "test-api-token")

	cmd := &cli.AuthStatusCmd{}
	if err := cmd.Run(&cli.Globals{}); err != nil {
		t.Fatalf("AuthStatusCmd.Run() returned error: %v", err)
	}
}

func TestAuthStatusCmd_StoredOAuth_FetchUserInfoFails(t *testing.T) {
	keyring.MockInit()
	clearAuthEnv(t)

	// Store a token in the mock keyring so ResolveAuth finds it.
	store := auth.NewTokenStore()
	err := store.Save(&auth.StoredTokens{
		AccessToken:  "stored-access-token",
		RefreshToken: "stored-refresh-token",
		ClientID:     "test-client-id",
	})
	if err != nil {
		t.Fatalf("failed to save token to mock keyring: %v", err)
	}

	// AuthStatusCmd hardcodes NewOAuthClient() which points to production URLs.
	// The FetchUserInfo call will fail with a non-200 status — exercises the
	// error branch at auth.go:61-65.
	cmd := &cli.AuthStatusCmd{}
	if err := cmd.Run(&cli.Globals{}); err != nil {
		t.Fatalf("AuthStatusCmd.Run() returned error: %v", err)
	}
}

// ---------- Help printer tests ----------

// newTestParser builds a kong parser for CLI with output directed to buf.
func newTestParser(t *testing.T, app *cli.CLI, buf *bytes.Buffer) *kong.Kong {
	t.Helper()
	parser, err := kong.New(app,
		kong.Name("test-app"),
		kong.Description("A test application."),
		kong.Writers(buf, buf),
		kong.Exit(func(int) {}),
		kong.NoDefaultHelp(),
	)
	if err != nil {
		t.Fatalf("kong.New() failed: %v", err)
	}
	return parser
}

func TestColorHelpPrinter_RootApp(t *testing.T) {
	var app cli.CLI
	var buf bytes.Buffer
	parser := newTestParser(t, &app, &buf)

	// Trace with empty args → ctx.Selected() == nil, ctx.Empty() == true.
	ctx, err := kong.Trace(parser, nil)
	if err != nil {
		t.Fatalf("kong.Trace() failed: %v", err)
	}

	if err := cli.ColorHelpPrinter(kong.HelpOptions{}, ctx); err != nil {
		t.Fatalf("ColorHelpPrinter() returned error: %v", err)
	}

	output := buf.String()
	if output == "" {
		t.Fatal("ColorHelpPrinter() produced empty output")
	}
	if !strings.Contains(output, "test-app") {
		t.Errorf("expected output to contain 'test-app', got:\n%s", output)
	}
}

func TestColorHelpPrinter_Subcommand(t *testing.T) {
	var app cli.CLI
	var buf bytes.Buffer
	parser := newTestParser(t, &app, &buf)

	// Trace through "config get" to select a leaf command.
	ctx, err := kong.Trace(parser, []string{"config", "get"})
	if err != nil {
		t.Fatalf("kong.Trace() failed: %v", err)
	}

	if err := cli.ColorHelpPrinter(kong.HelpOptions{}, ctx); err != nil {
		t.Fatalf("ColorHelpPrinter() returned error: %v", err)
	}

	output := buf.String()
	if output == "" {
		t.Fatal("ColorHelpPrinter() produced empty output")
	}
}

func TestColorHelpPrinter_CommandGroup(t *testing.T) {
	var app cli.CLI
	var buf bytes.Buffer
	parser := newTestParser(t, &app, &buf)

	// Trace "auth" — a command group (not a leaf).
	ctx, err := kong.Trace(parser, []string{"auth"})
	if err != nil {
		t.Fatalf("kong.Trace() failed: %v", err)
	}

	if err := cli.ColorHelpPrinter(kong.HelpOptions{}, ctx); err != nil {
		t.Fatalf("ColorHelpPrinter() returned error: %v", err)
	}

	output := buf.String()
	if output == "" {
		t.Fatal("ColorHelpPrinter() produced empty output")
	}
}

func TestColorHelpPrinter_SubcommandWithPositionalArgs(t *testing.T) {
	var app cli.CLI
	var buf bytes.Buffer
	parser := newTestParser(t, &app, &buf)

	// "config set" has required positional args — exercises writeColorPositionals.
	ctx, err := kong.Trace(parser, []string{"config", "set"})
	if err != nil {
		t.Fatalf("kong.Trace() failed: %v", err)
	}

	if err := cli.ColorHelpPrinter(kong.HelpOptions{}, ctx); err != nil {
		t.Fatalf("ColorHelpPrinter() returned error: %v", err)
	}

	output := buf.String()
	if output == "" {
		t.Fatal("ColorHelpPrinter() produced empty output")
	}
}

func TestColorHelpPrinter_WithHelpOptions(t *testing.T) {
	tests := []struct {
		name string
		args []string
		opts kong.HelpOptions
	}{
		{"RootSummary", nil, kong.HelpOptions{Summary: true}},
		{"RootNoAppSummary", nil, kong.HelpOptions{NoAppSummary: true}},
		{"RootFlagsLast", nil, kong.HelpOptions{FlagsLast: true}},
		{"RootNoExpandSubcommands", nil, kong.HelpOptions{NoExpandSubcommands: true}},
		{"RootWrapUpperBound", nil, kong.HelpOptions{WrapUpperBound: 60}},
		{"SubSummary", []string{"config", "get"}, kong.HelpOptions{Summary: true}},
		{"SubNoAppSummary", []string{"config", "get"}, kong.HelpOptions{NoAppSummary: true}},
		{"SubFlagsLast", []string{"config", "get"}, kong.HelpOptions{FlagsLast: true}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var app cli.CLI
			var buf bytes.Buffer
			parser := newTestParser(t, &app, &buf)

			ctx, err := kong.Trace(parser, tt.args)
			if err != nil {
				t.Fatalf("kong.Trace() failed: %v", err)
			}

			if err := cli.ColorHelpPrinter(tt.opts, ctx); err != nil {
				t.Fatalf("ColorHelpPrinter() returned error: %v", err)
			}

			if buf.Len() == 0 {
				t.Fatal("ColorHelpPrinter() produced empty output")
			}
		})
	}
}

// newTestParserWithHelp builds a kong parser WITH the default help flag.
// This exercises the HelpFlag != nil branches in printColorApp/printColorCommand.
func newTestParserWithHelp(t *testing.T, app *cli.CLI, buf *bytes.Buffer) *kong.Kong {
	t.Helper()
	parser, err := kong.New(app,
		kong.Name("test-app"),
		kong.Description("A test application."),
		kong.Writers(buf, buf),
		kong.Exit(func(int) {}),
		// NOTE: no kong.NoDefaultHelp() — help flag is present.
	)
	if err != nil {
		t.Fatalf("kong.New() failed: %v", err)
	}
	return parser
}

func TestColorHelpPrinter_WithHelpFlag_RootApp(t *testing.T) {
	var app cli.CLI
	var buf bytes.Buffer
	parser := newTestParserWithHelp(t, &app, &buf)

	ctx, err := kong.Trace(parser, nil)
	if err != nil {
		t.Fatalf("kong.Trace() failed: %v", err)
	}

	// Default options: not Summary → hits the "else" branch (line 66).
	if err := cli.ColorHelpPrinter(kong.HelpOptions{}, ctx); err != nil {
		t.Fatalf("ColorHelpPrinter() returned error: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "--help") {
		t.Errorf("expected help hint in output, got:\n%s", output)
	}
}

func TestColorHelpPrinter_WithHelpFlag_RootSummary(t *testing.T) {
	var app cli.CLI
	var buf bytes.Buffer
	parser := newTestParserWithHelp(t, &app, &buf)

	ctx, err := kong.Trace(parser, nil)
	if err != nil {
		t.Fatalf("kong.Trace() failed: %v", err)
	}

	// Summary: true → hits the "if opts.Summary" branch (line 64).
	if err := cli.ColorHelpPrinter(kong.HelpOptions{Summary: true}, ctx); err != nil {
		t.Fatalf("ColorHelpPrinter() returned error: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "--help") {
		t.Errorf("expected help hint in output, got:\n%s", output)
	}
}

func TestColorHelpPrinter_WithHelpFlag_SubSummary(t *testing.T) {
	var app cli.CLI
	var buf bytes.Buffer
	parser := newTestParserWithHelp(t, &app, &buf)

	ctx, err := kong.Trace(parser, []string{"config", "get"})
	if err != nil {
		t.Fatalf("kong.Trace() failed: %v", err)
	}

	// Summary on a subcommand → hits printColorCommand line 76-79.
	if err := cli.ColorHelpPrinter(kong.HelpOptions{Summary: true}, ctx); err != nil {
		t.Fatalf("ColorHelpPrinter() returned error: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "--help") {
		t.Errorf("expected help hint in output, got:\n%s", output)
	}
}

func TestColorHelpPrinter_AllLeafCommands(t *testing.T) {
	// Exercise help for every leaf command to maximize coverage of
	// writeColorCommandList, writeColorFlags, formatFlagPlain, formatFlagColored.
	commands := [][]string{
		{"auth", "login"},
		{"auth", "logout"},
		{"auth", "status"},
		{"config", "get"},
		{"config", "set"},
		{"config", "delete"},
		{"config", "diff"},
		{"config", "apply"},
		{"config", "validate"},
		{"workspace", "list"},
		{"project", "list"},
		{"environment", "list"},
	}

	for _, args := range commands {
		name := strings.Join(args, "_")
		t.Run(name, func(t *testing.T) {
			var app cli.CLI
			var buf bytes.Buffer
			parser := newTestParser(t, &app, &buf)

			ctx, err := kong.Trace(parser, args)
			if err != nil {
				t.Fatalf("kong.Trace(%v) failed: %v", args, err)
			}

			if err := cli.ColorHelpPrinter(kong.HelpOptions{}, ctx); err != nil {
				t.Fatalf("ColorHelpPrinter() returned error: %v", err)
			}

			if buf.Len() == 0 {
				t.Fatalf("ColorHelpPrinter() produced empty output for %v", args)
			}
		})
	}
}
