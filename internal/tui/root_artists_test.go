package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
)

// sampleLibraryTracks returns a set of sorted tracks suitable for artists
// view testing. The tracks are sorted by artist as LibraryIndex provides.
func sampleLibraryTracks() []library.Track {
	return []library.Track{
		{Artist: "Alice", Album: "Alpha", Title: "Track A1", Duration: 183, Hash: "h1", Path: "/a1.flac"},
		{Artist: "Alice", Album: "Alpha", Title: "Track A2", Duration: 245, Hash: "h2", Path: "/a2.flac"},
		{Artist: "Alice", Album: "Beta", Title: "Track B1", Duration: 300, Hash: "h3", Path: "/b1.flac"},
		{Artist: "Bob", Album: "Gamma", Title: "Track G1", Duration: 120, Hash: "h4", Path: "/g1.flac"},
		{Artist: "Bob", Album: "Gamma", Title: "Track G2", Duration: 60, Hash: "h5", Path: "/g2.flac"},
		{Artist: "Charlie", Album: "Delta", Title: "Track D1", Duration: 3661, Hash: "h6", Path: "/d1.flac"},
	}
}

// newModelWithTracks creates a Model pre-loaded with sample tracks.
func newModelWithTracks() Model {
	return New(context.Background(), WithLibraryTracks(sampleLibraryTracks()))
}

// pressKey sends a key press and returns the resulting Model.
func pressKey(m Model, key string) Model {
	next, _ := m.Update(tea.KeyPressMsg{Code: rune(key[0]), Text: key})
	return next.(Model)
}

// pressEnter sends an Enter key press and returns the resulting Model.
func pressEnter(m Model) Model {
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	return next.(Model)
}

// pressEsc sends an Escape key press and returns the resulting Model.
func pressEsc(m Model) Model {
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	return next.(Model)
}

// TestKey4SwitchesToArtistsView verifies that pressing 4 switches to the
// Artists view and derives the artist list from the library.
func TestKey4SwitchesToArtistsView(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "4")

	if m.activeView != viewArtists {
		t.Fatalf("expected activeView=%d (viewArtists), got %d", viewArtists, m.activeView)
	}
	if len(m.artists.artists) != 3 {
		t.Fatalf("expected 3 artists, got %d", len(m.artists.artists))
	}
	want := []string{"Alice", "Bob", "Charlie"}
	for i, w := range want {
		if m.artists.artists[i] != w {
			t.Errorf("artists[%d]: expected %q, got %q", i, w, m.artists.artists[i])
		}
	}
}

// TestArtistsDrillDownToAlbums verifies that pressing Enter on an artist
// derives albums and transitions to AlbumLevel.
func TestArtistsDrillDownToAlbums(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "4") // switch to artists

	// Navigate to Bob (index 1).
	m = pressKey(m, "j")
	m = pressEnter(m)

	if m.artists.level != arttui.AlbumLevel {
		t.Fatalf("expected AlbumLevel, got %v", m.artists.level)
	}
	if m.artists.selectedArtist != "Bob" {
		t.Errorf("expected selectedArtist='Bob', got %q", m.artists.selectedArtist)
	}
	if len(m.artists.albums) != 1 || m.artists.albums[0] != "Gamma" {
		t.Errorf("expected albums=[Gamma], got %v", m.artists.albums)
	}
}

// TestArtistsDrillDownToTracks verifies the full Artists -> Albums -> Tracks
// drill-down path.
func TestArtistsDrillDownToTracks(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "4")

	// Select Alice (index 0) -> Albums.
	m = pressEnter(m)
	if m.artists.level != arttui.AlbumLevel {
		t.Fatalf("expected AlbumLevel, got %v", m.artists.level)
	}
	if m.artists.selectedArtist != "Alice" {
		t.Fatalf("expected selectedArtist='Alice', got %q", m.artists.selectedArtist)
	}

	// Select Alpha (index 0) -> Tracks.
	m = pressEnter(m)
	if m.artists.level != arttui.TrackLevel {
		t.Fatalf("expected TrackLevel, got %v", m.artists.level)
	}
	if m.artists.selectedAlbum != "Alpha" {
		t.Errorf("expected selectedAlbum='Alpha', got %q", m.artists.selectedAlbum)
	}
	if len(m.artists.tracks) != 2 {
		t.Fatalf("expected 2 tracks, got %d", len(m.artists.tracks))
	}
	if m.artists.tracks[0].Title != "Track A1" {
		t.Errorf("expected first track 'Track A1', got %q", m.artists.tracks[0].Title)
	}
}

