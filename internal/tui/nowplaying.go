package tui

import (
	"fmt"

	"github.com/sglavoie/muse/internal/tui/styles"
)

// NowPlayingOverlay renders a centered overlay box showing information about
// the currently playing track.
type NowPlayingOverlay struct {
	artist  string
	title   string
	album   string
	elapsed float64
	total   float64
	state   PlaybackState
}

// Render produces a centered overlay string for the now-playing info.
func (n NowPlayingOverlay) Render(termWidth, termHeight int, s styles.Styles) string {
	title := n.title
	if title == "" {
		title = "(unknown)"
	}
	artist := n.artist
	if artist == "" {
		artist = "(unknown)"
	}

	rows := []OverlayRow{
		{Label: "Title:", Text: title},
		{Label: "Artist:", Text: artist},
	}
	if n.album != "" {
		rows = append(rows, OverlayRow{Label: "Album:", Text: n.album})
	}

	// Blank separator.
	rows = append(rows, OverlayRow{Blank: true})

	// State icon + elapsed / total.
	icon := "▶"
	if n.state == PlaybackPaused {
		icon = "⏸"
	}
	progressLine := fmt.Sprintf("%s %s / %s", icon, formatTime(n.elapsed), formatTime(n.total))
	rows = append(rows, OverlayRow{Text: progressLine, Styled: true})

	box := OverlayBox{
		Title: " ♪ Now Playing ",
		Rows:  rows,
		Hint:  "I or esc to dismiss",
	}
	return box.Render(termWidth, termHeight, s)
}
