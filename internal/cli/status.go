package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/hamishmorgan/fat-controller/internal/config"
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

	projID, envID, err := railway.ResolveProjectEnvironment(ctx, client, c.Workspace, c.Project, c.Environment, interactivePicker)
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

	// Fetch live config for sub-resource info (domains, volumes, etc.).
	live, liveErr := railway.FetchLiveConfig(ctx, client, projID, envID, nil)
	if liveErr != nil {
		// Non-fatal: we can still show deployment status without sub-resource info.
		live = nil
	}

	return RunStatus(ctx, globals, envID, targets, live, func(ctx context.Context, environmentID, serviceID string) ([]railway.DeploymentInfo, error) {
		d, _, err := railway.ListDeployments(ctx, client, environmentID, serviceID, 1, nil)
		return d, err
	}, os.Stdout, os.Stderr)
}

type StatusDomain struct {
	Domain string `json:"domain" toml:"domain"`
	Type   string `json:"type" toml:"type"` // "custom" or "service"
	Port   int    `json:"port,omitempty" toml:"port"`
}

type StatusVolume struct {
	Name  string `json:"name" toml:"name"`
	Mount string `json:"mount" toml:"mount"`
}

type StatusItem struct {
	Service      string         `json:"service" toml:"service"`
	ServiceID    string         `json:"service_id" toml:"service_id"`
	Status       string         `json:"status" toml:"status"`
	DeploymentID string         `json:"deployment_id,omitempty" toml:"deployment_id"`
	CreatedAt    string         `json:"created_at,omitempty" toml:"created_at"`
	Domains      []StatusDomain `json:"domains,omitempty" toml:"domains"`
	Volumes      []StatusVolume `json:"volumes,omitempty" toml:"volumes"`
	TCPProxies   int            `json:"tcp_proxies,omitempty" toml:"tcp_proxies"`
	Network      bool           `json:"network,omitempty" toml:"network"`
	Healthcheck  string         `json:"healthcheck,omitempty" toml:"healthcheck"`
	Error        string         `json:"error,omitempty" toml:"error"`
}

type StatusOutput struct {
	EnvironmentID string       `json:"environment_id" toml:"environment_id"`
	Statuses      []StatusItem `json:"statuses" toml:"statuses"`
}

// RunStatus is the testable core of `status`.
func RunStatus(ctx context.Context, globals *Globals, environmentID string, targets []serviceTarget, live *config.LiveConfig, listLatest func(ctx context.Context, environmentID, serviceID string) ([]railway.DeploymentInfo, error), out, errOut io.Writer) error {
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
			} else if len(deployments) == 0 {
				it.Status = "no deployments"
			} else {
				d := deployments[0]
				it.DeploymentID = d.ID
				it.Status = strings.ToLower(d.Status)
				it.CreatedAt = d.CreatedAt.Format("2006-01-02T15:04:05Z07:00")
			}
			populateSubResourceStatus(&it, live, svc.Name)
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
			status = fmt.Sprintf("%s (deployed %s)", strings.ToLower(d.Status), d.CreatedAt.Format("2006-01-02 15:04"))
		}

		// Build sub-resource summary.
		var extras []string
		if live != nil {
			if liveSvc, ok := live.Services[svc.Name]; ok {
				if len(liveSvc.Domains) > 0 {
					extras = append(extras, fmt.Sprintf("%d domains", len(liveSvc.Domains)))
				}
				if len(liveSvc.Volumes) > 0 {
					extras = append(extras, fmt.Sprintf("%d volumes", len(liveSvc.Volumes)))
				}
				if len(liveSvc.TCPProxies) > 0 {
					extras = append(extras, fmt.Sprintf("%d tcp", len(liveSvc.TCPProxies)))
				}
				if liveSvc.Network != nil {
					extras = append(extras, "network")
				}
				if liveSvc.Deploy.HealthcheckPath != nil {
					extras = append(extras, "healthcheck:"+*liveSvc.Deploy.HealthcheckPath)
				}
			}
		}

		line := fmt.Sprintf("%-20s %s", svc.Name, status)
		if len(extras) > 0 {
			line += "  [" + strings.Join(extras, ", ") + "]"
		}
		_, _ = fmt.Fprintln(out, line)
	}
	return nil
}

// populateSubResourceStatus fills in domain, volume, healthcheck info from the live config.
func populateSubResourceStatus(it *StatusItem, live *config.LiveConfig, serviceName string) {
	if live == nil {
		return
	}
	liveSvc, ok := live.Services[serviceName]
	if !ok {
		return
	}

	for _, d := range liveSvc.Domains {
		domType := "custom"
		if d.IsService {
			domType = "service"
		}
		port := 0
		if d.TargetPort != nil {
			port = *d.TargetPort
		}
		it.Domains = append(it.Domains, StatusDomain{
			Domain: d.Domain,
			Type:   domType,
			Port:   port,
		})
	}

	for _, v := range liveSvc.Volumes {
		it.Volumes = append(it.Volumes, StatusVolume{
			Name:  v.Name,
			Mount: v.MountPath,
		})
	}

	it.TCPProxies = len(liveSvc.TCPProxies)
	it.Network = liveSvc.Network != nil
	if liveSvc.Deploy.HealthcheckPath != nil {
		it.Healthcheck = *liveSvc.Deploy.HealthcheckPath
	}
}
