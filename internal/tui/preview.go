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
	album   string
	elapsed float64
	total   float64
}

// Render produces a centered overlay string for the preview. The caller should
// pass the current terminal dimensions so the box is positioned in the middle
// of the screen.
func (p PreviewOverlay) Render(termWidth, termHeight int, s styles.Styles) string {
	title := p.title
	if title == "" {
		title = "(unknown)"
	}
	artist := p.artist
	if artist == "" {
		artist = "(unknown)"
	}

	progressLine := fmt.Sprintf("▶ %s / %s", formatTime(p.elapsed), formatTime(p.total))

	rows := []OverlayRow{
		{Label: "Title:", Text: title},
		{Label: "Artist:", Text: artist},
	}
	if p.album != "" {
		rows = append(rows, OverlayRow{Label: "Album:", Text: p.album})
	}
	rows = append(rows, OverlayRow{Blank: true})
	rows = append(rows, OverlayRow{Text: progressLine, Styled: true})

	box := OverlayBox{
		Title: " ♪ Preview ",
		Rows:  rows,
		Hint:  "P or esc to dismiss",
	}
	return box.Render(termWidth, termHeight, s)
}
