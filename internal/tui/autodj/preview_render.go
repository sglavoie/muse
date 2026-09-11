package autodj

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

// previewChromeLines is what the Preview page costs before any content: the top
// border, the tab bar, its separator, the bottom border, and the hint.
const previewChromeLines = 5

// continuationIndent hangs a wrapped explanation under its own line so a reader
// can tell a continuation from the next track.
const continuationIndent = 2

// buildPreviewLines renders the Preview page as a box sized to the terminal.
func (p TunePanel) buildPreviewLines(termWidth, termHeight int, s styles.Styles, v Preview) []string {
	inner := innerWidth(termWidth)
	content := max(inner-2, 0)
	title, titleWidth := tabBarTitle(tabPreview, s)
	lines := boxHeaderRendered(title, titleWidth, inner)

	body := wrapPreview(v.previewLines(p.tagNames()), content)
	start, end := previewWindow(len(body), p.previewOffset, termHeight)
	for i := start; i < end; i++ {
		lines = append(lines, boxBorderV+" "+toneStyle(body[i].Tone, s).Render(
			text.Pad(text.Truncate(body[i].Text, content), content))+" "+boxBorderV)
	}
	lines = append(lines, boxCornerBL+strings.Repeat(boxBorderH, inner)+boxCornerBR)
	return append(lines, "  "+s.Dim.Render(
		text.Truncate(previewHint(end-start < len(body), p.CommitLabel()), inner)))
}

// previewWindow returns the half-open range of wrapped lines to draw.
//
// The offset is clamped here rather than when the key was pressed: the number of
// lines depends on the box width, so the same offset is legal at one terminal
// size and past the end at another, and a resize must not be able to leave the
// page scrolled into empty space.
func previewWindow(total, offset, termHeight int) (int, int) {
	size := total
	if termHeight > 0 {
		size = max(termHeight-previewChromeLines, 1)
	}
	if size >= total {
		return 0, total
	}
	start := min(max(offset, 0), total-size)
	return start, start + size
}

// wrapPreview folds the logical lines onto the box width.
func wrapPreview(lines []previewLine, width int) []StatusLine {
	out := make([]StatusLine, 0, len(lines))
	for _, l := range lines {
		chunks := wrapText(l.text, max(width-l.indent-continuationIndent, 1))
		for i, chunk := range chunks {
			indent := l.indent
			if i > 0 {
				indent += continuationIndent
			}
			out = append(out, StatusLine{Text: strings.Repeat(" ", indent) + chunk, Tone: l.tone})
		}
	}
	return out
}

// wrapText breaks s onto lines of at most width display columns, on spaces where
// it can and mid-word only when a single word does not fit at all.
func wrapText(s string, width int) []string {
	words := strings.Fields(s)
	if len(words) == 0 {
		return []string{""}
	}
	var (
		out  []string
		line string
	)
	for _, word := range words {
		switch {
		case line == "":
			line = word
		case ansi.StringWidth(line)+1+ansi.StringWidth(word) <= width:
			line += " " + word
		default:
			out = append(out, line)
			line = word
		}
		// A word wider than the whole column is truncated rather than dropped:
		// it is usually a path or a very long title, and the reader is better
		// served by its beginning than by nothing.
		if ansi.StringWidth(line) > width {
			out = append(out, text.Truncate(line, width))
			line = ""
		}
	}
	if line != "" {
		out = append(out, line)
	}
	return out
}

// previewHint is the key legend of the Preview page.
func previewHint(scrolled bool, commit string) string {
	scroll := ""
	if scrolled {
		scroll = "j/k scrolls · "
	}
	return scroll + "tab recipes · r replan · ctrl+s " + strings.ToLower(commit) + " · esc cancel"
}

// tabBarTitle renders the overlay title with the active tab marked, and reports
// the display width of the plain text behind it so the border arithmetic does
// not have to measure escape sequences.
func tabBarTitle(active panelTab, s styles.Styles) (string, int) {
	const name = " Auto-DJ "
	rendered := s.Heading.Render(name)
	width := ansi.StringWidth(name)
	for i := range int(tabCount) {
		label := " " + tabLabels[i] + " "
		width += ansi.StringWidth(label)
		if panelTab(i) == active {
			rendered += s.ActiveTab.Render(label)
			continue
		}
		rendered += s.TextTertiary.Render(label)
	}
	return rendered, width
}
