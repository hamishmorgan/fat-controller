package cli

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hamishmorgan/fat-controller/internal/config"
	"github.com/hamishmorgan/fat-controller/internal/prompt"
	"github.com/hamishmorgan/fat-controller/internal/railway"
)

func ensureGitignoreHasLine(dir, line string) (bool, error) {
	gitignorePath := filepath.Join(dir, ".gitignore")

	b, err := os.ReadFile(gitignorePath)
	if err != nil {
		if os.IsNotExist(err) {
			if err := os.WriteFile(gitignorePath, []byte(line+"\n"), 0o644); err != nil {
				return false, err
			}
			return true, nil
		}
		return false, err
	}

	lines := strings.Split(string(b), "\n")
	for _, existing := range lines {
		if strings.TrimSpace(existing) == line {
			return false, nil
		}
	}

	f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return false, err
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()

	if len(b) > 0 && b[len(b)-1] != '\n' {
		if _, err := f.WriteString("\n"); err != nil {
			return false, err
		}
	}
	if _, err := f.WriteString(line + "\n"); err != nil {
		return false, err
	}
	return true, nil
}

// initResolver provides step-by-step resolution for `config init`,
// returning both name and ID for each entity so summaries can be printed.
type initResolver interface {
	ResolveWorkspace(ctx context.Context, workspace string) (name, id string, err error)
	ResolveProject(ctx context.Context, workspaceID, project string) (name, id string, err error)
	ResolveEnvironment(ctx context.Context, projectID, env string) (name, id string, err error)
	Fetch(ctx context.Context, projectID, environmentID string) (*config.LiveConfig, error)
}

// railwayInitResolver implements initResolver using the Railway API.
type railwayInitResolver struct {
	client *railway.Client
}

func (r *railwayInitResolver) ResolveWorkspace(ctx context.Context, workspace string) (string, string, error) {
	e, err := railway.ResolveWorkspaceNamed(ctx, r.client, workspace, prompt.PickOpts{ForcePrompt: true})
	if err != nil {
		return "", "", err
	}
	return e.Name, e.ID, nil
}

func (r *railwayInitResolver) ResolveProject(ctx context.Context, workspaceID, project string) (string, string, error) {
	e, err := railway.ResolveProjectNamed(ctx, r.client, workspaceID, project, prompt.PickOpts{ForcePrompt: true})
	if err != nil {
		return "", "", err
	}
	return e.Name, e.ID, nil
}

func (r *railwayInitResolver) ResolveEnvironment(ctx context.Context, projectID, env string) (string, string, error) {
	e, err := railway.ResolveEnvironmentNamed(ctx, r.client, projectID, env, prompt.PickOpts{ForcePrompt: true})
	if err != nil {
		return "", "", err
	}
	return e.Name, e.ID, nil
}

func (r *railwayInitResolver) Fetch(ctx context.Context, projectID, environmentID string) (*config.LiveConfig, error) {
	return railway.FetchLiveConfig(ctx, r.client, projectID, environmentID, "")
}

// Run implements `config init`.
func (c *ConfigInitCmd) Run(globals *Globals) error {
	ctx, cancel := globals.TimeoutContext(globals.BaseCtx)
	defer cancel()
	client, err := newClient(globals)
	if err != nil {
		return err
	}
	resolver := &railwayInitResolver{client: client}

	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	return RunConfigInit(ctx, wd, globals.Workspace, globals.Project, globals.Environment, resolver, prompt.StdinIsInteractive(), os.Stdout)
}

