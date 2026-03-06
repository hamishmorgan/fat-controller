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

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/huh/spinner"

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

// initResolver provides step-by-step data fetching for `config init`.
// Each Fetch method performs only the API call, returning a list of
// selectable items. The picker/selection logic lives in RunConfigInit
// so that loading spinners can wrap just the network call.
type initResolver interface {
	FetchWorkspaces(ctx context.Context) ([]prompt.Item, error)
	FetchProjects(ctx context.Context, workspaceID string) ([]prompt.Item, error)
	FetchEnvironments(ctx context.Context, projectID string) ([]prompt.Item, error)
	FetchLiveState(ctx context.Context, projectID, environmentID string) (*config.LiveConfig, error)
}

// railwayInitResolver implements initResolver using the Railway API.
type railwayInitResolver struct {
	client *railway.Client
}

func (r *railwayInitResolver) FetchWorkspaces(ctx context.Context) ([]prompt.Item, error) {
	resp, err := railway.ApiToken(ctx, r.client.GQL())
	if err != nil {
		return nil, err
	}
	items := make([]prompt.Item, len(resp.ApiToken.Workspaces))
	for i, ws := range resp.ApiToken.Workspaces {
		items[i] = prompt.Item{Name: ws.Name, ID: ws.Id}
	}
	return items, nil
}

func (r *railwayInitResolver) FetchProjects(ctx context.Context, workspaceID string) ([]prompt.Item, error) {
	resp, err := railway.Projects(ctx, r.client.GQL(), &workspaceID)
	if err != nil {
		return nil, err
	}
	items := make([]prompt.Item, len(resp.Projects.Edges))
	for i, edge := range resp.Projects.Edges {
		items[i] = prompt.Item{Name: edge.Node.Name, ID: edge.Node.Id}
	}
	return items, nil
}

func (r *railwayInitResolver) FetchEnvironments(ctx context.Context, projectID string) ([]prompt.Item, error) {
	resp, err := railway.Environments(ctx, r.client.GQL(), projectID)
	if err != nil {
		return nil, err
	}
	items := make([]prompt.Item, len(resp.Environments.Edges))
	for i, edge := range resp.Environments.Edges {
		items[i] = prompt.Item{Name: edge.Node.Name, ID: edge.Node.Id}
	}
	return items, nil
}

func (r *railwayInitResolver) FetchLiveState(ctx context.Context, projectID, environmentID string) (*config.LiveConfig, error) {
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

	return RunConfigInit(ctx, wd, globals.Workspace, globals.Project, globals.Environment, resolver, prompt.StdinIsInteractive(), globals.DryRun, globals.Yes, os.Stdout)
}

// withSpinner wraps an action in a loading spinner when interactive mode is
// enabled. In non-interactive mode the action runs directly.
func withSpinner(ctx context.Context, title string, interactive bool, action func()) error {
	if !interactive {
		action()
		return nil
	}
	return spinner.New().
		Title(title).
		Context(ctx).
		Action(action).
		Run()
}

// selectByName looks up an item by name from a fetched list.
// Used when a CLI flag provides the name directly.
func selectByName(label string, items []prompt.Item, name string) (string, string, error) {
	for _, it := range items {
		if it.Name == name {
			return it.Name, it.ID, nil
		}
	}
	return "", "", fmt.Errorf("%s not found: %s", label, name)
}

// lookupName returns the display name for an ID in the items list.
func lookupName(items []prompt.Item, id string) string {
	for _, it := range items {
		if it.ID == id {
			return it.Name
		}
	}
	return id
}

// summaryNote creates a huh.Note field displaying a completed selection.
func summaryNote(label, value string) huh.Field {
	return huh.NewNote().Title(fmt.Sprintf("%s: %s", label, value))
}

