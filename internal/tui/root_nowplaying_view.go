package tui

import (
	"image/color"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/artwork"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/overlay"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/queue"
	"github.com/sglavoie/muse/internal/tui/statusbar"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

// nowPlayingUpNext is how many upcoming songs the Now Playing view lists.
const nowPlayingUpNext = 3

// nowPlayingShown reports whether the full-screen Now Playing view is what
// the screen shows, rather than another overlay drawn in front of it.
func (m Model) nowPlayingShown() bool {
	return m.overlay.nowPlaying != nil && m.ActiveOverlayName() == "now_playing"
}

// nowPlayingArtSize is the cover size the Now Playing view uses at the
// current terminal size: as tall as the screen comfortably allows, but never
// wider than about half of it so the text keeps room. It is zero when the
// screen is too small for a cover worth showing. The view owns the whole
// screen apart from its own key hint, so the status bar's rows go to it too.
func (m Model) nowPlayingArtSize() (cols, rows int) {
	bodyHeight := max(m.termHeight-2, 1)
	rows = min(bodyHeight-2, 22)
	rows = min(rows, (m.termWidth*48/100)/2)
	if rows < 6 || m.termWidth < 70 {
		return 0, 0
	}
	return 2 * rows, rows
}

// renderNowPlayingScreen draws the Now Playing view across the whole
// terminal. The status bar would only repeat the song, so it is left out;
// notices that would have appeared in it take the key hint's place.
func (m Model) renderNowPlayingScreen() tea.View {
	now := time.Now()
	st := *m.styles
	st.Background = m.nowPlayingTint(now)
	np := m.decorateNowPlaying(*m.overlay.nowPlaying, st, now)
	body := np.Render(m.termWidth, m.termHeight, st)
	v := tea.NewView(st.RenderScreenFaded(body, m.termWidth, m.termHeight, m.overlayReveal(now), 0, m.termHeight))
	v.AltScreen = true
	v = m.finishView(v)
	if v.BackgroundColor != nil {
		v.BackgroundColor = st.Background
	}
	return v
}

// decorateNowPlaying fills the Now Playing view with live state: position,
// cover, loudness, listening details, modes, what plays next, and Auto-DJ's
// reason.
func (m Model) decorateNowPlaying(now overlay.NowPlayingOverlay, st styles.Styles, clock time.Time) overlay.NowPlayingOverlay {
	now = m.decorateNowPlayingText(now, st, clock)
	now.Loudness = m.nowPlayingLoudness(st)
	if t, ok := m.playingTrack(); ok {
		if cols, rows := m.nowPlayingArtSize(); cols > 0 {
			now.Cover, now.CoverCols, now.CoverRows = m.nowPlayingCover(t, st, cols, rows, clock), cols, rows
		}
	}
	return now
}

// decorateNowPlayingText fills in everything but the cover and the loudness
// strip, which is all the view's layout needs to decide what fits.
func (m Model) decorateNowPlayingText(now overlay.NowPlayingOverlay, st styles.Styles, clock time.Time) overlay.NowPlayingOverlay {
	now.State, now.Elapsed, now.Total = m.playback.state, m.displayElapsed(clock), m.playback.totalSecs
	elapsed, total := now.Elapsed, now.Total
	now.Progress = func(width int) string { return statusbar.ProgressBar(elapsed, total, width, st) }
	if t, ok := m.playingTrack(); ok {
		now.Details = trackDetailsLine(t, clock)
	}
	now.Modes = m.nowPlayingModes()
	var ahead []queue.Entry
	ahead, now.Reason = m.nowPlayingAhead()
	now.UpNext = nil
	for _, e := range ahead {
		now.UpNext = append(now.UpNext, upNextSong(e))
	}
	now.Marquee = m.motion.enabled && m.playback.state == playback.StatePlaying && !m.motion.titleSince.IsZero()
	if now.Marquee {
		now.TitleScroll = int(clock.Sub(m.motion.titleSince) / motionSteadyFrame)
	}
	now.Notice, now.NoticeError = m.nowPlayingNotice()
	now.Lyrics = m.nowPlayingLyrics(clock)
	return now
}

// nowPlayingNotice is what the view shows in place of its key hint: a
// pending confirmation first, then the status bar's current notification.
func (m Model) nowPlayingNotice() (string, bool) {
	if m.confirmationBar != "" {
		return m.confirmationBar, false
	}
	top := m.statusBar.TopNotification()
	return top.Msg, top.Severity == statusbar.SeverityError
}

// nowPlayingAhead returns the songs that play after the current one, at most
// nowPlayingUpNext of them, and Auto-DJ's reason for the current song.
func (m Model) nowPlayingAhead() ([]queue.Entry, string) {
	ahead := m.djAheadEntries()
	reason := ""
	if len(ahead) > 0 && ahead[0].Hash == m.playback.currentHash {
		if reasons := ahead[0].DJReasons; ahead[0].DJOwned && len(reasons) > 0 {
			reason = reasons.Summary()
		}
		ahead = ahead[1:]
	}
	ahead = ahead[:min(len(ahead), nowPlayingUpNext)]
	return append(ahead, m.repeatWrapEntries(nowPlayingUpNext-len(ahead))...), reason
}

// repeatWrapEntries returns up to n songs from the start of the queue that
// play once the end is reached under Repeat all. Only in order: under shuffle
// the next pass is reshuffled when it starts, so nobody knows its order yet
// and listing the queue's first rows would be a guess.
func (m Model) repeatWrapEntries(n int) []queue.Entry {
	if n <= 0 || m.playback.repeatMode != playback.RepeatAll || m.playback.shuffle || m.playback.currentIndex < 0 {
		return nil
	}
	before := indexRange(0, m.playback.currentIndex)
	if t, ok := m.playerTraversal(); ok {
		if t.CurrentPosition < 0 {
			return nil
		}
		before = t.Order[:min(t.CurrentPosition, len(t.Order))]
	}
	entries := m.queueEntriesAt(before)
	return entries[:min(len(entries), n)]
}

// upNextSong names a queue row title first, falling back to its display
// label or file name when it has no title tag.
func upNextSong(e queue.Entry) overlay.UpNextSong {
	if e.Title != "" {
		return overlay.UpNextSong{Title: e.Title, Artist: e.Artist}
	}
	return overlay.UpNextSong{Title: queueEntryLabel(e)}
}

// nowPlayingCover draws the playing song's cover. Right after a track change
// in blocks mode it cross-fades from the previous cover, holding the old one
// until the new one has loaded. Kitty covers swap at once instead: a blend
// would mean uploading a new image every frame, so the terminal shows the new
// image as soon as its single upload lands.
func (m Model) nowPlayingCover(t library.Track, st styles.Styles, cols, rows int, now time.Time) string {
	art, done := m.artFor(t)
	if from := m.motion.coverFrom; from != nil && m.motion.enabled {
		switch {
		case !done:
			return artwork.Blocks(from.Image, cols, rows)
		case art != nil && m.coverFading(now):
			progress := easeInOutCubic(now.Sub(m.motion.coverFadeStart).Seconds() / motionAccentDuration.Seconds())
			return artwork.Blocks(artwork.Blend(from.Image, art.Image, cols, rows*2, progress), cols, rows)
		}
	}
	cover := m.renderArt(t, cols, rows)
	if cover == "" && m.art != nil {
		cover = artPlaceholder(st, cols, rows)
	}
	return cover
}

func (m Model) coverFading(now time.Time) bool {
	return m.motion.coverFrom != nil && now.Sub(m.motion.coverFadeStart) < motionAccentDuration
}

// noteNowPlayingMotion restarts the title marquee whenever the title starts
// over (a new song, the view opening, playback resuming) and starts the cover
// cross-fade when the song changes while the view is open.
func (m Model) noteNowPlayingMotion(prev Model) Model {
	shown := m.nowPlayingShown()
	if !shown {
		m.motion.titleSince = time.Time{}
		return m
	}
	trackChanged := m.playback.currentHash != prev.playback.currentHash
	if trackChanged || !prev.nowPlayingShown() || m.playback.state != prev.playback.state || m.motion.titleSince.IsZero() {
		m.motion.titleSince = time.Now()
	}
	if trackChanged && prev.nowPlayingShown() {
		m = m.startCoverFade(prev)
	}
	return m
}

// startCoverFade remembers the cover prev showed so the new one can fade in
// over it. Only blocks are blended; see nowPlayingCover.
func (m Model) startCoverFade(prev Model) Model {
	if m.art == nil || m.art.mode != artBlocks {
		return m
	}
	old, ok := prev.playingTrack()
	if !ok {
		return m
	}
	oldArt, _ := prev.artFor(old)
	if oldArt == nil {
		return m
	}
	if t, ok := m.playingTrack(); ok {
		if art, done := m.artFor(t); done && art == oldArt {
			return m // same album: nothing to fade
		}
	}
	m.motion.coverFrom = oldArt
	m.motion.coverFadeStart = time.Now()
	return m
}

// nowPlayingTintAmount is how far the Now Playing background leans toward
// the cover's accent at most; nowPlayingTintStep is how much less it tries
// when that would cost text contrast.
const (
	nowPlayingTintAmount = 0.16
	nowPlayingTintStep   = 0.02
)

// nowPlayingTint is the Now Playing background: the canvas tinted toward the
// cover's accent, as strongly as the theme's text stays comfortably readable
// on it. Without a cover accent (or with Cover tint off) it is the canvas.
func (m Model) nowPlayingTint(now time.Time) color.Color {
	s := *m.styles
	weight := m.coverTintWeight(now)
	if weight <= 0 {
		return s.Background
	}
	for amount := nowPlayingTintAmount; amount > nowPlayingTintStep/2; amount -= nowPlayingTintStep {
		tint := styles.Mix(s.Background, s.Primary, amount*weight)
		if tintReadable(s, tint) {
			return tint
		}
	}
	return s.Background
}

// tintReadable reports whether the text the Now Playing view draws keeps
// enough contrast on bg.
func tintReadable(s styles.Styles, bg color.Color) bool {
	return styles.Contrast(s.TextPrimary.GetForeground(), bg) >= 7 &&
		styles.Contrast(s.Dim.GetForeground(), bg) >= 4.5 &&
		styles.Contrast(s.AccentMark.GetForeground(), bg) >= 4.5
}

// coverTintWeight is how much of the cover tint applies, from 0 to 1. It
// follows the accent transition, so the tint eases in and out with it.
func (m Model) coverTintWeight(now time.Time) float64 {
	to := 0.0
	if m.motion.accentTarget != "" {
		to = 1
	}
	if !m.accentAnimating(now) {
		return to
	}
	from := 0.0
	if m.motion.accentFromArt {
		from = 1
	}
	t := easeInOutCubic(now.Sub(m.motion.accentStart).Seconds() / motionAccentDuration.Seconds())
	return from + (to-from)*t
}

// nowPlayingModes names the active playback modes, or "" when none are on.
func (m Model) nowPlayingModes() string {
	// When playback is about to stop comes first: it is the one thing on the
	// line that changes what happens next, and a narrow line truncates the
	// tail. The sleep timer's own marker says it when the timer armed the stop.
	var labels []string
	sleep := m.sleepTimerView()
	if m.playback.stopAfterCurrent && sleep.mode != sleepEndOfSong {
		labels = append(labels, "Stops after this song")
	}
	if label := sleep.label(m.sleepNow()); label != "" {
		labels = append(labels, label)
	}
	for i, indicator := range playback.ModeIndicators(m.playback.repeatMode, m.playback.shuffle, m.playback.stopAfterCurrent, m.radio.enabled) {
		if indicator.Active && i != playback.ModeIndexStopAfter {
			labels = append(labels, indicator.Label())
		}
	}
	if len(m.queue.Entries) > 0 {
		labels = append(labels, text.Pluralize(len(m.queue.Entries), "1 song queued", text.FormatCount(len(m.queue.Entries))+" songs queued"))
	}
	return strings.Join(labels, " · ")
}
