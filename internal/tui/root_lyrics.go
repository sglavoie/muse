package tui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/config"
	"github.com/sglavoie/muse/internal/lyrics"
	"github.com/sglavoie/muse/internal/tui/overlay"
)

// lyricsMemoryEntries bounds the songs whose lyrics are kept in memory.
const lyricsMemoryEntries = 64

// lyricsLoadTimeout bounds one lookup, so a stuck tag reader never leaves
// the song marked as loading for good.
const lyricsLoadTimeout = 8 * time.Second

// Lyrics motion: the highlight eases onto a new line over
// motionLyricsDuration, and the window glides after it one line per frame
// when it is at most lyricsGlideMax lines behind. A bigger jump, such as a
// seek, moves the window at once.
const (
	motionLyricsDuration = 300 * time.Millisecond
	lyricsGlideMax       = 3
)

// lyricsScrollTail is how many lines unsynced lyrics keep on screen when
// scrolled to the end.
const lyricsScrollTail = 3

// lyricsState is shared by every copy of the Model, like artworkState:
// lookups finish in the background and fill one cache.
type lyricsState struct {
	loader  lyrics.Loader
	instant bool                      // the loader answers at once; look up synchronously
	cache   map[string]*lyrics.Lyrics // audio path → lyrics; nil value means "none"
	order   []string                  // insertion order, for eviction
	pending map[string]bool
}

// lyricsViewState is where the Now Playing lyrics window is: the line it
// follows, the line last seen playing and when that changed, and how far
// unsynced lyrics are scrolled. It belongs to the song at path.
type lyricsViewState struct {
	path      string
	shown     int
	line      int
	changedAt time.Time
	scroll    int
}

// lyricsLoadedMsg delivers a finished lookup; lyr is nil when the song has
// no lyrics.
type lyricsLoadedMsg struct {
	path string
	lyr  *lyrics.Lyrics
}

// WithLyrics enables lyrics in the Now Playing view, looked up with loader.
// A nil loader disables them.
func WithLyrics(loader lyrics.Loader) Option {
	return func(m *Model) {
		if loader == nil {
			m.lyrics = nil
			return
		}
		instant := false
		if i, ok := loader.(interface{ Instant() bool }); ok {
			instant = i.Instant()
		}
		m.lyrics = &lyricsState{
			loader:  loader,
			instant: instant,
			cache:   map[string]*lyrics.Lyrics{},
			pending: map[string]bool{},
		}
	}
}

// syncLyrics looks up the playing song's lyrics while the Now Playing view
// is open with lyrics on. Nothing is read for songs nobody looks at.
func (m Model) syncLyrics() (Model, tea.Cmd) {
	path, ok := m.lyricsWanted()
	if !ok {
		return m, nil
	}
	loader := m.lyrics.loader
	ctx := m.appCtx
	if ctx == nil {
		ctx = context.Background()
	}
	load := func() lyricsLoadedMsg {
		ctx, cancel := context.WithTimeout(ctx, lyricsLoadTimeout)
		defer cancel()
		lyr, err := loader.Load(ctx, path)
		if err != nil || lyr.Empty() {
			lyr = nil
		}
		return lyricsLoadedMsg{path: path, lyr: lyr}
	}
	if m.lyrics.instant {
		return m.handleLyricsLoaded(load()), nil
	}
	m.lyrics.pending[path] = true
	return m, func() tea.Msg { return load() }
}

// lyricsWanted returns the playing song's path when its lyrics should be
// looked up now: lyrics are on, Now Playing shows, and the song has not been
// looked up yet.
func (m Model) lyricsWanted() (string, bool) {
	if m.lyrics == nil || !m.cfg.Lyrics || !m.nowPlayingShown() {
		return "", false
	}
	t, ok := m.playingTrack()
	if !ok || t.Path == "" {
		return "", false
	}
	_, done := m.lyrics.cache[t.Path]
	return t.Path, !done && !m.lyrics.pending[t.Path]
}

func (m Model) handleLyricsLoaded(msg lyricsLoadedMsg) Model {
	if m.lyrics == nil {
		return m
	}
	delete(m.lyrics.pending, msg.path)
	if _, exists := m.lyrics.cache[msg.path]; !exists {
		m.lyrics.order = append(m.lyrics.order, msg.path)
	}
	m.lyrics.cache[msg.path] = msg.lyr
	for len(m.lyrics.order) > lyricsMemoryEntries {
		delete(m.lyrics.cache, m.lyrics.order[0])
		m.lyrics.order = m.lyrics.order[1:]
	}
	return m
}

// playingLyrics returns the playing song's lyrics and its path, or nil
// when it has none or they have not loaded yet.
func (m Model) playingLyrics() (*lyrics.Lyrics, string) {
	if m.lyrics == nil {
		return nil, ""
	}
	t, ok := m.playingTrack()
	if !ok {
		return nil, ""
	}
	return m.lyrics.cache[t.Path], t.Path
}

// lyricsLine returns the synced line playing at now, or -1.
func (m Model) lyricsLine(lyr *lyrics.Lyrics, now time.Time) int {
	return lyr.Index(time.Duration(m.displayElapsed(now) * float64(time.Second)))
}

