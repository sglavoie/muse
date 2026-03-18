package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	libtui "github.com/sglavoie/muse/internal/tui/library"
)

// TestKey5SwitchesToAlbumsView verifies that pressing 5 switches to the
// Albums view and derives the album list from the library.
func TestKey5SwitchesToAlbumsView(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "5")

	if m.activeView != viewAlbums {
		t.Fatalf("expected activeView=%d (viewAlbums), got %d", viewAlbums, m.activeView)
	}
	if len(m.albums.albums) != 4 {
		t.Fatalf("expected 4 albums, got %d: %v", len(m.albums.albums), m.albums.albums)
	}
	want := []string{"Alpha", "Beta", "Delta", "Gamma"}
	for i, w := range want {
		if m.albums.albums[i] != w {
			t.Errorf("albums[%d]: expected %q, got %q", i, w, m.albums.albums[i])
		}
	}
}

// TestAlbumsDrillDownToTracks verifies that pressing Enter on an album
// transitions to TrackLevel with the correct tracks.
func TestAlbumsDrillDownToTracks(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "5")

	// Alpha is first (index 0). Press Enter.
	m = pressEnter(m)

	if m.albums.level != albtui.TrackLevel {
		t.Fatalf("expected TrackLevel, got %v", m.albums.level)
	}
	if m.albums.selectedAlbum != "Alpha" {
		t.Errorf("expected selectedAlbum='Alpha', got %q", m.albums.selectedAlbum)
	}
	if len(m.albums.tracks) != 2 {
		t.Fatalf("expected 2 tracks for Alpha, got %d", len(m.albums.tracks))
	}
	if m.albums.tracks[0].Title != "Track A1" {
		t.Errorf("expected first track 'Track A1', got %q", m.albums.tracks[0].Title)
	}
}

// TestAlbumsEnqueueTrack verifies that pressing Enter at TrackLevel enqueues
// the focused track.
func TestAlbumsEnqueueTrack(t *testing.T) {
	m := newModelWithTracks()
	qa := &stubQueueAppender{}
	m.SetQueueWriter(qa)
	m = pressKey(m, "5")

	// Drill down to Alpha's tracks.
	m = pressEnter(m)
	if m.albums.level != albtui.TrackLevel {
		t.Fatalf("expected TrackLevel, got %v", m.albums.level)
	}

	// Enqueue the first track.
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("expected non-nil cmd from Enter at TrackLevel")
	}

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

// TestAlbumsEscGoesBack verifies that Esc navigates back from TrackLevel to
// AlbumLevel.
func TestAlbumsEscGoesBack(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "5")

	// Drill down to tracks.
	m = pressEnter(m)
	if m.albums.level != albtui.TrackLevel {
		t.Fatalf("expected TrackLevel, got %v", m.albums.level)
	}

	// Back to AlbumLevel.
	m = pressEsc(m)
	if m.albums.level != albtui.AlbumLevel {
		t.Errorf("expected AlbumLevel after Esc, got %v", m.albums.level)
	}
	if len(m.albums.albums) == 0 {
		t.Error("expected albums to be preserved after back from TrackLevel")
	}
}

// TestAlbumsEscAtAlbumLevelIsNoOp verifies that Esc at AlbumLevel does not
// change the view.
func TestAlbumsEscAtAlbumLevelIsNoOp(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "5")
	m = pressEsc(m)

	if m.activeView != viewAlbums {
		t.Errorf("expected to remain on viewAlbums, got %d", m.activeView)
	}
	if m.albums.level != albtui.AlbumLevel {
		t.Errorf("expected AlbumLevel, got %v", m.albums.level)
	}
}

