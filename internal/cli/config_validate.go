package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/hamishmorgan/fat-controller/internal/config"
)

// Run implements `config validate`.
func (c *ConfigValidateCmd) Run(globals *Globals) error {
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	return RunConfigValidate(globals, wd, globals.ConfigFiles, os.Stdout)
}

// RunConfigValidate is the testable core of `config validate`.
func RunConfigValidate(globals *Globals, configDir string, extraFiles []string, out io.Writer) error {
	if out == nil {
		out = os.Stdout
	}

	// Load and merge config files.
	desired, err := config.LoadConfigs(configDir, extraFiles)
	if err != nil {
		return err
	}

	// Interpolation is not required for validation — we check the raw config.
	// No API calls: liveServiceNames is nil (W040 skipped in offline mode).
	warnings := config.Validate(desired, nil)

	if len(warnings) == 0 {
		if !globals.Quiet {
			_, _ = fmt.Fprintln(out, "No warnings found.")
		}
		return nil
	}

	for _, w := range warnings {
		path := ""
		if w.Path != "" {
			path = " (" + w.Path + ")"
		}
		_, _ = fmt.Fprintf(out, "[%s]%s %s\n", w.Code, path, w.Message)
	}

	// Exit cleanly — warnings are advisory, not errors.
	return nil
}
