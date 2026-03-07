package cli

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/hamishmorgan/fat-controller/internal/railway"
)

// RedeployCmd implements the `redeploy` command.
type RedeployCmd struct {
	EnvironmentFlags `kong:"embed"`
	PromptFlags      `kong:"embed"`
	Services         []string `arg:"" optional:"" help:"Services to redeploy (default: all)."`
}

func (c *RedeployCmd) Run(globals *Globals) error {
	return runDeploymentAction(globals, &c.ApiFlags, c.Workspace, c.Project, c.Environment, c.Services, "redeploy", func(ctx context.Context, client *railway.Client, deploymentID string) (string, error) {
		return railway.RedeployDeployment(ctx, client, deploymentID)
	})
}

// RestartCmd implements the `restart` command.
type RestartCmd struct {
	EnvironmentFlags `kong:"embed"`
	PromptFlags      `kong:"embed"`
	Services         []string `arg:"" optional:"" help:"Services to restart (default: all)."`
}

func (c *RestartCmd) Run(globals *Globals) error {
	return runDeploymentAction(globals, &c.ApiFlags, c.Workspace, c.Project, c.Environment, c.Services, "restart", func(ctx context.Context, client *railway.Client, deploymentID string) (string, error) {
		err := railway.RestartDeployment(ctx, client, deploymentID)
		return "", err
	})
}

// RollbackCmd implements the `rollback` command.
type RollbackCmd struct {
	EnvironmentFlags `kong:"embed"`
	PromptFlags      `kong:"embed"`
	Services         []string `arg:"" optional:"" help:"Services to roll back (default: all)."`
}

func (c *RollbackCmd) Run(globals *Globals) error {
	return runDeploymentAction(globals, &c.ApiFlags, c.Workspace, c.Project, c.Environment, c.Services, "rollback", func(ctx context.Context, client *railway.Client, deploymentID string) (string, error) {
		err := railway.RollbackDeployment(ctx, client, deploymentID)
		return "", err
	})
}

// StopCmd implements the `stop` command.
type StopCmd struct {
	EnvironmentFlags `kong:"embed"`
	PromptFlags      `kong:"embed"`
	Services         []string `arg:"" optional:"" help:"Services to stop (default: all)."`
}

func (c *StopCmd) Run(globals *Globals) error {
	return runDeploymentAction(globals, &c.ApiFlags, c.Workspace, c.Project, c.Environment, c.Services, "stop", func(ctx context.Context, client *railway.Client, deploymentID string) (string, error) {
		err := railway.CancelDeployment(ctx, client, deploymentID)
		return "", err
	})
}

// runDeploymentAction handles the common pattern for deployment lifecycle commands.
// It resolves services, finds the latest deployment for each, then calls the action.
func runDeploymentAction(globals *Globals, apiFlags *ApiFlags, workspace, project, environment string, serviceNames []string, actionName string, action func(ctx context.Context, client *railway.Client, deploymentID string) (string, error)) error {
	ctx, cancel := apiFlags.TimeoutContext(globals.BaseCtx)
	defer cancel()
	client, err := newClient(apiFlags, globals.BaseCtx)
	if err != nil {
		return err
	}

	projID, envID, err := railway.ResolveProjectEnvironment(ctx, client, workspace, project, environment)
	if err != nil {
		return err
	}

	targets, err := resolveServiceTargets(ctx, client, projID, envID, serviceNames)
	if err != nil {
		return err
	}

	return RunDeploymentAction(ctx, globals, envID, targets, actionName,
		func(ctx context.Context, environmentID, serviceID string) ([]railway.DeploymentInfo, error) {
			d, _, err := railway.ListDeployments(ctx, client, environmentID, serviceID, 1, nil)
			return d, err
		},
		func(ctx context.Context, deploymentID string) (string, error) {
			return action(ctx, client, deploymentID)
		},
		os.Stdout, os.Stderr,
	)
}

type DeploymentActionResult struct {
	Service         string `json:"service" toml:"service"`
	ServiceID       string `json:"service_id" toml:"service_id"`
	DeploymentID    string `json:"deployment_id,omitempty" toml:"deployment_id"`
	NewDeploymentID string `json:"new_deployment_id,omitempty" toml:"new_deployment_id"`
	Error           string `json:"error,omitempty" toml:"error"`
}

type DeploymentActionOutput struct {
	Action        string                   `json:"action" toml:"action"`
	EnvironmentID string                   `json:"environment_id" toml:"environment_id"`
	Results       []DeploymentActionResult `json:"results" toml:"results"`
}

// RunDeploymentAction is the testable core of deployment lifecycle commands.
func RunDeploymentAction(
	ctx context.Context,
	globals *Globals,
	environmentID string,
	targets []serviceTarget,
	actionName string,
	listLatest func(ctx context.Context, environmentID, serviceID string) ([]railway.DeploymentInfo, error),
	action func(ctx context.Context, deploymentID string) (string, error),
	out, errOut io.Writer,
) error {
	if out == nil {
		out = os.Stdout
	}
	if errOut == nil {
		errOut = os.Stderr
	}

	if isStructuredOutput(globals) {
		payload := DeploymentActionOutput{Action: actionName, EnvironmentID: environmentID, Results: make([]DeploymentActionResult, 0, len(targets))}
		for _, svc := range targets {
			res := DeploymentActionResult{Service: svc.Name, ServiceID: svc.ID}
			deployments, err := listLatest(ctx, environmentID, svc.ID)
			if err != nil {
				res.Error = fmt.Sprintf("listing deployments: %v", err)
				payload.Results = append(payload.Results, res)
				continue
			}
			if len(deployments) == 0 {
				res.Error = "no deployments found"
				payload.Results = append(payload.Results, res)
				continue
			}
			res.DeploymentID = deployments[0].ID
			newID, err := action(ctx, res.DeploymentID)
			if err != nil {
				res.Error = err.Error()
			} else if newID != "" {
				res.NewDeploymentID = newID
			}
			payload.Results = append(payload.Results, res)
		}
		return writeStructured(out, globals.Output, payload)
	}

	for _, svc := range targets {
		slog.Debug(actionName+" service", "name", svc.Name, "id", svc.ID)

		deployments, err := listLatest(ctx, environmentID, svc.ID)
		if err != nil {
			_, _ = fmt.Fprintf(errOut, "failed to list deployments for %s: %v\n", svc.Name, err)
			continue
		}
		if len(deployments) == 0 {
			_, _ = fmt.Fprintf(errOut, "no deployments found for %s\n", svc.Name)
			continue
		}

		deploymentID := deployments[0].ID
		if _, err := action(ctx, deploymentID); err != nil {
			_, _ = fmt.Fprintf(errOut, "failed to %s %s: %v\n", actionName, svc.Name, err)
			continue
		}
		if globals == nil || globals.Quiet == 0 {
			_, _ = fmt.Fprintf(out, "%s triggered for %s\n", actionName, svc.Name)
		}
	}
	return nil
}
