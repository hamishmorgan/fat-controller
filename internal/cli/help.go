package cli

import (
	"bytes"
	"fmt"
	"go/doc"
	"io"
	"os"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"
)

// Styles for help output. These are package-level so they can be
// reconfigured (e.g. for testing or theming).
var (
	styleHeading     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("3")) // yellow
	styleBold        = lipgloss.NewStyle().Bold(true)
	styleCmdName     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6")) // cyan
	styleFlag        = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))            // green
	stylePlaceholder = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))            // bright black / gray
	styleDim         = lipgloss.NewStyle().Faint(true)
)

const (
	helpIndent        = 2
	helpColumnPadding = 4
)

// ColorHelpPrinter is a kong.HelpPrinter that outputs colored help text.
func ColorHelpPrinter(options kong.HelpOptions, ctx *kong.Context) error {
	if ctx.Empty() {
		options.Summary = false
	}
	w := &helpBuf{width: guessWidth(ctx.Stdout)}
	if options.WrapUpperBound > 0 && w.width > options.WrapUpperBound {
		w.width = options.WrapUpperBound
	}
	if options.ValueFormatter == nil {
		options.ValueFormatter = kong.DefaultHelpValueFormatter
	}

	selected := ctx.Selected()
	if selected == nil {
		printColorApp(w, ctx.Model, options)
	} else {
		printColorCommand(w, ctx.Model, selected, options)
	}
	return w.Flush(ctx.Stdout)
}

func printColorApp(w *helpBuf, app *kong.Application, opts kong.HelpOptions) {
	if !opts.NoAppSummary {
		w.Linef("%s %s%s", styleHeading.Render("Usage:"), styleBold.Render(app.Name), app.Summary())
	}
	printColorNodeDetail(w, app.Node, opts)
	cmds := app.Leaves(true)
	if len(cmds) > 0 && app.HelpFlag != nil {
		w.Line("")
		if opts.Summary {
			w.Linef(`Run "%s --help" for more information.`, styleBold.Render(app.Name))
		} else {
			w.Linef(`Run "%s" for more information on a command.`, styleBold.Render(app.Name+" <command> --help"))
		}
	}
}

func printColorCommand(w *helpBuf, app *kong.Application, cmd *kong.Command, opts kong.HelpOptions) {
	if !opts.NoAppSummary {
		w.Linef("%s %s %s", styleHeading.Render("Usage:"), styleBold.Render(app.Name), cmd.Summary())
	}
	printColorNodeDetail(w, cmd, opts)
	if opts.Summary && app.HelpFlag != nil {
		w.Line("")
		w.Linef(`Run "%s --help" for more information.`, styleBold.Render(cmd.FullPath()))
	}
}

func printColorNodeDetail(w *helpBuf, node *kong.Node, opts kong.HelpOptions) {
	if node.Help != "" {
		w.Line("")
		w.Wrap(node.Help, "")
	}
	if opts.Summary {
		return
	}
	if node.Detail != "" {
		w.Line("")
		w.Wrap(node.Detail, "")
	}
	if len(node.Positional) > 0 {
		w.Line("")
		w.Line(styleHeading.Render("Arguments:"))
		writeColorPositionals(w, node.Positional, opts)
	}

	printFlags := func() {
		if flags := node.AllFlags(true); len(flags) > 0 {
			groups := collectColorFlagGroups(flags)
			for _, group := range groups {
				w.Line("")
				if group.title != "" {
					w.Line(styleHeading.Render(group.title))
				}
				writeColorFlags(w, group.flags, opts)
			}
		}
	}

	if !opts.FlagsLast {
		printFlags()
	}

	var cmds []*kong.Node
	if opts.NoExpandSubcommands {
		cmds = node.Children
	} else {
		cmds = node.Leaves(true)
	}
	if len(cmds) > 0 {
		groups := collectColorCommandGroups(cmds)
		for _, group := range groups {
			w.Line("")
			if group.title != "" {
				w.Line(styleHeading.Render(group.title))
			}
			writeColorCommandList(w, group.commands)
		}
	}

	if opts.FlagsLast {
		printFlags()
	}
}

