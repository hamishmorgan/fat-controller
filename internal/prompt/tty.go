package prompt

import (
	"errors"
	"os"

	"github.com/charmbracelet/huh"
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

// Confirm asks the user a yes/no question using an interactive huh confirm
// widget. The defaultYes parameter sets the initial selection.
func Confirm(msg string, defaultYes bool) (bool, error) {
	confirmed := defaultYes
	err := huh.NewConfirm().
		Title(msg).
		Affirmative("Yes").
		Negative("No").
		Value(&confirmed).
		Inline(true).
		Run()
	if err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return false, nil
		}
		return false, err
	}
	return confirmed, nil
}
