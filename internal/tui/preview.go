package tui

import (
	"fmt"

	"github.com/sglavoie/muse/internal/tui/styles"
)

// PreviewOverlay renders a centered overlay box showing preview track info
// and live playback progress.
type PreviewOverlay struct {
	hash      string
	artist    string
	title     string
	album     string
	elapsed   float64
	total     float64
	playCount int
}

// Render produces a centered overlay string for the preview. The caller should
// pass the current terminal dimensions so the box is positioned in the middle
// of the screen.
func (p PreviewOverlay) Render(termWidth, termHeight int, s styles.Styles) string {
	progress := fmt.Sprintf("▶ %s / %s", formatTime(p.elapsed), formatTime(p.total))
	return renderTrackOverlay(p.title, p.artist, p.album, progress, p.playCount,
		" ♪ Preview ", "space pause, arrows seek, shift+arrows long seek, P/esc dismiss", termWidth, termHeight, s)
}
