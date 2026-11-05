package tui

import (
	"context"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/artwork"
	"github.com/sglavoie/muse/internal/config"
	"github.com/sglavoie/muse/internal/library"
)

// artMode is how cover art is drawn, resolved once from the artwork setting
// and the terminal.
type artMode int

const (
	artOff artMode = iota
	artBlocks
	artKitty
)

// artFocusDelay is how long focus must rest on a track before its art is
// looked up, so scrolling through a list never starts one lookup per row.
const artFocusDelay = 120 * time.Millisecond

// artMemoryEntries bounds the decoded covers kept in memory.
const artMemoryEntries = 48

// artworkState is shared by every copy of the Model: covers load in the
// background and the cache and Kitty placements describe the one terminal.
type artworkState struct {
	loader artwork.Loader
	mode   artMode

	cache   map[string]*artwork.Art // key → art; nil value means "no art"
	order   []string                // insertion order, for eviction
	pending map[string]bool

	focusKey   string        // key focus last settled on
	focusGen   int           // bumps whenever focus moves to another album
	focusDelay time.Duration // how long focus rests before a lookup starts
	instant    bool          // the loader answers at once; look up synchronously

	kitty *kittyPlacements

	// prefetch looks up covers the screen is likely to want next; see
	// root_artwork_prefetch.go.
	prefetch artPrefetchState
}

type kittyPlacements struct {
	ids    map[kittySlot]int // placed slot → image id
	byID   map[int]kittySlot
	nextID int
}

type kittySlot struct {
	key        string
	cols, rows int
}

// artLoadedMsg delivers a finished lookup. art is nil when the album has none.
type artLoadedMsg struct {
	key string
	art *artwork.Art
}

// artFocusSettledMsg fires after artFocusDelay; gen identifies the focus it
// was scheduled for.
type artFocusSettledMsg struct{ gen int }

// WithArtwork enables cover art drawn in mode ("auto", "kitty", "blocks" or
// "off") using loader. A nil loader or "off" disables art.
func WithArtwork(loader artwork.Loader, mode string) Option {
	return func(m *Model) {
		resolved := resolveArtMode(mode, os.Getenv)
		if loader == nil || resolved == artOff {
			m.art = nil
			return
		}
		delay, instant := artFocusDelay, false
		if i, ok := loader.(interface{ Instant() bool }); ok && i.Instant() {
			delay, instant = 0, true
		}
		m.art = &artworkState{
			instant:    instant,
			focusDelay: delay,
			loader:     loader,
			mode:       resolved,
			cache:      map[string]*artwork.Art{},
			pending:    map[string]bool{},
			kitty:      &kittyPlacements{ids: map[kittySlot]int{}, byID: map[int]kittySlot{}},
		}
	}
}

// resolveArtMode maps the setting to a drawing mode. "auto" uses Kitty
// graphics only in terminals known to implement Unicode placeholders, and
// never through a multiplexer, which would not pass them on.
func resolveArtMode(mode string, getenv func(string) string) artMode {
	switch mode {
	case config.ArtworkOff:
		return artOff
	case config.ArtworkBlocks:
		return artBlocks
	case config.ArtworkKitty:
		return artKitty
	}
	if terminalShowsKittyGraphics(getenv) {
		return artKitty
	}
	return artBlocks
}

// terminalShowsKittyGraphics reports whether the terminal is one known to
// draw Kitty Unicode placeholders, reached directly rather than through a
// multiplexer.
func terminalShowsKittyGraphics(getenv func(string) string) bool {
	if getenv("TMUX") != "" || getenv("STY") != "" || getenv("ZELLIJ") != "" {
		return false
	}
	term := strings.ToLower(getenv("TERM"))
	return getenv("KITTY_WINDOW_ID") != "" || strings.EqualFold(getenv("TERM_PROGRAM"), "ghostty") ||
		strings.Contains(term, "kitty") || strings.Contains(term, "ghostty")
}

// artRequestFor returns the lookup request and cache key for a track.
func artRequestFor(t library.Track) (artwork.Request, string) {
	req := artwork.Request{Path: t.Path, Album: t.Album, AlbumArtist: t.AlbumArtist, Artist: t.Artist}
	return req, req.Key()
}

