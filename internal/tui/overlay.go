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
	// Find the widest label for right-alignment.
	maxLabelW := 0
	for _, r := range o.Rows {
		if lw := ansi.StringWidth(r.Label); lw > maxLabelW {
			maxLabelW = lw
		}
	}

	// Collect plain-text row content for width calculation.
	// For labelled rows the full visual width is: label + " " + text.
	rowTexts := make([]string, len(o.Rows))
	for i, r := range o.Rows {
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
	if innerWidth < 1 {
		innerWidth = 1
	}
	contentWidth := innerWidth - OverlayHPadding // padding inside borders
	if contentWidth < 0 {
		contentWidth = 0
	}

	var lines []string

	// Top border.
	lines = append(lines, cornerTL+strings.Repeat(borderH, innerWidth)+cornerTR)

	// Title row (centered).
	titlePad := max(0, innerWidth-ansi.StringWidth(o.Title))
	leftTP := titlePad / 2
	rightTP := titlePad - leftTP
	lines = append(lines, borderV+strings.Repeat(" ", leftTP)+s.Heading.Render(o.Title)+strings.Repeat(" ", rightTP)+borderV)

	// Separator.
	lines = append(lines, crossL+strings.Repeat(borderH, innerWidth)+crossR)

	// Content rows.
	for _, r := range o.Rows {
		if r.Blank {
			lines = append(lines, borderV+strings.Repeat(" ", innerWidth)+borderV)
			continue
		}
		var row string
		if r.Label != "" {
			// Right-align the label, then left-align the value.
			valueWidth := max(0, contentWidth-maxLabelW-1) // 1 for the space between label and value
			truncated := truncateTitle(r.Text, valueWidth)
			// Use text.PadLeft for the label and text.Pad for the value
			// so that multi-byte characters are padded by display width,
			// not byte count.
			row = "  " + text.PadLeft(r.Label, maxLabelW) + " " + text.Pad(truncated, valueWidth) + "  "
		} else {
			truncated := truncateTitle(r.Text, contentWidth)
			row = "  " + text.Pad(truncated, contentWidth) + "  "
		}
		if r.Styled {
			lines = append(lines, borderV+s.Accent.Render(row)+borderV)
		} else {
			lines = append(lines, borderV+row+borderV)
		}
	}

	// Bottom border.
	lines = append(lines, cornerBL+strings.Repeat(borderH, innerWidth)+cornerBR)

	// Hint.
	if o.Hint != "" {
		lines = append(lines, "  "+o.Hint)
	}

	// Center the box.
	boxW := ansi.StringWidth(lines[0])
	boxH := len(lines)

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
	return b.String()
}

// renderTrackOverlay builds an OverlayBox for a track (preview or now-playing)
// and renders it centered. Callers provide the pre-formatted progress line and
// box chrome (title, hint).
func renderTrackOverlay(title, artist, album, progressLine string, playCount int, boxTitle, hint string,
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
	rows = append(rows, OverlayRow{Blank: true})
	rows = append(rows, OverlayRow{Text: progressLine, Styled: true})

	box := OverlayBox{Title: boxTitle, Rows: rows, Hint: hint}
	return box.Render(termWidth, termHeight, s)
}
