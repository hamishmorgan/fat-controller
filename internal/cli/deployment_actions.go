package cli

import (
	"context"
	"fmt"
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
	return runDeploymentAction(globals, &c.ApiFlags, c.Workspace, c.Project, c.Environment, c.Services, "redeploy", func(ctx context.Context, client *railway.Client, deploymentID string) error {
		_, err := railway.RedeployDeployment(ctx, client, deploymentID)
		return err
	})
}

// RestartCmd implements the `restart` command.
type RestartCmd struct {
	EnvironmentFlags `kong:"embed"`
	PromptFlags      `kong:"embed"`
	Services         []string `arg:"" optional:"" help:"Services to restart (default: all)."`
}

func (c *RestartCmd) Run(globals *Globals) error {
	return runDeploymentAction(globals, &c.ApiFlags, c.Workspace, c.Project, c.Environment, c.Services, "restart", func(ctx context.Context, client *railway.Client, deploymentID string) error {
		return railway.RestartDeployment(ctx, client, deploymentID)
	})
}

// RollbackCmd implements the `rollback` command.
type RollbackCmd struct {
	EnvironmentFlags `kong:"embed"`
	PromptFlags      `kong:"embed"`
	Services         []string `arg:"" optional:"" help:"Services to roll back (default: all)."`
}

func (c *RollbackCmd) Run(globals *Globals) error {
	return runDeploymentAction(globals, &c.ApiFlags, c.Workspace, c.Project, c.Environment, c.Services, "rollback", func(ctx context.Context, client *railway.Client, deploymentID string) error {
		return railway.RollbackDeployment(ctx, client, deploymentID)
	})
}

// StopCmd implements the `stop` command.
type StopCmd struct {
	EnvironmentFlags `kong:"embed"`
	PromptFlags      `kong:"embed"`
	Services         []string `arg:"" optional:"" help:"Services to stop (default: all)."`
}

func (c *StopCmd) Run(globals *Globals) error {
	return runDeploymentAction(globals, &c.ApiFlags, c.Workspace, c.Project, c.Environment, c.Services, "stop", func(ctx context.Context, client *railway.Client, deploymentID string) error {
		return railway.CancelDeployment(ctx, client, deploymentID)
	})
}

// runDeploymentAction handles the common pattern for deployment lifecycle commands.
// It resolves services, finds the latest deployment for each, then calls the action.
func runDeploymentAction(globals *Globals, apiFlags *ApiFlags, workspace, project, environment string, serviceNames []string, actionName string, action func(ctx context.Context, client *railway.Client, deploymentID string) error) error {
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

	for _, svc := range targets {
		slog.Debug(actionName+" service", "name", svc.Name, "id", svc.ID)

		// Find the latest deployment.
		deployments, _, err := railway.ListDeployments(ctx, client, envID, svc.ID, 1, nil)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "failed to list deployments for %s: %v\n", svc.Name, err)
			continue
		}
		if len(deployments) == 0 {
			_, _ = fmt.Fprintf(os.Stderr, "no deployments found for %s\n", svc.Name)
			continue
		}

		deploymentID := deployments[0].ID
		if err := action(ctx, client, deploymentID); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "failed to %s %s: %v\n", actionName, svc.Name, err)
			continue
		}
		if !globals.Quiet {
			_, _ = fmt.Fprintf(os.Stdout, "%s triggered for %s\n", actionName, svc.Name)
		}
	}
	return nil
}
