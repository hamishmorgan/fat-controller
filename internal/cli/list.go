package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/hamishmorgan/fat-controller/internal/railway"
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
	case "services":
		return c.runListServices(globals)
	case "deployments":
		return c.runListDeployments(globals)
	case "domains":
		return c.runListDomains(globals)
	case "all":
		return c.runListAll(globals)
	case "volumes":
		return c.runListVolumes(globals)
	case "buckets":
		return c.runListBuckets(globals)
	default:
		return fmt.Errorf("list %s: unknown entity type", c.Type)
	}
}

func (c *ListCmd) runListServices(globals *Globals) error {
	ctx, cancel := c.TimeoutContext(globals.BaseCtx)
	defer cancel()
	client, err := newClient(&c.ApiFlags, globals.BaseCtx)
	if err != nil {
		return err
	}

	projectID, _, err := resolveProjectEnv(ctx, client, c.Workspace, c.Project, c.Environment)
	if err != nil {
		return err
	}

	resp, err := railway.ProjectServices(ctx, client.GQL(), projectID)
	if err != nil {
		return fmt.Errorf("listing services: %w", err)
	}

	type serviceOut struct {
		ID   string `json:"id" toml:"id"`
		Name string `json:"name" toml:"name"`
	}

	services := make([]serviceOut, 0, len(resp.Project.Services.Edges))
	for _, edge := range resp.Project.Services.Edges {
		services = append(services, serviceOut{ID: edge.Node.Id, Name: edge.Node.Name})
	}

	if isStructuredOutput(globals) {
		return writeStructured(os.Stdout, globals.Output, services)
	}

	for _, svc := range services {
		if _, err := fmt.Fprintf(os.Stdout, "%-40s %s\n", svc.Name, svc.ID); err != nil {
			return err
		}
	}
	return nil
}

func (c *ListCmd) runListDeployments(globals *Globals) error {
	ctx, cancel := c.TimeoutContext(globals.BaseCtx)
	defer cancel()
	client, err := newClient(&c.ApiFlags, globals.BaseCtx)
	if err != nil {
		return err
	}

	_, envID, err := resolveProjectEnv(ctx, client, c.Workspace, c.Project, c.Environment)
	if err != nil {
		return err
	}

	if c.Service == "" {
		return fmt.Errorf("--service is required for list deployments")
	}

	// Resolve service name to ID.
	svcID, err := resolveServiceID(ctx, client, c.Workspace, c.Project, c.Service)
	if err != nil {
		return err
	}

	deployments, _, err := railway.ListDeployments(ctx, client, envID, svcID, 10, nil)
	if err != nil {
		return fmt.Errorf("listing deployments: %w", err)
	}

	type deployOut struct {
		ID        string `json:"id" toml:"id"`
		Status    string `json:"status" toml:"status"`
		CreatedAt string `json:"created_at" toml:"created_at"`
	}

	out := make([]deployOut, 0, len(deployments))
	for _, d := range deployments {
		out = append(out, deployOut{
			ID:        d.ID,
			Status:    string(d.Status),
			CreatedAt: d.CreatedAt.Format("2006-01-02T15:04:05Z"),
		})
	}

	if isStructuredOutput(globals) {
		return writeStructured(os.Stdout, globals.Output, out)
	}

	for _, d := range out {
		if _, err := fmt.Fprintf(os.Stdout, "%-40s %-15s %s\n", d.ID, d.Status, d.CreatedAt); err != nil {
			return err
		}
	}
	return nil
}

func (c *ListCmd) runListDomains(globals *Globals) error {
	ctx, cancel := c.TimeoutContext(globals.BaseCtx)
	defer cancel()
	client, err := newClient(&c.ApiFlags, globals.BaseCtx)
	if err != nil {
		return err
	}

	projectID, envID, err := resolveProjectEnv(ctx, client, c.Workspace, c.Project, c.Environment)
	if err != nil {
		return err
	}

	live, err := railway.FetchLiveConfig(ctx, client, projectID, envID, c.Service)
	if err != nil {
		return err
	}

	type domainOut struct {
		Service string `json:"service" toml:"service"`
		Domain  string `json:"domain" toml:"domain"`
		Type    string `json:"type" toml:"type"`
	}

	var domains []domainOut
	for _, svc := range live.Services {
		for _, d := range svc.Domains {
			dtype := "custom"
			if d.IsService {
				dtype = "service"
			}
			domains = append(domains, domainOut{Service: svc.Name, Domain: d.Domain, Type: dtype})
		}
	}

	if isStructuredOutput(globals) {
		return writeStructured(os.Stdout, globals.Output, domains)
	}

	for _, d := range domains {
		if _, err := fmt.Fprintf(os.Stdout, "%-30s %-50s %s\n", d.Service, d.Domain, d.Type); err != nil {
			return err
		}
	}
	return nil
}

