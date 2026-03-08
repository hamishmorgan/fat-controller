package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/hamishmorgan/fat-controller/internal/config"
)

// ValidateCmd implements the top-level `validate` command.
type ValidateCmd struct {
	ConfigFileFlags `kong:"embed"`
	Path            string `arg:"" optional:"" help:"Dot-path to scope validation (e.g. api)."`
}

// Run implements `validate`.
func (c *ValidateCmd) Run(globals *Globals) error {
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}
	return RunConfigValidateScoped(globals, wd, c.ConfigFile, c.Path, os.Stdout)
}

// RunConfigValidateScoped is a scoped variant of RunConfigValidate.
func RunConfigValidateScoped(globals *Globals, configDir string, configFile string, path string, out io.Writer) error {
	// Path scoping for validate filters warnings by path prefix.
	warnings, err := collectValidateWarnings(configDir, configFile)
	if err != nil {
		return err
	}
	if path != "" {
		warnings = filterWarningsByPath(warnings, path)
	}
	return writeValidateWarnings(globals, warnings, out)
}

// RunConfigValidate is the testable core of `config validate`.
func RunConfigValidate(globals *Globals, configDir string, configFile string, out io.Writer) error {
	slog.Debug("starting config validate")
	if out == nil {
		out = os.Stdout
	}

	warnings, err := collectValidateWarnings(configDir, configFile)
	if err != nil {
		return err
	}
	return writeValidateWarnings(globals, warnings, out)
}

func collectValidateWarnings(configDir string, configFile string) ([]config.Warning, error) {
	// Load config files (cascade or single --config-file).
	result, err := config.LoadCascade(config.LoadOptions{
		WorkDir:    configDir,
		ConfigFile: configFile,
	})
	if err != nil {
		return nil, err
	}
	desired := result.Config

	// Interpolation is not required for validation — we check the raw config.
	// No API calls: liveServiceNames is nil (W040 skipped in offline mode).
	warnings := config.ValidateWithOptions(desired, config.ValidateOptions{EnvFileVars: result.EnvVars})
	warnings = append(warnings, config.ValidateFiles(configDir)...)
	return warnings, nil
}

func filterWarningsByPath(warnings []config.Warning, path string) []config.Warning {
	if path == "" {
		return warnings
	}
	filtered := warnings[:0]
	for _, w := range warnings {
		if w.Path == path || strings.HasPrefix(w.Path, path+".") {
			filtered = append(filtered, w)
		}
	}
	return filtered
}

func writeValidateWarnings(globals *Globals, warnings []config.Warning, out io.Writer) error {
	if out == nil {
		out = os.Stdout
	}

	if len(warnings) == 0 {
		if globals.Quiet == 0 {
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
