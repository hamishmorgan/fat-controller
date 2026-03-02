package prompt

import (
	"os"

	"github.com/charmbracelet/x/term"
)

// IsInteractive checks if the given file is a terminal.
func IsInteractive(f *os.File) bool {
	return term.IsTerminal(f.Fd())
}

// StdinIsInteractive checks if os.Stdin is a TTY.
func StdinIsInteractive() bool {
	return IsInteractive(os.Stdin)
}
