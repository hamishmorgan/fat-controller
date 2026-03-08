package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/hamishmorgan/fat-controller/internal/config"
)

// NewCmd implements the `new` command group.
type NewCmd struct {
	Project     NewProjectCmd     `cmd:"" help:"Create a new project."`
	Environment NewEnvironmentCmd `cmd:"" help:"Create a new environment."`
	Service     NewServiceCmd     `cmd:"" help:"Add a service to config."`
}

// NewProjectCmd implements `new project`.
type NewProjectCmd struct {
	WorkspaceFlags `kong:"embed"`
	PromptFlags    `kong:"embed"`
	Name           string `arg:"" optional:"" help:"Project name."`
}

// Run implements `new project`.
func (c *NewProjectCmd) Run(globals *Globals) error {
	if c.Name == "" {
		return fmt.Errorf("project name is required")
	}

	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	// Try to load existing config.
	result, loadErr := config.LoadCascade(config.LoadOptions{WorkDir: wd})
	if loadErr == nil && result.Config.Project != nil && result.Config.Project.Name != "" {
		return fmt.Errorf("project already defined in config as %q", result.Config.Project.Name)
	}

	configPath := ""
	if result != nil && result.PrimaryFile != "" {
		configPath = result.PrimaryFile
	} else {
		configPath = filepath.Join(wd, "fat-controller.toml")
	}

	snippet := struct {
		Project struct {
			Name string `toml:"name"`
		} `toml:"project"`
	}{}
	snippet.Project.Name = c.Name

	f, err := os.OpenFile(configPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("opening %s: %w", configPath, err)
	}
	defer func() { _ = f.Close() }()

	if _, err := f.WriteString("\n"); err != nil {
		return fmt.Errorf("writing to %s: %w", configPath, err)
	}
	if err := toml.NewEncoder(f).Encode(snippet); err != nil {
		return fmt.Errorf("writing to %s: %w", configPath, err)
	}

	if globals.Quiet == 0 {
		_, _ = fmt.Fprintf(os.Stdout, "Added project %q to %s\n", c.Name, configPath)
	}
	return nil
}

// NewEnvironmentCmd implements `new environment`.
type NewEnvironmentCmd struct {
	ProjectFlags `kong:"embed"`
	PromptFlags  `kong:"embed"`
	Name         string `arg:"" optional:"" help:"Environment name."`
}

// Run implements `new environment`.
func (c *NewEnvironmentCmd) Run(globals *Globals) error {
	if c.Name == "" {
		return fmt.Errorf("environment name is required")
	}

	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	// Try to load existing config.
	result, loadErr := config.LoadCascade(config.LoadOptions{WorkDir: wd})
	if loadErr == nil && result.Config.Name != "" {
		return fmt.Errorf("environment already defined in config as %q", result.Config.Name)
	}

	configPath := ""
	if result != nil && result.PrimaryFile != "" {
		configPath = result.PrimaryFile
	} else {
		configPath = filepath.Join(wd, "fat-controller.toml")
	}

	// The environment name is the top-level `name` field.
	snippet := struct {
		Name string `toml:"name"`
	}{Name: c.Name}

	f, err := os.OpenFile(configPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("opening %s: %w", configPath, err)
	}
	defer func() { _ = f.Close() }()

	if err := toml.NewEncoder(f).Encode(snippet); err != nil {
		return fmt.Errorf("writing to %s: %w", configPath, err)
	}

	if globals.Quiet == 0 {
		_, _ = fmt.Fprintf(os.Stdout, "Added environment %q to %s\n", c.Name, configPath)
	}
	return nil
}

// NewServiceCmd implements `new service`.
type NewServiceCmd struct {
	EnvironmentFlags `kong:"embed"`
	PromptFlags      `kong:"embed"`
	Name             string `arg:"" optional:"" help:"Service name."`
	Database         string `help:"Database type to pre-fill (e.g. postgres, redis, mysql)." name:"database"`
	Repo             string `help:"GitHub repo (org/name) for source." name:"repo"`
	Image            string `help:"Docker image for source." name:"image"`
}

// Run implements `new service`.
func (c *NewServiceCmd) Run(globals *Globals) error {
	if c.Name == "" {
		return fmt.Errorf("service name is required")
	}

	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	// Try to load existing config to check for duplicates.
	result, err := config.LoadCascade(config.LoadOptions{WorkDir: wd})
	if err == nil {
		for _, svc := range result.Config.Services {
			if svc.Name == c.Name {
				return fmt.Errorf("service %q already exists in config", c.Name)
			}
		}
	}

	// Build the service entry.
	type deploySnippet struct {
		Image *string `toml:"image,omitempty"`
		Repo  *string `toml:"repo,omitempty"`
	}
	type serviceEntry struct {
		Name   string         `toml:"name"`
		Deploy *deploySnippet `toml:"deploy,omitempty"`
	}
	entry := serviceEntry{Name: c.Name}

	if c.Database != "" {
		img := databaseImage(c.Database)
		entry.Deploy = &deploySnippet{Image: &img}
	} else if c.Image != "" {
		entry.Deploy = &deploySnippet{Image: &c.Image}
	} else if c.Repo != "" {
		entry.Deploy = &deploySnippet{Repo: &c.Repo}
	}

	snippet := struct {
		Service []serviceEntry `toml:"service"`
	}{Service: []serviceEntry{entry}}

	// Find or create the config file.
	configPath := ""
	if result != nil && result.PrimaryFile != "" {
		configPath = result.PrimaryFile
	} else {
		configPath = filepath.Join(wd, "fat-controller.toml")
	}

	// Append to the file.
	f, err := os.OpenFile(configPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("opening %s: %w", configPath, err)
	}
	defer func() { _ = f.Close() }()

	if err := toml.NewEncoder(f).Encode(snippet); err != nil {
		return fmt.Errorf("writing to %s: %w", configPath, err)
	}

	if globals.Quiet == 0 {
		_, _ = fmt.Fprintf(os.Stdout, "Added service %q to %s\n", c.Name, configPath)
	}
	return nil
}

// databaseImage returns a default container image for common database types.
func databaseImage(dbType string) string {
	switch strings.ToLower(dbType) {
	case "postgres", "postgresql":
		return "ghcr.io/railwayapp/postgres:latest"
	case "mysql":
		return "ghcr.io/railwayapp/mysql:latest"
	case "redis":
		return "ghcr.io/railwayapp/redis:latest"
	case "mongo", "mongodb":
		return "ghcr.io/railwayapp/mongo:latest"
	default:
		return dbType // pass through as image
	}
}
