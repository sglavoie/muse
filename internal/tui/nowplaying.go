package tui

import (
	"fmt"

	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

// NowPlayingOverlay renders a centered overlay box showing information about
// the currently playing track.
type NowPlayingOverlay struct {
	artist    string
	title     string
	album     string
	elapsed   float64
	total     float64
	state     playback.State
	playCount int
	tagsLine  string // pre-rendered colored tag names; empty when none
}

// Render produces a centered overlay string for the now-playing info.
func (n NowPlayingOverlay) Render(termWidth, termHeight int, s styles.Styles) string {
	var icon string
	switch n.state {
	case playback.StatePaused:
		icon = "⏸"
	case playback.StateStopped:
		icon = "■"
	case playback.StatePlaying:
		icon = "▶"
	default:
		// PlaybackIdle (zero value) or any unrecognised state: avoid
		// implying playback is in progress. Production guards open this
		// overlay only when not idle/stopped, but rendering "▶" for an
		// idle state on tests or defensive paths would be misleading.
		icon = "■"
	}
	progress := fmt.Sprintf("%s %s / %s", icon, text.FormatPositionSecs(n.elapsed), text.FormatPositionSecs(n.total))
	return renderTrackOverlay(n.title, n.artist, n.album, progress, n.playCount, n.tagsLine,
		" ♪ Now Playing ", "I or esc to dismiss", termWidth, termHeight, s)
}
