package styles

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// EmptyAction is one row in an empty-state card: a key hint and the action
// label that follows it.
type EmptyAction struct {
	Key   string
	Label string
}

// RenderEmptyCard returns a horizontally-centered (and, when height > 0,
// vertically-centered) rounded card containing a bold title, a blank, and
// one row per action formatted as `key  label`. width and height describe
// the viewport the card should sit inside; when either is non-positive the
// corresponding axis is rendered without centering.
func (s Styles) RenderEmptyCard(width, height int, title string, actions []EmptyAction) string {
	inner := padLinesToWidest(s.emptyCardRows(title, actions))
	cardLines := strings.Split(s.EmptyCard.Render(strings.Join(inner, "\n")), "\n")

	if width > 0 {
		cardLines = clampLinesToWidth(cardLines, width)
		cardLines = centerLinesInWidth(cardLines, width)
	}

	return padTopToCenter(strings.Join(cardLines, "\n"), len(cardLines), height)
}

// emptyCardRows builds the card's contents: a bold title, a blank, and one
// `key  label` row per action with the key column padded to a common width so
// the labels line up.
func (s Styles) emptyCardRows(title string, actions []EmptyAction) []string {
	keyW := 0
	for _, a := range actions {
		keyW = max(keyW, lipgloss.Width(a.Key))
	}

	rows := make([]string, 0, len(actions)+2)
	rows = append(rows, s.Bold.Render(title), "")
	for _, a := range actions {
		keyStr := s.EmptyKey.Render(padRightToWidth(a.Key, keyW))
		labelStr := s.EmptyHint.Render(a.Label)
		rows = append(rows, keyStr+"  "+labelStr)
	}
	return rows
}

// padLinesToWidest right-pads every line to the widest one, so the card border
// wraps a rectangle rather than a ragged block.
func padLinesToWidest(lines []string) []string {
	widest := maxLineWidth(lines)
	for i, line := range lines {
		if w := lipgloss.Width(line); w < widest {
			lines[i] = line + strings.Repeat(" ", widest-w)
		}
	}
	return lines
}

// clampLinesToWidth truncates every line to width. The join/split round trip is
// deliberate: it keeps one slice entry per rendered row even if a clamped line
// were to gain a break, which is what the vertical centering below counts.
func clampLinesToWidth(lines []string, width int) []string {
	for i, line := range lines {
		lines[i] = clampLineWidth(line, width)
	}
	return strings.Split(strings.Join(lines, "\n"), "\n")
}

// centerLinesInWidth left-pads every line by the same amount so the block sits
// centered in a viewport of width columns.
func centerLinesInWidth(lines []string, width int) []string {
	leftPad := max(0, (width-maxLineWidth(lines))/2)
	if leftPad == 0 {
		return lines
	}
	pad := strings.Repeat(" ", leftPad)
	for i, line := range lines {
		lines[i] = pad + line
	}
	return lines
}

// padTopToCenter prefixes the blank rows that center a block of lineCount rows
// in a viewport of height rows. A non-positive height leaves out uncentered.
func padTopToCenter(out string, lineCount, height int) string {
	if height <= 0 {
		return out
	}
	topPad := max(0, (height-lineCount)/2)
	if topPad == 0 {
		return out
	}
	return strings.Repeat("\n", topPad) + out
}

// maxLineWidth reports the display width of the widest line.
func maxLineWidth(lines []string) int {
	widest := 0
	for _, line := range lines {
		widest = max(widest, lipgloss.Width(line))
	}
	return widest
}

func padRightToWidth(s string, w int) string {
	sw := lipgloss.Width(s)
	if sw >= w {
		return s
	}
	return s + strings.Repeat(" ", w-sw)
}

func clampLineWidth(s string, width int) string {
	if width <= 0 || lipgloss.Width(s) <= width {
		return s
	}
	return lipgloss.NewStyle().MaxWidth(width).Render(s)
}
