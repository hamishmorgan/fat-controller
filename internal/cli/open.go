package cli

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/hamishmorgan/fat-controller/internal/auth"
	"github.com/hamishmorgan/fat-controller/internal/railway"
)

// OpenCmd implements the `open` command.
type OpenCmd struct {
	EnvironmentFlags `kong:"embed"`
	Print            bool `help:"Print URL instead of opening browser." short:"p"`
}

func (c *OpenCmd) Run(globals *Globals) error {
	ctx, cancel := c.TimeoutContext(globals.BaseCtx)
	defer cancel()
	client, err := newClient(&c.ApiFlags, globals.BaseCtx)
	if err != nil {
		return err
	}

	projID, envID, err := railway.ResolveProjectEnvironment(ctx, client, c.Workspace, c.Project, c.Environment)
	if err != nil {
		return err
	}

	return RunOpen(ctx, globals, projID, envID, c.Print, auth.OpenBrowser, os.Stdout, os.Stderr)
}

type OpenOutput struct {
	URL           string `json:"url" toml:"url"`
	ProjectID     string `json:"project_id" toml:"project_id"`
	EnvironmentID string `json:"environment_id" toml:"environment_id"`
	Opened        bool   `json:"opened" toml:"opened"`
	Error         string `json:"error,omitempty" toml:"error"`
}

// RunOpen is the testable core of `open`.
func RunOpen(ctx context.Context, globals *Globals, projectID, environmentID string, printOnly bool, openFn func(string) error, out, errOut io.Writer) error {
	if out == nil {
		out = os.Stdout
	}
	if errOut == nil {
		errOut = os.Stderr
	}
	_ = ctx

	url := fmt.Sprintf("https://railway.com/project/%s/environment/%s", projectID, environmentID)
	if isStructuredOutput(globals) {
		payload := OpenOutput{URL: url, ProjectID: projectID, EnvironmentID: environmentID}
		if printOnly {
			payload.Opened = false
			return writeStructured(out, globals.Output, payload)
		}
		err := openFn(url)
		payload.Opened = err == nil
		if err != nil {
			payload.Error = err.Error()
		}
		return writeStructured(out, globals.Output, payload)
	}

	if printOnly {
		_, _ = fmt.Fprintln(out, url)
		return nil
	}
	if err := openFn(url); err != nil {
		_, _ = fmt.Fprintf(errOut, "Could not open browser: %v\n", err)
		_, _ = fmt.Fprintln(out, url)
	}
	return nil
}
