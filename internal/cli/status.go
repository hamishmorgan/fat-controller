package cli

import (
	"fmt"
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

	for _, svc := range targets {
		deployments, _, err := railway.ListDeployments(ctx, client, envID, svc.ID, 1, nil)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "failed to get status for %s: %v\n", svc.Name, err)
			continue
		}

		status := "no deployments"
		if len(deployments) > 0 {
			d := deployments[0]
			status = fmt.Sprintf("%s (deployed %s)", strings.ToLower(string(d.Status)), d.CreatedAt.Format("2006-01-02 15:04"))
		}

		_, _ = fmt.Fprintf(os.Stdout, "%-20s %s\n", svc.Name, status)
	}
	return nil
}
