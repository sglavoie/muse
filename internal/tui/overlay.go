package tui

import (
	"fmt"
	"strings"

	"github.com/sglavoie/muse/internal/tui/styles"
)

// OverlayRow represents a single content row inside an OverlayBox.
type OverlayRow struct {
	Text   string // text content of the row
	Styled bool   // if true, wrap with styles.Accent
	Blank  bool   // if true, render an empty row (Text is ignored)
}

// OverlayBox is a reusable centered-box renderer shared by all overlay types.
type OverlayBox struct {
	Title string       // e.g. " ♪ Preview ", " ♪ Now Playing "
	Rows  []OverlayRow // content rows between the title separator and bottom border
	Hint  string       // e.g. "P or esc to dismiss"
}

// Render produces a vertically and horizontally centered box string.
// The caller passes the current terminal dimensions so the box is positioned
// in the middle of the screen.
func (o OverlayBox) Render(termWidth, termHeight int, s styles.Styles) string {
	const (
		borderH  = "─"
		borderV  = "│"
		cornerTL = "╭"
		cornerTR = "╮"
		cornerBL = "╰"
		cornerBR = "╯"
		crossL   = "├"
		crossR   = "┤"
	)

	// Collect plain-text row content for width calculation.
	rowTexts := make([]string, len(o.Rows))
	for i, r := range o.Rows {
		if !r.Blank {
			rowTexts[i] = r.Text
		}
	}

	// Calculate inner width (space between the vertical borders).
	minInner := 24
	for _, text := range append([]string{o.Title}, rowTexts...) {
		if w := len([]rune(text)) + 4; w > minInner {
			minInner = w
		}
	}
	innerWidth := minInner
	contentWidth := innerWidth - 4 // padding inside borders

	var lines []string

	// Top border.
	lines = append(lines, cornerTL+strings.Repeat(borderH, innerWidth)+cornerTR)

	// Title row (centered).
	titlePad := innerWidth - len([]rune(o.Title))
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
		truncated := truncateTitle(r.Text, contentWidth)
		row := fmt.Sprintf("  %-*s  ", contentWidth, truncated)
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
	boxW := len([]rune(lines[0]))
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
