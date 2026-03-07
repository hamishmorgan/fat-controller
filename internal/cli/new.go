package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
	_ = globals
	return nil // stub
}

// NewEnvironmentCmd implements `new environment`.
type NewEnvironmentCmd struct {
	ProjectFlags `kong:"embed"`
	PromptFlags  `kong:"embed"`
	Name         string `arg:"" optional:"" help:"Environment name."`
}

// Run implements `new environment`.
func (c *NewEnvironmentCmd) Run(globals *Globals) error {
	_ = globals
	return nil // stub
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

	// Build the TOML snippet to append.
	var buf strings.Builder
	buf.WriteString("\n[[service]]\n")
	fmt.Fprintf(&buf, "name = %q\n", c.Name)

	if c.Database != "" {
		fmt.Fprintf(&buf, "\n[service.deploy]\n")
		fmt.Fprintf(&buf, "image = %q\n", databaseImage(c.Database))
	} else if c.Image != "" {
		fmt.Fprintf(&buf, "\n[service.deploy]\n")
		fmt.Fprintf(&buf, "image = %q\n", c.Image)
	} else if c.Repo != "" {
		fmt.Fprintf(&buf, "\n[service.deploy]\n")
		fmt.Fprintf(&buf, "repo = %q\n", c.Repo)
	}

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

	if _, err := f.WriteString(buf.String()); err != nil {
		return fmt.Errorf("writing to %s: %w", configPath, err)
	}

	if !globals.Quiet {
		fmt.Fprintf(os.Stdout, "Added service %q to %s\n", c.Name, configPath)
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
