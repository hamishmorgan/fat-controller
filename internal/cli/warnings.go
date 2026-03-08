package cli

import (
	"fmt"
	"log/slog"

	"github.com/hamishmorgan/fat-controller/internal/app"
	"github.com/hamishmorgan/fat-controller/internal/config"
)

// emitWarnings runs validation on the config pair and emits warnings to stderr via slog.
// Respects --quiet to suppress warnings. Callers that always want warnings (e.g. config validate)
// should call config.Validate directly.
func emitWarnings(pair *app.ConfigPair, quiet int, configDir string) {
	if quiet > 0 {
		return
	}
	if pair == nil {
		return
	}
	desired := pair.Desired
	if pair.RawDesired != nil {
		desired = pair.RawDesired
	}
	if desired == nil {
		return
	}
	// Extract live service names for W040.
	var liveNames []string
	for name := range pair.Live.Services {
		liveNames = append(liveNames, name)
	}

	warnings := config.ValidateWithOptions(desired, config.ValidateOptions{LiveServiceNames: liveNames, EnvFileVars: pair.EnvVars})
	warnings = append(warnings, config.ValidateFiles(configDir)...)

	// Filter suppressed warnings (Validate already filters, but ValidateFiles warnings need it too).
	var suppressWarnings []string
	if desired.Tool != nil {
		suppressWarnings = desired.Tool.SuppressWarnings
	}
	suppressed := make(map[string]bool, len(suppressWarnings))
	for _, code := range suppressWarnings {
		suppressed[code] = true
	}

	for _, w := range warnings {
		if suppressed[w.Code] {
			continue
		}
		path := ""
		if w.Path != "" {
			path = " (" + w.Path + ")"
		}
		slog.Warn(fmt.Sprintf("[%s]%s %s", w.Code, path, w.Message))
	}
}
