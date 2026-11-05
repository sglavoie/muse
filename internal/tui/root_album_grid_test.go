package tui

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/artwork"
	"github.com/sglavoie/muse/internal/config"
	"github.com/sglavoie/muse/internal/library"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	"github.com/sglavoie/muse/internal/tui/browse"
	"github.com/sglavoie/muse/internal/tui/keys"
)

func openAlbumGrid(t *testing.T, m Model, w, h int) Model {
	t.Helper()
	m = m.handleWindowSizeMsg(tea.WindowSizeMsg{Width: w, Height: h})
	m = pressKey(m, "4")
	for m.browse.Mode != browse.BrowseModeAlbums {
		m = pressKey(m, "`")
	}
	if !m.albumGridActive() {
		t.Fatalf("albums should open as a cover grid at %dx%d", w, h)
	}
	return m
}

func pressNamedKey(m Model, code rune) Model {
	next, _ := m.Update(tea.KeyPressMsg{Code: code})
	return next.(Model)
}

func TestAlbumGridMovesIn2DAndTogglesToTheList(t *testing.T) {
	m := openAlbumGrid(t, newRichFixture(), 80, 24)
	m = pressKey(m, "l")
	if m.browse.Albums.Cursor != 1 {
		t.Fatalf("l should move right, cursor %d", m.browse.Albums.Cursor)
	}
	m = pressNamedKey(m, tea.KeyRight)
	m = pressNamedKey(m, tea.KeyLeft)
	m = pressKey(m, "h")
	if m.browse.Albums.Cursor != 0 {
		t.Fatalf("h and the arrows should move sideways, cursor %d", m.browse.Albums.Cursor)
	}
	if a := m.resolveKey("left"); a != keys.ActionCursorLeft {
		t.Fatalf("left in the grid resolves to %v", a)
	}

	m = pressKey(m, "c")
	if m.browse.Albums.Layout != albtui.LayoutList || m.albumGridActive() {
		t.Fatal("c should switch to the list")
	}
	if a := m.resolveKey("left"); a != keys.ActionSeekBack {
		t.Fatalf("left in the album list should seek, got %v", a)
	}
	if a := m.resolveKey("l"); a != keys.ActionNone {
		t.Fatalf("l in the album list should do nothing, got %v", a)
	}
	if !strings.Contains(m.albumLayoutNotice(), "list") {
		t.Fatal(m.albumLayoutNotice())
	}
	m = pressKey(m, "c")
	if !m.albumGridActive() {
		t.Fatal("c should bring the grid back")
	}
}