// writeColorCommandList writes commands in the expanded format (one per block).
func writeColorCommandList(w *helpBuf, cmds []*kong.Node) {
	for i, cmd := range cmds {
		if cmd.Hidden {
			continue
		}
		w.Linef("  %s", styleCmdName.Render(cmd.Summary()))
		if cmd.Help != "" {
			w.Linef("    %s", styleDim.Render(cmd.Help))
		}
		if i != len(cmds)-1 {
			w.Line("")
		}
	}
}

// writeColorPositionals writes positional arguments in two-column format.
func writeColorPositionals(w *helpBuf, args []*kong.Positional, opts kong.HelpOptions) {
	rows := make([][2]string, 0, len(args))
	colorRows := make([][2]string, 0, len(args))
	for _, arg := range args {
		summary := arg.Summary()
		help := opts.ValueFormatter(arg)
		rows = append(rows, [2]string{summary, help})
		colorRows = append(colorRows, [2]string{styleCmdName.Render(summary), styleDim.Render(help)})
	}
	writeColorTwoColumns(w, rows, colorRows)
}

// writeColorFlags writes flags in two-column format with colors.
func writeColorFlags(w *helpBuf, groups [][]*kong.Flag, opts kong.HelpOptions) {
	haveShort := false
	for _, group := range groups {
		for _, flag := range group {
			if flag.Short != 0 {
				haveShort = true
				break
			}
		}
	}

	rows := make([][2]string, 0)
	colorRows := make([][2]string, 0)
	for i, group := range groups {
		if i > 0 {
			rows = append(rows, [2]string{"", ""})
			colorRows = append(colorRows, [2]string{"", ""})
		}
		for _, flag := range group {
			if flag.Hidden {
				continue
			}
			plain := formatFlagPlain(haveShort, flag)
			colored := formatFlagColored(haveShort, flag)
			help := opts.ValueFormatter(flag.Value)
			rows = append(rows, [2]string{plain, help})
			colorRows = append(colorRows, [2]string{colored, styleDim.Render(help)})
		}
	}
	writeColorTwoColumns(w, rows, colorRows)
}

// formatFlagPlain returns the flag string without any ANSI codes (for width calculation).
func formatFlagPlain(haveShort bool, flag *kong.Flag) string {
	var b strings.Builder
	name := flag.Name
	isBool := flag.IsBool()
	isCounter := flag.IsCounter()

	if flag.Short != 0 {
		fmt.Fprintf(&b, "-%s, ", string(flag.Short))
	} else if haveShort {
		b.WriteString("    ")
	}

	if isBool && flag.Tag.Negatable != "" {
		name = "[no-]" + name
	}

	fmt.Fprintf(&b, "--%s", name)

	if !isBool && !isCounter {
		fmt.Fprintf(&b, "=%s", flag.FormatPlaceHolder())
	}
	return b.String()
}

// formatFlagColored returns the flag string with ANSI color codes.
func formatFlagColored(haveShort bool, flag *kong.Flag) string {
	var b strings.Builder
	name := flag.Name
	isBool := flag.IsBool()
	isCounter := flag.IsCounter()

	if flag.Short != 0 {
		b.WriteString(styleFlag.Render("-" + string(flag.Short)))
		b.WriteString(", ")
	} else if haveShort {
		b.WriteString("    ")
	}

	if isBool && flag.Tag.Negatable != "" {
		name = "[no-]" + name
	}

	b.WriteString(styleFlag.Render("--" + name))

	if !isBool && !isCounter {
		b.WriteString("=")
		b.WriteString(stylePlaceholder.Render(flag.FormatPlaceHolder()))
	}
	return b.String()
}

