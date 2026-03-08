package prompt

import (
	"errors"
	"strings"

	"github.com/charmbracelet/huh"
)

// Input prompts the user for a single line of text. When the user aborts the
// prompt, returns a non-nil error.
func Input(title string, defaultValue string) (string, error) {
	value := defaultValue
	err := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title(title).
				Value(&value),
		),
	).Run()
	if err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return "", errors.New("input cancelled")
		}
		return "", err
	}
	return strings.TrimSpace(value), nil
}
