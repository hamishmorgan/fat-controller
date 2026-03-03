package prompt

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

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

// Confirm asks the user a yes/no question on stdout and reads from stdin.
// Returns true if the user enters 'y' or 'yes' (case-insensitive).
// If defaultYes is true, empty input returns true.
func Confirm(msg string, defaultYes bool) (bool, error) {
	return ConfirmRW(os.Stdin, os.Stdout, msg, defaultYes)
}

// ConfirmRW asks the user a yes/no question on w and reads a response from r.
// Returns true if the user enters 'y' or 'yes' (case-insensitive).
// If defaultYes is true, empty input returns true.
func ConfirmRW(r io.Reader, w io.Writer, msg string, defaultYes bool) (bool, error) {
	promptStr := " [y/N]: "
	if defaultYes {
		promptStr = " [Y/n]: "
	}
	if _, err := fmt.Fprint(w, msg+promptStr); err != nil {
		return false, err
	}

	reader := bufio.NewReader(r)
	response, err := reader.ReadString('\n')
	if err != nil {
		if err != io.EOF {
			return false, err
		}
		// If we got EOF but also got some bytes, still parse them.
		// If we got EOF and no bytes, treat as default.
	}

	response = strings.TrimSpace(strings.ToLower(response))
	if response == "" {
		return defaultYes, nil
	}

	return response == "y" || response == "yes", nil
}
