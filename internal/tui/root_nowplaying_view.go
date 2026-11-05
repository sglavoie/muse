package tui

import (
	"strings"
	"time"

	"github.com/sglavoie/muse/internal/tui/overlay"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/statusbar"
	"github.com/sglavoie/muse/internal/tui/text"
)

// nowPlayingUpNext is how many upcoming songs the Now Playing view lists.
const nowPlayingUpNext = 3

// nowPlayingArtSize is the cover size the Now Playing view uses at the
// current terminal size: as tall as the screen comfortably allows, but never
// wider than about half of it so the text keeps room. It is zero when the
// screen is too small for a cover worth showing.
func (m Model) nowPlayingArtSize() (cols, rows int) {
	bodyHeight := max(m.termHeight-3, 1) - 2
	rows = min(bodyHeight-2, 22)
	rows = min(rows, (m.termWidth*48/100)/2)
	if rows < 6 || m.termWidth < 70 {
		return 0, 0
	}
	return 2 * rows, rows
}

// decorateNowPlaying fills the Now Playing view with live state: position,
// cover, listening details, modes, what plays next, and Auto-DJ's reason.
func (m Model) decorateNowPlaying(now overlay.NowPlayingOverlay) overlay.NowPlayingOverlay {
	clock := time.Now()
	now.State, now.Elapsed, now.Total = m.playback.state, m.displayElapsed(clock), m.playback.totalSecs
	st := *m.styles
	elapsed, total := now.Elapsed, now.Total
	now.Progress = func(width int) string { return statusbar.ProgressBar(elapsed, total, width, st) }
	if t, ok := m.playingTrack(); ok {
		now.Details = trackDetailsLine(t, clock)
		if cols, rows := m.nowPlayingArtSize(); cols > 0 {
			cover := m.renderArt(t, cols, rows)
			if cover == "" && m.art != nil {
				cover = artPlaceholder(st, cols, rows)
			}
			now.Cover, now.CoverCols, now.CoverRows = cover, cols, rows
		}
	}
	now.Modes = m.nowPlayingModes()
	ahead := m.djAheadEntries()
	if len(ahead) > 0 && ahead[0].Hash == m.playback.currentHash {
		if reasons := ahead[0].DJReasons; ahead[0].DJOwned && len(reasons) > 0 {
			now.Reason = reasons.Summary()
		}
		ahead = ahead[1:]
	}
	now.UpNext = nil
	for _, e := range ahead[:min(len(ahead), nowPlayingUpNext)] {
		now.UpNext = append(now.UpNext, queueEntryLabel(e))
	}
	return now
}

// nowPlayingModes names the active playback modes, or "" when none are on.
func (m Model) nowPlayingModes() string {
	var labels []string
	for _, indicator := range playback.ModeIndicators(m.playback.repeatMode, m.playback.shuffle, m.playback.stopAfterCurrent, m.radio.enabled) {
		if indicator.Active {
			labels = append(labels, indicator.Label())
		}
	}
	if len(m.queue.Entries) > 0 {
		labels = append(labels, text.Pluralize(len(m.queue.Entries), "1 song queued", text.FormatCount(len(m.queue.Entries))+" songs queued"))
	}
	return strings.Join(labels, " · ")
}