// TestArtistsEnqueueTrack verifies that pressing Enter at TrackLevel enqueues
// the focused track.
func TestArtistsEnqueueTrack(t *testing.T) {
	qa := &stubQueueAppender{}
	m := New(context.Background(), WithLibraryTracks(sampleLibraryTracks()), WithQueueWriter(qa))
	m = pressKey(m, "4")

	// Drill down: Alice -> Alpha -> tracks.
	m = pressEnter(m)
	m = pressEnter(m)
	if m.artists.level != arttui.TrackLevel {
		t.Fatalf("expected TrackLevel, got %v", m.artists.level)
	}

	// Enqueue the first track.
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("expected non-nil cmd from Enter at TrackLevel")
	}

	// Execute the command and verify it produces a TrackEnqueuedMsg.
	msg := cmd()
	enqueued, ok := msg.(libtui.TrackEnqueuedMsg)
	if !ok {
		t.Fatalf("expected TrackEnqueuedMsg, got %T", msg)
	}
	if enqueued.Track.Title != "Track A1" {
		t.Errorf("expected enqueued track 'Track A1', got %q", enqueued.Track.Title)
	}
	if len(qa.tracks) != 1 {
		t.Errorf("expected 1 track in queue appender, got %d", len(qa.tracks))
	}
}

// TestArtistsEscGoesBackThroughLevels verifies that Esc navigates back one
// level at a time and eventually returns to ArtistLevel.
func TestArtistsEscGoesBackThroughLevels(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "4")

	// Drill down to track level.
	m = pressEnter(m) // -> AlbumLevel
	m = pressEnter(m) // -> TrackLevel
	if m.artists.level != arttui.TrackLevel {
		t.Fatalf("expected TrackLevel, got %v", m.artists.level)
	}

	// Back to AlbumLevel — albums must still be populated.
	m = pressEsc(m)
	if m.artists.level != arttui.AlbumLevel {
		t.Errorf("expected AlbumLevel after first Esc, got %v", m.artists.level)
	}
	if len(m.artists.albums) == 0 {
		t.Error("expected albums to be preserved after back from TrackLevel")
	}

	// Back to ArtistLevel.
	m = pressEsc(m)
	if m.artists.level != arttui.ArtistLevel {
		t.Errorf("expected ArtistLevel after second Esc, got %v", m.artists.level)
	}
}

// TestArtistsEscAtArtistLevelIsNoOp verifies that Esc at ArtistLevel does not
// change the view.
func TestArtistsEscAtArtistLevelIsNoOp(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "4")
	m = pressEsc(m)

	if m.activeView != viewArtists {
		t.Errorf("expected to remain on viewArtists, got %d", m.activeView)
	}
	if m.artists.level != arttui.ArtistLevel {
		t.Errorf("expected ArtistLevel, got %v", m.artists.level)
	}
}

// TestArtistsBackRestoresCursors verifies that navigating back restores the
// cursor position from each level.
func TestArtistsBackRestoresCursors(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "4")

	// Move to Charlie (index 2).
	m = pressKey(m, "j")
	m = pressKey(m, "j")
	if m.artists.cursor != 2 {
		t.Fatalf("expected cursor=2, got %d", m.artists.cursor)
	}

	// Drill down to albums (Charlie has 1 album: Delta).
	m = pressEnter(m)

	// Back should restore cursor=2 at ArtistLevel.
	m = pressEsc(m)
	if m.artists.cursor != 2 {
		t.Errorf("expected restored cursor=2 at ArtistLevel, got %d", m.artists.cursor)
	}
}

// TestArtistsViewRenderOutput verifies that switching to the Artists view
// produces non-empty render output containing artist names.
func TestArtistsViewRenderOutput(t *testing.T) {
	m := newModelWithTracks()
	// Set terminal dimensions so render produces output.
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)
	m = pressKey(m, "4")

	v := m.View()
	if v.Content == "" {
		t.Fatal("expected non-empty View content")
	}
	for _, want := range []string{"Alice", "Bob", "Charlie"} {
		if !strings.Contains(v.Content, want) {
			t.Errorf("expected artist %q in view output", want)
		}
	}
}

// TestArtistsViewNavigationKeys verifies that j/k keys move the cursor.
func TestArtistsViewNavigationKeys(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "4")
	if m.artists.cursor != 0 {
		t.Fatalf("expected initial cursor=0, got %d", m.artists.cursor)
	}

	m = pressKey(m, "j")
	if m.artists.cursor != 1 {
		t.Errorf("expected cursor=1 after j, got %d", m.artists.cursor)
	}

	m = pressKey(m, "k")
	if m.artists.cursor != 0 {
		t.Errorf("expected cursor=0 after k, got %d", m.artists.cursor)
	}
}

