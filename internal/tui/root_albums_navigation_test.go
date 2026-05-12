package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	libtui "github.com/sglavoie/muse/internal/tui/library"
)

// switchToAlbumsMode is a helper that switches to the Browse view in Albums mode.
func switchToAlbumsMode(m Model) Model {
	m = pressKey(m, "4") // Browse view (Artists mode by default)
	m = pressKey(m, "`") // cycle to Albums mode
	return m
}

// TestKey5SwitchesToAlbumsView verifies that pressing 4+` switches to the
// Browse view in Albums mode and derives the album list from the library.
func TestKey5SwitchesToAlbumsView(t *testing.T) {
	m := newModelWithTracks()
	m = switchToAlbumsMode(m)

	if m.activeView != viewBrowse {
		t.Fatalf("expected activeView=%d (viewBrowse), got %d", viewBrowse, m.activeView)
	}
	if len(m.browse.albums.albums) != 4 {
		t.Fatalf("expected 4 albums, got %d: %v", len(m.browse.albums.albums), m.browse.albums.albums)
	}
	want := []string{"Alpha", "Beta", "Delta", "Gamma"}
	for i, w := range want {
		if m.browse.albums.albums[i] != w {
			t.Errorf("albums[%d]: expected %q, got %q", i, w, m.browse.albums.albums[i])
		}
	}
}

// TestAlbumsDrillDownToTracks verifies that pressing Enter on an album
// transitions to TrackLevel with the correct tracks.
func TestAlbumsDrillDownToTracks(t *testing.T) {
	m := newModelWithTracks()
	m = switchToAlbumsMode(m)

	// Alpha is first (index 0). Press Enter.
	m = pressEnter(m)

	if m.browse.albums.level != albtui.TrackLevel {
		t.Fatalf("expected TrackLevel, got %v", m.browse.albums.level)
	}
	if m.browse.albums.selectedAlbum != "Alpha" {
		t.Errorf("expected selectedAlbum='Alpha', got %q", m.browse.albums.selectedAlbum)
	}
	if len(m.browse.albums.tracks) != 2 {
		t.Fatalf("expected 2 tracks for Alpha, got %d", len(m.browse.albums.tracks))
	}
	if m.browse.albums.tracks[0].Title != "Track A1" {
		t.Errorf("expected first track 'Track A1', got %q", m.browse.albums.tracks[0].Title)
	}
}

// TestAlbumsEnqueueTrack verifies that pressing Enter at TrackLevel enqueues
// the focused track.
func TestAlbumsEnqueueTrack(t *testing.T) {
	qa := &stubQueueAppender{}
	m := New(context.Background(), WithLibraryTracks(sampleLibraryTracks()), WithQueueWriter(qa))
	m = switchToAlbumsMode(m)

	// Drill down to Alpha's tracks.
	m = pressEnter(m)
	if m.browse.albums.level != albtui.TrackLevel {
		t.Fatalf("expected TrackLevel, got %v", m.browse.albums.level)
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
	m = switchToAlbumsMode(m)

	// Drill down to tracks.
	m = pressEnter(m)
	if m.browse.albums.level != albtui.TrackLevel {
		t.Fatalf("expected TrackLevel, got %v", m.browse.albums.level)
	}

	// Back to AlbumLevel.
	m = pressEsc(m)
	if m.browse.albums.level != albtui.AlbumLevel {
		t.Errorf("expected AlbumLevel after Esc, got %v", m.browse.albums.level)
	}
	if len(m.browse.albums.albums) == 0 {
		t.Error("expected albums to be preserved after back from TrackLevel")
	}
}

// TestAlbumsEscAtAlbumLevelIsNoOp verifies that Esc at AlbumLevel does not
// change the view.
func TestAlbumsEscAtAlbumLevelIsNoOp(t *testing.T) {
	m := newModelWithTracks()
	m = switchToAlbumsMode(m)
	m = pressEsc(m)

	if m.activeView != viewBrowse {
		t.Errorf("expected to remain on viewBrowse, got %d", m.activeView)
	}
	if m.browse.albums.level != albtui.AlbumLevel {
		t.Errorf("expected AlbumLevel, got %v", m.browse.albums.level)
	}
}

// TestAlbumsBackRestoresCursors verifies that navigating back restores the
// cursor position from album level.
func TestAlbumsBackRestoresCursors(t *testing.T) {
	m := newModelWithTracks()
	m = switchToAlbumsMode(m)

	// Move to Gamma (index 3: Alpha=0, Beta=1, Delta=2, Gamma=3).
	m = pressKey(m, "j")
	m = pressKey(m, "j")
	m = pressKey(m, "j")
	if m.browse.albums.cursor != 3 {
		t.Fatalf("expected cursor=3, got %d", m.browse.albums.cursor)
	}

	// Drill down to Gamma's tracks.
	m = pressEnter(m)

	// Back should restore cursor=3 at AlbumLevel.
	m = pressEsc(m)
	if m.browse.albums.cursor != 3 {
		t.Errorf("expected restored cursor=3 at AlbumLevel, got %d", m.browse.albums.cursor)
	}
}

// TestAlbumsViewRenderOutput verifies that switching to the Albums view
// produces non-empty render output containing album names.
func TestAlbumsViewRenderOutput(t *testing.T) {
	m := newModelWithTracks()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)
	m = switchToAlbumsMode(m)

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
	m = switchToAlbumsMode(m)
	if m.browse.albums.cursor != 0 {
		t.Fatalf("expected initial cursor=0, got %d", m.browse.albums.cursor)
	}

	m = pressKey(m, "j")
	if m.browse.albums.cursor != 1 {
		t.Errorf("expected cursor=1 after j, got %d", m.browse.albums.cursor)
	}

	m = pressKey(m, "k")
	if m.browse.albums.cursor != 0 {
		t.Errorf("expected cursor=0 after k, got %d", m.browse.albums.cursor)
	}
}

// TestAlbumsEmptyLibrary verifies that switching to the Albums view with an
// empty library shows the "(no albums)" placeholder.
func TestAlbumsEmptyLibrary(t *testing.T) {
	m := New(context.Background())
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	m = next.(Model)
	m = switchToAlbumsMode(m)

	v := m.View()
	if !strings.Contains(v.Content, "No albums") {
		t.Error("expected empty albums hint in albums view")
	}
}
