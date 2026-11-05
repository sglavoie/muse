package tui

import (
	"context"
	"slices"
	"strconv"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/artwork"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/queue"
)

// Cover prefetching looks up, at low priority, the covers a short scroll or
// the next track change will want: the albums just above and below the
// focused row, and the next songs in the queue. It waits for the screen to
// rest, runs only a couple of lookups at once, and keeps few covers that have
// not been shown, so it never crowds out the ones on screen.
const (
	// artPrefetchDelay is how long focus and playback must rest before a
	// prefetch starts; longer than artFocusDelay so the focused cover
	// always goes first.
	artPrefetchDelay = 350 * time.Millisecond
	// artPrefetchInflight caps the prefetch lookups running at once.
	artPrefetchInflight = 2
	// artPrefetchMemory bounds the prefetched covers kept in memory that
	// nothing has shown yet; older ones make room for newer ones.
	artPrefetchMemory = 8
	// artPrefetchAlbums is how many distinct albums are prefetched on each
	// side of the cursor, found within artPrefetchRows rows.
	artPrefetchAlbums = 3
	artPrefetchRows   = 40
	// artPrefetchQueued is how many upcoming songs get their cover ready.
	artPrefetchQueued = 2
)

// artPrefetchState is the current prefetch plan, shared through
// artworkState like the rest of the cover state.
type artPrefetchState struct {
	sig string // what the plan was made for; a change starts a new plan
	gen int    // bumps with every new plan

	want   []artPrefetchItem // lookups still to start, in priority order
	wanted map[string]bool   // every key of the current plan

	inflight map[string]bool // keys being prefetched
	// promoted marks in-flight prefetches the screen has since asked for:
	// their result is kept as an ordinary lookup.
	promoted map[string]bool

	// marked lists prefetched covers in the cache that nothing has shown yet,
	// oldest first.
	marked []string
}

type artPrefetchItem struct {
	req artwork.Request
	key string
}

// artPrefetchMsg starts the plan made for gen once the screen has rested.
type artPrefetchMsg struct{ gen int }

// artPrefetchedMsg delivers a finished prefetch lookup.
type artPrefetchedMsg struct {
	key string
	art *artwork.Art
	err error
}

// prefetchEnabled reports whether prefetching is worth doing: generated
// covers cost nothing to make, so fixtures and demos never prefetch.
func (m Model) prefetchEnabled() bool {
	return m.art != nil && !m.art.instant
}

// artPrefetchSig identifies what a prefetch plan depends on.
func (m Model) artPrefetchSig() string {
	return m.art.focusKey + "|" + m.playback.currentHash + "|" +
		strconv.Itoa(len(m.queue.Entries)) + "|" + strconv.Itoa(int(m.ActiveView))
}

// scheduleArtPrefetch starts a new plan when focus, the playing song or the
// queue changed; the plan runs once they have rested for artPrefetchDelay.
func (m Model) scheduleArtPrefetch() tea.Cmd {
	if !m.prefetchEnabled() {
		return nil
	}
	p := &m.art.prefetch
	sig := m.artPrefetchSig()
	if sig == p.sig {
		return nil
	}
	p.sig = sig
	p.gen++
	p.want, p.wanted = nil, nil
	gen := p.gen
	return tea.Tick(artPrefetchDelay, func(time.Time) tea.Msg { return artPrefetchMsg{gen: gen} })
}

func (m Model) handleArtPrefetch(msg artPrefetchMsg) (Model, tea.Cmd) {
	if !m.prefetchEnabled() || msg.gen != m.art.prefetch.gen {
		return m, nil
	}
	plan := m.artPrefetchPlan()
	p := &m.art.prefetch
	p.want = plan
	p.wanted = make(map[string]bool, len(plan))
	for _, it := range plan {
		p.wanted[it.key] = true
	}
	return m, m.startArtPrefetches()
}

// artPrefetchPlan lists the covers to prefetch, most useful first: the next
// queued songs, then the albums nearest the cursor, alternating below and
// above. Covers already cached, on screen or playing are left out.
func (m Model) artPrefetchPlan() []artPrefetchItem {
	seen := map[string]bool{m.art.focusKey: true}
	if t, ok := m.playingTrack(); ok {
		_, key := artRequestFor(t)
		seen[key] = true
	}
	pick := func(tracks []library.Track, limit int) []artPrefetchItem {
		var out []artPrefetchItem
		albums := 0
		for _, t := range tracks {
			if albums >= limit {
				break
			}
			req, key := artRequestFor(t)
			if t.Path == "" || seen[key] {
				continue
			}
			seen[key] = true
			albums++
			if _, done := m.art.cache[key]; !done {
				out = append(out, artPrefetchItem{req: req, key: key})
			}
		}
		return out
	}
	plan := pick(m.upcomingArtTracks(), artPrefetchQueued)
	above, below := m.focusedNeighbors(artPrefetchRows)
	down, up := pick(below, artPrefetchAlbums), pick(above, artPrefetchAlbums)
	for i := range max(len(down), len(up)) {
		if i < len(down) {
			plan = append(plan, down[i])
		}
		if i < len(up) {
			plan = append(plan, up[i])
		}
	}
	return plan
}

// upcomingArtTracks are the songs that play next, as Now Playing lists them.
func (m Model) upcomingArtTracks() []library.Track {
	ahead, _ := m.nowPlayingAhead()
	tracks := make([]library.Track, 0, len(ahead))
	for _, e := range ahead {
		if !e.Missing {
			tracks = append(tracks, queueEntryArtTrack(e))
		}
	}
	return tracks
}