// TestScanBatchRefreshesArtistList verifies that receiving a ScanBatchMsg
// while at ArtistLevel re-derives the artist list.
func TestScanBatchRefreshesArtistList(t *testing.T) {
	m := New(context.Background(), WithLibraryTracks([]library.Track{
		{Artist: "Alice", Album: "Alpha", Title: "T1", Hash: "h1"},
	}))

	// Switch to artists view.
	m = pressKey(m, "4")
	if len(m.artists.artists) != 1 {
		t.Fatalf("expected 1 artist initially, got %d", len(m.artists.artists))
	}

	// Simulate a scan batch adding a new artist.
	next, _ := m.Update(library.ScanBatchMsg{Tracks: []library.Track{
		{Artist: "Bob", Album: "Beta", Title: "T2", Hash: "h2"},
	}})
	m = next.(Model)

	if len(m.artists.artists) != 2 {
		t.Fatalf("expected 2 artists after scan batch, got %d", len(m.artists.artists))
	}
}

// TestWatchBatchRefreshesArtistList verifies that receiving a watchBatchMsg
// while at ArtistLevel re-derives the artist list.
func TestWatchBatchRefreshesArtistList(t *testing.T) {
	m := New(context.Background(), WithLibraryTracks([]library.Track{
		{Artist: "Alice", Album: "Alpha", Title: "T1", Hash: "h1", Path: "/a.flac"},
		{Artist: "Bob", Album: "Beta", Title: "T2", Hash: "h2", Path: "/b.flac"},
	}))

	// Switch to artists view.
	m = pressKey(m, "4")
	if len(m.artists.artists) != 2 {
		t.Fatalf("expected 2 artists initially, got %d", len(m.artists.artists))
	}

	// Simulate the watch deleting Bob's track.
	next, _ := m.Update(watchBatchMsg{deleted: []string{"/b.flac"}})
	m = next.(Model)

	if len(m.artists.artists) != 1 {
		t.Fatalf("expected 1 artist after watch batch, got %d", len(m.artists.artists))
	}
	if m.artists.artists[0] != "Alice" {
		t.Errorf("expected remaining artist='Alice', got %q", m.artists.artists[0])
	}
}

// TestArtistsClampCursorAfterShrink verifies that the cursor is clamped when
// the artist list shrinks during a library update.
func TestArtistsClampCursorAfterShrink(t *testing.T) {
	m := New(context.Background(), WithLibraryTracks([]library.Track{
		{Artist: "Alice", Album: "Alpha", Title: "T1", Hash: "h1", Path: "/a.flac"},
		{Artist: "Bob", Album: "Beta", Title: "T2", Hash: "h2", Path: "/b.flac"},
		{Artist: "Charlie", Album: "Gamma", Title: "T3", Hash: "h3", Path: "/c.flac"},
	}))

	// Switch to artists view and move cursor to Charlie (index 2).
	m = pressKey(m, "4")
	m = pressKey(m, "j")
	m = pressKey(m, "j")
	if m.artists.cursor != 2 {
		t.Fatalf("expected cursor=2, got %d", m.artists.cursor)
	}

	// Delete Charlie — cursor should clamp to index 1.
	next, _ := m.Update(watchBatchMsg{deleted: []string{"/c.flac"}})
	m = next.(Model)

	if m.artists.cursor != 1 {
		t.Errorf("expected cursor clamped to 1, got %d", m.artists.cursor)
	}
}

// TestRescanClearsArtistsView verifies that actionRescan resets the artists
// view state so it gets re-derived from the new library.
func TestRescanClearsArtistsView(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "4")
	if len(m.artists.artists) != 3 {
		t.Fatalf("expected 3 artists, got %d", len(m.artists.artists))
	}

	// Simulate rescan: set libraryRoot and extractor to enable rescan.
	m.libraryRoot = "/tmp/music"
	m.extractor = stubExtractor{}

	next, _ := m.Update(tea.KeyPressMsg{Code: 'R', Mod: tea.ModShift, Text: "R"})
	m = next.(Model)

	if len(m.artists.artists) != 0 {
		t.Errorf("expected empty artist list after rescan, got %d", len(m.artists.artists))
	}
	if m.artists.level != arttui.ArtistLevel {
		t.Errorf("expected ArtistLevel after rescan, got %v", m.artists.level)
	}
}

// TestArtistsEmptyLibrary verifies that switching to the Artists view with an
// empty library shows the "(no artists)" placeholder.
func TestArtistsEmptyLibrary(t *testing.T) {
	m := New(context.Background())
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	m = next.(Model)
	m = pressKey(m, "4")

	v := m.View()
	if !strings.Contains(v.Content, "no artists") {
		t.Error("expected '(no artists)' placeholder in empty artists view")
	}
}