func TestAlbumGridFallsBackWithoutArtOrRoom(t *testing.T) {
	tracks := []library.Track{{Path: "/m/a/1.flac", Hash: "h1", Title: "T", Artist: "A", Album: "Alb"}}
	m := New(context.Background(), WithLibraryTracks(tracks))
	m = m.handleWindowSizeMsg(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.ActiveView, m.browse.Mode = viewBrowse, browse.BrowseModeAlbums
	m = m.deriveCurrentBrowseData()
	if m.albumGridActive() {
		t.Fatal("without artwork the albums stay a list")
	}
	m = pressKey(pressKey(m, "c"), "c")
	if m.browse.Albums.Layout != albtui.LayoutGrid {
		t.Fatal("two toggles return to the grid preference")
	}
	if !strings.Contains(m.albumLayoutNotice(), "needs artwork") {
		t.Fatalf("toggle should explain why no grid shows: %q", m.albumLayoutNotice())
	}
}

func TestAlbumGridRendersCoversInTheFixture(t *testing.T) {
	m := openAlbumGrid(t, newRichFixture(), 80, 24)
	out := ansi.Strip(m.renderPrimaryBody())
	if !strings.Contains(out, "▀▀▀▀▀▀▀▀▀▀▀▀") || !strings.Contains(out, "▔▔▔▔") {
		t.Fatalf("grid should draw generated covers and the focus bar:\n%s", out)
	}
	for _, want := range []string{"Alpha", "Alice"} {
		if !strings.Contains(out, want) {
			t.Fatalf("tile text %q missing:\n%s", want, out)
		}
	}
}

// countingLoader answers asynchronously and counts lookups.
type countingLoader struct{ calls atomic.Int32 }

func (l *countingLoader) Load(_ context.Context, req artwork.Request) (*artwork.Art, error) {
	l.calls.Add(1)
	return artwork.Synthetic(req.Album), nil
}

func manyAlbumTracks(n int) []library.Track {
	tracks := make([]library.Track, n)
	for i := range tracks {
		tracks[i] = library.Track{Path: fmt.Sprintf("/m/%04d/1.flac", i), Hash: fmt.Sprintf("h%04d", i),
			Title: "Song", Artist: "Artist", Album: fmt.Sprintf("Album %04d", i)}
	}
	return tracks
}

func TestAlbumGridLoadsOnlyVisibleCoversAFewAtATime(t *testing.T) {
	loader := &countingLoader{}
	m := New(context.Background(), WithLibraryTracks(manyAlbumTracks(500)), WithArtwork(loader, config.ArtworkBlocks))
	m = openAlbumGrid(t, m, 80, 24)
	visible := len(m.visibleGridSlots())
	if visible == 0 || visible >= 500 {
		t.Fatalf("visible tiles: %d", visible)
	}
	if n := len(m.albumGrid.pending); n != gridMaxInflight {
		t.Fatalf("opening the grid should start %d lookups, started %d", gridMaxInflight, n)
	}
	// Run the lookups the grid asks for until none are left, as the program
	// loop would. The ones opening the grid started were dropped with their
	// commands, so start over.
	m.albumGrid.pending = map[string]bool{}
	for range 100 {
		cmds := m.requestGridThumbs()
		if len(cmds) == 0 {
			break
		}
		if len(cmds) > gridMaxInflight {
			t.Fatalf("%d lookups at once, cap is %d", len(cmds), gridMaxInflight)
		}
		for _, cmd := range cmds {
			m = m.handleGridArtLoaded(cmd().(gridArtLoadedMsg))
		}
	}
	if got := int(loader.calls.Load()); got != visible {
		t.Fatalf("looked up %d covers, want only the %d visible", got, visible)
	}
	if cover := m.albumTileCover(m.browse.Albums.Albums[0], 12, 6); !strings.Contains(cover, "▀") {
		t.Fatal("a loaded cover should draw with blocks")
	}
}

func TestAlbumGridThumbnailsEvictOffscreenOnly(t *testing.T) {
	m := New(context.Background(), WithLibraryTracks(manyAlbumTracks(gridThumbEntries+40)),
		WithArtwork(artwork.SyntheticLoader{}, config.ArtworkBlocks))
	m = openAlbumGrid(t, m, 80, 24)
	for _, album := range m.browse.Albums.Albums {
		rel := m.browse.Albums.Releases[album]
		_, key := artRequestFor(rel.Track)
		m = m.handleGridArtLoaded(gridArtLoadedMsg{key: key})
	}
	if n := len(m.albumGrid.thumbs); n > gridThumbEntries {
		t.Fatalf("%d thumbnails held, cap %d", n, gridThumbEntries)
	}
	for _, slot := range m.visibleGridSlots() {
		if _, ok := m.albumGrid.thumbs[slot.key]; !ok {
			t.Fatal("a visible tile's thumbnail was evicted")
		}
	}
}

func TestAlbumGridKittyUploadsOnceAndIsFreedOnQuit(t *testing.T) {
	m := newRichFixture()
	m.art.mode = artKitty
	m = openAlbumGrid(t, m, 80, 24)
	slots := m.visibleGridSlots()
	for _, slot := range slots {
		if _, ok := m.art.kitty.ids[slot]; !ok {
			t.Fatal("every visible tile with a cover should be placed after an update")
		}
	}
	if m.placeAlbumGrid(slots) != nil {
		t.Fatal("placed tiles must not be uploaded again")
	}
	album := m.browse.Albums.Albums[0]
	if out := m.albumTileCover(album, slots[0].cols, slots[0].rows); !strings.ContainsRune(out, '\U0010EEEE') {
		t.Fatal("placed tiles must draw as Kitty placeholders")
	}
	if m.ArtworkCleanup() == "" {
		t.Fatal("uploaded tiles must be freed on exit")
	}
}

func TestKittyAllocationNeverRecyclesALiveID(t *testing.T) {
	k := &kittyPlacements{ids: map[kittySlot]int{}, byID: map[int]kittySlot{}}
	live := map[int]bool{}
	for i := range artwork.MaxKittyID - 16 {
		id, ok := k.allocateAvoiding(kittySlot{key: fmt.Sprint(i), cols: 1, rows: 1}, live)
		if !ok {
			t.Fatalf("ran out of ids after %d", i)
		}
		live[id] = true
	}
	id, ok := k.allocateAvoiding(kittySlot{key: "last", cols: 1, rows: 1}, live)
	if !ok || live[id] {
		t.Fatalf("the one free id should be used, got %d (%v)", id, ok)
	}
	live[id] = true
	if _, ok := k.allocateAvoiding(kittySlot{key: "none", cols: 1, rows: 1}, live); ok {
		t.Fatal("with every id on screen, allocation must refuse rather than recycle")
	}
}

func TestBrowsingSessionRemembersAlbumLayout(t *testing.T) {
	m := openAlbumGrid(t, newRichFixture(), 100, 30)
	m = pressKey(m, "c")
	if s := m.captureBrowsingSession(); s.AlbumLayout != albtui.LayoutList {
		t.Fatalf("captured layout %v", s.AlbumLayout)
	}
	m.dataDir = t.TempDir()
	if err := m.SaveBrowsingSession(); err != nil {
		t.Fatal(err)
	}
	fresh := newRichFixture()
	fresh.dataDir = m.dataDir
	fresh = fresh.loadBrowsingSession()
	if fresh.browse.Albums.Layout != albtui.LayoutList {
		t.Fatal("the list choice should survive a restart")
	}
}
