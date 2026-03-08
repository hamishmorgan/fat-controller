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

	"github.com/hamishmorgan/fat-controller/internal/app"
	"github.com/hamishmorgan/fat-controller/internal/config"
	"github.com/hamishmorgan/fat-controller/internal/prompt"
	"github.com/hamishmorgan/fat-controller/internal/railway"
)

// initResolver provides step-by-step data fetching for `config init`.
// Each Fetch method performs only the API call, returning a list of
// selectable items. The picker/selection logic lives in RunConfigInit
// so that loading spinners can wrap just the network call.
type initResolver interface {
	FetchWorkspaces(ctx context.Context) ([]prompt.Item, error)
	FetchProjects(ctx context.Context, workspaceID string) ([]prompt.Item, error)
	FetchEnvironments(ctx context.Context, projectID string) ([]prompt.Item, error)
	FetchServiceList(ctx context.Context, projectID string) ([]railway.ServiceInfo, error)
	FetchLiveState(ctx context.Context, projectID, environmentID string, services []string) (*config.LiveConfig, error)
}

// railwayInitResolver implements initResolver using the Railway API.
type railwayInitResolver struct {
	client *railway.Client
}

func (r *railwayInitResolver) FetchWorkspaces(ctx context.Context) ([]prompt.Item, error) {
	workspaces, err := railway.ListWorkspaces(ctx, r.client)
	if err != nil {
		return nil, err
	}
	items := make([]prompt.Item, len(workspaces))
	for i, ws := range workspaces {
		items[i] = prompt.Item{Name: ws.Name, ID: ws.ID}
	}
	return items, nil
}

func (r *railwayInitResolver) FetchProjects(ctx context.Context, workspaceID string) ([]prompt.Item, error) {
	projects, err := railway.ListProjects(ctx, r.client, workspaceID)
	if err != nil {
		return nil, err
	}
	items := make([]prompt.Item, len(projects))
	for i, p := range projects {
		items[i] = prompt.Item{Name: p.Name, ID: p.ID}
	}
	return items, nil
}

func (r *railwayInitResolver) FetchEnvironments(ctx context.Context, projectID string) ([]prompt.Item, error) {
	envs, err := railway.ListEnvironments(ctx, r.client, projectID)
	if err != nil {
		return nil, err
	}
	items := make([]prompt.Item, len(envs))
	for i, e := range envs {
		items[i] = prompt.Item{Name: e.Name, ID: e.ID}
	}
	return items, nil
}

func (r *railwayInitResolver) FetchServiceList(ctx context.Context, projectID string) ([]railway.ServiceInfo, error) {
	return railway.ListServices(ctx, r.client, projectID)
}

