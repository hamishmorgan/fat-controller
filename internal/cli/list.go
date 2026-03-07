package cli

import (
	"fmt"
)

// ListCmd implements the unified `list` command.
type ListCmd struct {
	ServiceFlags `kong:"embed"`
	Type         string `arg:"" optional:"" help:"Entity type: all, workspaces, projects, environments, services, deployments, volumes, buckets, domains." default:"services" enum:"all,workspaces,projects,environments,services,deployments,volumes,buckets,domains"`
}

// Run implements `list`.
func (c *ListCmd) Run(globals *Globals) error {
	switch c.Type {
	case "workspaces":
		cmd := &WorkspaceListCmd{ApiFlags: c.ApiFlags}
		return cmd.Run(globals)
	case "projects":
		cmd := &ProjectListCmd{WorkspaceFlags: c.WorkspaceFlags}
		return cmd.Run(globals)
	case "environments":
		cmd := &EnvironmentListCmd{ProjectFlags: c.ProjectFlags}
		return cmd.Run(globals)
	case "all":
		return fmt.Errorf("list all: not yet implemented")
	case "services":
		return fmt.Errorf("list services: not yet implemented")
	case "deployments":
		return fmt.Errorf("list deployments: not yet implemented")
	case "volumes":
		return fmt.Errorf("list volumes: not yet implemented")
	case "buckets":
		return fmt.Errorf("list buckets: not yet implemented")
	case "domains":
		return fmt.Errorf("list domains: not yet implemented")
	default:
		return fmt.Errorf("list %s: unknown entity type", c.Type)
	}
}
