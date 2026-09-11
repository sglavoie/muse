package autodj

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

// Tone classifies a status line so the panel can style it without knowing what
// it says.
type Tone int

const (
	// ToneInfo is an ordinary explanation.
	ToneInfo Tone = iota
	// ToneWarn is something the user probably wants to act on.
	ToneWarn
	// ToneError is a refusal or a failure.
	ToneError
)

// StatusLine is one explanation the root model supplies at render time.
//
// The panel does not keep these. What the last refresh did, and whether the
// refill is parked, are facts about the running session rather than about the
// draft, and a copy taken when the panel opened would be stale by the time it
// mattered.
type StatusLine struct {
	Text string
	Tone Tone
}

const (
	boxBorderH  = "─"
	boxBorderV  = "│"
	boxCornerTL = "╭"
	boxCornerTR = "╮"
	boxCornerBL = "╰"
	boxCornerBR = "╯"
	boxCrossL   = "├"
	boxCrossR   = "┤"

	// labelWidth is the left column. "Include artists" is the longest label.
	labelWidth = 16

	// maxInnerWidth caps the box on a wide terminal. The Preview page's
	// explanations are the widest thing the overlay draws, and they read badly
	// stretched across a full-screen terminal; narrower terminals get whatever
	// they have.
	maxInnerWidth = 76

	// chromeLines is the top border, the title, its separator, the bottom
	// border, and the hint line — everything a box costs before any content.
	chromeLines = 5

	// minRowLines is the smallest row window worth showing. Below it the status
	// block is dropped instead, because a panel that cannot show the settings is
	// not a settings panel.
	minRowLines = 3

	// maxStatusLines bounds the explanation block so a long-winded outcome
	// cannot squeeze the rows off the screen.
	maxStatusLines = 3
)

// Render produces the centered overlay body. An open picker takes the whole
// overlay: it is modal, so showing the rows behind it would invite editing them.
//
// Both the status block and the preview are passed in at render time rather than
// held by the panel. They describe the running session, not the draft, and a
// copy taken when the panel opened would be stale by the time it mattered.
func (p TunePanel) Render(
	termWidth, termHeight int, s styles.Styles, status []StatusLine, preview Preview,
) string {
	if p.picker != nil {
		return centerBox(p.picker.buildBoxLines(termWidth, termHeight, s), termWidth, termHeight)
	}
	if p.prompt != nil {
		return centerBox(p.buildPromptLines(termWidth, s), termWidth, termHeight)
	}
	switch p.tab {
	case tabPreview:
		return centerBox(p.buildPreviewLines(termWidth, termHeight, s, preview), termWidth, termHeight)
	case tabRecipes:
		return centerBox(p.buildRecipesLines(termWidth, termHeight, s), termWidth, termHeight)
	}
	return centerBox(p.buildBoxLines(termWidth, termHeight, s, status), termWidth, termHeight)
}

// innerWidth returns the box's content width for the given terminal width.
func innerWidth(termWidth int) int {
	width := maxInnerWidth
	if maxInner := termWidth - 4; termWidth > 0 && width > maxInner {
		width = maxInner
	}
	return max(width, 1)
}

func (p TunePanel) buildBoxLines(termWidth, termHeight int, s styles.Styles, status []StatusLine) []string {
	inner := innerWidth(termWidth)
	content := max(inner-2, 0)
	title, titleWidth := tabBarTitle(tabTune, s)
	lines := boxHeaderRendered(title, titleWidth, inner)

	rowLines, statusLines := p.splitBudget(termHeight, p.statusLines(status))
	start, end := p.rowWindow(rowLines)
	for r := start; r < end; r++ {
		lines = append(lines, p.renderRow(row(r), content, s))
	}
	if len(statusLines) > 0 {
		lines = append(lines, boxCrossL+strings.Repeat(boxBorderH, inner)+boxCrossR)
		for _, line := range statusLines {
			lines = append(lines, boxBorderV+" "+toneStyle(line.Tone, s).Render(
				text.Pad(text.Truncate(line.Text, content), content))+" "+boxBorderV)
		}
	}
	lines = append(lines, boxCornerBL+strings.Repeat(boxBorderH, inner)+boxCornerBR)
	return append(lines, "  "+s.Dim.Render(
		text.Truncate(p.hint(end-start < int(rowCount)), inner)))
}