// TestArtistsViewWrapperHandleAction verifies that the ArtistsView HandleAction
// properly delegates cursor navigation.
func TestArtistsViewWrapperHandleAction(t *testing.T) {
	v := ArtistsView{
		artists: []string{"Alice", "Bob", "Charlie"},
		cursor:  0,
		height:  20,
	}

	v = v.HandleAction(actionCursorDown)
	if v.cursor != 1 {
		t.Errorf("CursorDown: expected 1, got %d", v.cursor)
	}

	v = v.HandleAction(actionCursorUp)
	if v.cursor != 0 {
		t.Errorf("CursorUp: expected 0, got %d", v.cursor)
	}

	v = v.HandleAction(actionGoToBottom)
	if v.cursor != 2 {
		t.Errorf("GoToBottom: expected 2, got %d", v.cursor)
	}

	v = v.HandleAction(actionGoToTop)
	if v.cursor != 0 {
		t.Errorf("GoToTop: expected 0, got %d", v.cursor)
	}
}

// TestArtistsViewFocusedMethods verifies focusedArtist, focusedAlbum, and
// focusedTrack return the correct values and false when out of bounds.
func TestArtistsViewFocusedMethods(t *testing.T) {
	v := ArtistsView{
		level:   arttui.ArtistLevel,
		artists: []string{"Alice", "Bob"},
		cursor:  1,
	}
	artist, ok := v.focusedArtist()
	if !ok || artist != "Bob" {
		t.Errorf("focusedArtist: expected (Bob, true), got (%q, %v)", artist, ok)
	}

	// Out of bounds.
	v.cursor = 5
	_, ok = v.focusedArtist()
	if ok {
		t.Error("focusedArtist: expected false when cursor out of bounds")
	}

	// Wrong level.
	v.level = arttui.AlbumLevel
	v.cursor = 0
	_, ok = v.focusedArtist()
	if ok {
		t.Error("focusedArtist: expected false at AlbumLevel")
	}

	// Album.
	v.level = arttui.AlbumLevel
	v.albums = []string{"Alpha", "Beta"}
	v.cursor = 0
	album, ok := v.focusedAlbum()
	if !ok || album != "Alpha" {
		t.Errorf("focusedAlbum: expected (Alpha, true), got (%q, %v)", album, ok)
	}

	// Track.
	v.level = arttui.TrackLevel
	v.tracks = []library.Track{{Title: "Song A"}}
	v.cursor = 0
	track, ok := v.focusedTrack()
	if !ok || track.Title != "Song A" {
		t.Errorf("focusedTrack: expected (Song A, true), got (%q, %v)", track.Title, ok)
	}
}

// TestArtistsViewClampCursor verifies clampCursor clamps at each level.
func TestArtistsViewClampCursor(t *testing.T) {
	t.Run("empty artist list", func(t *testing.T) {
		v := ArtistsView{cursor: 5, scrollOffset: 3}
		v = v.clampCursor()
		if v.cursor != 0 || v.scrollOffset != 0 {
			t.Errorf("expected cursor=0 scrollOffset=0, got cursor=%d scrollOffset=%d", v.cursor, v.scrollOffset)
		}
	})

	t.Run("cursor beyond list end", func(t *testing.T) {
		v := ArtistsView{
			artists: []string{"Alice", "Bob"},
			cursor:  5,
		}
		v = v.clampCursor()
		if v.cursor != 1 {
			t.Errorf("expected cursor=1, got %d", v.cursor)
		}
	})

	t.Run("cursor within bounds is unchanged", func(t *testing.T) {
		v := ArtistsView{
			artists: []string{"Alice", "Bob", "Charlie"},
			cursor:  1,
		}
		v = v.clampCursor()
		if v.cursor != 1 {
			t.Errorf("expected cursor=1 (unchanged), got %d", v.cursor)
		}
	})

	t.Run("album level", func(t *testing.T) {
		v := ArtistsView{
			level:  arttui.AlbumLevel,
			albums: []string{"Alpha"},
			cursor: 3,
		}
		v = v.clampCursor()
		if v.cursor != 0 {
			t.Errorf("expected cursor=0, got %d", v.cursor)
		}
	})

	t.Run("track level", func(t *testing.T) {
		v := ArtistsView{
			level:  arttui.TrackLevel,
			tracks: []library.Track{{Title: "T1"}},
			cursor: 5,
		}
		v = v.clampCursor()
		if v.cursor != 0 {
			t.Errorf("expected cursor=0, got %d", v.cursor)
		}
	})
}

