package cli

import (
	"github.com/alecthomas/kong"
	kongcompletion "github.com/jotaen/kong-completion"
)

// CompletionCmd wraps kong-completion's Completion type. We define our own
// struct rather than embedding the library type directly so that its -c
// short flag (on Code) doesn't conflict with any global short flags.
// Since we renamed the global --config flag from -c to -f, Code can keep -c.
type CompletionCmd struct {
	Shell string `arg:"" help:"The name of the shell you are using" enum:"bash,zsh,fish," default:""`
	Code  bool   `short:"c" help:"Generate the initialization code"`
}

// Help returns extended help text for the completion command.
func (c *CompletionCmd) Help() string {
	return (&kongcompletion.Completion{}).Help()
}

// Run delegates to kong-completion's Completion.Run.
func (c *CompletionCmd) Run(ctx *kong.Context) error {
	inner := &kongcompletion.Completion{
		Shell: c.Shell,
		Code:  c.Code,
	}
	return inner.Run(ctx)
}
