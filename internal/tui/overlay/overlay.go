package overlay

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

const (
	// MinInnerWidth is the minimum number of characters between the
	// vertical borders of an overlay box.
	MinInnerWidth = 24

	// HPadding is the horizontal padding (in characters) inside each
	// content row, split equally between left and right.
	HPadding = 4

	BorderH  = "─"
	BorderV  = "│"
	CornerTL = "╭"
	CornerTR = "╮"
	CornerBL = "╰"
	CornerBR = "╯"
	CrossL   = "├"
	CrossR   = "┤"
)

// Row represents a single content row inside a Box.
type Row struct {
	Label  string // optional right-aligned label (e.g. "Title:")
	Text   string // text content of the row
	Styled bool   // if true, wrap with styles.Accent
	Blank  bool   // if true, render an empty row (Text is ignored)
}

// Box is a reusable top-anchored renderer for informational overlays.
type Box struct {
	Title         string // e.g. " ♪ Preview ", " ♪ Now Playing "
	Rows          []Row  // content rows between the title separator and bottom border
	Hint          string // e.g. "P or esc to dismiss"
	MaxInnerWidth int    // if > 0, clamp inner width to this value
}

// Render produces a horizontally centered box anchored to the top of the screen.
func (o Box) Render(termWidth, termHeight int, s styles.Styles) string {
	rows := o.rowsForHeight(termHeight)
	maxLabelW := boxLabelWidth(rows)
	innerWidth := o.innerWidth(rows, maxLabelW)
	contentWidth := max(innerWidth-HPadding, 0) // padding inside borders
	padRow := BorderV + strings.Repeat(" ", innerWidth) + BorderV

	boxLines := []string{
		// Top border.
		CornerTL + strings.Repeat(BorderH, innerWidth) + CornerTR,
		o.titleLine(innerWidth, s),
		// Shared single rule below the title.
		CrossL + strings.Repeat(BorderH, innerWidth) + CrossR,
		// Inner top padding row.
		padRow,
	}
	for _, r := range rows {
		boxLines = append(boxLines, renderBoxRow(r, padRow, maxLabelW, contentWidth, s))
	}
	boxLines = append(boxLines,
		// Inner bottom padding row.
		padRow,
		// Bottom border.
		CornerBL+strings.Repeat(BorderH, innerWidth)+CornerBR,
	)

	return RenderTopLines(boxLines, o.Hint, termWidth)
}

// boxLabelWidth is the display width of the widest row label, which fixes the
// column every label is right-aligned into.
func boxLabelWidth(rows []Row) int {
	maxLabelW := 0
	for _, r := range rows {
		maxLabelW = max(maxLabelW, ansi.StringWidth(r.Label))
	}
	return maxLabelW
}

// boxRowTexts returns the plain-text content of each row for width
// calculation. A labelled row measures as label + " " + text.
func boxRowTexts(rows []Row, maxLabelW int) []string {
	rowTexts := make([]string, len(rows))
	for i, r := range rows {
		if r.Blank {
			continue
		}
		if r.Label != "" {
			rowTexts[i] = text.PadLeft(r.Label, maxLabelW) + " " + r.Text
			continue
		}
		rowTexts[i] = r.Text
	}
	return rowTexts
}

// innerWidth is the space between the vertical borders: wide enough for the
// title and every row, clamped to MaxInnerWidth, and floored at HPadding so
// content rows (which always reserve that many characters for padding) cannot
// exceed it and misalign the borders when a caller passes a tiny clamp.
func (o Box) innerWidth(rows []Row, maxLabelW int) int {
	inner := MinInnerWidth
	for _, t := range append([]string{o.Title}, boxRowTexts(rows, maxLabelW)...) {
		inner = max(inner, ansi.StringWidth(t)+HPadding)
	}
	if o.MaxInnerWidth > 0 && inner > o.MaxInnerWidth {
		inner = o.MaxInnerWidth
	}
	return max(inner, HPadding)
}

// titleLine renders the centered title row. The title is truncated first so a
// MaxInnerWidth clamp narrower than the title cannot overflow and misalign the
// borders.
func (o Box) titleLine(innerWidth int, s styles.Styles) string {
	displayTitle := text.Truncate(o.Title, innerWidth)
	titlePad := max(0, innerWidth-ansi.StringWidth(displayTitle))
	leftTP := titlePad / 2
	return BorderV + strings.Repeat(" ", leftTP) + s.Heading.Render(displayTitle) +
		strings.Repeat(" ", titlePad-leftTP) + BorderV
}

// renderBoxRow renders one content row, falling back to padRow for a blank one.
func renderBoxRow(r Row, padRow string, maxLabelW, contentWidth int, s styles.Styles) string {
	if r.Blank {
		return padRow
	}
	row := "  " + boxRowBody(r, maxLabelW, contentWidth) + "  "
	if r.Styled {
		return BorderV + s.Accent.Render(row) + BorderV
	}
	return BorderV + row + BorderV
}