// nowPlayingLyrics describes the lyrics section for the Now Playing view,
// or nil when the song has none. With lyrics turned off it still reports
// them as hidden, so the key hint can offer to show them.
func (m Model) nowPlayingLyrics(now time.Time) *overlay.NowPlayingLyrics {
	lyr, path := m.playingLyrics()
	if lyr == nil {
		return nil
	}
	view := &overlay.NowPlayingLyrics{Synced: lyr.Synced, Current: -1, Fade: 1, Hidden: !m.cfg.Lyrics}
	view.Lines = make([]string, len(lyr.Lines))
	for i, line := range lyr.Lines {
		view.Lines[i] = line.Text
	}
	st := m.lyricsView
	tracked := st.path == path
	if !lyr.Synced {
		if tracked {
			view.Anchor = st.scroll
		}
		return view
	}
	view.Current = m.lyricsLine(lyr, now)
	view.Anchor = view.Current
	// The frame loop tracks the window only with motion on and once it has
	// seen this song; until then the window sits on the current line.
	if !m.motion.enabled || !tracked {
		return view
	}
	if abs(view.Current-st.shown) <= lyricsGlideMax {
		view.Anchor = st.shown
	}
	switch {
	case st.line != view.Current:
		// A new line the frame loop has not seen yet starts its fade now.
		view.Fade = 0
	case !st.changedAt.IsZero():
		view.Fade = easeOutCubic(now.Sub(st.changedAt).Seconds() / motionLyricsDuration.Seconds())
	}
	return view
}

// stepLyrics advances the lyrics window by one motion frame: it notices a
// new line, starting its highlight fade, and moves the window one line
// closer to it. A big jump such as a seek moves the window at once and
// skips the fade.
func (m Model) stepLyrics(now time.Time) Model {
	lyr, path := m.playingLyrics()
	if lyr == nil || !lyr.Synced || !m.cfg.Lyrics || !m.nowPlayingShown() {
		return m
	}
	idx := m.lyricsLine(lyr, now)
	st := &m.lyricsView
	if st.path != path {
		*st = lyricsViewState{path: path, shown: idx, line: idx}
		return m
	}
	st.notice(idx, now)
	switch {
	case abs(idx-st.shown) > lyricsGlideMax:
		st.shown = idx
	case st.shown < idx:
		st.shown++
	case st.shown > idx:
		st.shown--
	}
	return m
}

// notice records that line idx is playing, starting its highlight fade
// when it is new; after a big jump the window moves at once and no fade
// plays.
func (st *lyricsViewState) notice(idx int, now time.Time) {
	if idx == st.line {
		return
	}
	st.line = idx
	st.changedAt = now
	if abs(idx-st.shown) > lyricsGlideMax {
		st.changedAt = time.Time{}
		st.shown = idx
	}
}

// lyricsMoving reports whether the lyrics window is gliding or a highlight
// is fading in, which asks for the smoother frame rate.
func (m Model) lyricsMoving(now time.Time) bool {
	st := m.lyricsView
	if st.path == "" || !m.cfg.Lyrics || !m.nowPlayingShown() {
		return false
	}
	if !st.changedAt.IsZero() && now.Sub(st.changedAt) < motionLyricsDuration {
		return true
	}
	lyr, path := m.playingLyrics()
	if lyr == nil || !lyr.Synced || path != st.path {
		return false
	}
	idx := m.lyricsLine(lyr, now)
	return idx != st.line || idx != st.shown
}

// toggleLyrics shows or hides lyrics in Now Playing and remembers the
// choice in the config.
func (m Model) toggleLyrics() (Model, tea.Cmd) {
	m.cfg.Lyrics = !m.cfg.Lyrics
	m.statusBar.Notify("Lyrics: " + onOff(m.cfg.Lyrics))
	if m.configPath == "" {
		return m, nil
	}
	path, cfg := m.configPath, m.cfg
	return m, func() tea.Msg { return settingsSavedMsg{err: config.Save(path, cfg)} }
}

// scrollLyrics moves unsynced lyrics by delta lines. Synced lyrics follow
// the song, so it does nothing for them.
func (m Model) scrollLyrics(delta int) Model {
	lyr, path := m.playingLyrics()
	if lyr == nil || lyr.Synced || !m.cfg.Lyrics {
		return m
	}
	if m.lyricsView.path != path {
		m.lyricsView = lyricsViewState{path: path, shown: -1, line: -1}
	}
	// Stop where the last few lines still fill the window.
	m.lyricsView.scroll = max(0, min(m.lyricsView.scroll+delta, len(lyr.Lines)-lyricsScrollTail))
	return m
}

// handleNowPlayingLyricsKey handles the lyrics keys of the Now Playing view.
func (m Model) handleNowPlayingLyricsKey(key string) (Model, tea.Cmd, bool) {
	switch key {
	case "L":
		next, cmd := m.toggleLyrics()
		return next, cmd, true
	case "up", "k":
		return m.scrollLyrics(-1), nil, true
	case "down", "j":
		return m.scrollLyrics(1), nil, true
	}
	return m, nil, false
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
