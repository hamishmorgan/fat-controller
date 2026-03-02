package prompt

import (
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/huh"
)

// Item represents a selectable item with a display name and an ID.
type Item struct {
	Name string
	ID   string
}

// pickItem selects an item from the list:
//   - 0 items: error
//   - 1 item: auto-select
//   - multiple + interactive: huh Select picker
//   - multiple + non-interactive: error with listing
func pickItem(label string, items []Item, interactive bool) (string, error) {
	if len(items) == 0 {
		return "", fmt.Errorf("no %ss found", label)
	}
	if len(items) == 1 {
		return items[0].ID, nil
	}
	if !interactive {
		return "", ambiguousError(label, items)
	}
	return runPicker(label, items)
}

func ambiguousError(label string, items []Item) error {
	var b strings.Builder
	fmt.Fprintf(&b, "multiple %ss available — specify with --%s flag:", label, label)
	for _, item := range items {
		fmt.Fprintf(&b, "\n  %s (%s)", item.Name, item.ID)
	}
	return errors.New(b.String())
}

func runPicker(label string, items []Item) (string, error) {
	var selected string
	opts := make([]huh.Option[string], len(items))
	for i, item := range items {
		opts[i] = huh.NewOption(item.Name, item.ID)
	}
	err := huh.NewSelect[string]().
		Title(fmt.Sprintf("Select a %s:", label)).
		Options(opts...).
		Value(&selected).
		Run()
	if err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return "", errors.New("selection cancelled")
		}
		return "", err
	}
	return selected, nil
}

// PickProject selects a project from the list.
func PickProject(items []Item, interactive bool) (string, error) {
	return pickItem("project", items, interactive)
}

// PickEnvironment selects an environment from the list.
func PickEnvironment(items []Item, interactive bool) (string, error) {
	return pickItem("environment", items, interactive)
}

// PickWorkspace selects a workspace from the list.
func PickWorkspace(items []Item, interactive bool) (string, error) {
	return pickItem("workspace", items, interactive)
}