// TestArtistsViewEnqueueFocusedTrack verifies the enqueue helper.
func TestArtistsViewEnqueueFocusedTrack(t *testing.T) {
	v := ArtistsView{
		level:  arttui.TrackLevel,
		tracks: []library.Track{{Title: "Song A", Hash: "h1"}},
		cursor: 0,
	}
	qa := &stubQueueAppender{}
	cmd := v.enqueueFocusedTrack(qa, 0)
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
	msg := cmd()
	enqueued, ok := msg.(libtui.TrackEnqueuedMsg)
	if !ok {
		t.Fatalf("expected TrackEnqueuedMsg, got %T", msg)
	}
	if enqueued.Track.Title != "Song A" {
		t.Errorf("expected 'Song A', got %q", enqueued.Track.Title)
	}
	if !enqueued.WasEmpty {
		t.Error("expected WasEmpty=true when prevQueueLen=0")
	}
}

// TestArtistsViewEnqueueFocusedTrackNotAtTrackLevel returns nil.
func TestArtistsViewEnqueueFocusedTrackNotAtTrackLevel(t *testing.T) {
	v := ArtistsView{
		level:   arttui.ArtistLevel,
		artists: []string{"Alice"},
		cursor:  0,
	}
	qa := &stubQueueAppender{}
	cmd := v.enqueueFocusedTrack(qa, 0)
	if cmd != nil {
		t.Fatal("expected nil cmd when not at TrackLevel")
	}
}

// --- filter integration tests ---

// pressSlash sends a "/" keypress to activate the filter bar, then executes
// the resulting FilterBarMsg command to complete activation.
func pressSlash(m Model) Model {
	next, cmd := m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m = next.(Model)
	if cmd != nil {
		next, _ = m.Update(cmd())
		m = next.(Model)
	}
	return m
}

// typeText sends each rune of s as a keypress (simulating typing in filter bar).
func typeText(m Model, s string) Model {
	for _, ch := range s {
		next, _ := m.Update(tea.KeyPressMsg{Code: ch, Text: string(ch)})
		m = next.(Model)
	}
	return m
}

func TestArtistsFilterActivateAndType(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "4")

	// Activate filter bar.
	m = pressSlash(m)
	if !m.artists.filter.Active {
		t.Fatal("expected filter to be active after /")
	}

	// Type "bob" — should filter to just Bob.
	m = typeText(m, "bob")
	if m.artists.filter.Text != "bob" {
		t.Fatalf("expected filterText='bob', got %q", m.artists.filter.Text)
	}

	// focusedArtist should be Bob.
	artist, ok := m.artists.focusedArtist()
	if !ok || artist != "Bob" {
		t.Errorf("expected focused artist='Bob', got %q (ok=%v)", artist, ok)
	}
}

func TestArtistsFilterEscClears(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "4")
	m = pressSlash(m)
	m = typeText(m, "ali")

	// Esc should deactivate and clear the filter.
	m = pressEsc(m)
	if m.artists.filter.Active {
		t.Error("expected filter deactivated after Esc")
	}
	if m.artists.filter.Text != "" {
		t.Errorf("expected filterText cleared, got %q", m.artists.filter.Text)
	}
	// All 3 artists should be visible again.
	if len(arttui.FilterStrings(m.artists.artists, m.artists.filter.Text)) != 3 {
		t.Error("expected full artist list after filter cleared")
	}
}

func TestArtistsFilterEnterCommits(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "4")
	m = pressSlash(m)
	m = typeText(m, "ali")

	// Enter commits: filter stays but input mode exits.
	m = pressEnter(m)
	if m.artists.filter.Active {
		t.Error("expected filter not active after Enter")
	}
	if m.artists.filter.Text != "ali" {
		t.Errorf("expected filterText='ali' after commit, got %q", m.artists.filter.Text)
	}
}

func TestArtistsFilterDrillDownClearsFilter(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "4")

	// Filter to "Ali" and commit.
	m = pressSlash(m)
	m = typeText(m, "ali")
	m = pressEnter(m) // commit filter
	if m.artists.filter.Text != "ali" {
		t.Fatalf("expected filterText='ali', got %q", m.artists.filter.Text)
	}

	// Drill down into Alice's albums.
	m = pressEnter(m)
	if m.artists.level != arttui.AlbumLevel {
		t.Fatalf("expected AlbumLevel, got %v", m.artists.level)
	}
	if m.artists.selectedArtist != "Alice" {
		t.Errorf("expected selectedArtist='Alice', got %q", m.artists.selectedArtist)
	}
	// Filter should be cleared on drill-down.
	if m.artists.filter.Text != "" {
		t.Errorf("expected filterText cleared after drill-down, got %q", m.artists.filter.Text)
	}
}

func TestArtistsFilterBackClearsFilter(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "4")
	m = pressEnter(m) // drill down to albums

	// Filter albums.
	m = pressSlash(m)
	m = typeText(m, "al")
	m = pressEnter(m) // commit

	// Go back — filter should be cleared.
	m = pressEsc(m)
	if m.artists.filter.Text != "" {
		t.Errorf("expected filterText cleared after back, got %q", m.artists.filter.Text)
	}
}