// boxRowBody lays a row's content out across contentWidth: a right-aligned
// label followed by a left-aligned value, or plain padded text. text.PadLeft
// and text.Pad pad by display width, not byte count, so multi-byte characters
// stay aligned.
func boxRowBody(r Row, maxLabelW, contentWidth int) string {
	if r.Label == "" {
		return text.Pad(text.Truncate(r.Text, contentWidth), contentWidth)
	}
	valueWidth := max(0, contentWidth-maxLabelW-1) // 1 for the space between label and value
	row := text.PadLeft(r.Label, maxLabelW) + " " + text.Pad(text.Truncate(r.Text, valueWidth), valueWidth)
	// Truncate/pad the assembled content to contentWidth so a tiny
	// MaxInnerWidth that cannot fit the label cannot overflow.
	return text.Pad(text.Truncate(row, contentWidth), contentWidth)
}

// RenderCenteredLines places the box in the middle of the terminal, both
// horizontally and vertically.
func RenderCenteredLines(lines []string, hint string, termWidth, termHeight int) string {
	if len(lines) == 0 {
		return ""
	}

	boxH := len(lines)
	if hint != "" {
		boxH++
	}
	topPad := 0
	if termHeight > boxH {
		topPad = (termHeight - boxH) / 2
	}
	return renderLinesAt(lines, hint, termWidth, topPad)
}

// RenderTopLines centers the box horizontally but anchors it to the top of the
// terminal. Overlays whose height changes with their content (the command
// palette as its result list shrinks, Settings as tabs swap) use this so the
// box stays put instead of drifting up and down the screen.
func RenderTopLines(lines []string, hint string, termWidth int) string {
	if len(lines) == 0 {
		return ""
	}
	return renderLinesAt(lines, hint, termWidth, 0)
}

// renderLinesAt writes the box lines (and optional hint) horizontally centered
// in termWidth, preceded by topPad blank lines.
func renderLinesAt(lines []string, hint string, termWidth, topPad int) string {
	boxW := ansi.StringWidth(lines[0])
	leftPad := 0
	if termWidth > boxW {
		leftPad = (termWidth - boxW) / 2
	}

	prefix := strings.Repeat(" ", leftPad)
	var b strings.Builder
	for range topPad {
		b.WriteByte('\n')
	}
	for _, line := range lines {
		b.WriteString(prefix + line + "\n")
	}
	if hint != "" {
		b.WriteString(centeredOverlayHintLine(hint, termWidth) + "\n")
	}
	return b.String()
}

func centeredOverlayHintLine(hint string, termWidth int) string {
	hint = sanitizeHint(hint)
	if termWidth > 0 {
		hint = text.Truncate(hint, termWidth)
	}
	hintPad := 0
	hintWidth := ansi.StringWidth(hint)
	if termWidth > hintWidth {
		hintPad = (termWidth - hintWidth) / 2
	}
	return strings.Repeat(" ", hintPad) + hint
}

func (o Box) rowsForHeight(termHeight int) []Row {
	if termHeight <= 0 {
		return o.Rows
	}
	chromeLines := 6
	if o.Hint != "" {
		chromeLines++
	}
	maxRows := termHeight - chromeLines
	if maxRows >= len(o.Rows) {
		return o.Rows
	}
	if maxRows <= 0 {
		return nil
	}

	rows := make([]Row, 0, len(o.Rows))
	for _, row := range o.Rows {
		if !row.Blank {
			rows = append(rows, row)
		}
	}
	for len(rows) > maxRows {
		drop := len(rows) - 2
		if drop < 0 {
			drop = len(rows) - 1
		}
		rows = append(rows[:drop], rows[drop+1:]...)
	}
	return rows
}

// sanitizeHint flattens embedded newlines/tabs to single spaces so the hint
// can be centered as a single line without leaking extra rows.
func sanitizeHint(s string) string {
	if !strings.ContainsAny(s, "\r\n\t") {
		return s
	}
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	return strings.ReplaceAll(s, "\t", " ")
}

// renderTrackOverlay builds a Box for a track (preview or now-playing)
// and renders it centered. Callers provide the pre-formatted progress line,
// box chrome (title, hint), and an optional tagsLine (empty = no Tags row).
func renderTrackOverlay(title, artist, album, progressLine string, playCount int, tagsLine, boxTitle, hint string,
	termWidth, termHeight int, s styles.Styles) string {
	if title == "" {
		title = "(unknown title)"
	}
	if artist == "" {
		artist = "(unknown artist)"
	}
	rows := []Row{
		{Label: "Title:", Text: title},
		{Label: "Artist:", Text: artist},
	}
	if album != "" {
		rows = append(rows, Row{Label: "Album:", Text: album})
	}
	rows = append(rows, Row{Label: "Play Count:", Text: strconv.Itoa(playCount)})
	if tagsLine != "" {
		rows = append(rows, Row{Label: "Tags:", Text: tagsLine, Styled: true})
	}
	rows = append(rows, Row{Blank: true})
	rows = append(rows, Row{Text: progressLine, Styled: true})

	box := Box{Title: boxTitle, Rows: rows, Hint: hint}
	return box.Render(termWidth, termHeight, s)
}
