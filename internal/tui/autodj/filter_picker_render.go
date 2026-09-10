package autodj

import (
	"strings"

	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

// pickerChromeLines is everything a picker box costs before its rows: the top
// border, the title, its separator, the search line, the separator under it,
// the bottom border, and the hint.
const pickerChromeLines = 7

// buildBoxLines renders the picker as a box sized to the terminal.
func (fp filterPicker) buildBoxLines(termWidth, termHeight int, s styles.Styles) []string {
	inner := innerWidth(termWidth)
	content := max(inner-2, 0)
	lines := boxHeader(fp.title(), inner, s)

	search := "/ " + fp.query
	if fp.query == "" {
		search = s.Dim.Render("/ type to search…")
	}
	lines = append(lines,
		boxBorderV+" "+text.Pad(search, content)+" "+boxBorderV,
		boxCrossL+strings.Repeat(boxBorderH, inner)+boxCrossR)

	if len(fp.filtered) == 0 {
		lines = append(lines, boxBorderV+" "+text.Pad("(no matches)", content)+" "+boxBorderV)
	}
	start, end := fp.rowWindow(termHeight)
	for i := start; i < end; i++ {
		lines = append(lines, fp.renderRow(fp.filtered[i], i == fp.cursor, content, s))
	}
	lines = append(lines, boxCornerBL+strings.Repeat(boxBorderH, inner)+boxCornerBR)
	return append(lines, "  "+s.Dim.Render(text.Truncate(
		"↑↓ move · tab toggle · enter confirm · esc cancel", inner)))
}

// rowWindow returns the half-open range of filtered rows to draw, always
// containing the cursor.
func (fp filterPicker) rowWindow(termHeight int) (int, int) {
	total := len(fp.filtered)
	if total == 0 {
		return 0, 0
	}
	size := total
	if termHeight > 0 {
		size = max(termHeight-pickerChromeLines, 1)
	}
	if size >= total {
		return 0, total
	}
	start := 0
	if fp.cursor >= size {
		start = fp.cursor - size + 1
	}
	return start, start + size
}

// renderRow draws one option with its checkbox and, for tags, its swatch.
func (fp filterPicker) renderRow(optIdx int, focused bool, content int, s styles.Styles) string {
	opt := fp.options[optIdx]
	mark := "[ ] "
	if _, on := fp.chosen[opt.key]; on {
		mark = "[x] "
	}
	label := opt.label
	if opt.color != "" {
		label = styles.ColoredDot(opt.color) + " " + label
	}
	body := text.Pad(text.Truncate(mark+label, content), content)
	if focused {
		return boxBorderV + s.Accent.Render(" "+body+" ") + boxBorderV
	}
	return boxBorderV + " " + body + " " + boxBorderV
}
