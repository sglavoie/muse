package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/artwork"
	"github.com/sglavoie/muse/internal/config"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/queue"
)

// newPrefetchModel builds a Library of one-track albums with an
// asynchronous loader, focused on row cursor.
func newPrefetchModel(t *testing.T, albums, cursor int) (Model, *countingLoader) {
	t.Helper()
	loader := &countingLoader{}
	m := New(context.Background(), WithLibraryTracks(manyAlbumTracks(albums)), WithArtwork(loader, config.ArtworkBlocks))
	m = m.handleWindowSizeMsg(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.library.Cursor = cursor
	m, _ = m.syncArtwork()
	return m, loader
}

func albumKey(t *testing.T, m Model, row int) string {
	t.Helper()
	_, key := artRequestFor(m.library.Panel().VisibleTracks()[row])
	return key
}

// runPrefetch makes the current plan and runs its lookups to the end, as
// the program loop would, returning the most started at once.
func runPrefetch(t *testing.T, m Model) (Model, int) {
	t.Helper()
	m, cmd := m.handleArtPrefetch(artPrefetchMsg{gen: m.art.prefetch.gen})
	peak := 0
	for range 50 {
		cmds := flattenCmds(cmd)
		if len(cmds) == 0 {
			break
		}
		peak = max(peak, len(m.art.prefetch.inflight))
		var next []tea.Cmd
		for _, c := range cmds {
			msg, ok := c().(artPrefetchedMsg)
			if !ok {
				continue
			}
			var more tea.Cmd
			m, more = m.handleArtPrefetched(msg)
			next = append(next, more)
		}
		cmd = batchNonNil(next...)
	}
	return m, peak
}

// flattenCmds runs cmd, expanding batches, and returns each message
// wrapped back into a command that replays it.
func flattenCmds(cmd tea.Cmd) []tea.Cmd {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Cmd
		for _, c := range batch {
			out = append(out, flattenCmds(c)...)
		}
		return out
	}
	return []tea.Cmd{func() tea.Msg { return msg }}
}

func TestArtPrefetchPlansNearestAlbumsBelowThenAbove(t *testing.T) {
	m, _ := newPrefetchModel(t, 100, 10)
	m, _ = m.handleArtPrefetch(artPrefetchMsg{gen: m.art.prefetch.gen})
	var got []string
	for k := range m.art.prefetch.wanted {
		got = append(got, k)
	}
	want := []int{11, 9, 12, 8, 13, 7}
	if len(got) != len(want) {
		t.Fatalf("planned %d covers, want %d", len(got), len(want))
	}
	for _, row := range want {
		if !m.art.prefetch.wanted[albumKey(t, m, row)] {
			t.Fatalf("row %d's album is not planned", row)
		}
	}
	if m.art.prefetch.wanted[albumKey(t, m, 10)] {
		t.Fatal("the focused album is prefetched; the focus lookup owns it")
	}
}

func TestArtPrefetchPutsUpcomingSongsFirst(t *testing.T) {
	m, _ := newPrefetchModel(t, 100, 10)
	tracks := m.library.Panel().VisibleTracks()
	for _, i := range []int{50, 60, 70, 80} {
		tr := tracks[i]
		m.queue.Entries = append(m.queue.Entries, queue.Entry{Hash: tr.Hash, Path: tr.Path, Album: tr.Album, Artist: tr.Artist})
	}
	m.playback.currentIndex = 0
	m.playback.currentHash = tracks[50].Hash
	m.playback.state = playback.StatePlaying
	plan := m.artPrefetchPlan()
	if len(plan) < 2 || plan[0].key != albumKey(t, m, 60) || plan[1].key != albumKey(t, m, 70) {
		t.Fatalf("the next two queued songs should lead the plan: %+v", plan)
	}
	for _, it := range plan {
		if it.key == albumKey(t, m, 50) || it.key == albumKey(t, m, 80) {
			t.Fatal("the playing song or a third upcoming song was planned")
		}
	}
}

func TestArtPrefetchBoundsConcurrencyAndDedupes(t *testing.T) {
	m, loader := newPrefetchModel(t, 100, 10)
	m, peak := runPrefetch(t, m)
	if peak > artPrefetchInflight {
		t.Fatalf("%d prefetches at once, cap %d", peak, artPrefetchInflight)
	}
	if got := loader.calls.Load(); got != 6 {
		t.Fatalf("loaded %d covers, want 6", got)
	}
	if len(m.art.prefetch.marked) != 6 {
		t.Fatalf("%d prefetched covers held, want 6", len(m.art.prefetch.marked))
	}
	// The same plan again finds everything cached and loads nothing.
	m, _ = runPrefetch(t, m)
	if got := loader.calls.Load(); got != 6 {
		t.Fatalf("a repeated plan reloaded covers: %d calls", got)
	}
}

