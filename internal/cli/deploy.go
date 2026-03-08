package cli

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/hamishmorgan/fat-controller/internal/app"
	"github.com/hamishmorgan/fat-controller/internal/railway"
)

// DeployCmd implements the `deploy` command.
type DeployCmd struct {
	EnvironmentFlags `kong:"embed"`
	PromptFlags      `kong:"embed"`
	DryRun           bool     `help:"Preview what would be deployed without triggering." name:"dry-run" env:"FAT_CONTROLLER_DRY_RUN"`
	Services         []string `arg:"" optional:"" help:"Services to deploy (default: all)."`
}

// Run implements `deploy`.
func (c *DeployCmd) Run(globals *Globals) error {
	ctx, cancel := c.TimeoutContext(globals.BaseCtx)
	defer cancel()
	client, err := newClient(&c.ApiFlags, globals.BaseCtx)
	if err != nil {
		return err
	}

	resolved, err := railway.ResolveProjectEnvironment(ctx, client, c.Workspace, c.Project, c.Environment, interactivePicker)
	if err != nil {
		return err
	}
	projID, envID := resolved.ProjectID, resolved.EnvironmentID

	fetcher := &defaultConfigFetcher{client: client}
	targets, err := app.ResolveServiceTargets(ctx, fetcher, projID, envID, c.Services)
	if err != nil {
		return err
	}

	if c.DryRun {
		return RunDeployDryRun(globals, "deploy", envID, targets, os.Stdout)
	}

	return RunDeploy(ctx, globals, envID, targets, func(ctx context.Context, environmentID, serviceID string) (string, error) {
		return railway.DeployService(ctx, client, environmentID, serviceID, nil)
	}, os.Stdout, os.Stderr)
}

type DeployResult struct {
	Service      string `json:"service" toml:"service"`
	ServiceID    string `json:"service_id" toml:"service_id"`
	DeploymentID string `json:"deployment_id,omitempty" toml:"deployment_id"`
	Error        string `json:"error,omitempty" toml:"error"`
}

type DeployOutput struct {
	Action        string         `json:"action" toml:"action"`
	EnvironmentID string         `json:"environment_id" toml:"environment_id"`
	Results       []DeployResult `json:"results" toml:"results"`
}

// RunDeploy is the testable core of `deploy`.
func RunDeploy(ctx context.Context, globals *Globals, environmentID string, targets []serviceTarget, deployFn func(ctx context.Context, environmentID, serviceID string) (string, error), out, errOut io.Writer) error {
	if out == nil {
		out = os.Stdout
	}
	if errOut == nil {
		errOut = os.Stderr
	}

	if isStructuredOutput(globals) {
		payload := DeployOutput{Action: "deploy", EnvironmentID: environmentID, Results: make([]DeployResult, 0, len(targets))}
		for _, svc := range targets {
			res := DeployResult{Service: svc.Name, ServiceID: svc.ID}
			deploymentID, err := deployFn(ctx, environmentID, svc.ID)
			if err != nil {
				res.Error = err.Error()
			} else {
				res.DeploymentID = deploymentID
			}
			payload.Results = append(payload.Results, res)
		}
		return writeStructured(out, globals.Output, payload)
	}

	for _, svc := range targets {
		slog.Debug("deploying service", "name", svc.Name, "id", svc.ID)
		_, err := deployFn(ctx, environmentID, svc.ID)
		if err != nil {
			_, _ = fmt.Fprintf(errOut, "failed to deploy %s: %v\n", svc.Name, err)
			continue
		}
		if globals == nil || globals.Quiet == 0 {
			_, _ = fmt.Fprintf(out, "Triggered deploy for %s\n", svc.Name)
		}
	}
	return nil
}

// RunDeployDryRun previews what a deploy/redeploy/restart/rollback/stop would
// do without actually triggering anything.
func RunDeployDryRun(globals *Globals, action, environmentID string, targets []serviceTarget, out io.Writer) error {
	if out == nil {
		out = os.Stdout
	}

	if isStructuredOutput(globals) {
		payload := DeployOutput{Action: action + " (dry run)", EnvironmentID: environmentID, Results: make([]DeployResult, 0, len(targets))}
		for _, svc := range targets {
			payload.Results = append(payload.Results, DeployResult{Service: svc.Name, ServiceID: svc.ID})
		}
		return writeStructured(out, globals.Output, payload)
	}

	_, _ = fmt.Fprintf(out, "dry run: would %s %d service(s):\n", action, len(targets))
	for _, svc := range targets {
		_, _ = fmt.Fprintf(out, "  %s (%s)\n", svc.Name, svc.ID)
	}
	return nil
}
