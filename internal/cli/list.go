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

	services, err := railway.ListServices(ctx, client, projectID)
	if err != nil {
		return fmt.Errorf("listing services: %w", err)
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
			Status:    d.Status,
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

	var svcFilter []string
	if c.Service != "" {
		svcFilter = []string{c.Service}
	}
	live, err := railway.FetchLiveConfig(ctx, client, projectID, envID, svcFilter, nil)
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

	var volFilter []string
	if c.Service != "" {
		volFilter = []string{c.Service}
	}
	live, err := railway.FetchLiveConfig(ctx, client, projectID, envID, volFilter, nil)
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

	buckets, err := railway.ListBuckets(ctx, client, projectID)
	if err != nil {
		return fmt.Errorf("listing buckets: %w", err)
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

// allTreeWorkspace is the structured output type for `list all`.
type allTreeWorkspace struct {
	Name     string           `json:"name" toml:"name"`
	ID       string           `json:"id" toml:"id"`
	Projects []allTreeProject `json:"projects" toml:"projects"`
}

type allTreeProject struct {
	Name         string               `json:"name" toml:"name"`
	ID           string               `json:"id" toml:"id"`
	Environments []allTreeEnvironment `json:"environments" toml:"environments"`
}

type allTreeEnvironment struct {
	Name     string                `json:"name" toml:"name"`
	ID       string                `json:"id" toml:"id"`
	Services []railway.ServiceInfo `json:"services" toml:"services"`
}

func (c *ListCmd) runListAll(globals *Globals) error {
	ctx, cancel := c.TimeoutContext(globals.BaseCtx)
	defer cancel()
	client, err := newClient(&c.ApiFlags, globals.BaseCtx)
	if err != nil {
		return err
	}

	workspaces, err := railway.ListWorkspaces(ctx, client)
	if err != nil {
		return fmt.Errorf("listing workspaces: %w", err)
	}

	var tree []allTreeWorkspace
	for _, ws := range workspaces {
		wsNode := allTreeWorkspace{Name: ws.Name, ID: ws.ID}
		projects, err := railway.ListProjects(ctx, client, ws.ID)
		if err != nil {
			return fmt.Errorf("listing projects for %s: %w", ws.Name, err)
		}
		for _, proj := range projects {
			projNode := allTreeProject{Name: proj.Name, ID: proj.ID}
			envs, err := railway.ListEnvironments(ctx, client, proj.ID)
			if err != nil {
				return fmt.Errorf("listing environments for %s: %w", proj.Name, err)
			}
			// Services are project-scoped (shared across environments).
			services, err := railway.ListServices(ctx, client, proj.ID)
			if err != nil {
				return fmt.Errorf("listing services for %s: %w", proj.Name, err)
			}
			for _, env := range envs {
				envNode := allTreeEnvironment{Name: env.Name, ID: env.ID}
				envNode.Services = services
				projNode.Environments = append(projNode.Environments, envNode)
			}
			wsNode.Projects = append(wsNode.Projects, projNode)
		}
		tree = append(tree, wsNode)
	}

	if isStructuredOutput(globals) {
		return writeStructured(os.Stdout, globals.Output, tree)
	}

	// Text tree output matching ARCHITECTURE.md example.
	for _, ws := range tree {
		if _, err := fmt.Fprintln(os.Stdout, ws.Name); err != nil {
			return err
		}
		for _, proj := range ws.Projects {
			if _, err := fmt.Fprintf(os.Stdout, "  %s\n", proj.Name); err != nil {
				return err
			}
			for _, env := range proj.Environments {
				svcNames := make([]string, len(env.Services))
				for i, s := range env.Services {
					svcNames[i] = s.Name
				}
				if len(svcNames) > 0 {
					if _, err := fmt.Fprintf(os.Stdout, "    %s\n      %s\n", env.Name, joinComma(svcNames)); err != nil {
						return err
					}
				} else {
					if _, err := fmt.Fprintf(os.Stdout, "    %s\n", env.Name); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

// joinComma joins strings with ", ".
func joinComma(ss []string) string {
	if len(ss) == 0 {
		return ""
	}
	result := ss[0]
	for _, s := range ss[1:] {
		result += ", " + s
	}
	return result
}

// resolveProjectEnv resolves workspace/project/environment names to IDs.
func resolveProjectEnv(ctx context.Context, client *railway.Client, workspace, project, environment string) (string, string, error) {
	fetcher := &defaultConfigFetcher{client: client}
	resolved, err := fetcher.Resolve(ctx, workspace, project, environment)
	if err != nil {
		return "", "", err
	}
	return resolved.ProjectID, resolved.EnvironmentID, nil
}

// resolveServiceID resolves a service name to its ID within a project.
func resolveServiceID(ctx context.Context, client *railway.Client, workspace, project, service string) (string, error) {
	projID, _, err := resolveProjectEnv(ctx, client, workspace, project, "")
	if err != nil {
		return "", err
	}
	return railway.ResolveServiceID(ctx, client, projID, service)
}