func TestArtPrefetchKeepsFewUnshownCovers(t *testing.T) {
	m, _ := newPrefetchModel(t, 400, 10)
	for _, cursor := range []int{10, 60, 110, 160, 210, 260} {
		m.library.Cursor = cursor
		m, _ = m.syncArtwork()
		m, _ = runPrefetch(t, m)
		if n := len(m.art.prefetch.marked); n > artPrefetchMemory {
			t.Fatalf("%d unshown prefetched covers, cap %d", n, artPrefetchMemory)
		}
		if n := len(m.art.cache); n > artMemoryEntries {
			t.Fatalf("%d covers in memory, cap %d", n, artMemoryEntries)
		}
	}
}

func TestArtPrefetchIgnoresStaleResults(t *testing.T) {
	m, _ := newPrefetchModel(t, 100, 10)
	m, cmd := m.handleArtPrefetch(artPrefetchMsg{gen: m.art.prefetch.gen})
	cmds := flattenCmds(cmd)
	// Focus jumps far away before the lookups finish.
	m.library.Cursor = 80
	m, _ = m.syncArtwork()
	m, _ = m.handleArtPrefetch(artPrefetchMsg{gen: m.art.prefetch.gen})
	for _, c := range cmds {
		m, _ = m.handleArtPrefetched(c().(artPrefetchedMsg))
	}
	for _, row := range []int{11, 9} {
		if _, cached := m.art.cache[albumKey(t, m, row)]; cached {
			t.Fatalf("a stale prefetch for row %d was kept", row)
		}
	}
	// An old plan's tick does nothing.
	if _, cmd := m.handleArtPrefetch(artPrefetchMsg{gen: m.art.prefetch.gen - 1}); cmd != nil {
		t.Fatal("a stale plan tick started lookups")
	}
}

func TestArtPrefetchJoinsFocusLookup(t *testing.T) {
	m, loader := newPrefetchModel(t, 100, 10)
	m, cmd := m.handleArtPrefetch(artPrefetchMsg{gen: m.art.prefetch.gen})
	cmds := flattenCmds(cmd)
	// Focus lands on row 11 while its prefetch is loading.
	m.library.Cursor = 11
	m, _ = m.syncArtwork()
	if _, lookup := m.handleArtFocusSettled(artFocusSettledMsg{gen: m.art.focusGen}); lookup != nil {
		t.Fatal("focus started a second lookup for a cover already loading")
	}
	m, _ = m.handleArtFocusSettled(artFocusSettledMsg{gen: m.art.focusGen})
	for _, c := range cmds {
		m, _ = m.handleArtPrefetched(c().(artPrefetchedMsg))
	}
	key := albumKey(t, m, 11)
	if art, cached := m.art.cache[key]; !cached || art == nil {
		t.Fatal("the joined prefetch did not deliver the focused cover")
	}
	for _, k := range m.art.prefetch.marked {
		if k == key {
			t.Fatal("a cover on screen still counts as an unshown prefetch")
		}
	}
	if got := loader.calls.Load(); got != 2 {
		t.Fatalf("loader called %d times, want only the 2 prefetches", got)
	}
}

func TestArtEvictionKeepsPlayingCover(t *testing.T) {
	m, _ := newPrefetchModel(t, 200, 0)
	tracks := m.library.Panel().VisibleTracks()
	m.playback.currentHash = tracks[150].Hash
	m.playback.state = playback.StatePlaying
	_, playing := artRequestFor(tracks[150])
	m, _ = m.handleArtLoaded(artLoadedMsg{key: playing, art: artwork.Synthetic("p")})
	for i := range artMemoryEntries + 10 {
		_, key := artRequestFor(tracks[i])
		m, _ = m.handleArtLoaded(artLoadedMsg{key: key})
	}
	if _, ok := m.art.cache[playing]; !ok {
		t.Fatal("the playing cover was evicted")
	}
	if n := len(m.art.cache); n > artMemoryEntries {
		t.Fatalf("%d covers held, cap %d", n, artMemoryEntries)
	}
}

func TestArtPrefetchOffForInstantCovers(t *testing.T) {
	m := New(context.Background(), WithLibraryTracks(manyAlbumTracks(20)), WithArtwork(artwork.SyntheticLoader{}, config.ArtworkBlocks))
	m = m.handleWindowSizeMsg(tea.WindowSizeMsg{Width: 100, Height: 30})
	if cmd := m.scheduleArtPrefetch(); cmd != nil {
		t.Fatal("generated covers should never be prefetched")
	}
}
