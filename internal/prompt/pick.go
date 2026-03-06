package prompt

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/huh"
)

// Item represents a selectable item with a display name and an ID.
type Item struct {
	Name string
	ID   string
}

// PickOpts controls picker behaviour.
type PickOpts struct {
	// ForcePrompt shows the picker even when there is only one item,
	// so the user explicitly confirms the selection.
	ForcePrompt bool
}

// PickItem selects an item from the list:
//   - 0 items: error
//   - 1 item + !forcePrompt: auto-select
//   - 1 item + forcePrompt + interactive: show picker
//   - multiple + interactive: huh Select picker
//   - multiple + non-interactive: error with listing
func PickItem(label string, items []Item, interactive bool, opts PickOpts) (string, error) {
	if len(items) == 0 {
		return "", fmt.Errorf("no %ss found", label)
	}
	if len(items) == 1 && !opts.ForcePrompt {
		return items[0].ID, nil
	}
	if len(items) == 1 && opts.ForcePrompt && !interactive {
		// Non-interactive with forcePrompt: auto-select (can't show picker).
		return items[0].ID, nil
	}
	if !interactive {
		return "", ambiguousError(label, items)
	}
	return runPicker(label, items)
}

func ambiguousError(label string, items []Item) error {
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "multiple %ss available — specify with --%s flag:", label, label)
	for _, item := range items {
		_, _ = fmt.Fprintf(&b, "\n  %s (%s)", item.Name, item.ID)
	}
	return errors.New(b.String())
}

// SelectField builds a huh Select field for the given items, writing the
// selected ID into *value. Callers can combine this with other fields
// (e.g. Notes) in a huh.NewGroup before running.
func SelectField(label string, items []Item, value *string) *huh.Select[string] {
	opts := make([]huh.Option[string], len(items))
	for i, item := range items {
		opts[i] = huh.NewOption(item.Name, item.ID)
	}
	return huh.NewSelect[string]().
		Title(fmt.Sprintf("Select a %s:", label)).
		Options(opts...).
		Value(value)
}

// MultiSelectField builds a huh MultiSelect field for the given names,
// with all options pre-selected. Callers can combine this with other
// fields in a huh.NewGroup before running.
func MultiSelectField(title string, names []string, value *[]string) *huh.MultiSelect[string] {
	sort.Strings(names)
	opts := make([]huh.Option[string], len(names))
	for i, name := range names {
		opts[i] = huh.NewOption(name, name).Selected(true)
	}
	return huh.NewMultiSelect[string]().
		Title(title).
		Options(opts...).
		Value(value)
}

// RunFields wraps the given fields in a Form/Group and runs them.
func RunFields(fields ...huh.Field) error {
	err := huh.NewForm(
		huh.NewGroup(fields...),
	).Run()
	if err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return errors.New("selection cancelled")
		}
	}
	return err
}

func runPicker(label string, items []Item) (string, error) {
	var selected string
	if err := RunFields(SelectField(label, items, &selected)); err != nil {
		return "", err
	}
	return selected, nil
}

// PickProject selects a project from the list.
func PickProject(items []Item, interactive bool, opts PickOpts) (string, error) {
	return PickItem("project", items, interactive, opts)
}

// PickEnvironment selects an environment from the list.
func PickEnvironment(items []Item, interactive bool, opts PickOpts) (string, error) {
	return PickItem("environment", items, interactive, opts)
}

// PickWorkspace selects a workspace from the list.
func PickWorkspace(items []Item, interactive bool, opts PickOpts) (string, error) {
	return PickItem("workspace", items, interactive, opts)
}

// PickServices shows a multi-select picker for services. All services are
// selected by default. Returns the names of services the user kept selected.
// In non-interactive mode, all services are returned.
func PickServices(names []string, interactive bool) ([]string, error) {
	if len(names) == 0 {
		return nil, nil
	}
	if !interactive {
		sorted := make([]string, len(names))
		copy(sorted, names)
		sort.Strings(sorted)
		return sorted, nil
	}

	var selected []string
	if err := RunFields(MultiSelectField("Select services to include:", names, &selected)); err != nil {
		return nil, err
	}
	sort.Strings(selected)
	return selected, nil
}
