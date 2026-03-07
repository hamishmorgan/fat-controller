package cli

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sort"

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
		sort.Slice(targets, func(i, j int) bool { return targets[i].Name < targets[j].Name })
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
