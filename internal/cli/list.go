package cli

import (
	"errors"
)

// ListCmd implements the unified `list` command.
type ListCmd struct {
	ServiceFlags `kong:"embed"`
	Type         string `arg:"" optional:"" help:"Entity type: all, workspaces, projects, environments, services, deployments, volumes, buckets, domains." default:"services" enum:"all,workspaces,projects,environments,services,deployments,volumes,buckets,domains"`
}

// Run implements `list`.
func (c *ListCmd) Run(globals *Globals) error {
	_ = globals
	return errors.New("list: not implemented")
}