func queueEntryArtTrack(e queue.Entry) library.Track {
	return library.Track{Hash: e.Hash, Path: e.Path, Album: e.Album, AlbumArtist: e.AlbumArtist, Artist: e.Artist}
}

// focusedNeighbors returns the rows around the focused one, nearest first,
// in the views whose rows are cheap to reach: Library and Queue. Behind an
// overlay, or under the album grid with its own tile loading, there are none.
func (m Model) focusedNeighbors(radius int) (above, below []library.Track) {
	if m.hasActiveOverlay() || m.albumGridActive() {
		return nil, nil
	}
	switch m.ActiveView {
	case viewLibrary:
		return m.library.Panel().TracksAround(radius)
	case viewQueue:
		return queueNeighbors(m.queue.FilteredEntries(), m.queue.Cursor, radius)
	}
	return nil, nil
}

func queueNeighbors(entries []queue.Entry, cur, radius int) (above, below []library.Track) {
	if cur < 0 || cur >= len(entries) {
		return nil, nil
	}
	for d := 1; d <= radius; d++ {
		if i := cur - d; i >= 0 && !entries[i].Missing {
			above = append(above, queueEntryArtTrack(entries[i]))
		}
		if i := cur + d; i < len(entries) && !entries[i].Missing {
			below = append(below, queueEntryArtTrack(entries[i]))
		}
	}
	return above, below
}

// startArtPrefetches starts planned lookups up to artPrefetchInflight,
// skipping covers that arrived or started loading since the plan was made.
func (m Model) startArtPrefetches() tea.Cmd {
	p := &m.art.prefetch
	if p.inflight == nil {
		p.inflight = map[string]bool{}
	}
	ctx := m.appCtx
	if ctx == nil {
		ctx = context.Background()
	}
	loader := m.art.loader
	var cmds []tea.Cmd
	for len(p.inflight) < artPrefetchInflight && len(p.want) > 0 {
		it := p.want[0]
		p.want = p.want[1:]
		if _, done := m.art.cache[it.key]; done || m.art.pending[it.key] || p.inflight[it.key] {
			continue
		}
		p.inflight[it.key] = true
		cmds = append(cmds, func() tea.Msg {
			art, err := loader.Load(ctx, it.req)
			return artPrefetchedMsg{key: it.key, art: art, err: err}
		})
	}
	return batchNonNil(cmds...)
}

// handleArtPrefetched files a finished prefetch: as an ordinary lookup when
// the screen asked for it meanwhile, as a prefetched cover when the current
// plan still wants it, and otherwise not at all (the disk cache still has
// it). Then the next planned lookup starts.
func (m Model) handleArtPrefetched(msg artPrefetchedMsg) (Model, tea.Cmd) {
	if m.art == nil {
		return m, nil
	}
	p := &m.art.prefetch
	delete(p.inflight, msg.key)
	var cmd tea.Cmd
	switch {
	case p.promoted[msg.key]:
		delete(p.promoted, msg.key)
		art := msg.art
		if msg.err != nil {
			art = nil
		}
		m, cmd = m.handleArtLoaded(artLoadedMsg{key: msg.key, art: art})
	case msg.err == nil && p.wanted[msg.key]:
		m.storePrefetchedArt(msg.key, msg.art)
	}
	return m, batchNonNil(cmd, m.startArtPrefetches())
}

// storePrefetchedArt caches a prefetched cover, first dropping the oldest
// prefetched cover nothing has shown when there are artPrefetchMemory.
func (m Model) storePrefetchedArt(key string, art *artwork.Art) {
	a := m.art
	if _, done := a.cache[key]; done {
		return
	}
	if len(a.prefetch.marked) >= artPrefetchMemory {
		a.dropArt(a.prefetch.marked[0])
	}
	a.cache[key] = art
	a.order = append(a.order, key)
	a.prefetch.marked = append(a.prefetch.marked, key)
	m.evictArt()
}

// notePrefetchUsed records that the screen asked for key: a prefetched cover
// becomes an ordinary one, and a prefetch still loading it is promoted. It
// reports whether such a prefetch is on its way.
func (a *artworkState) notePrefetchUsed(key string) (inflight bool) {
	p := &a.prefetch
	a.unmarkPrefetched(key)
	if !p.inflight[key] {
		return false
	}
	if p.promoted == nil {
		p.promoted = map[string]bool{}
	}
	p.promoted[key] = true
	return true
}

func (a *artworkState) unmarkPrefetched(key string) {
	a.prefetch.marked = slices.DeleteFunc(a.prefetch.marked, func(k string) bool { return k == key })
}

// dropArt removes key from the in-memory cache.
func (a *artworkState) dropArt(key string) {
	delete(a.cache, key)
	a.unmarkPrefetched(key)
	if i := slices.Index(a.order, key); i >= 0 {
		a.order = slices.Delete(a.order, i, i+1)
	}
}

// evictArt trims the cache to artMemoryEntries, oldest first, keeping the
// playing and focused covers however old they are.
func (m Model) evictArt() {
	a := m.art
	keep := map[string]bool{a.focusKey: true}
	if t, ok := m.playingTrack(); ok {
		_, key := artRequestFor(t)
		keep[key] = true
	}
	for i := 0; len(a.order) > artMemoryEntries && i < len(a.order); {
		if keep[a.order[i]] {
			i++
			continue
		}
		a.dropArt(a.order[i])
	}
}