// writeColorTwoColumns writes rows in a two-column layout.
// `plain` rows are used for width calculation, `colored` rows for actual output.
func writeColorTwoColumns(w *helpBuf, plain, colored [][2]string) {
	maxLeft := 375 * w.width / 1000
	if maxLeft < 30 {
		maxLeft = 30
	}

	leftSize := 0
	for _, row := range plain {
		if c := len(row[0]); c > leftSize && c < maxLeft {
			leftSize = c
		}
	}

	offsetStr := strings.Repeat(" ", leftSize+helpColumnPadding)

	for i, row := range plain {
		cr := colored[i]

		buf := bytes.NewBuffer(nil)
		doc.ToText(buf, row[1], "", strings.Repeat(" ", helpIndent), w.width-leftSize-helpColumnPadding) //nolint:staticcheck
		lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")

		// Pad using plain width, but print colored content.
		// Use ansi.StringWidth for correct display width with ANSI codes.
		displayWidth := ansi.StringWidth(cr[0])
		padding := leftSize - displayWidth
		if padding < 0 {
			padding = 0
		}
		line := cr[0] + strings.Repeat(" ", padding)

		if len(row[0]) < maxLeft {
			line += fmt.Sprintf("%*s%s", helpColumnPadding, "", styleDim.Render(strings.TrimSpace(lines[0])))
			lines = lines[1:]
		}
		w.Line("  " + line)
		for _, l := range lines {
			w.Linef("  %s%s", offsetStr, styleDim.Render(l))
		}
	}
}

// Flag and command grouping helpers.

type colorFlagGroup struct {
	title string
	flags [][]*kong.Flag
}

func collectColorFlagGroups(flags [][]*kong.Flag) []colorFlagGroup {
	groups := []*kong.Group{}
	flagsByGroup := map[string][][]*kong.Flag{}

	for _, levelFlags := range flags {
		levelFlagsByGroup := map[string][]*kong.Flag{}
		for _, flag := range levelFlags {
			key := ""
			if flag.Group != nil {
				key = flag.Group.Key
				seen := false
				for _, g := range groups {
					if key == g.Key {
						seen = true
						break
					}
				}
				if !seen {
					groups = append(groups, flag.Group)
				}
			}
			levelFlagsByGroup[key] = append(levelFlagsByGroup[key], flag)
		}
		for key, fs := range levelFlagsByGroup {
			flagsByGroup[key] = append(flagsByGroup[key], fs)
		}
	}

	out := []colorFlagGroup{}
	if ungrouped, ok := flagsByGroup[""]; ok {
		out = append(out, colorFlagGroup{title: "Flags:", flags: ungrouped})
	}
	for _, g := range groups {
		out = append(out, colorFlagGroup{title: g.Title, flags: flagsByGroup[g.Key]})
	}
	return out
}

type colorCommandGroup struct {
	title    string
	commands []*kong.Node
}

func collectColorCommandGroups(nodes []*kong.Node) []colorCommandGroup {
	groups := []*kong.Group{}
	nodesByGroup := map[string][]*kong.Node{}

	for _, node := range nodes {
		key := ""
		if g := node.ClosestGroup(); g != nil {
			key = g.Key
			if _, ok := nodesByGroup[key]; !ok {
				groups = append(groups, g)
			}
		}
		nodesByGroup[key] = append(nodesByGroup[key], node)
	}

	out := []colorCommandGroup{}
	if ungrouped, ok := nodesByGroup[""]; ok {
		out = append(out, colorCommandGroup{title: "Commands:", commands: ungrouped})
	}
	for _, g := range groups {
		out = append(out, colorCommandGroup{title: g.Title, commands: nodesByGroup[g.Key]})
	}
	return out
}

// helpBuf accumulates lines of help output.
type helpBuf struct {
	lines []string
	width int
}

func (h *helpBuf) Line(text string) {
	h.lines = append(h.lines, text)
}

func (h *helpBuf) Linef(format string, args ...any) {
	h.lines = append(h.lines, fmt.Sprintf(format, args...))
}

func (h *helpBuf) Wrap(text string, indent string) {
	buf := bytes.NewBuffer(nil)
	doc.ToText(buf, strings.TrimSpace(text), indent, indent+"    ", h.width) //nolint:staticcheck
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		h.Line(line)
	}
}

func (h *helpBuf) Flush(w io.Writer) error {
	for _, line := range h.lines {
		if _, err := io.WriteString(w, line+"\n"); err != nil {
			return err
		}
	}
	return nil
}

// guessWidth attempts to determine terminal width.
// Falls back to 80 columns.
func guessWidth(w io.Writer) int {
	if f, ok := w.(*os.File); ok {
		if width, _, err := term.GetSize(f.Fd()); err == nil && width > 0 {
			return width
		}
	}
	return 80
}
