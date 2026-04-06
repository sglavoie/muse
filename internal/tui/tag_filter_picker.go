package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

// TagFilterPicker is a single-select tag overlay opened by ctrl+t.
// Selecting a tag sets the cross-view activeTagFilter on the root Model.
type TagFilterPicker struct {
	tags     []db.TagRow
	filtered []int // indices into tags after filter
	cursor   int
	filter   string
}

// tagFilterPickedMsg is dispatched when the user selects a tag in the picker.
type tagFilterPickedMsg struct {
	tag db.TagRow
}

// tagFilterDismissedMsg is dispatched when the picker is closed without picking.
type tagFilterDismissedMsg struct{}

// NewTagFilterPicker creates a TagFilterPicker populated with all active tags.
func NewTagFilterPicker(tags []db.TagRow) TagFilterPicker {
	tp := TagFilterPicker{tags: tags}
	tp = tp.refilter()
	return tp
}

func (tp TagFilterPicker) refilter() TagFilterPicker {
	q := strings.ToLower(tp.filter)
	if q == "" {
		tp.filtered = make([]int, len(tp.tags))
		for i := range tp.tags {
			tp.filtered[i] = i
		}
	} else {
		tp.filtered = nil
		for i, t := range tp.tags {
			if strings.Contains(strings.ToLower(t.Name), q) {
				tp.filtered = append(tp.filtered, i)
			}
		}
	}
	if tp.cursor >= len(tp.filtered) {
		tp.cursor = max(len(tp.filtered)-1, 0)
	}
	return tp
}

// HandleKey processes a key press and returns the updated picker and a command.
func (tp TagFilterPicker) HandleKey(key string) (TagFilterPicker, tea.Cmd) {
	switch key {
	case "esc", "ctrl+t":
		return tp, func() tea.Msg { return tagFilterDismissedMsg{} }
	case "up", "k":
		if tp.cursor > 0 {
			tp.cursor--
		}
	case "down", "j":
		if tp.cursor < len(tp.filtered)-1 {
			tp.cursor++
		}
	case "enter":
		if len(tp.filtered) == 0 {
			return tp, func() tea.Msg { return tagFilterDismissedMsg{} }
		}
		tag := tp.tags[tp.filtered[tp.cursor]]
		return tp, func() tea.Msg { return tagFilterPickedMsg{tag: tag} }
	case "backspace":
		if len(tp.filter) > 0 {
			runes := []rune(tp.filter)
			tp.filter = string(runes[:len(runes)-1])
			tp = tp.refilter()
		}
	default:
		if len(key) == 1 {
			tp.filter += key
			tp = tp.refilter()
		}
	}
	return tp, nil
}

// Render produces a centered overlay string for the tag filter picker.
func (tp TagFilterPicker) Render(termWidth, termHeight int, s styles.Styles) string {
	boxLines := tp.buildBoxLines(termWidth, s)

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

func (tp TagFilterPicker) buildBoxLines(termWidth int, s styles.Styles) []string {
	title := " Filter by Tag "
	const minInner = 36

	innerWidth := max(minInner, ansi.StringWidth(title)+2)
	for _, i := range tp.filtered {
		label := styles.ColoredDot(tp.tags[i].Color) + " " + tp.tags[i].Name
		innerWidth = max(innerWidth, ansi.StringWidth(label)+6)
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
	lines = append(lines, mpCornerTL+strings.Repeat(mpBorderH, innerWidth)+mpCornerTR)

	// Title row.
	titlePad := max(0, innerWidth-ansi.StringWidth(title))
	leftTP := titlePad / 2
	rightTP := titlePad - leftTP
	lines = append(lines, mpBorderV+strings.Repeat(" ", leftTP)+s.Heading.Render(title)+strings.Repeat(" ", rightTP)+mpBorderV)

	// Separator.
	lines = append(lines, mpCrossL+strings.Repeat(mpBorderH, innerWidth)+mpCrossR)

	// Filter row.
	filterStr := "/ " + tp.filter
	if tp.filter == "" {
		filterStr = s.Dim.Render("/ type to filter…")
	}
	filterPadded := text.Pad(filterStr, contentWidth)
	lines = append(lines, mpBorderV+" "+filterPadded+" "+mpBorderV)

	// Separator before items.
	lines = append(lines, mpCrossL+strings.Repeat(mpBorderH, innerWidth)+mpCrossR)

	// Tag rows.
	if len(tp.filtered) == 0 {
		msg := text.Pad("(no matching tags)", contentWidth)
		lines = append(lines, mpBorderV+" "+msg+" "+mpBorderV)
	} else {
		for rowIdx, tagIdx := range tp.filtered {
			tag := tp.tags[tagIdx]
			label := styles.ColoredDot(tag.Color) + " " + tag.Name
			padded := text.Pad(label, contentWidth)
			rowContent := " " + padded + " "
			if rowIdx == tp.cursor {
				lines = append(lines, mpBorderV+s.Accent.Render(rowContent)+mpBorderV)
			} else {
				lines = append(lines, mpBorderV+rowContent+mpBorderV)
			}
		}
	}

	// Bottom border.
	lines = append(lines, mpCornerBL+strings.Repeat(mpBorderH, innerWidth)+mpCornerBR)

	// Hint line.
	lines = append(lines, "  "+s.Dim.Render("j/k navigate · enter filter · esc cancel"))

	return lines
}
