package prompt

import (
	"fmt"

	"github.com/charmbracelet/huh"
)

// Item represents a selectable item with a display name and an ID.
type Item struct {
	Name string
	ID   string
}

// pickItem selects an item from the list:
// - 0 items: error
// - 1 item: auto-select
// - multiple + interactive: huh Select picker
// - multiple + non-interactive: error with listing
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
	msg := fmt.Sprintf("multiple %ss available — specify with --%s flag:\n", label, label)
	for _, item := range items {
		msg += fmt.Sprintf("  %s (%s)\n", item.Name, item.ID)
	}
	return fmt.Errorf("%s", msg)
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
