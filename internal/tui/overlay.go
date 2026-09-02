package tui

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

const (
	// OverlayMinInnerWidth is the minimum number of characters between the
	// vertical borders of an overlay box.
	OverlayMinInnerWidth = 24

	// OverlayHPadding is the horizontal padding (in characters) inside each
	// content row, split equally between left and right.
	OverlayHPadding = 4

	borderH  = "─"
	borderV  = "│"
	cornerTL = "╭"
	cornerTR = "╮"
	cornerBL = "╰"
	cornerBR = "╯"
	crossL   = "├"
	crossR   = "┤"
	// Doubled rule below modal titles strengthens Common Region by visually
	// detaching the title from the body.
	borderDoubleH = "═"
	crossDoubleL  = "╞"
	crossDoubleR  = "╡"
)

// OverlayRow represents a single content row inside an OverlayBox.
type OverlayRow struct {
	Label  string // optional right-aligned label (e.g. "Title:")
	Text   string // text content of the row
	Styled bool   // if true, wrap with styles.Accent
	Blank  bool   // if true, render an empty row (Text is ignored)
}

// OverlayBox is a reusable centered-box renderer shared by all overlay types.
type OverlayBox struct {
	Title         string       // e.g. " ♪ Preview ", " ♪ Now Playing "
	Rows          []OverlayRow // content rows between the title separator and bottom border
	Hint          string       // e.g. "P or esc to dismiss"
	MaxInnerWidth int          // if > 0, clamp inner width to this value
}