// TestAlbumsBackRestoresCursors verifies that navigating back restores the
// cursor position from album level.
func TestAlbumsBackRestoresCursors(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "5")

	// Move to Gamma (index 3: Alpha=0, Beta=1, Delta=2, Gamma=3).
	m = pressKey(m, "j")
	m = pressKey(m, "j")
	m = pressKey(m, "j")
	if m.albums.cursor != 3 {
		t.Fatalf("expected cursor=3, got %d", m.albums.cursor)
	}

	// Drill down to Gamma's tracks.
	m = pressEnter(m)

	// Back should restore cursor=3 at AlbumLevel.
	m = pressEsc(m)
	if m.albums.cursor != 3 {
		t.Errorf("expected restored cursor=3 at AlbumLevel, got %d", m.albums.cursor)
	}
}

// TestAlbumsViewRenderOutput verifies that switching to the Albums view
// produces non-empty render output containing album names.
func TestAlbumsViewRenderOutput(t *testing.T) {
	m := newModelWithTracks()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)
	m = pressKey(m, "5")

	v := m.View()
	if v.Content == "" {
		t.Fatal("expected non-empty View content")
	}
	for _, want := range []string{"Alpha", "Beta", "Delta", "Gamma"} {
		if !strings.Contains(v.Content, want) {
			t.Errorf("expected album %q in view output", want)
		}
	}
}

// TestAlbumsViewNavigationKeys verifies that j/k keys move the cursor.
func TestAlbumsViewNavigationKeys(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "5")
	if m.albums.cursor != 0 {
		t.Fatalf("expected initial cursor=0, got %d", m.albums.cursor)
	}

	m = pressKey(m, "j")
	if m.albums.cursor != 1 {
		t.Errorf("expected cursor=1 after j, got %d", m.albums.cursor)
	}

	m = pressKey(m, "k")
	if m.albums.cursor != 0 {
		t.Errorf("expected cursor=0 after k, got %d", m.albums.cursor)
	}
}

// TestScanBatchRefreshesAlbumList verifies that receiving a ScanBatchMsg
// while at AlbumLevel re-derives the album list.
func TestScanBatchRefreshesAlbumList(t *testing.T) {
	m := New(context.Background(), "", false, nil, nil, "", nil, nil)
	m.SetLibraryTracks([]library.Track{
		{Artist: "Alice", Album: "Alpha", Title: "T1", Hash: "h1"},
	})

	// Switch to albums view.
	m = pressKey(m, "5")
	if len(m.albums.albums) != 1 {
		t.Fatalf("expected 1 album initially, got %d", len(m.albums.albums))
	}

	// Simulate a scan batch adding a new album.
	next, _ := m.Update(library.ScanBatchMsg{Tracks: []library.Track{
		{Artist: "Bob", Album: "Beta", Title: "T2", Hash: "h2"},
	}})
	m = next.(Model)

	if len(m.albums.albums) != 2 {
		t.Fatalf("expected 2 albums after scan batch, got %d", len(m.albums.albums))
	}
}

// TestWatchBatchRefreshesAlbumList verifies that receiving a watchBatchMsg
// while at AlbumLevel re-derives the album list.
func TestWatchBatchRefreshesAlbumList(t *testing.T) {
	m := New(context.Background(), "", false, nil, nil, "", nil, nil)
	m.SetLibraryTracks([]library.Track{
		{Artist: "Alice", Album: "Alpha", Title: "T1", Hash: "h1", Path: "/a.flac"},
		{Artist: "Bob", Album: "Beta", Title: "T2", Hash: "h2", Path: "/b.flac"},
	})

	// Switch to albums view.
	m = pressKey(m, "5")
	if len(m.albums.albums) != 2 {
		t.Fatalf("expected 2 albums initially, got %d", len(m.albums.albums))
	}

	// Simulate the watch deleting Bob's track.
	next, _ := m.Update(watchBatchMsg{deleted: []string{"/b.flac"}})
	m = next.(Model)

	if len(m.albums.albums) != 1 {
		t.Fatalf("expected 1 album after watch batch, got %d", len(m.albums.albums))
	}
	if m.albums.albums[0] != "Alpha" {
		t.Errorf("expected remaining album='Alpha', got %q", m.albums.albums[0])
	}
}

