package autodj

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/tui/overlay"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

// previewChromeLines is what the Preview page costs before any content: the top
// border, the tab bar, its separator, the bottom border, and the hint.
const previewChromeLines = 7

// continuationIndent hangs a wrapped explanation under its own line so a reader
// can tell a continuation from the next track.
const continuationIndent = 2

// buildPreviewLines renders the Preview page as a box sized to the terminal.
func (p TunePanel) buildPreviewLines(termWidth, termHeight int, s styles.Styles, v Preview) []string {
	inner := innerWidth(termWidth)
	content := max(inner-4, 0)
	lines := panelHeader(tabPreview, s, inner)

	pinned, body := v.presentationLines(p.tagNames(), content)
	maxPinned := max(0, overlay.PanelContentHeight(termHeight)-previewChromeLines-1)
	if len(pinned) > maxPinned {
		body = append(append([]StatusLine(nil), pinned[maxPinned:]...), body...)
		pinned = pinned[:maxPinned]
	}
	for _, line := range pinned {
		lines = append(lines, boxBorderV+"  "+s.Heading.Render(text.Pad(line.Text, content))+"  "+boxBorderV)
	}
	start, end := previewWindow(len(body), p.previewOffset, overlay.PanelContentHeight(termHeight)-len(pinned))
	for i := start; i < end; i++ {
		lines = append(lines, boxBorderV+"  "+toneStyle(body[i].Tone, s).Render(
			text.Pad(text.Truncate(body[i].Text, content), content))+"  "+boxBorderV)
	}
	lines = append(lines, boxCornerBL+strings.Repeat(boxBorderH, inner)+boxCornerBR)
	hint := previewHint(end-start < len(body), p.CommitLabel())
	if end-start < len(body) {
		hint = strings.Replace(hint, "j/k scrolls", fmt.Sprintf("j/k scrolls %d–%d/%d", start+1, end, len(body)), 1)
	}
	return appendPanelHints(lines, hint, inner, termHeight, s)
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
func tabBarTitle(active panelTab, s styles.Styles) string {
	rendered := ""
	for i := range int(tabCount) {
		label := " " + tabLabels[i] + " "
		if panelTab(i) == active {
			label = "[" + tabLabels[i] + "]"
		}
		if panelTab(i) == active {
			rendered += s.ActiveTab.Render(label)
			continue
		}
		rendered += s.TextTertiary.Render(label)
	}

	return rendered
}

func (v Preview) presentationLines(names map[int64]string, content int) ([]StatusLine, []StatusLine) {
	logical := v.previewLines(names)
	if v.Status != PreviewReady {
		return nil, wrapPreview(logical, content)
	}
	discovery := 0
	for _, track := range v.Tracks {
		if track.Discovery.IsDiscovery() {
			discovery++
		}
	}
	summary := []previewLine{
		{text: fmt.Sprintf("Would add %d %s · after %d queued", len(v.Tracks), text.Pluralize(len(v.Tracks), "track", "tracks"), len(v.Ahead)+v.AheadMore)},
		{text: fmt.Sprintf("%d discovery · %d familiar · target %d discovery", discovery, len(v.Tracks)-discovery, v.DiscoveryTarget)},
	}
	if v.Replanning {
		summary = append(summary, previewLine{text: "Inputs moved — replanning…", tone: ToneWarn})
	} else if v.Stale {
		summary = append(summary, previewLine{text: "Inputs moved — press r to replan", tone: ToneWarn})
	}
	pinned := wrapPreview(summary, content)
	batchSummary := v.batchSummary()
	filtered := make([]previewLine, 0, len(logical))
	for _, line := range logical {
		if line.text != batchSummary {
			filtered = append(filtered, line)
		}
	}
	return pinned, wrapPreview(filtered, content)
}

func panelHeader(active panelTab, s styles.Styles, inner int) []string {
	lines := boxHeader("Auto-DJ · tab/shift+tab tabs", inner, s)
	tabs := tabBarTitle(active, s)
	lines = append(lines, boxBorderV+"  "+text.Pad(tabs, inner-4)+"  "+boxBorderV,
		boxCrossL+strings.Repeat(boxBorderH, inner)+boxCrossR)
	return lines
}
