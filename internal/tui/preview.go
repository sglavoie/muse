package tui

import (
	"fmt"
	"strings"

	"github.com/sglavoie/muse/internal/tui/styles"
)

// PreviewOverlay renders a centered overlay box showing preview track info
// and live playback progress.
type PreviewOverlay struct {
	artist  string
	title   string
	elapsed float64
	total   float64
}

// Render produces a centered overlay string for the preview. The caller should
// pass the current terminal dimensions so the box is positioned in the middle
// of the screen.
func (p PreviewOverlay) Render(termWidth, termHeight int, s styles.Styles) string {
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

	// Build content lines.
	trackLine := p.artist + " – " + p.title
	if p.artist == "" && p.title == "" {
		trackLine = "(unknown)"
	} else if p.artist == "" {
		trackLine = p.title
	} else if p.title == "" {
		trackLine = p.artist
	}

	progressLine := fmt.Sprintf("▶ %s / %s", formatTime(p.elapsed), formatTime(p.total))

	// Calculate inner width.
	const modalTitle = " ♪ Preview "
	minInner := 24
	for _, line := range []string{modalTitle, trackLine, progressLine} {
		if w := len([]rune(line)) + 4; w > minInner {
			minInner = w
		}
	}
	innerWidth := minInner
	contentWidth := innerWidth - 4 // padding inside borders

	var lines []string

	// Top border.
	lines = append(lines, cornerTL+strings.Repeat(borderH, innerWidth)+cornerTR)

	// Title row (centered).
	titlePad := innerWidth - len([]rune(modalTitle))
	leftTP := titlePad / 2
	rightTP := titlePad - leftTP
	lines = append(lines, borderV+strings.Repeat(" ", leftTP)+s.Heading.Render(modalTitle)+strings.Repeat(" ", rightTP)+borderV)

	// Separator.
	lines = append(lines, crossL+strings.Repeat(borderH, innerWidth)+crossR)

	// Track info row.
	truncated := truncateTitle(trackLine, contentWidth)
	row := fmt.Sprintf("  %-*s  ", contentWidth, truncated)
	lines = append(lines, borderV+row+borderV)

	// Blank row.
	lines = append(lines, borderV+strings.Repeat(" ", innerWidth)+borderV)

	// Progress row.
	progRow := fmt.Sprintf("  %-*s  ", contentWidth, progressLine)
	lines = append(lines, borderV+s.Accent.Render(progRow)+borderV)

	// Bottom border.
	lines = append(lines, cornerBL+strings.Repeat(borderH, innerWidth)+cornerBR)

	// Hint.
	lines = append(lines, "  P or esc to dismiss")

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