// TestAlbumsClampCursorAfterShrink verifies that the cursor is clamped when
// the album list shrinks during a library update.
func TestAlbumsClampCursorAfterShrink(t *testing.T) {
	m := New(context.Background(), "", false, nil, nil, "", nil, nil)
	m.SetLibraryTracks([]library.Track{
		{Artist: "Alice", Album: "Alpha", Title: "T1", Hash: "h1", Path: "/a.flac"},
		{Artist: "Bob", Album: "Beta", Title: "T2", Hash: "h2", Path: "/b.flac"},
		{Artist: "Charlie", Album: "Gamma", Title: "T3", Hash: "h3", Path: "/c.flac"},
	})

	// Switch to albums view and move cursor to Gamma (index 2).
	m = pressKey(m, "5")
	m = pressKey(m, "j")
	m = pressKey(m, "j")
	if m.albums.cursor != 2 {
		t.Fatalf("expected cursor=2, got %d", m.albums.cursor)
	}

	// Delete Charlie — cursor should clamp to index 1.
	next, _ := m.Update(watchBatchMsg{deleted: []string{"/c.flac"}})
	m = next.(Model)

	if m.albums.cursor != 1 {
		t.Errorf("expected cursor clamped to 1, got %d", m.albums.cursor)
	}
}

// TestRescanClearsAlbumsView verifies that actionRescan resets the albums
// view state so it gets re-derived from the new library.
func TestRescanClearsAlbumsView(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "5")
	if len(m.albums.albums) != 4 {
		t.Fatalf("expected 4 albums, got %d", len(m.albums.albums))
	}

	// Simulate rescan: set libraryRoot and extractor to enable rescan.
	m.libraryRoot = "/tmp/music"
	m.extractor = stubExtractor{}

	next, _ := m.Update(tea.KeyPressMsg{Code: 'R', Mod: tea.ModShift, Text: "R"})
	m = next.(Model)

	if len(m.albums.albums) != 0 {
		t.Errorf("expected empty album list after rescan, got %d", len(m.albums.albums))
	}
	if m.albums.level != albtui.AlbumLevel {
		t.Errorf("expected AlbumLevel after rescan, got %v", m.albums.level)
	}
}

// TestAlbumsEmptyLibrary verifies that switching to the Albums view with an
// empty library shows the "(no albums)" placeholder.
func TestAlbumsEmptyLibrary(t *testing.T) {
	m := New(context.Background(), "", false, nil, nil, "", nil, nil)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	m = next.(Model)
	m = pressKey(m, "5")

	v := m.View()
	if !strings.Contains(v.Content, "no albums") {
		t.Error("expected '(no albums)' placeholder in empty albums view")
	}
}