// Render produces a vertically and horizontally centered box string.
// The caller passes the current terminal dimensions so the box is positioned
// in the middle of the screen.
func (o OverlayBox) Render(termWidth, termHeight int, s styles.Styles) string {
	rows := o.rowsForHeight(termHeight)

	// Find the widest label for right-alignment.
	maxLabelW := 0
	for _, r := range rows {
		if lw := ansi.StringWidth(r.Label); lw > maxLabelW {
			maxLabelW = lw
		}
	}

	// Collect plain-text row content for width calculation.
	// For labelled rows the full visual width is: label + " " + text.
	rowTexts := make([]string, len(rows))
	for i, r := range rows {
		if r.Blank {
			continue
		}
		if r.Label != "" {
			rowTexts[i] = text.PadLeft(r.Label, maxLabelW) + " " + r.Text
		} else {
			rowTexts[i] = r.Text
		}
	}

	// Calculate inner width (space between the vertical borders).
	minInner := OverlayMinInnerWidth
	for _, text := range append([]string{o.Title}, rowTexts...) {
		if w := ansi.StringWidth(text) + OverlayHPadding; w > minInner {
			minInner = w
		}
	}
	innerWidth := minInner
	if o.MaxInnerWidth > 0 && innerWidth > o.MaxInnerWidth {
		innerWidth = o.MaxInnerWidth
	}
	// Enforce a floor of OverlayHPadding so content rows (which always reserve
	// that many characters for padding) cannot exceed innerWidth and misalign
	// the borders when a caller passes a tiny MaxInnerWidth.
	if innerWidth < OverlayHPadding {
		innerWidth = OverlayHPadding
	}
	contentWidth := innerWidth - OverlayHPadding // padding inside borders
	if contentWidth < 0 {
		contentWidth = 0
	}

	var boxLines []string

	// Top border.
	boxLines = append(boxLines, cornerTL+strings.Repeat(borderH, innerWidth)+cornerTR)

	// Title row (centered). Truncate first so a MaxInnerWidth clamp narrower
	// than the title cannot overflow and misalign the borders.
	displayTitle := text.Truncate(o.Title, innerWidth)
	titlePad := max(0, innerWidth-ansi.StringWidth(displayTitle))
	leftTP := titlePad / 2
	rightTP := titlePad - leftTP
	boxLines = append(boxLines, borderV+strings.Repeat(" ", leftTP)+s.Heading.Render(displayTitle)+strings.Repeat(" ", rightTP)+borderV)

	// Doubled rule below the title (Common Region + Uniform Connectedness).
	boxLines = append(boxLines, crossDoubleL+strings.Repeat(borderDoubleH, innerWidth)+crossDoubleR)

	// Inner top padding row.
	boxLines = append(boxLines, borderV+strings.Repeat(" ", innerWidth)+borderV)

	// Content rows.
	for _, r := range rows {
		if r.Blank {
			boxLines = append(boxLines, borderV+strings.Repeat(" ", innerWidth)+borderV)
			continue
		}
		var row string
		if r.Label != "" {
			// Right-align the label, then left-align the value.
			valueWidth := max(0, contentWidth-maxLabelW-1) // 1 for the space between label and value
			truncated := text.Truncate(r.Text, valueWidth)
			// Use text.PadLeft for the label and text.Pad for the value
			// so that multi-byte characters are padded by display width,
			// not byte count.
			row = text.PadLeft(r.Label, maxLabelW) + " " + text.Pad(truncated, valueWidth)
			// Truncate/pad the assembled content to contentWidth so a tiny
			// MaxInnerWidth that cannot fit the label cannot overflow.
			row = text.Pad(text.Truncate(row, contentWidth), contentWidth)
			row = "  " + row + "  "
		} else {
			truncated := text.Truncate(r.Text, contentWidth)
			row = "  " + text.Pad(truncated, contentWidth) + "  "
		}
		if r.Styled {
			boxLines = append(boxLines, borderV+s.Accent.Render(row)+borderV)
		} else {
			boxLines = append(boxLines, borderV+row+borderV)
		}
	}

	// Inner bottom padding row.
	boxLines = append(boxLines, borderV+strings.Repeat(" ", innerWidth)+borderV)

	// Bottom border.
	boxLines = append(boxLines, cornerBL+strings.Repeat(borderH, innerWidth)+cornerBR)

	// Center the box.
	boxW := ansi.StringWidth(boxLines[0])
	boxH := len(boxLines)
	if o.Hint != "" {
		boxH++
	}

	leftPadBox := 0
	if termWidth > boxW {
		leftPadBox = (termWidth - boxW) / 2
	}
	topPad := 0
	if termHeight > boxH {
		topPad = (termHeight - boxH) / 2
	}

	boxPrefix := strings.Repeat(" ", leftPadBox)
	var b strings.Builder
	for range topPad {
		b.WriteByte('\n')
	}
	for _, line := range boxLines {
		b.WriteString(boxPrefix + line + "\n")
	}
	if o.Hint != "" {
		b.WriteString(centeredOverlayHintLine(o.Hint, termWidth) + "\n")
	}
	return b.String()
}

func renderCenteredOverlayLines(lines []string, hint string, termWidth, termHeight int) string {
	if len(lines) == 0 {
		return ""
	}

	boxW := ansi.StringWidth(lines[0])
	boxH := len(lines)
	if hint != "" {
		boxH++
	}

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

func (o OverlayBox) rowsForHeight(termHeight int) []OverlayRow {
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

	rows := make([]OverlayRow, 0, len(o.Rows))
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

// renderTrackOverlay builds an OverlayBox for a track (preview or now-playing)
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
	rows := []OverlayRow{
		{Label: "Title:", Text: title},
		{Label: "Artist:", Text: artist},
	}
	if album != "" {
		rows = append(rows, OverlayRow{Label: "Album:", Text: album})
	}
	rows = append(rows, OverlayRow{Label: "Play Count:", Text: strconv.Itoa(playCount)})
	if tagsLine != "" {
		rows = append(rows, OverlayRow{Label: "Tags:", Text: tagsLine, Styled: true})
	}
	rows = append(rows, OverlayRow{Blank: true})
	rows = append(rows, OverlayRow{Text: progressLine, Styled: true})

	box := OverlayBox{Title: boxTitle, Rows: rows, Hint: hint}
	return box.Render(termWidth, termHeight, s)
}
