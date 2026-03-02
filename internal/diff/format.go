package diff

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/hamishmorgan/fat-controller/internal/config"
)

var (
	headerStyle = lipgloss.NewStyle().Bold(true).Underline(true)
	createStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("2")) // green
	updateStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("3")) // yellow
	deleteStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("1")) // red
	dimStyle    = lipgloss.NewStyle().Faint(true)
)

// Format renders a diff Result as human-readable text.
// When showSecrets is false, live values of sensitive variables are masked.
func Format(result *Result, showSecrets bool) string {
	if result.IsEmpty() {
		return "No changes."
	}

	var masker *config.Masker
	if !showSecrets {
		masker = config.NewMasker(nil, nil)
	}

	var out strings.Builder

	// Shared section first.
	if result.Shared != nil && (len(result.Shared.Variables) > 0 || len(result.Shared.Settings) > 0) {
		out.WriteString(headerStyle.Render("shared") + "\n")
		writeChanges(&out, result.Shared, masker, true)
		out.WriteString("\n")
	}

	// Services in sorted order.
	svcNames := make([]string, 0, len(result.Services))
	for name := range result.Services {
		svcNames = append(svcNames, name)
	}
	sort.Strings(svcNames)

	for _, name := range svcNames {
		sd := result.Services[name]
		out.WriteString(headerStyle.Render(name) + "\n")
		writeChanges(&out, sd, masker, true)
		out.WriteString("\n")
	}

	// Summary.
	out.WriteString(formatSummary(result))

	return strings.TrimRight(out.String(), "\n")
}

func writeChanges(out *strings.Builder, sd *SectionDiff, masker *config.Masker, maskVariables bool) {
	// If you want deterministic output, ensure Variables/Settings are sorted
	// by key before rendering (Variables are already sorted in diffVariables).
	for _, ch := range sd.Variables {
		writeChange(out, ch, masker, maskVariables)
	}
	for _, ch := range sd.Settings {
		writeChange(out, ch, nil, false) // settings are not masked
	}
}

func writeChange(out *strings.Builder, ch Change, masker *config.Masker, isVariable bool) {
	prefix, style := actionPrefixAndStyle(ch.Action)
	liveDisplay := ch.LiveValue
	desiredDisplay := ch.DesiredValue

	// Mask live values for sensitive variables.
	if masker != nil && isVariable {
		liveDisplay = masker.MaskValue(ch.Key, liveDisplay)
	}

	switch ch.Action {
	case ActionCreate:
		line := fmt.Sprintf("  %s %s = %s", prefix, ch.Key, desiredDisplay)
		out.WriteString(style.Render(line) + "\n")
	case ActionUpdate:
		oldLine := fmt.Sprintf("  - %s = %s", ch.Key, liveDisplay)
		newLine := fmt.Sprintf("  + %s = %s", ch.Key, desiredDisplay)
		out.WriteString(dimStyle.Render(oldLine) + "\n")
		out.WriteString(style.Render(newLine) + "\n")
	case ActionDelete:
		line := fmt.Sprintf("  %s %s = %s", prefix, ch.Key, liveDisplay)
		out.WriteString(style.Render(line) + "\n")
	}
}

func actionPrefixAndStyle(a Action) (string, lipgloss.Style) {
	switch a {
	case ActionCreate:
		return "+", createStyle
	case ActionUpdate:
		return "~", updateStyle
	case ActionDelete:
		return "-", deleteStyle
	default:
		return "?", lipgloss.NewStyle()
	}
}

func formatSummary(result *Result) string {
	var creates, updates, deletes int

	countSection := func(sd *SectionDiff) {
		for _, ch := range sd.Variables {
			switch ch.Action {
			case ActionCreate:
				creates++
			case ActionUpdate:
				updates++
			case ActionDelete:
				deletes++
			}
		}
		for _, ch := range sd.Settings {
			switch ch.Action {
			case ActionCreate:
				creates++
			case ActionUpdate:
				updates++
			case ActionDelete:
				deletes++
			}
		}
	}

	if result.Shared != nil {
		countSection(result.Shared)
	}
	for _, sd := range result.Services {
		countSection(sd)
	}

	var parts []string
	if creates > 0 {
		parts = append(parts, createStyle.Render(fmt.Sprintf("%d create", creates)))
	}
	if updates > 0 {
		parts = append(parts, updateStyle.Render(fmt.Sprintf("%d update", updates)))
	}
	if deletes > 0 {
		parts = append(parts, deleteStyle.Render(fmt.Sprintf("%d delete", deletes)))
	}

	return "\n" + strings.Join(parts, ", ")
}