func (r *railwayInitResolver) FetchLiveState(ctx context.Context, projectID, environmentID string, services []string) (*config.LiveConfig, error) {
	return railway.FetchLiveConfig(ctx, r.client, projectID, environmentID, services, nil)
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
// configFile overrides the output path for the config file. When empty the
// default <dir>/fat-controller.toml is used.
func RunConfigInit(ctx context.Context, dir, configFile, workspace, project, environment string, resolver initResolver, interactive, dryRun, yes bool, out io.Writer) error {
	if out == nil {
		out = os.Stdout
	}

	slog.Debug("starting config init", "dir", dir, "config_file", configFile)
	configPath := filepath.Join(dir, config.BaseConfigFile)
	if configFile != "" {
		configPath = configFile
	}

	// 1. Resolve workspace → project → environment step by step.
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

	// 3. Fetch service list (lightweight — single query).
	var svcList []railway.ServiceInfo
	if err := withSpinner(ctx, "Fetching services…", interactive, func() {
		svcList, fetchErr = resolver.FetchServiceList(ctx, projID)
	}); err != nil {
		return err
	}
	if fetchErr != nil {
		return fetchErr
	}

	serviceNames := make([]string, 0, len(svcList))
	for _, si := range svcList {
		serviceNames = append(serviceNames, si.Name)
	}

	// 4. Let the user choose which services to include.
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

	// 5. Fetch full live state for only the selected services.
	var filtered *config.LiveConfig
	if err := withSpinner(ctx, "Fetching live state…", interactive, func() {
		filtered, fetchErr = resolver.FetchLiveState(ctx, projID, envID, selected)
	}); err != nil {
		return err
	}
	if fetchErr != nil {
		return fetchErr
	}

	_, _ = fmt.Fprintln(out)

	// 5. Render the config file.
	slog.Debug("rendering config file", "services", len(filtered.Services))

	configDisplayName := filepath.Base(configPath)
	envPath := defaultSecretsPath(configPath)
	envFileName := filepath.Base(envPath)

	// Compute the env_file setting relative to the config dir so the TOML
	// reference works regardless of where the config lives.
	configDir := filepath.Dir(configPath)
	envFileSetting := envPath
	if rel, err := filepath.Rel(configDir, envPath); err == nil {
		envFileSetting = rel
	}

	// Collect secrets for the secrets file.
	envContent := app.RenderEnvFile(filtered)

	var content string
	if envContent != "" {
		content = config.RenderInitTOMLWithEnvFile(wsName, projName, envName, *filtered, envFileSetting)
	} else {
		content = config.RenderInitTOML(wsName, projName, envName, *filtered)
	}

	if dryRun {
		_, _ = fmt.Fprintf(out, "dry run: would write %s (%d services)\n\n%s\n",
			configDisplayName, len(filtered.Services), content)
		if envContent != "" {
			_, _ = fmt.Fprintf(out, "\ndry run: would write %s\n\n%s\n",
				envFileName, envContent)
			_, _ = fmt.Fprintf(out, "dry run: would ensure %s is in .gitignore\n",
				envFileName)
		}
		return nil
	}

	if !yes && !interactive {
		_, _ = fmt.Fprintf(out, "would write %s (%d services)\n\n%s\n", configDisplayName, len(filtered.Services), content)
		if envContent != "" {
			_, _ = fmt.Fprintf(out, "\nwould write %s\n\n%s\n", envFileName, envContent)
		}
		_, _ = fmt.Fprintf(out, "use --yes to write files\n")
		return nil
	}

	// 6. Write the config file (prompt to overwrite if it exists).
	writeConfig, err := confirmWrite(configPath, configDisplayName, yes, interactive)
	if err != nil {
		return err
	}
	if writeConfig {
		if err := os.MkdirAll(configDir, 0o755); err != nil {
			return fmt.Errorf("creating config dir: %w", err)
		}
		if err := os.WriteFile(configPath, []byte(content+"\n"), 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", configDisplayName, err)
		}
		_, _ = fmt.Fprintf(out, "wrote %s (%d services)\n", configPath, len(filtered.Services))
	} else {
		_, _ = fmt.Fprintf(out, "skipped %s (already exists)\n", configDisplayName)
	}

	// 7. Write secrets file with actual secret values.
	if envContent != "" {
		writeEnv, err := confirmWrite(envPath, envFileName, yes, interactive)
		if err != nil {
			return err
		}
		if writeEnv {
			if err := os.MkdirAll(filepath.Dir(envPath), 0o755); err != nil {
				return fmt.Errorf("creating secrets dir: %w", err)
			}
			if err := os.WriteFile(envPath, []byte(envContent), 0o600); err != nil {
				return fmt.Errorf("writing %s: %w", envFileName, err)
			}
			_, _ = fmt.Fprintf(out, "wrote %s (secret values — do not commit)\n",
				envFileName)

			added, err := app.EnsureGitignoreHasLine(filepath.Dir(envPath), envFileName)
			if err != nil {
				return fmt.Errorf("updating .gitignore: %w", err)
			}
			slog.Debug("gitignore check", "line", envFileName, "added", added)
			if added {
				_, _ = fmt.Fprintf(out, "updated .gitignore (added %s)\n",
					envFileName)
			}
		} else {
			_, _ = fmt.Fprintf(out, "skipped %s (already exists)\n", envFileName)
		}
	}

	return nil
}

// confirmWrite checks if a file already exists and asks for confirmation
// to overwrite. Returns true if the file should be written. With --yes the
// file is always written. In interactive mode the user is prompted. In
// non-interactive mode without --yes, existing files are skipped.
func confirmWrite(path, displayName string, yes, interactive bool) (bool, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return true, nil // file doesn't exist, safe to write
	} else if err != nil {
		return false, fmt.Errorf("checking %s: %w", displayName, err)
	}
	// File exists.
	if yes {
		slog.Debug("overwriting existing file (--yes)", "path", path)
		return true, nil
	}
	if !interactive {
		return false, nil // skip silently in non-interactive mode
	}
	return prompt.Confirm(displayName+" already exists — overwrite?", true)
}
