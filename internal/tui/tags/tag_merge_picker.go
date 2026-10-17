package tags

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

// tagMergeStep is the two-step state machine for the merge flow.
type tagMergeStep int

const (
	tagMergeStepPicking    tagMergeStep = iota // user picks the destination tag
	tagMergeStepConfirming                     // user confirms the irreversible merge
)

// TagMergePicker is the overlay for the tag merge flow. The user first picks a
// destination tag from a filtered list, then confirms the irreversible merge.
type TagMergePicker struct {
	src      db.TagRow   // tag to merge from
	dests    []db.TagRow // destination candidates (all active tags except src)
	filtered []int       // indices into dests after filter
	cursor   int
	filter   string // typed filter text
	step     tagMergeStep
	dest     db.TagRow // the chosen destination (set when step == confirming)
}

// TagMergePickedMsg is dispatched when the user confirms a merge.
type TagMergePickedMsg struct {
	Src  db.TagRow
	Dest db.TagRow
}

// TagMergeDismissedMsg is dispatched when the picker is closed without merging.
type TagMergeDismissedMsg struct{}

// NewTagMergePicker builds the picker with src excluded from the destination list.
func NewTagMergePicker(src db.TagRow, all []db.TagRow) TagMergePicker {
	dests := make([]db.TagRow, 0, len(all))
	for _, t := range all {
		if t.ID != src.ID {
			dests = append(dests, t)
		}
	}
	tp := TagMergePicker{src: src, dests: dests}
	tp = tp.refilter()
	return tp
}

func (tp TagMergePicker) refilter() TagMergePicker {
	q := strings.ToLower(tp.filter)
	if q == "" {
		tp.filtered = make([]int, len(tp.dests))
		for i := range tp.dests {
			tp.filtered[i] = i
		}
	} else {
		tp.filtered = nil
		for i, d := range tp.dests {
			if strings.Contains(strings.ToLower(d.Name), q) {
				tp.filtered = append(tp.filtered, i)
			}
		}
	}
	if tp.cursor >= len(tp.filtered) {
		tp.cursor = max(len(tp.filtered)-1, 0)
	}
	return tp
}

// HandleKey processes a key press for the currently active step.
func (tp TagMergePicker) HandleKey(key string) (TagMergePicker, tea.Cmd) {
	switch tp.step {
	case tagMergeStepPicking:
		return tp.handlePickKey(key)
	case tagMergeStepConfirming:
		return tp.handleConfirmKey(key)
	}
	return tp, nil
}

func (tp TagMergePicker) handlePickKey(key string) (TagMergePicker, tea.Cmd) {
	switch key {
	case "esc":
		return tp, func() tea.Msg { return TagMergeDismissedMsg{} }
	case "enter":
		return tp.advanceToConfirm(), nil
	default:
		return tp.editSelection(key), nil
	}
}

// advanceToConfirm locks in the focused destination tag and moves to the
// confirmation step, or stays put when the filtered list is empty.
func (tp TagMergePicker) advanceToConfirm() TagMergePicker {
	if len(tp.filtered) == 0 {
		return tp
	}
	tp.dest = tp.dests[tp.filtered[tp.cursor]]
	tp.step = tagMergeStepConfirming
	return tp
}

// editSelection applies the cursor-movement and filter-editing keys of the
// picking step.
func (tp TagMergePicker) editSelection(key string) TagMergePicker {
	switch key {
	case "up", "k":
		if tp.cursor > 0 {
			tp.cursor--
		}
	case "down", "j":
		if tp.cursor < len(tp.filtered)-1 {
			tp.cursor++
		}
	case "backspace":
		if len(tp.filter) > 0 {
			runes := []rune(tp.filter)
			tp.filter = string(runes[:len(runes)-1])
			tp = tp.refilter()
		}
	default:
		// Typing characters filters the list.
		runes := []rune(key)
		if len(runes) == 1 {
			tp.filter += key
			tp = tp.refilter()
		}
	}
	return tp
}

