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

// Run implements `config diff`.
func (c *ConfigDiffCmd) Run(globals *Globals) error {
	slog.Warn("'config diff' is deprecated; use 'diff' instead")
	ctx, cancel := c.TimeoutContext(globals.BaseCtx)
	defer cancel()
	client, err := newClient(&c.ApiFlags, globals.BaseCtx)
	if err != nil {
		return err
	}
	fetcher := &defaultConfigFetcher{client: client}

	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	return RunConfigDiff(ctx, globals, c.Workspace, c.Project, c.Environment, wd, c.ConfigFiles, c.Service, c.ShowSecrets, fetcher, os.Stdout)
}

// RunConfigDiff is the testable core of `config diff`.
func RunConfigDiff(ctx context.Context, globals *Globals, workspace, project, environment, configDir string, extraFiles []string, service string, showSecrets bool, fetcher configFetcher, out io.Writer) error {
	if out == nil {
		out = os.Stdout
	}

	pair, err := loadAndFetch(ctx, workspace, project, environment, configDir, extraFiles, service, fetcher)
	if err != nil {
		return err
	}

	// Emit validation warnings to stderr.
	emitWarnings(pair, globals.Quiet, configDir)

	// Compute diff.
	result := diff.Compute(pair.Desired, pair.Live)
	slog.Debug("diff computed", "is_empty", result.IsEmpty())

	if isStructuredOutput(globals) {
		payload := renderDiffStructured(result, showSecrets)
		return writeStructured(out, globals.Output, payload)
	}

	// Format and display (live values are masked unless --show-secrets is set).
	formatted := diff.Format(result, showSecrets)
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
	Variables []DiffChangeOut `json:"variables,omitempty" toml:"variables"`
	Settings  []DiffChangeOut `json:"settings,omitempty" toml:"settings"`
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
				if masker != nil {
					liveVal = masker.MaskValue(ch.Key, liveVal)
				}
				sec.Variables = append(sec.Variables, DiffChangeOut{Key: ch.Key, Action: ch.Action.String(), LiveValue: liveVal, DesiredValue: ch.DesiredValue})
			}
		}
		if len(sd.Settings) > 0 {
			sec.Settings = make([]DiffChangeOut, 0, len(sd.Settings))
			for _, ch := range sd.Settings {
				sec.Settings = append(sec.Settings, DiffChangeOut{Key: ch.Key, Action: ch.Action.String(), LiveValue: ch.LiveValue, DesiredValue: ch.DesiredValue})
			}
		}
		if len(sec.Variables) == 0 && len(sec.Settings) == 0 {
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