// TestAlbumsViewWrapperHandleAction verifies that the AlbumsView HandleAction
// properly delegates cursor navigation.
func TestAlbumsViewWrapperHandleAction(t *testing.T) {
	v := AlbumsView{
		albums: []string{"Alpha", "Beta", "Delta", "Gamma"},
		cursor: 0,
		height: 20,
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
	if v.cursor != 3 {
		t.Errorf("GoToBottom: expected 3, got %d", v.cursor)
	}

	v = v.HandleAction(actionGoToTop)
	if v.cursor != 0 {
		t.Errorf("GoToTop: expected 0, got %d", v.cursor)
	}
}

// TestAlbumsViewFocusedMethods verifies focusedAlbum and focusedTrack return
// the correct values and false when out of bounds or at the wrong level.
func TestAlbumsViewFocusedMethods(t *testing.T) {
	v := AlbumsView{
		level:  albtui.AlbumLevel,
		albums: []string{"Alpha", "Beta"},
		cursor: 1,
	}
	album, ok := v.focusedAlbum()
	if !ok || album != "Beta" {
		t.Errorf("focusedAlbum: expected (Beta, true), got (%q, %v)", album, ok)
	}

	// Out of bounds.
	v.cursor = 5
	_, ok = v.focusedAlbum()
	if ok {
		t.Error("focusedAlbum: expected false when cursor out of bounds")
	}

	// Wrong level.
	v.level = albtui.TrackLevel
	v.cursor = 0
	_, ok = v.focusedAlbum()
	if ok {
		t.Error("focusedAlbum: expected false at TrackLevel")
	}

	// Track.
	v.level = albtui.TrackLevel
	v.tracks = []library.Track{{Title: "Song A"}}
	v.cursor = 0
	track, ok := v.focusedTrack()
	if !ok || track.Title != "Song A" {
		t.Errorf("focusedTrack: expected (Song A, true), got (%q, %v)", track.Title, ok)
	}

	// Wrong level for track.
	v.level = albtui.AlbumLevel
	_, ok = v.focusedTrack()
	if ok {
		t.Error("focusedTrack: expected false at AlbumLevel")
	}
}

// TestAlbumsViewClampCursor verifies clampCursor clamps at each level.
func TestAlbumsViewClampCursor(t *testing.T) {
	t.Run("empty album list", func(t *testing.T) {
		v := AlbumsView{cursor: 5, scrollOffset: 3}
		v = v.clampCursor()
		if v.cursor != 0 || v.scrollOffset != 0 {
			t.Errorf("expected cursor=0 scrollOffset=0, got cursor=%d scrollOffset=%d", v.cursor, v.scrollOffset)
		}
	})

	t.Run("cursor beyond list end", func(t *testing.T) {
		v := AlbumsView{
			albums: []string{"Alpha", "Beta"},
			cursor: 5,
		}
		v = v.clampCursor()
		if v.cursor != 1 {
			t.Errorf("expected cursor=1, got %d", v.cursor)
		}
	})

	t.Run("cursor within bounds is unchanged", func(t *testing.T) {
		v := AlbumsView{
			albums: []string{"Alpha", "Beta", "Gamma"},
			cursor: 1,
		}
		v = v.clampCursor()
		if v.cursor != 1 {
			t.Errorf("expected cursor=1 (unchanged), got %d", v.cursor)
		}
	})

	t.Run("track level", func(t *testing.T) {
		v := AlbumsView{
			level:  albtui.TrackLevel,
			tracks: []library.Track{{Title: "T1"}},
			cursor: 5,
		}
		v = v.clampCursor()
		if v.cursor != 0 {
			t.Errorf("expected cursor=0, got %d", v.cursor)
		}
	})
}

// TestAlbumsViewEnqueueFocusedTrack verifies the enqueue helper.
func TestAlbumsViewEnqueueFocusedTrack(t *testing.T) {
	v := AlbumsView{
		level:  albtui.TrackLevel,
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

// TestAlbumsViewEnqueueFocusedTrackNotAtTrackLevel returns nil.
func TestAlbumsViewEnqueueFocusedTrackNotAtTrackLevel(t *testing.T) {
	v := AlbumsView{
		level:  albtui.AlbumLevel,
		albums: []string{"Alpha"},
		cursor: 0,
	}
	qa := &stubQueueAppender{}
	cmd := v.enqueueFocusedTrack(qa, 0)
	if cmd != nil {
		t.Fatal("expected nil cmd when not at TrackLevel")
	}
}

// --- filter integration tests ---

func TestAlbumsFilterActivateAndType(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "5")

	// Activate filter bar.
	m = pressSlash(m)
	if !m.albums.filter.Active {
		t.Fatal("expected filter to be active after /")
	}

	// Type "gam" — should filter to just Gamma.
	m = typeText(m, "gam")
	if m.albums.filter.Text != "gam" {
		t.Fatalf("expected filterText='gam', got %q", m.albums.filter.Text)
	}

	// focusedAlbum should be Gamma.
	album, ok := m.albums.focusedAlbum()
	if !ok || album != "Gamma" {
		t.Errorf("expected focused album='Gamma', got %q (ok=%v)", album, ok)
	}
}

func TestAlbumsFilterEscClears(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "5")
	m = pressSlash(m)
	m = typeText(m, "alp")

	// Esc should deactivate and clear the filter.
	m = pressEsc(m)
	if m.albums.filter.Active {
		t.Error("expected filter deactivated after Esc")
	}
	if m.albums.filter.Text != "" {
		t.Errorf("expected filterText cleared, got %q", m.albums.filter.Text)
	}
	// All 4 albums should be visible again.
	if len(arttui.FilterStrings(m.albums.albums, m.albums.filter.Text)) != 4 {
		t.Error("expected full album list after filter cleared")
	}
}

func TestAlbumsFilterEnterCommits(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "5")
	m = pressSlash(m)
	m = typeText(m, "alp")

	// Enter commits: filter stays but input mode exits.
	m = pressEnter(m)
	if m.albums.filter.Active {
		t.Error("expected filter not active after Enter")
	}
	if m.albums.filter.Text != "alp" {
		t.Errorf("expected filterText='alp' after commit, got %q", m.albums.filter.Text)
	}
}