func TestArtistsFilterRenderOutput(t *testing.T) {
	m := newModelWithTracks()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)
	m = pressKey(m, "4")
	m = pressSlash(m)
	m = typeText(m, "bob")

	v := m.View()
	if !strings.Contains(v.Content, "Filter: > bob") {
		t.Errorf("expected filter bar in view output, got:\n%s", v.Content)
	}
	if !strings.Contains(v.Content, "Bob") {
		t.Error("expected 'Bob' in filtered view output")
	}
}

func TestArtistsFilterBackspace(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "4")
	m = pressSlash(m)
	m = typeText(m, "bob")

	// Backspace should remove last char.
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	m = next.(Model)
	if m.artists.filter.Text != "bo" {
		t.Errorf("expected filterText='bo' after backspace, got %q", m.artists.filter.Text)
	}
}

// TestArtistsBulkActionAtTrackLevel verifies that pressing 'a' at TrackLevel
// opens the destination picker with the focused track.
func TestArtistsBulkActionAtTrackLevel(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "4")

	// Drill down: Alice -> Alpha -> tracks.
	m = pressEnter(m) // -> AlbumLevel
	m = pressEnter(m) // -> TrackLevel
	if m.artists.level != arttui.TrackLevel {
		t.Fatalf("expected TrackLevel, got %v", m.artists.level)
	}

	// Press 'a' to open destination picker.
	m = pressKey(m, "a")
	if m.overlay.destPicker == nil {
		t.Fatal("expected destPicker to be non-nil after pressing 'a' at TrackLevel")
	}
	if len(m.overlay.destPickerTracks) != 1 {
		t.Fatalf("expected 1 destPickerTrack, got %d", len(m.overlay.destPickerTracks))
	}
	if m.overlay.destPickerTracks[0].Title != "Track A1" {
		t.Errorf("expected destPickerTrack 'Track A1', got %q", m.overlay.destPickerTracks[0].Title)
	}
}

// TestArtistsBulkActionAtArtistLevel verifies that pressing 'a' at ArtistLevel
// opens the destination picker with all tracks by the focused artist.
func TestArtistsBulkActionAtArtistLevel(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "4")

	// Press 'a' at ArtistLevel (focused on Alice).
	m = pressKey(m, "a")
	if m.overlay.destPicker == nil {
		t.Fatal("expected destPicker to be non-nil at ArtistLevel")
	}
	if len(m.overlay.destPickerTracks) != 3 {
		t.Fatalf("expected 3 destPickerTracks (all Alice tracks), got %d", len(m.overlay.destPickerTracks))
	}
	for _, tr := range m.overlay.destPickerTracks {
		if tr.Artist != "Alice" {
			t.Errorf("expected all tracks by Alice, got artist %q", tr.Artist)
		}
	}
}

// TestArtistsBulkActionAtAlbumLevel verifies that pressing 'a' at AlbumLevel
// opens the destination picker with all tracks from the focused album.
func TestArtistsBulkActionAtAlbumLevel(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "4")

	// Drill down: Alice -> AlbumLevel.
	m = pressEnter(m)
	if m.artists.level != arttui.AlbumLevel {
		t.Fatalf("expected AlbumLevel, got %v", m.artists.level)
	}

	// Press 'a' at AlbumLevel (focused on Alpha which has 2 tracks).
	m = pressKey(m, "a")
	if m.overlay.destPicker == nil {
		t.Fatal("expected destPicker to be non-nil at AlbumLevel")
	}
	if len(m.overlay.destPickerTracks) != 2 {
		t.Fatalf("expected 2 destPickerTracks (Alpha tracks), got %d", len(m.overlay.destPickerTracks))
	}
}

// TestArtistsDrillDownPreservesWidthHeight verifies that openAlbums and
// openTracks do not reset width/height.
func TestArtistsDrillDownPreservesWidthHeight(t *testing.T) {
	v := ArtistsView{
		level:   arttui.ArtistLevel,
		artists: []string{"Alice"},
		width:   120,
		height:  40,
	}

	v = v.openAlbums([]string{"Alpha"})
	if v.width != 120 || v.height != 40 {
		t.Errorf("openAlbums changed width/height: %d/%d", v.width, v.height)
	}

	v = v.openTracks([]library.Track{{Title: "T1"}})
	if v.width != 120 || v.height != 40 {
		t.Errorf("openTracks changed width/height: %d/%d", v.width, v.height)
	}
}