// RunConfigInit is the testable core of `config init`.
func RunConfigInit(ctx context.Context, dir, workspace, project, environment string, resolver initResolver, interactive, dryRun, yes bool, out io.Writer) error {
	if out == nil {
		out = os.Stdout
	}

	slog.Debug("starting config init", "dir", dir)
	// 1. Check for existing config — prompt to overwrite unless --yes.
	configPath := filepath.Join(dir, config.BaseConfigFile)
	if _, err := os.Stat(configPath); err == nil {
		if yes {
			// --yes: proceed to overwrite without prompting.
			slog.Debug("overwriting existing config (--yes)", "path", configPath)
		} else if !interactive {
			return fmt.Errorf("%s already exists — pass --yes to overwrite", config.BaseConfigFile)
		} else {
			ok, confirmErr := prompt.Confirm(config.BaseConfigFile+" already exists — overwrite?", false)
			if confirmErr != nil {
				return confirmErr
			}
			if !ok {
				return fmt.Errorf("%s already exists — aborting", config.BaseConfigFile)
			}
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("checking %s: %w", config.BaseConfigFile, err)
	}

	// 2. Resolve workspace → project → environment step by step.
	//    Each API call is wrapped in a spinner (interactive only).
	//    In interactive mode, each picker form includes Note fields
	//    showing all previous selections for context.

	var err error
	var wsItems []prompt.Item
	var fetchErr error
	if err := withSpinner(ctx, "Fetching workspaces…", interactive, func() {
		wsItems, fetchErr = resolver.FetchWorkspaces(ctx)
	}); err != nil {
		return err
	}
	if fetchErr != nil {
		return fetchErr
	}
	var wsName, wsID string
	if workspace != "" {
		wsName, wsID, err = selectByName("workspace", wsItems, workspace)
	} else if interactive {
		var picked string
		err = prompt.RunFields(prompt.SelectField("workspace", wsItems, &picked))
		if err == nil {
			wsName = lookupName(wsItems, picked)
			wsID = picked
		}
	} else {
		var picked string
		picked, err = prompt.PickItem("workspace", wsItems, false, prompt.PickOpts{ForcePrompt: true})
		if err == nil {
			wsName = lookupName(wsItems, picked)
			wsID = picked
		}
	}
	if err != nil {
		return err
	}
	if !interactive {
		_, _ = fmt.Fprintf(out, "  Workspace: %s\n", wsName)
	}

	var projItems []prompt.Item
	if err := withSpinner(ctx, "Fetching projects…", interactive, func() {
		projItems, fetchErr = resolver.FetchProjects(ctx, wsID)
	}); err != nil {
		return err
	}
	if fetchErr != nil {
		return fetchErr
	}
	var projName, projID string
	if project != "" {
		projName, projID, err = selectByName("project", projItems, project)
	} else if interactive {
		var picked string
		err = prompt.RunFields(
			summaryNote("Workspace", wsName),
			prompt.SelectField("project", projItems, &picked),
		)
		if err == nil {
			projName = lookupName(projItems, picked)
			projID = picked
		}
	} else {
		var picked string
		picked, err = prompt.PickItem("project", projItems, false, prompt.PickOpts{ForcePrompt: true})
		if err == nil {
			projName = lookupName(projItems, picked)
			projID = picked
		}
	}
	if err != nil {
		return err
	}
	if !interactive {
		_, _ = fmt.Fprintf(out, "  Project: %s\n", projName)
	}

	var envItems []prompt.Item
	if err := withSpinner(ctx, "Fetching environments…", interactive, func() {
		envItems, fetchErr = resolver.FetchEnvironments(ctx, projID)
	}); err != nil {
		return err
	}
	if fetchErr != nil {
		return fetchErr
	}
	var envName, envID string
	if environment != "" {
		envName, envID, err = selectByName("environment", envItems, environment)
	} else if interactive {
		var picked string
		err = prompt.RunFields(
			summaryNote("Workspace", wsName),
			summaryNote("Project", projName),
			prompt.SelectField("environment", envItems, &picked),
		)
		if err == nil {
			envName = lookupName(envItems, picked)
			envID = picked
		}
	} else {
		var picked string
		picked, err = prompt.PickItem("environment", envItems, false, prompt.PickOpts{ForcePrompt: true})
		if err == nil {
			envName = lookupName(envItems, picked)
			envID = picked
		}
	}
	if err != nil {
		return err
	}
	if !interactive {
		_, _ = fmt.Fprintf(out, "  Environment: %s\n", envName)
	}

	// 3. Fetch live state.
	var live *config.LiveConfig
	if err := withSpinner(ctx, "Fetching live state…", interactive, func() {
		live, fetchErr = resolver.FetchLiveState(ctx, projID, envID)
	}); err != nil {
		return err
	}
	if fetchErr != nil {
		return fetchErr
	}

	// 4. Let the user choose which services to include.
	serviceNames := make([]string, 0, len(live.Services))
	for name := range live.Services {
		serviceNames = append(serviceNames, name)
	}
	var selected []string
	if interactive {
		err = prompt.RunFields(
			summaryNote("Workspace", wsName),
			summaryNote("Project", projName),
			summaryNote("Environment", envName),
			prompt.MultiSelectField("Select services to include:", serviceNames, &selected),
		)
		if err != nil {
			return err
		}
		sort.Strings(selected)
	} else {
		selected, err = prompt.PickServices(serviceNames, false)
		if err != nil {
			return err
		}
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

	// 5. Render the config file.
	slog.Debug("rendering config file", "services", len(filtered.Services))
	content := config.RenderInitTOML(wsName, projName, envName, *filtered)

	if dryRun {
		_, _ = fmt.Fprintf(out, "dry run: would write %s (%d services)\n\n%s\n", config.BaseConfigFile, len(filtered.Services), content)

		localPath := filepath.Join(dir, config.LocalConfigFile)
		if _, statErr := os.Stat(localPath); os.IsNotExist(statErr) {
			localContent := renderLocalTOML(filtered)
			_, _ = fmt.Fprintf(out, "\ndry run: would write %s\n\n%s\n", config.LocalConfigFile, localContent)
		}

		_, _ = fmt.Fprintf(out, "dry run: would ensure %s is in .gitignore\n", config.LocalConfigFile)
		return nil
	}

	if !yes && !interactive {
		_, _ = fmt.Fprintf(out, "would write %s (%d services)\n\n%s\n", config.BaseConfigFile, len(filtered.Services), content)
		_, _ = fmt.Fprintf(out, "use --yes to write files\n")
		return nil
	}

	// 6. Write the config file.
	if err := os.WriteFile(configPath, []byte(content+"\n"), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", config.BaseConfigFile, err)
	}
	_, _ = fmt.Fprintf(out, "wrote %s (%d services)\n", config.BaseConfigFile, len(filtered.Services))

	// 7. Create .local.toml with interpolation refs for secrets.
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
