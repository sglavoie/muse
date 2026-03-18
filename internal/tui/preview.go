package tui

import (
	"fmt"

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

	box := OverlayBox{
		Title: " ♪ Preview ",
		Rows: []OverlayRow{
			{Text: trackLine},
			{Blank: true},
			{Text: progressLine, Styled: true},
		},
		Hint: "P or esc to dismiss",
	}
	return box.Render(termWidth, termHeight, s)
}