func (tp TagMergePicker) handleConfirmKey(key string) (TagMergePicker, tea.Cmd) {
	switch key {
	case "y", "Y", "enter":
		src, dest := tp.src, tp.dest
		return tp, func() tea.Msg { return TagMergePickedMsg{Src: src, Dest: dest} }
	case "n", "N", "esc":
		return tp, func() tea.Msg { return TagMergeDismissedMsg{} }
	}
	return tp, nil
}

// --- Rendering ---

const (
	mpBorderH  = "─"
	mpBorderV  = "│"
	mpCornerTL = "╭"
	mpCornerTR = "╮"
	mpCornerBL = "╰"
	mpCornerBR = "╯"
	mpCrossL   = "├"
	mpCrossR   = "┤"
)

// Render produces a centered overlay string for the merge picker.
func (tp TagMergePicker) Render(termWidth, termHeight int, s styles.Styles) string {
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

func (tp TagMergePicker) buildBoxLines(termWidth int, s styles.Styles) []string {
	if tp.step == tagMergeStepConfirming {
		return tp.buildConfirmLines(termWidth, s)
	}
	return tp.buildPickLines(termWidth, s)
}

func (tp TagMergePicker) buildPickLines(termWidth int, s styles.Styles) []string {
	title := " Merge Tag "
	const minInner = 36

	innerWidth := max(minInner, ansi.StringWidth(title)+2)
	for _, i := range tp.filtered {
		label := styles.ColoredDot(tp.dests[i].Color) + " " + tp.dests[i].Name
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

	// Source row.
	srcDot := styles.ColoredDot(tp.src.Color)
	srcLine := text.Pad("Merge "+srcDot+" "+tp.src.Name+" into:", contentWidth)
	lines = append(lines, mpBorderV+" "+srcLine+" "+mpBorderV)

	// Filter row.
	filterStr := "/ " + tp.filter
	if tp.filter == "" {
		filterStr = s.Dim.Render("/ type to filter…")
	}
	filterPadded := text.Pad(filterStr, contentWidth)
	lines = append(lines, mpBorderV+" "+filterPadded+" "+mpBorderV)

	// Separator before items.
	lines = append(lines, mpCrossL+strings.Repeat(mpBorderH, innerWidth)+mpCrossR)

	// Destination rows.
	if len(tp.filtered) == 0 {
		msg := text.Pad("(no matching tags)", contentWidth)
		lines = append(lines, mpBorderV+" "+msg+" "+mpBorderV)
	} else {
		for rowIdx, destIdx := range tp.filtered {
			dest := tp.dests[destIdx]
			label := styles.ColoredDot(dest.Color) + " " + dest.Name
			padded := text.FocusMarker(rowIdx == tp.cursor) + text.Pad(label, contentWidth-2)
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
	lines = append(lines, "  "+s.Dim.Render("j/k navigate · enter select · esc cancel"))

	return lines
}

func (tp TagMergePicker) buildConfirmLines(termWidth int, s styles.Styles) []string {
	title := " Confirm Merge "
	confirmMsg := `Merge "` + tp.src.Name + `" into "` + tp.dest.Name + `"?`
	warnMsg := "This cannot be undone."
	const minInner = 40

	innerWidth := max(minInner, ansi.StringWidth(title)+2,
		ansi.StringWidth(confirmMsg)+4, ansi.StringWidth(warnMsg)+4)
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

	// Confirmation message rows.
	lines = append(lines, mpBorderV+" "+text.Pad(confirmMsg, contentWidth)+" "+mpBorderV)
	lines = append(lines, mpBorderV+" "+text.Pad(s.Warning.Render(warnMsg), contentWidth)+" "+mpBorderV)
	lines = append(lines, mpBorderV+" "+text.Pad("", contentWidth)+" "+mpBorderV)
	lines = append(lines, mpBorderV+" "+text.Pad(s.Accent.Render("  [y] Merge  ")+"   [n] Cancel  ", contentWidth)+" "+mpBorderV)

	// Bottom border.
	lines = append(lines, mpCornerBL+strings.Repeat(mpBorderH, innerWidth)+mpCornerBR)

	// Hint line.
	lines = append(lines, "  "+s.Dim.Render("y/enter confirm · n/esc cancel"))

	return lines
}
