package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/BurntSushi/toml"
	"github.com/hamishmorgan/fat-controller/internal/config"
)

// Run implements `config validate`.
func (c *ConfigValidateCmd) Run(globals *Globals) error {
	slog.Warn("'config validate' is deprecated; use 'validate' instead")
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	return RunConfigValidate(globals, wd, c.ConfigFiles, os.Stdout)
}

// RunConfigValidate is the testable core of `config validate`.
func RunConfigValidate(globals *Globals, configDir string, extraFiles []string, out io.Writer) error {
	slog.Debug("starting config validate")
	if out == nil {
		out = os.Stdout
	}

	// Load and merge config files via cascade.
	result, err := config.LoadCascade(config.LoadOptions{WorkDir: configDir})
	if err != nil {
		return err
	}
	desired := result.Config

	// Merge extra config files (--file flags) on top.
	for _, f := range extraFiles {
		extra, err := config.ParseFile(f)
		if err != nil {
			return fmt.Errorf("parsing %s: %w", f, err)
		}
		desired = config.Merge(desired, extra)
	}

	// Interpolation is not required for validation — we check the raw config.
	// No API calls: liveServiceNames is nil (W040 skipped in offline mode).
	warnings := config.ValidateWithOptions(desired, config.ValidateOptions{EnvFileVars: result.EnvVars})
	warnings = append(warnings, config.ValidateFiles(configDir)...)

	if len(warnings) == 0 {
		if !globals.Quiet {
			if _, err := fmt.Fprintln(out, "No warnings found."); err != nil {
				return err
			}
		}
		return nil
	}

	// Structured output: emit machine-readable warnings.
	if globals.Output == "json" || globals.Output == "toml" {
		type warningOut struct {
			Code    string `json:"code" toml:"code"`
			Message string `json:"message" toml:"message"`
			Path    string `json:"path" toml:"path"`
		}
		payload := struct {
			Warnings []warningOut `json:"warnings" toml:"warnings"`
		}{Warnings: make([]warningOut, 0, len(warnings))}
		for _, w := range warnings {
			payload.Warnings = append(payload.Warnings, warningOut{Code: w.Code, Message: w.Message, Path: w.Path})
		}
		switch globals.Output {
		case "json":
			enc := json.NewEncoder(out)
			enc.SetIndent("", "  ")
			return enc.Encode(payload)
		case "toml":
			return toml.NewEncoder(out).Encode(payload)
		}
	}

	for _, w := range warnings {
		path := ""
		if w.Path != "" {
			path = " (" + w.Path + ")"
		}
		if _, err := fmt.Fprintf(out, "[%s]%s %s\n", w.Code, path, w.Message); err != nil {
			return err
		}
	}

	// Exit cleanly — warnings are advisory, not errors.
	return nil
}
