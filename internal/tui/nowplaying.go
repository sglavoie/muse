package tui

import (
	"fmt"

	"github.com/sglavoie/muse/internal/tui/styles"
)

// NowPlayingOverlay renders a centered overlay box showing information about
// the currently playing track.
type NowPlayingOverlay struct {
	artist    string
	title     string
	album     string
	elapsed   float64
	total     float64
	state     PlaybackState
	playCount int
}

// Render produces a centered overlay string for the now-playing info.
func (n NowPlayingOverlay) Render(termWidth, termHeight int, s styles.Styles) string {
	icon := "▶"
	if n.state == PlaybackPaused {
		icon = "⏸"
	}
	progress := fmt.Sprintf("%s %s / %s", icon, formatTime(n.elapsed), formatTime(n.total))
	return renderTrackOverlay(n.title, n.artist, n.album, progress, n.playCount,
		" ♪ Now Playing ", "I or esc to dismiss", termWidth, termHeight, s)
}