// artFor returns the cached art for track, and whether a lookup has finished.
func (m Model) artFor(t library.Track) (*artwork.Art, bool) {
	if m.art == nil || t.Path == "" {
		return nil, false
	}
	_, key := artRequestFor(t)
	art, done := m.art.cache[key]
	return art, done
}

// playingTrack returns the library track that is playing, if any.
func (m Model) playingTrack() (library.Track, bool) {
	if m.playback.currentHash == "" {
		return library.Track{}, false
	}
	return m.library.Index.FindByHash(m.playback.currentHash)
}

// syncArtwork starts the lookups the current screen needs: the playing
// track's art at once, the focused track's after focus settles.
func (m Model) syncArtwork() (Model, tea.Cmd) {
	if m.art == nil {
		return m, nil
	}
	var cmds []tea.Cmd
	if t, ok := m.playingTrack(); ok {
		var cmd tea.Cmd
		m, cmd = m.requestArt(t)
		cmds = append(cmds, cmd)
	}
	var cmd tea.Cmd
	m, cmd = m.syncFocusedArt()
	cmds = append(cmds, cmd, m.scheduleArtPrefetch())
	return m, batchNonNil(cmds...)
}

// syncFocusedArt notices focus moving to another album and looks its cover
// up once focus has rested there for focusDelay.
func (m Model) syncFocusedArt() (Model, tea.Cmd) {
	t, ok := m.focusedTrackForInspection()
	if !ok || t.Path == "" {
		return m, nil
	}
	_, key := artRequestFor(t)
	if key == m.art.focusKey {
		return m, nil
	}
	m.art.focusKey = key
	m.art.focusGen++
	if _, done := m.art.cache[key]; done {
		m.art.unmarkPrefetched(key)
		return m, nil
	}
	if m.art.focusDelay <= 0 {
		return m.requestArt(t)
	}
	gen := m.art.focusGen
	return m, tea.Tick(m.art.focusDelay, func(time.Time) tea.Msg {
		return artFocusSettledMsg{gen: gen}
	})
}

// requestArt starts a lookup for t's art unless it is cached or already on
// its way. Instant loaders (generated covers) answer synchronously, so they
// never add a command to the update.
func (m Model) requestArt(t library.Track) (Model, tea.Cmd) {
	if m.art == nil || t.Path == "" {
		return m, nil
	}
	req, key := artRequestFor(t)
	// A prefetch already loading this cover answers for this lookup too.
	prefetching := m.art.notePrefetchUsed(key)
	if _, done := m.art.cache[key]; done || m.art.pending[key] || prefetching {
		return m, nil
	}
	loader := m.art.loader
	ctx := m.appCtx
	if ctx == nil {
		ctx = context.Background()
	}
	load := func() artLoadedMsg {
		art, err := loader.Load(ctx, req)
		if err != nil {
			art = nil
		}
		return artLoadedMsg{key: key, art: art}
	}
	if m.art.instant {
		return m.handleArtLoaded(load())
	}
	m.art.pending[key] = true
	return m, func() tea.Msg { return load() }
}

func (m Model) handleArtFocusSettled(msg artFocusSettledMsg) (Model, tea.Cmd) {
	if m.art == nil || msg.gen != m.art.focusGen {
		return m, nil
	}
	t, ok := m.focusedTrackForInspection()
	if !ok {
		return m, nil
	}
	return m.requestArt(t)
}

func (m Model) handleArtLoaded(msg artLoadedMsg) (Model, tea.Cmd) {
	if m.art == nil {
		return m, nil
	}
	delete(m.art.pending, msg.key)
	if _, exists := m.art.cache[msg.key]; !exists {
		m.art.order = append(m.art.order, msg.key)
	}
	m.art.cache[msg.key] = msg.art
	m.evictArt()
	// The cross-fade to a new cover starts once that cover has arrived.
	if m.motion.coverFrom != nil {
		if t, ok := m.playingTrack(); ok {
			if _, key := artRequestFor(t); key == msg.key {
				m.motion.coverFadeStart = time.Now()
			}
		}
	}
	return m.refreshArtworkAccent(), nil
}

