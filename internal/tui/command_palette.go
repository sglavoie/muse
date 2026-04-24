package tui

import (
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/filter"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

// PaletteCommand describes a single entry in the command palette.
type PaletteCommand struct {
	Name    string
	Exec    func() tea.Cmd
	Enabled func() bool // nil means always enabled
}

func (c PaletteCommand) isEnabled() bool {
	return c.Enabled == nil || c.Enabled()
}

// CommandPalettePickedMsg is dispatched when the user confirms a command.
type CommandPalettePickedMsg struct {
	Command PaletteCommand
}

// CommandPaletteDismissedMsg is dispatched when the user dismisses the palette.
type CommandPaletteDismissedMsg struct{}

// CommandPalette is the overlay sub-model for the searchable command palette.
type CommandPalette struct {
	all      []PaletteCommand // full sorted list
	filtered []PaletteCommand // subset matching current filter
	cursor   int
	filter   filter.State // always-active text input
}

// NewCommandPalette creates a CommandPalette with the given commands sorted
// alphabetically. The filter text input is activated immediately.
func NewCommandPalette(commands []PaletteCommand) (CommandPalette, tea.Cmd) {
	slices.SortFunc(commands, func(a, b PaletteCommand) int {
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	cp := CommandPalette{
		all:      commands,
		filtered: commands,
	}
	var cmd tea.Cmd
	cp.filter, cmd = filter.New().Activate()
	return cp, cmd
}

// HandleKey processes a key press and returns the updated palette and optional Cmd.
func (cp CommandPalette) HandleKey(key string, msg tea.KeyPressMsg) (CommandPalette, tea.Cmd) {
	switch key {
	case "esc":
		return cp, func() tea.Msg { return CommandPaletteDismissedMsg{} }
	case "enter":
		if cp.cursor >= 0 && cp.cursor < len(cp.filtered) {
			picked := cp.filtered[cp.cursor]
			if !picked.isEnabled() {
				return cp, nil
			}
			return cp, func() tea.Msg { return CommandPalettePickedMsg{Command: picked} }
		}
	case "up", "ctrl+k", "ctrl+p":
		if cp.cursor > 0 {
			cp.cursor--
		}
	case "down", "ctrl+j", "ctrl+n":
		if cp.cursor < len(cp.filtered)-1 {
			cp.cursor++
		}
	default:
		var cmd tea.Cmd
		cp.filter, cmd = cp.filter.Update(msg)
		cp.refilter()
		return cp, cmd
	}
	return cp, nil
}

// refilter rebuilds the filtered list from all commands using case-insensitive
// substring matching, then clamps the cursor.
func (cp *CommandPalette) refilter() {
	query := strings.ToLower(cp.filter.Text)
	if query == "" {
		cp.filtered = cp.all
	} else {
		cp.filtered = nil
		for _, c := range cp.all {
			if strings.Contains(strings.ToLower(c.Name), query) {
				cp.filtered = append(cp.filtered, c)
			}
		}
	}
	if cp.cursor >= len(cp.filtered) {
		cp.cursor = max(len(cp.filtered)-1, 0)
	}
}

// Border characters.
const (
	cpBorderH  = "─"
	cpBorderV  = "│"
	cpCornerTL = "╭"
	cpCornerTR = "╮"
	cpCornerBL = "╰"
	cpCornerBR = "╯"
	cpCrossL   = "├"
	cpCrossR   = "┤"
)

// Render produces a centered overlay string for the palette.
func (cp CommandPalette) Render(termWidth, termHeight int, s styles.Styles) string {
	boxLines := cp.buildBoxLines(termWidth, s)

	boxW := ansi.StringWidth(boxLines[0])
	boxH := len(boxLines)

	leftPad := 0
	if termWidth > boxW {
		leftPad = (termWidth - boxW) / 2
	}
	topPad := 0
	if termHeight > boxH {
		topPad = (termHeight - boxH) / 2
	}

	prefix := strings.Repeat(" ", leftPad)
	var b strings.Builder
	for range topPad {
		b.WriteByte('\n')
	}
	for _, line := range boxLines {
		b.WriteString(prefix + line + "\n")
	}
	return b.String()
}

func (cp CommandPalette) buildBoxLines(termWidth int, s styles.Styles) []string {
	const modalTitle = " Command Palette "
	const minInner = 30

	// Compute inner width from title, filter, and command names.
	innerWidth := minInner
	if ansi.StringWidth(modalTitle)+2 > innerWidth {
		innerWidth = ansi.StringWidth(modalTitle) + 2
	}
	for _, c := range cp.filtered {
		if ansi.StringWidth(c.Name)+4 > innerWidth {
			innerWidth = ansi.StringWidth(c.Name) + 4
		}
	}
	if maxInner := termWidth - 4; termWidth > 0 && innerWidth > maxInner {
		innerWidth = maxInner
	}
	if innerWidth < 1 {
		innerWidth = 1
	}
	contentWidth := max(innerWidth-2, 0)

	var lines []string

	// Top border.
	lines = append(lines, cpCornerTL+strings.Repeat(cpBorderH, innerWidth)+cpCornerTR)

	// Title row (centered).
	titlePad := max(0, innerWidth-ansi.StringWidth(modalTitle))
	leftTP := titlePad / 2
	rightTP := titlePad - leftTP
	lines = append(lines, cpBorderV+strings.Repeat(" ", leftTP)+s.Heading.Render(modalTitle)+strings.Repeat(" ", rightTP)+cpBorderV)

	// Separator.
	lines = append(lines, cpCrossL+strings.Repeat(cpBorderH, innerWidth)+cpCrossR)

	// Filter input row.
	filterText := cp.filter.View()
	if !cp.filter.Active {
		filterText = cp.filter.Text
	}
	filterDisplay := " / " + filterText
	filterPadded := text.Pad(filterDisplay, contentWidth)
	lines = append(lines, cpBorderV+" "+filterPadded+" "+cpBorderV)

	// Separator before commands.
	lines = append(lines, cpCrossL+strings.Repeat(cpBorderH, innerWidth)+cpCrossR)

	// Command rows.
	if len(cp.filtered) == 0 {
		noMatch := text.Pad("(no matches)", contentWidth)
		lines = append(lines, cpBorderV+" "+noMatch+" "+cpBorderV)
	} else {
		dimItalic := s.Dim.Italic(true)
		accentDim := s.Accent.Faint(true).Italic(true)
		for i, cmd := range cp.filtered {
			display := text.Pad(cmd.Name, contentWidth)
			rowContent := " " + display + " "
			switch {
			case i == cp.cursor && !cmd.isEnabled():
				lines = append(lines, cpBorderV+accentDim.Render(rowContent)+cpBorderV)
			case i == cp.cursor:
				lines = append(lines, cpBorderV+s.Accent.Render(rowContent)+cpBorderV)
			case !cmd.isEnabled():
				lines = append(lines, cpBorderV+dimItalic.Render(rowContent)+cpBorderV)
			default:
				lines = append(lines, cpBorderV+rowContent+cpBorderV)
			}
		}
	}

	// Bottom border.
	lines = append(lines, cpCornerBL+strings.Repeat(cpBorderH, innerWidth)+cpCornerBR)

	// Hint line.
	lines = append(lines, "  "+s.Dim.Render("↑/↓ navigate · enter run · esc dismiss"))

	return lines
}
