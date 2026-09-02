package overlay

import (
	"fmt"

	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

// NowPlayingOverlay renders a centered overlay box showing information about
// the currently playing track.
type NowPlayingOverlay struct {
	Artist    string
	Title     string
	Album     string
	Elapsed   float64
	Total     float64
	State     playback.State
	PlayCount int
	TagsLine  string // pre-rendered colored tag names; empty when none
}

// Render produces a centered overlay string for the now-playing info.
func (n NowPlayingOverlay) Render(termWidth, termHeight int, s styles.Styles) string {
	var icon string
	switch n.State {
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
	progress := fmt.Sprintf("%s %s / %s", icon, text.FormatPositionSecs(n.Elapsed), text.FormatPositionSecs(n.Total))
	return renderTrackOverlay(n.Title, n.Artist, n.Album, progress, n.PlayCount, n.TagsLine,
		" ♪ Now Playing ", "I or esc to dismiss", termWidth, termHeight, s)
}
