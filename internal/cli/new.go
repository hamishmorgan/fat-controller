package cli

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
	_ = globals
	return nil // stub
}
