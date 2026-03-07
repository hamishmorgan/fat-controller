package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/hamishmorgan/fat-controller/internal/railway"
)

// StatusCmd implements the `status` command.
type StatusCmd struct {
	EnvironmentFlags `kong:"embed"`
	Services         []string `arg:"" optional:"" help:"Services to check (default: all)."`
}

func (c *StatusCmd) Run(globals *Globals) error {
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

	// Sort by name for consistent output.
	sort.Slice(targets, func(i, j int) bool {
		return targets[i].Name < targets[j].Name
	})

	return RunStatus(ctx, globals, envID, targets, func(ctx context.Context, environmentID, serviceID string) ([]railway.DeploymentInfo, error) {
		d, _, err := railway.ListDeployments(ctx, client, environmentID, serviceID, 1, nil)
		return d, err
	}, os.Stdout, os.Stderr)
}

type StatusItem struct {
	Service      string `json:"service" toml:"service"`
	ServiceID    string `json:"service_id" toml:"service_id"`
	Status       string `json:"status" toml:"status"`
	DeploymentID string `json:"deployment_id,omitempty" toml:"deployment_id"`
	CreatedAt    string `json:"created_at,omitempty" toml:"created_at"`
	Error        string `json:"error,omitempty" toml:"error"`
}

type StatusOutput struct {
	EnvironmentID string       `json:"environment_id" toml:"environment_id"`
	Statuses      []StatusItem `json:"statuses" toml:"statuses"`
}

// RunStatus is the testable core of `status`.
func RunStatus(ctx context.Context, globals *Globals, environmentID string, targets []serviceTarget, listLatest func(ctx context.Context, environmentID, serviceID string) ([]railway.DeploymentInfo, error), out, errOut io.Writer) error {
	if out == nil {
		out = os.Stdout
	}
	if errOut == nil {
		errOut = os.Stderr
	}

	if isStructuredOutput(globals) {
		payload := StatusOutput{EnvironmentID: environmentID, Statuses: make([]StatusItem, 0, len(targets))}
		for _, svc := range targets {
			it := StatusItem{Service: svc.Name, ServiceID: svc.ID}
			deployments, err := listLatest(ctx, environmentID, svc.ID)
			if err != nil {
				it.Error = err.Error()
				payload.Statuses = append(payload.Statuses, it)
				continue
			}
			if len(deployments) == 0 {
				it.Status = "no deployments"
				payload.Statuses = append(payload.Statuses, it)
				continue
			}
			d := deployments[0]
			it.DeploymentID = d.ID
			it.Status = strings.ToLower(string(d.Status))
			it.CreatedAt = d.CreatedAt.Format("2006-01-02T15:04:05Z07:00")
			payload.Statuses = append(payload.Statuses, it)
		}
		return writeStructured(out, globals.Output, payload)
	}

	for _, svc := range targets {
		deployments, err := listLatest(ctx, environmentID, svc.ID)
		if err != nil {
			_, _ = fmt.Fprintf(errOut, "failed to get status for %s: %v\n", svc.Name, err)
			continue
		}

		status := "no deployments"
		if len(deployments) > 0 {
			d := deployments[0]
			status = fmt.Sprintf("%s (deployed %s)", strings.ToLower(string(d.Status)), d.CreatedAt.Format("2006-01-02 15:04"))
		}

		_, _ = fmt.Fprintf(out, "%-20s %s\n", svc.Name, status)
	}
	return nil
}