// TestArtistsClampCursor_WithFilter verifies that clampCursor accounts for
// the active filter rather than the raw list length.
func TestArtistsClampCursor_WithFilter(t *testing.T) {
	m := newModelWithTracks()
	m.artists.artists = arttui.DeriveArtists(m.library.index.Tracks())
	m.artists.level = arttui.ArtistLevel
	m.artists.filter.Text = "ali" // matches only "Alice"
	m.artists.cursor = 2          // beyond filtered bounds
	m.artists.scrollOffset = 2

	m.artists = m.artists.clampCursor()

	if m.artists.cursor != 0 {
		t.Errorf("expected cursor=0 (only 1 filtered match), got %d", m.artists.cursor)
	}
	if m.artists.scrollOffset != 0 {
		t.Errorf("expected scrollOffset=0, got %d", m.artists.scrollOffset)
	}
}

// TestArtistsClampCursor_ScrollOffsetClamped verifies that clampCursor brings
// scrollOffset down when it ends up above the cursor.
func TestArtistsClampCursor_ScrollOffsetClamped(t *testing.T) {
	m := newModelWithTracks()
	m.artists.artists = []string{"Alice"}
	m.artists.level = arttui.ArtistLevel
	m.artists.cursor = 5
	m.artists.scrollOffset = 10

	m.artists = m.artists.clampCursor()

	if m.artists.cursor != 0 {
		t.Errorf("expected cursor=0, got %d", m.artists.cursor)
	}
	if m.artists.scrollOffset != 0 {
		t.Errorf("expected scrollOffset=0, got %d", m.artists.scrollOffset)
	}
}

// TestArtistsClampCursor_TrackLevelWithFilter verifies clampCursor at
// TrackLevel with an active filter.
func TestArtistsClampCursor_TrackLevelWithFilter(t *testing.T) {
	m := newModelWithTracks()
	m.artists.level = arttui.TrackLevel
	m.artists.tracks = []library.Track{
		{Hash: "h1", Title: "Foo", Artist: "A"},
		{Hash: "h2", Title: "Bar", Artist: "B"},
		{Hash: "h3", Title: "Baz", Artist: "C"},
	}
	m.artists.filter.Text = "ba" // matches "Bar", "Baz" = 2 items
	m.artists.cursor = 3         // beyond filtered bounds
	m.artists.scrollOffset = 3

	m.artists = m.artists.clampCursor()

	if m.artists.cursor != 1 {
		t.Errorf("expected cursor=1 (last filtered index), got %d", m.artists.cursor)
	}
	if m.artists.scrollOffset > m.artists.cursor {
		t.Errorf("scrollOffset %d should not be above cursor %d", m.artists.scrollOffset, m.artists.cursor)
	}
}

// TestBulkActionArtistLevelUnknownArtistCollectsTracks verifies that the
// actionBulkAction handler at ArtistLevel correctly collects tracks for the
// "(unknown artist)" sentinel by converting it to an empty string for matching
// against raw track metadata.
func TestBulkActionArtistLevelUnknownArtistCollectsTracks(t *testing.T) {
	tracks := []library.Track{
		{Artist: "", Album: "NoArtist Album", Title: "Track UA1", Hash: "hu1", Path: "/ua1.flac"},
		{Artist: "", Album: "NoArtist Album", Title: "Track UA2", Hash: "hu2", Path: "/ua2.flac"},
		{Artist: "Alice", Album: "Alpha", Title: "Track A1", Hash: "h1", Path: "/a1.flac"},
	}
	m := New(context.Background(), WithLibraryTracks(tracks))
	m = pressKey(m, "4") // switches to viewArtists and derives artist list
	m.artists.width = 80
	m.artists.height = 40

	// Find the "(unknown artist)" entry and position cursor on it.
	unknownIdx := -1
	for i, a := range m.artists.artists {
		if a == arttui.UnknownArtist {
			unknownIdx = i
			break
		}
	}
	if unknownIdx < 0 {
		t.Fatal("expected (unknown artist) in artist list")
	}
	m.artists.cursor = unknownIdx

	// Trigger bulk action. It should collect tracks with empty artist.
	next, _, handled := m.handleLibraryKeyAction(actionBulkAction)
	if !handled {
		t.Fatal("actionBulkAction should be handled")
	}
	nm := next.(Model)

	// The destination picker should be opened with the 2 unknown-artist tracks.
	if nm.overlay.destPicker == nil {
		t.Fatal("expected destination picker to be opened for unknown artist tracks")
	}
	if len(nm.overlay.destPickerTracks) != 2 {
		t.Fatalf("expected 2 tracks for unknown artist, got %d", len(nm.overlay.destPickerTracks))
	}
}