// playingArtPending reports whether the playing track's cover is still being
// looked up.
func (m Model) playingArtPending() bool {
	t, ok := m.playingTrack()
	if !ok || m.art == nil {
		return false
	}
	_, done := m.artFor(t)
	return !done
}

// renderArt draws t's cover into cols×rows cells, or returns "" when there is
// none to draw. Kitty placements are only used once placed by placeArtwork;
// until then the cover is drawn with blocks so it never flickers in empty.
func (m Model) renderArt(t library.Track, cols, rows int) string {
	art, _ := m.artFor(t)
	if art == nil || cols <= 0 || rows <= 0 {
		return ""
	}
	if m.art.mode == artKitty {
		_, key := artRequestFor(t)
		if id, ok := m.art.kitty.ids[kittySlot{key: key, cols: cols, rows: rows}]; ok {
			return artwork.KittyPlaceholders(id, cols, rows)
		}
	}
	return artwork.Blocks(art.Image, cols, rows)
}

// placeArtwork uploads every cover slot the next frame draws that the
// terminal does not hold yet. It runs after each update, so a View always
// finds its placements ready.
func (m Model) placeArtwork() tea.Cmd {
	if m.art == nil || m.art.mode != artKitty {
		return nil
	}
	var seqs []string
	for _, slot := range m.artSlots() {
		art := m.art.cache[slot.key]
		if art == nil {
			continue
		}
		k := m.art.kitty
		if _, ok := k.ids[slot]; ok {
			continue
		}
		id := k.allocate(slot)
		// Upload only the resolution the slot can show: a thumbnail needs a
		// fraction of the full cover's bytes.
		side := min(artwork.Size, max(slot.cols*12, slot.rows*24))
		seq, err := artwork.KittyTransmit(id, artwork.Resize(art.Image, side, side), slot.cols, slot.rows)
		if err != nil {
			delete(k.ids, slot)
			delete(k.byID, id)
			continue
		}
		seqs = append(seqs, seq)
	}
	if len(seqs) == 0 {
		return nil
	}
	return tea.Raw(strings.Join(seqs, ""))
}

// allocate assigns slot the next image id, recycling the oldest when every id
// is in use. Ids start above the 16 basic colors so no terminal remaps them.
func (k *kittyPlacements) allocate(slot kittySlot) int {
	const first = 16
	if k.nextID < first || k.nextID > artwork.MaxKittyID {
		k.nextID = first
	}
	id := k.nextID
	k.nextID++
	if old, ok := k.byID[id]; ok {
		delete(k.ids, old)
	}
	k.ids[slot] = id
	k.byID[id] = slot
	return id
}

// artSlots lists the cover placements the current screen draws.
func (m Model) artSlots() []kittySlot {
	var slots []kittySlot
	add := func(t library.Track, cols, rows int) {
		if cols <= 0 || rows <= 0 {
			return
		}
		_, key := artRequestFor(t)
		slots = append(slots, kittySlot{key: key, cols: cols, rows: rows})
	}
	if m.overlay.nowPlaying != nil {
		if t, ok := m.playingTrack(); ok {
			cols, rows := m.nowPlayingArtSize()
			add(t, cols, rows)
		}
		return slots
	}
	// The albums grid fills the panel, so the focused-song card is hidden
	// beneath it and must not upload a cover of its own.
	if t, ok := m.focusedCardTrack(); ok && !m.albumGridActive() {
		cols, rows := m.focusedCardArtSize()
		add(t, cols, rows)
	}
	if t, ok := m.trackCardTrack(time.Now()); ok {
		add(t, trackCardArtCols, trackCardArtRows)
	}
	return slots
}

// ArtworkCleanup returns the escape sequence that frees every cover uploaded
// to the terminal, for the caller to write once Bubble Tea has exited; it is
// empty when nothing was uploaded.
func (m Model) ArtworkCleanup() string {
	if m.art == nil || m.art.mode != artKitty || len(m.art.kitty.byID) == 0 {
		return ""
	}
	ids := make([]int, 0, len(m.art.kitty.byID))
	for id := range m.art.kitty.byID {
		ids = append(ids, id)
	}
	return artwork.KittyDeleteAll(ids)
}
