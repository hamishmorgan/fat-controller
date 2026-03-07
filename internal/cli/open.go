package cli

import (
	"fmt"
	"os"

	"github.com/hamishmorgan/fat-controller/internal/auth"
	"github.com/hamishmorgan/fat-controller/internal/railway"
)

// OpenCmd implements the `open` command.
type OpenCmd struct {
	EnvironmentFlags `kong:"embed"`
	Print            bool `help:"Print URL instead of opening browser." short:"p"`
}

func (c *OpenCmd) Run(globals *Globals) error {
	ctx, cancel := c.TimeoutContext(globals.BaseCtx)
	defer cancel()
	client, err := newClient(&c.ApiFlags, globals.BaseCtx)
	if err != nil {
		return err
	}

	projID, envID, err := railway.ResolveProjectEnvironment(ctx, client, c.Workspace, c.Project, c.Environment)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("https://railway.com/project/%s/environment/%s", projID, envID)

	if c.Print {
		_, _ = fmt.Fprintln(os.Stdout, url)
		return nil
	}

	if err := auth.OpenBrowser(url); err != nil {
		// Fall back to printing the URL.
		_, _ = fmt.Fprintf(os.Stderr, "Could not open browser: %v\n", err)
		_, _ = fmt.Fprintln(os.Stdout, url)
	}
	return nil
}