// TestArtistsSelectedHashesClearedOnDestPickerConfirm verifies that
// artists.selectedHashes is cleared when the destination picker is confirmed
// via DestinationPickedMsg, preventing stale selection markers.
func TestArtistsSelectedHashesClearedOnDestPickerConfirm(t *testing.T) {
	be := &stubBatchEnqueuer{}
	m := New(context.Background(), WithBatchEnqueuer(be), WithLibraryTracks(sampleLibraryTracks()))
	m = pressKey(m, "4") // switch to Artists

	// Drill down: Alice -> Alpha -> tracks.
	m = pressEnter(m) // albums
	m = pressEnter(m) // tracks

	// Select the first track.
	m = pressKey(m, "x")
	if len(m.artists.selectedHashes) == 0 {
		t.Fatal("expected at least one selected hash after 'x'")
	}

	// Open destination picker.
	m = pressKey(m, "a")
	if m.overlay.destPicker == nil {
		t.Fatal("expected destPicker to be open after 'a'")
	}

	// Simulate DestinationPickedMsg (queue).
	m2, _ := m.Update(pltui.DestinationPickedMsg{IsQueue: true})
	nm := m2.(Model)
	if len(nm.artists.selectedHashes) != 0 {
		t.Errorf("expected artists.selectedHashes cleared after DestinationPickedMsg, got %v", nm.artists.selectedHashes)
	}
}

// TestArtistsSelectedHashesClearedOnDestPickerDismiss verifies that
// artists.selectedHashes is cleared when the destination picker is dismissed.
func TestArtistsSelectedHashesClearedOnDestPickerDismiss(t *testing.T) {
	m := New(context.Background(), WithLibraryTracks(sampleLibraryTracks()))
	m = pressKey(m, "4")
	m = pressEnter(m) // albums
	m = pressEnter(m) // tracks
	m = pressKey(m, "x")

	m = pressKey(m, "a")
	if m.overlay.destPicker == nil {
		t.Fatal("expected destPicker to be open")
	}

	m2, _ := m.Update(pltui.DestinationPickerDismissedMsg{})
	nm := m2.(Model)
	if len(nm.artists.selectedHashes) != 0 {
		t.Errorf("expected artists.selectedHashes cleared after dismiss, got %v", nm.artists.selectedHashes)
	}
}

// TestArtistsSelectedHashesClearedOnDestPickerCreate verifies that
// artists.selectedHashes is cleared when the destination picker "create"
// option is chosen.
func TestArtistsSelectedHashesClearedOnDestPickerCreate(t *testing.T) {
	m := New(context.Background(), WithLibraryTracks(sampleLibraryTracks()))
	m = pressKey(m, "4")
	m = pressEnter(m) // albums
	m = pressEnter(m) // tracks
	m = pressKey(m, "x")

	m = pressKey(m, "a")
	if m.overlay.destPicker == nil {
		t.Fatal("expected destPicker to be open")
	}

	m2, _ := m.Update(pltui.DestinationPickerCreateMsg{})
	nm := m2.(Model)
	if len(nm.artists.selectedHashes) != 0 {
		t.Errorf("expected artists.selectedHashes cleared after create, got %v", nm.artists.selectedHashes)
	}
}

// TestArtistsGoToBottomWithSortAccountsForReserve verifies that GoToBottom
// calculates the correct scroll offset when a sort is active. The sort
// indicator line takes one extra row, so visibleRows should be one fewer.
func TestArtistsGoToBottomWithSortAccountsForReserve(t *testing.T) {
	// Build 20 artists so we need scrolling.
	artists := make([]string, 20)
	for i := range artists {
		artists[i] = fmt.Sprintf("Artist_%02d", i)
	}

	// Without sort: Height=12, ArtistLevel no filter no sort -> reserve=0, visible=12.
	v := ArtistsView{
		level:   arttui.ArtistLevel,
		cursor:  0,
		artists: artists,
		height:  12,
	}
	v = v.HandleAction(actionGoToBottom)
	if v.cursor != 19 {
		t.Fatalf("GoToBottom without sort: expected cursor=19, got %d", v.cursor)
	}
	scrollNoSort := v.scrollOffset

	// With sort: reserve=1 (sort indicator), visible=11.
	v2 := ArtistsView{
		level:     arttui.ArtistLevel,
		cursor:    0,
		artists:   artists,
		height:    12,
		sortState: SortState{Field: SortTitle, Dir: SortAsc},
	}
	v2 = v2.HandleAction(actionGoToBottom)
	if v2.cursor != 19 {
		t.Fatalf("GoToBottom with sort: expected cursor=19, got %d", v2.cursor)
	}
	// With sort active, one fewer row is visible, so scrollOffset must be
	// one greater to keep the cursor visible.
	if v2.scrollOffset <= scrollNoSort {
		t.Errorf("GoToBottom with sort: expected scrollOffset > %d (no-sort), got %d",
			scrollNoSort, v2.scrollOffset)
	}
}
