package cli

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/hamishmorgan/fat-controller/internal/config"
	"github.com/hamishmorgan/fat-controller/internal/diff"
)

// DiffOpts holds options for RunConfigDiffWithOpts.
type DiffOpts struct {
	ShowSecrets bool
	DiffOptions diff.Options
	Path        string // dot-path to scope diff (e.g. "api", "api.variables")
}

// RunConfigDiff is the testable core of `config diff`.
// Legacy: always includes creates, updates, and deletes.
func RunConfigDiff(ctx context.Context, globals *Globals, workspace, project, environment, configDir string, configFile string, service string, showSecrets bool, fetcher configFetcher, out io.Writer) error {
	return RunConfigDiffWithOpts(ctx, globals, workspace, project, environment, configDir, configFile, service, DiffOpts{
		ShowSecrets: showSecrets,
		DiffOptions: diff.Options{Create: true, Update: true, Delete: true},
	}, fetcher, out)
}

// RunConfigDiffWithOpts is the full-featured diff entrypoint.
func RunConfigDiffWithOpts(ctx context.Context, globals *Globals, workspace, project, environment, configDir string, configFile string, service string, opts DiffOpts, fetcher configFetcher, out io.Writer) error {
	if out == nil {
		out = os.Stdout
	}

	pair, err := loadAndFetch(ctx, workspace, project, environment, configDir, configFile, service, fetcher)
	if err != nil {
		return err
	}

	// Emit validation warnings to stderr.
	emitWarnings(pair, globals.Quiet, configDir)

	// Scope desired config by path if specified.
	desired := pair.Desired
	if opts.Path != "" {
		desired = scopeDesiredByPath(desired, opts.Path)
	}

	// Compute diff.
	result := diff.ComputeWithOptions(desired, pair.Live, opts.DiffOptions)
	slog.Debug("diff computed", "is_empty", result.IsEmpty())

	if isStructuredOutput(globals) {
		payload := renderDiffStructured(result, opts.ShowSecrets)
		return writeStructured(out, globals.Output, payload)
	}

	// Format and display (live values are masked unless --show-secrets is set).
	formatted := diff.Format(result, opts.ShowSecrets)
	_, err = fmt.Fprintln(out, formatted)
	return err
}

type DiffChangeOut struct {
	Key          string `json:"key" toml:"key"`
	Action       string `json:"action" toml:"action"`
	LiveValue    string `json:"live_value,omitempty" toml:"live_value"`
	DesiredValue string `json:"desired_value,omitempty" toml:"desired_value"`
}

type DiffSectionOut struct {
	Variables    []DiffChangeOut `json:"variables,omitempty" toml:"variables"`
	Settings     []DiffChangeOut `json:"settings,omitempty" toml:"settings"`
	SubResources []DiffChangeOut `json:"sub_resources,omitempty" toml:"sub_resources"`
}

type DiffOutput struct {
	Empty    bool                       `json:"empty" toml:"empty"`
	Shared   *DiffSectionOut            `json:"shared,omitempty" toml:"shared"`
	Services map[string]*DiffSectionOut `json:"services,omitempty" toml:"services"`
}

func renderDiffStructured(result *diff.Result, showSecrets bool) DiffOutput {
	out := DiffOutput{Empty: result == nil || result.IsEmpty()}
	if result == nil {
		return out
	}

	var masker *config.Masker
	if !showSecrets {
		masker = config.NewMasker(nil, nil)
	}

	convertSection := func(sd *diff.SectionDiff) *DiffSectionOut {
		if sd == nil {
			return nil
		}
		sec := &DiffSectionOut{}
		if len(sd.Variables) > 0 {
			sec.Variables = make([]DiffChangeOut, 0, len(sd.Variables))
			for _, ch := range sd.Variables {
				liveVal := ch.LiveValue
				desiredVal := ch.DesiredValue
				if masker != nil {
					liveVal = masker.MaskValue(ch.Key, liveVal)
					desiredVal = masker.MaskValue(ch.Key, desiredVal)
				}
				sec.Variables = append(sec.Variables, DiffChangeOut{Key: ch.Key, Action: ch.Action.String(), LiveValue: liveVal, DesiredValue: desiredVal})
			}
		}
		if len(sd.Settings) > 0 {
			sec.Settings = make([]DiffChangeOut, 0, len(sd.Settings))
			for _, ch := range sd.Settings {
				sec.Settings = append(sec.Settings, DiffChangeOut{Key: ch.Key, Action: ch.Action.String(), LiveValue: ch.LiveValue, DesiredValue: ch.DesiredValue})
			}
		}
		if len(sd.SubResources) > 0 {
			sec.SubResources = make([]DiffChangeOut, 0, len(sd.SubResources))
			for _, ch := range sd.SubResources {
				sec.SubResources = append(sec.SubResources, DiffChangeOut{
					Key:          ch.Type + ":" + ch.Key,
					Action:       ch.Action.String(),
					DesiredValue: ch.Key,
				})
			}
		}
		if len(sec.Variables) == 0 && len(sec.Settings) == 0 && len(sec.SubResources) == 0 {
			return nil
		}
		return sec
	}

	out.Shared = convertSection(result.Shared)
	if len(result.Services) > 0 {
		out.Services = make(map[string]*DiffSectionOut, len(result.Services))
		for name, sd := range result.Services {
			out.Services[name] = convertSection(sd)
		}
	}
	return out
}