func TestAlbumsFilterDrillDownClearsFilter(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "5")

	// Filter to "Alp" and commit.
	m = pressSlash(m)
	m = typeText(m, "alp")
	m = pressEnter(m) // commit filter
	if m.albums.filter.Text != "alp" {
		t.Fatalf("expected filterText='alp', got %q", m.albums.filter.Text)
	}

	// Drill down into Alpha's tracks.
	m = pressEnter(m)
	if m.albums.level != albtui.TrackLevel {
		t.Fatalf("expected TrackLevel, got %v", m.albums.level)
	}
	if m.albums.selectedAlbum != "Alpha" {
		t.Errorf("expected selectedAlbum='Alpha', got %q", m.albums.selectedAlbum)
	}
	// Filter should be cleared on drill-down.
	if m.albums.filter.Text != "" {
		t.Errorf("expected filterText cleared after drill-down, got %q", m.albums.filter.Text)
	}
}

func TestAlbumsFilterBackClearsFilter(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "5")
	m = pressEnter(m) // drill down to tracks

	// Filter tracks.
	m = pressSlash(m)
	m = typeText(m, "a1")
	m = pressEnter(m) // commit

	// Go back — filter should be cleared.
	m = pressEsc(m)
	if m.albums.filter.Text != "" {
		t.Errorf("expected filterText cleared after back, got %q", m.albums.filter.Text)
	}
}

func TestAlbumsFilterRenderOutput(t *testing.T) {
	m := newModelWithTracks()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)
	m = pressKey(m, "5")
	m = pressSlash(m)
	m = typeText(m, "gam")

	v := m.View()
	if !strings.Contains(v.Content, "Filter: > gam") {
		t.Errorf("expected filter bar in view output, got:\n%s", v.Content)
	}
	if !strings.Contains(v.Content, "Gamma") {
		t.Error("expected 'Gamma' in filtered view output")
	}
}

func TestAlbumsFilterBackspace(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "5")
	m = pressSlash(m)
	m = typeText(m, "gam")

	// Backspace should remove last char.
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	m = next.(Model)
	if m.albums.filter.Text != "ga" {
		t.Errorf("expected filterText='ga' after backspace, got %q", m.albums.filter.Text)
	}
}

// TestAlbumsDrillDownPreservesWidthHeight verifies that openTracks does not
// reset width/height.
func TestAlbumsDrillDownPreservesWidthHeight(t *testing.T) {
	v := AlbumsView{
		level:  albtui.AlbumLevel,
		albums: []string{"Alpha"},
		width:  120,
		height: 40,
	}

	v = v.openTracks([]library.Track{{Title: "T1"}})
	if v.width != 120 || v.height != 40 {
		t.Errorf("openTracks changed width/height: %d/%d", v.width, v.height)
	}
}