// RunConfigInit is the testable core of `config init`.
func RunConfigInit(ctx context.Context, dir, workspace, project, environment string, resolver initResolver, interactive bool, out io.Writer) error {
	if out == nil {
		out = os.Stdout
	}

	slog.Debug("starting config init", "dir", dir)
	// 1. Check for existing config — prompt to overwrite if interactive.
	configPath := filepath.Join(dir, config.BaseConfigFile)
	if _, err := os.Stat(configPath); err == nil {
		if !interactive {
			return fmt.Errorf("%s already exists — refusing to overwrite", config.BaseConfigFile)
		}
		ok, confirmErr := prompt.Confirm(config.BaseConfigFile+" already exists — overwrite?", false)
		if confirmErr != nil {
			return confirmErr
		}
		if !ok {
			return fmt.Errorf("%s already exists — aborting", config.BaseConfigFile)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("checking %s: %w", config.BaseConfigFile, err)
	}

	// 2. Resolve workspace → project → environment step by step,
	//    printing a summary line after each selection.
	wsName, wsID, err := resolver.ResolveWorkspace(ctx, workspace)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "  Workspace: %s\n", wsName)

	projName, projID, err := resolver.ResolveProject(ctx, wsID, project)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "  Project: %s\n", projName)

	envName, envID, err := resolver.ResolveEnvironment(ctx, projID, environment)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "  Environment: %s\n", envName)

	// 3. Fetch live state.
	live, err := resolver.Fetch(ctx, projID, envID)
	if err != nil {
		return err
	}

	// 4. Let the user choose which services to include.
	serviceNames := make([]string, 0, len(live.Services))
	for name := range live.Services {
		serviceNames = append(serviceNames, name)
	}
	selected, err := prompt.PickServices(serviceNames, interactive)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "  Services: %s (%d selected)\n", strings.Join(selected, ", "), len(selected))

	selectedSet := make(map[string]bool, len(selected))
	for _, name := range selected {
		selectedSet[name] = true
	}
	// Filter live config to only selected services.
	filtered := &config.LiveConfig{
		ProjectID:     live.ProjectID,
		EnvironmentID: live.EnvironmentID,
		Shared:        live.Shared,
		Services:      make(map[string]*config.ServiceConfig, len(selected)),
	}
	for name, svc := range live.Services {
		if selectedSet[name] {
			filtered.Services[name] = svc
		}
	}

	_, _ = fmt.Fprintln(out)

	// 5. Render and write the config file.
	slog.Debug("rendering config file", "services", len(filtered.Services))
	content := config.RenderInitTOML(wsName, projName, envName, *filtered)
	if err := os.WriteFile(configPath, []byte(content+"\n"), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", config.BaseConfigFile, err)
	}
	_, _ = fmt.Fprintf(out, "wrote %s (%d services)\n", config.BaseConfigFile, len(filtered.Services))

	// 6. Create .local.toml with interpolation refs for secrets.
	localPath := filepath.Join(dir, config.LocalConfigFile)
	if _, err := os.Stat(localPath); os.IsNotExist(err) {
		localContent := renderLocalTOML(filtered)
		if err := os.WriteFile(localPath, []byte(localContent), 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", config.LocalConfigFile, err)
		}
		_, _ = fmt.Fprintf(out, "wrote %s (local overrides, gitignored)\n", config.LocalConfigFile)
	}

	added, err := ensureGitignoreHasLine(dir, config.LocalConfigFile)
	if err != nil {
		return fmt.Errorf("updating .gitignore: %w", err)
	}
	slog.Debug("gitignore check", "line", config.LocalConfigFile, "added", added)
	if added {
		_, _ = fmt.Fprintf(out, "updated %s (added %s)\n", ".gitignore", config.LocalConfigFile)
	}

	return nil
}

// renderLocalTOML generates the contents of the .local.toml file.
// For each service (and shared), any variable whose name matches sensitive
// keywords gets a `VAR = "${VAR}"` interpolation reference. If no secrets
// are found, returns a commented stub.
func renderLocalTOML(cfg *config.LiveConfig) string {
	masker := config.NewMasker(nil, nil)

	var out strings.Builder
	out.WriteString("# Local overrides (gitignored). Use for secrets and per-developer settings.\n")
	out.WriteString("# Values use ${VAR} syntax to read from your local environment.\n\n")

	wrote := false

	// Shared variables.
	if len(cfg.Shared) > 0 {
		var secrets []string
		for name := range cfg.Shared {
			if masker.IsSensitive(name) {
				secrets = append(secrets, name)
			}
		}
		if len(secrets) > 0 {
			sort.Strings(secrets)
			out.WriteString("[shared.variables]\n")
			for _, name := range secrets {
				_, _ = fmt.Fprintf(&out, "%s = \"${%s}\"\n", name, name)
			}
			out.WriteString("\n")
			wrote = true
		}
	}

	// Per-service variables.
	serviceNames := make([]string, 0, len(cfg.Services))
	for name := range cfg.Services {
		serviceNames = append(serviceNames, name)
	}
	sort.Strings(serviceNames)

	for _, svcName := range serviceNames {
		svc := cfg.Services[svcName]
		if len(svc.Variables) == 0 {
			continue
		}
		var secrets []string
		for name := range svc.Variables {
			if masker.IsSensitive(name) {
				secrets = append(secrets, name)
			}
		}
		if len(secrets) > 0 {
			sort.Strings(secrets)
			_, _ = fmt.Fprintf(&out, "[%s.variables]\n", svcName)
			for _, name := range secrets {
				_, _ = fmt.Fprintf(&out, "%s = \"${%s}\"\n", name, name)
			}
			out.WriteString("\n")
			wrote = true
		}
	}

	if !wrote {
		out.WriteString("# No secrets detected. Add overrides here as needed.\n")
		out.WriteString("# Example:\n")
		out.WriteString("#   [api.variables]\n")
		out.WriteString("#   STRIPE_KEY = \"${STRIPE_KEY}\"\n")
	}

	return out.String()
}