func (c *ListCmd) runListVolumes(globals *Globals) error {
	ctx, cancel := c.TimeoutContext(globals.BaseCtx)
	defer cancel()
	client, err := newClient(&c.ApiFlags, globals.BaseCtx)
	if err != nil {
		return err
	}

	projectID, envID, err := resolveProjectEnv(ctx, client, c.Workspace, c.Project, c.Environment)
	if err != nil {
		return err
	}

	live, err := railway.FetchLiveConfig(ctx, client, projectID, envID, c.Service)
	if err != nil {
		return err
	}

	type volumeOut struct {
		Service string `json:"service" toml:"service"`
		Name    string `json:"name" toml:"name"`
		Mount   string `json:"mount" toml:"mount"`
		Region  string `json:"region,omitempty" toml:"region,omitempty"`
	}

	var volumes []volumeOut
	for _, svc := range live.Services {
		for _, v := range svc.Volumes {
			volumes = append(volumes, volumeOut{
				Service: svc.Name,
				Name:    v.Name,
				Mount:   v.MountPath,
				Region:  v.Region,
			})
		}
	}

	if isStructuredOutput(globals) {
		return writeStructured(os.Stdout, globals.Output, volumes)
	}

	for _, v := range volumes {
		if _, err := fmt.Fprintf(os.Stdout, "%-30s %-20s %-30s %s\n", v.Service, v.Name, v.Mount, v.Region); err != nil {
			return err
		}
	}
	return nil
}

func (c *ListCmd) runListBuckets(globals *Globals) error {
	ctx, cancel := c.TimeoutContext(globals.BaseCtx)
	defer cancel()
	client, err := newClient(&c.ApiFlags, globals.BaseCtx)
	if err != nil {
		return err
	}

	projectID, _, err := resolveProjectEnv(ctx, client, c.Workspace, c.Project, c.Environment)
	if err != nil {
		return err
	}

	resp, err := railway.ProjectBuckets(ctx, client.GQL(), projectID)
	if err != nil {
		return fmt.Errorf("listing buckets: %w", err)
	}

	type bucketOut struct {
		ID   string `json:"id" toml:"id"`
		Name string `json:"name" toml:"name"`
	}

	buckets := make([]bucketOut, 0, len(resp.Project.Buckets.Edges))
	for _, edge := range resp.Project.Buckets.Edges {
		buckets = append(buckets, bucketOut{ID: edge.Node.Id, Name: edge.Node.Name})
	}

	if isStructuredOutput(globals) {
		return writeStructured(os.Stdout, globals.Output, buckets)
	}

	for _, b := range buckets {
		if _, err := fmt.Fprintf(os.Stdout, "%-40s %s\n", b.Name, b.ID); err != nil {
			return err
		}
	}
	return nil
}

func (c *ListCmd) runListAll(globals *Globals) error {
	// Run services list (the most common "all" view).
	return c.runListServices(globals)
}

// resolveProjectEnv resolves workspace/project/environment names to IDs.
func resolveProjectEnv(ctx context.Context, client *railway.Client, workspace, project, environment string) (string, string, error) {
	fetcher := &defaultConfigFetcher{client: client}
	return fetcher.Resolve(ctx, workspace, project, environment)
}

// resolveServiceID resolves a service name to its ID within a project.
func resolveServiceID(ctx context.Context, client *railway.Client, workspace, project, service string) (string, error) {
	projID, _, err := resolveProjectEnv(ctx, client, workspace, project, "")
	if err != nil {
		return "", err
	}
	resp, err := railway.ProjectServices(ctx, client.GQL(), projID)
	if err != nil {
		return "", fmt.Errorf("listing services: %w", err)
	}
	for _, edge := range resp.Project.Services.Edges {
		if edge.Node.Name == service {
			return edge.Node.Id, nil
		}
	}
	return "", fmt.Errorf("service %q not found in project", service)
}