// splitBudget divides the vertical space between the rows and the explanation
// block, and reports how many of each fit.
func (p TunePanel) splitBudget(termHeight int, status []StatusLine) (int, []StatusLine) {
	if termHeight <= 0 {
		return int(rowCount), status
	}
	budget := termHeight - chromeLines
	if budget < 1 {
		budget = 1
	}
	shown := min(len(status), maxStatusLines)
	// The block costs a separator on top of its own lines, and it only earns
	// that cost while enough rows still fit to make the panel usable.
	for shown > 0 && budget-shown-1 < minRowLines {
		shown--
	}
	if shown == 0 {
		return min(budget, int(rowCount)), nil
	}
	return min(budget-shown-1, int(rowCount)), status[:shown]
}

// rowWindow returns the half-open range of rows to draw, always containing the
// cursor. The window is derived rather than remembered so a resize cannot leave
// a scroll offset pointing off the end of the list.
func (p TunePanel) rowWindow(size int) (int, int) {
	total := int(rowCount)
	if size >= total || size <= 0 {
		return 0, total
	}
	start := 0
	if p.cursor >= size {
		start = p.cursor - size + 1
	}
	return start, start + size
}

// renderRow draws one settings row, marking the focused one.
func (p TunePanel) renderRow(r row, content int, s styles.Styles) string {
	label := strings.Repeat(" ", labelWidth)
	if rowLabels[r] != "" {
		label = text.Pad(rowLabels[r], labelWidth)
	}
	body := text.Pad(text.Truncate(label+p.rowValue(r), content), content)
	if int(r) == p.cursor {
		return boxBorderV + s.Accent.Render(" "+body+" ") + boxBorderV
	}
	return boxBorderV + " " + body + " " + boxBorderV
}

// statusLines assembles what the panel has to say about the draft, ahead of
// whatever the root model has to say about the session.
func (p TunePanel) statusLines(fromRoot []StatusLine) []StatusLine {
	var lines []StatusLine
	if p.err != nil {
		lines = append(lines, StatusLine{Text: p.err.Error(), Tone: ToneError})
	}
	if missing := p.Unresolved(); len(missing) > 0 {
		lines = append(lines, StatusLine{
			Text: fmt.Sprintf("%d unresolved tag %s — press r to drop them",
				len(missing), text.Pluralize(len(missing), "reference", "references")),
			Tone: ToneWarn,
		})
	}
	switch {
	case p.busy:
		lines = append(lines, StatusLine{Text: p.CommitLabel() + " in progress…", Tone: ToneInfo})
	case p.Dirty():
		lines = append(lines, StatusLine{
			Text: "Unapplied changes — ctrl+s to " + strings.ToLower(p.CommitLabel()), Tone: ToneWarn})
	}
	return append(lines, fromRoot...)
}

// hint returns the widest key legend that fits the box.
func (p TunePanel) hint(scrolled bool) string {
	verb := strings.ToLower(p.CommitLabel())
	scroll := ""
	if scrolled {
		scroll = "j/k scrolls · "
	}
	return scroll + "h/l adjust · enter edit · x clear · tab preview · ctrl+s " + verb + " · esc cancel"
}

func toneStyle(tone Tone, s styles.Styles) lipgloss.Style {
	switch tone {
	case ToneWarn:
		return s.Warning
	case ToneError:
		return s.Error
	}
	return s.Dim
}

// boxHeader returns the top border, the title, and the separator under it.
func boxHeader(title string, inner int, s styles.Styles) []string {
	return boxHeaderRendered(s.Heading.Render(title), ansi.StringWidth(title), inner)
}

// boxHeaderRendered is boxHeader for a title that is already styled. The plain
// width is supplied because a rendered title carries escape sequences the
// centering arithmetic must not count.
func boxHeaderRendered(title string, titleWidth, inner int) []string {
	pad := max(0, inner-titleWidth)
	left := pad / 2
	return []string{
		boxCornerTL + strings.Repeat(boxBorderH, inner) + boxCornerTR,
		boxBorderV + strings.Repeat(" ", left) + title +
			strings.Repeat(" ", pad-left) + boxBorderV,
		boxCrossL + strings.Repeat(boxBorderH, inner) + boxCrossR,
	}
}

// centerBox places pre-built box lines in the middle of the overlay area.
func centerBox(boxLines []string, termWidth, termHeight int) string {
	if len(boxLines) == 0 {
		return ""
	}
	leftPad := 0
	if boxW := ansi.StringWidth(boxLines[0]); termWidth > boxW {
		leftPad = (termWidth - boxW) / 2
	}
	topPad := 0
	if termHeight > len(boxLines) {
		topPad = (termHeight - len(boxLines)) / 2
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
