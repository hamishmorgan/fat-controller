package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/hamishmorgan/fat-controller/internal/railway"
)

// DeployCmd implements the `deploy` command.
type DeployCmd struct {
	EnvironmentFlags `kong:"embed"`
	PromptFlags      `kong:"embed"`
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

	projID, envID, err := railway.ResolveProjectEnvironment(ctx, client, c.Workspace, c.Project, c.Environment)
	if err != nil {
		return err
	}

	targets, err := resolveServiceTargets(ctx, client, projID, envID, c.Services)
	if err != nil {
		return err
	}

	for _, svc := range targets {
		slog.Debug("deploying service", "name", svc.Name, "id", svc.ID)
		_, err := railway.DeployService(ctx, client, envID, svc.ID, nil)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "failed to deploy %s: %v\n", svc.Name, err)
			continue
		}
		if !globals.Quiet {
			_, _ = fmt.Fprintf(os.Stdout, "Triggered deploy for %s\n", svc.Name)
		}
	}
	return nil
}

// serviceTarget holds a resolved service name and ID.
type serviceTarget struct {
	Name string
	ID   string
}

// resolveServiceTargets resolves the service arguments to name+ID pairs.
// If no service names are given, returns all services in the environment.
func resolveServiceTargets(ctx context.Context, client *railway.Client, projectID, envID string, serviceNames []string) ([]serviceTarget, error) {
	live, err := railway.FetchLiveConfig(ctx, client, projectID, envID, "")
	if err != nil {
		return nil, fmt.Errorf("fetching services: %w", err)
	}

	if len(serviceNames) == 0 {
		// All services.
		targets := make([]serviceTarget, 0, len(live.Services))
		for _, svc := range live.Services {
			targets = append(targets, serviceTarget{Name: svc.Name, ID: svc.ID})
		}
		return targets, nil
	}

	// Specific services.
	targets := make([]serviceTarget, 0, len(serviceNames))
	for _, name := range serviceNames {
		svc, ok := live.Services[name]
		if !ok {
			return nil, fmt.Errorf("service %q not found", name)
		}
		targets = append(targets, serviceTarget{Name: svc.Name, ID: svc.ID})
	}
	return targets, nil
}
