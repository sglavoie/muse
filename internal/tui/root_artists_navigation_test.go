package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	libtui "github.com/sglavoie/muse/internal/tui/library"
)

// TestKey4SwitchesToArtistsView verifies that pressing 4 switches to the
// Artists view and derives the artist list from the library.
func TestKey4SwitchesToArtistsView(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "4")

	if m.activeView != viewBrowse {
		t.Fatalf("expected activeView=%d (viewBrowse), got %d", viewBrowse, m.activeView)
	}
	if len(m.browse.artists.artists) != 3 {
		t.Fatalf("expected 3 artists, got %d", len(m.browse.artists.artists))
	}
	want := []string{"Alice", "Bob", "Charlie"}
	for i, w := range want {
		if m.browse.artists.artists[i] != w {
			t.Errorf("artists[%d]: expected %q, got %q", i, w, m.browse.artists.artists[i])
		}
	}
}

// TestArtistsDrillDownToAlbums verifies that pressing Enter on an artist
// derives albums and transitions to AlbumLevel.
func TestArtistsDrillDownToAlbums(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "4")

	m = pressKey(m, "j")
	m = pressEnter(m)

	if m.browse.artists.level != arttui.AlbumLevel {
		t.Fatalf("expected AlbumLevel, got %v", m.browse.artists.level)
	}
	if m.browse.artists.selectedArtist != "Bob" {
		t.Errorf("expected selectedArtist='Bob', got %q", m.browse.artists.selectedArtist)
	}
	if len(m.browse.artists.albums) != 1 || m.browse.artists.albums[0] != "Gamma" {
		t.Errorf("expected albums=[Gamma], got %v", m.browse.artists.albums)
	}
}

// TestArtistsDrillDownToTracks verifies the full Artists -> Albums -> Tracks
// drill-down path.
func TestArtistsDrillDownToTracks(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "4")

	m = pressEnter(m)
	if m.browse.artists.level != arttui.AlbumLevel {
		t.Fatalf("expected AlbumLevel, got %v", m.browse.artists.level)
	}
	if m.browse.artists.selectedArtist != "Alice" {
		t.Fatalf("expected selectedArtist='Alice', got %q", m.browse.artists.selectedArtist)
	}

	m = pressEnter(m)
	if m.browse.artists.level != arttui.TrackLevel {
		t.Fatalf("expected TrackLevel, got %v", m.browse.artists.level)
	}
	if m.browse.artists.selectedAlbum != "Alpha" {
		t.Errorf("expected selectedAlbum='Alpha', got %q", m.browse.artists.selectedAlbum)
	}
	if len(m.browse.artists.tracks) != 2 {
		t.Fatalf("expected 2 tracks, got %d", len(m.browse.artists.tracks))
	}
	if m.browse.artists.tracks[0].Title != "Track A1" {
		t.Errorf("expected first track 'Track A1', got %q", m.browse.artists.tracks[0].Title)
	}
}

// TestArtistsEnqueueTrack verifies that pressing Enter at TrackLevel enqueues
// the focused track.
func TestArtistsEnqueueTrack(t *testing.T) {
	qa := &stubQueueAppender{}
	m := New(context.Background(), WithLibraryTracks(sampleLibraryTracks()), WithQueueWriter(qa))
	m = pressKey(m, "4")
	m = pressEnter(m)
	m = pressEnter(m)
	if m.browse.artists.level != arttui.TrackLevel {
		t.Fatalf("expected TrackLevel, got %v", m.browse.artists.level)
	}

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

// TestArtistsEscGoesBackThroughLevels verifies that Esc navigates back one
// level at a time and eventually returns to ArtistLevel.
func TestArtistsEscGoesBackThroughLevels(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "4")
	m = pressEnter(m)
	m = pressEnter(m)
	if m.browse.artists.level != arttui.TrackLevel {
		t.Fatalf("expected TrackLevel, got %v", m.browse.artists.level)
	}

	m = pressEsc(m)
	if m.browse.artists.level != arttui.AlbumLevel {
		t.Errorf("expected AlbumLevel after first Esc, got %v", m.browse.artists.level)
	}
	if len(m.browse.artists.albums) == 0 {
		t.Error("expected albums to be preserved after back from TrackLevel")
	}

	m = pressEsc(m)
	if m.browse.artists.level != arttui.ArtistLevel {
		t.Errorf("expected ArtistLevel after second Esc, got %v", m.browse.artists.level)
	}
}

// TestArtistsEscAtArtistLevelIsNoOp verifies that Esc at ArtistLevel does not
// change the view.
func TestArtistsEscAtArtistLevelIsNoOp(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "4")
	m = pressEsc(m)

	if m.activeView != viewBrowse {
		t.Errorf("expected to remain on viewBrowse, got %d", m.activeView)
	}
	if m.browse.artists.level != arttui.ArtistLevel {
		t.Errorf("expected ArtistLevel, got %v", m.browse.artists.level)
	}
}

// TestArtistsBackRestoresCursors verifies that navigating back restores the
// cursor position from each level.
func TestArtistsBackRestoresCursors(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "4")
	m = pressKey(m, "j")
	m = pressKey(m, "j")
	if m.browse.artists.cursor != 2 {
		t.Fatalf("expected cursor=2, got %d", m.browse.artists.cursor)
	}

	m = pressEnter(m)
	m = pressEsc(m)
	if m.browse.artists.cursor != 2 {
		t.Errorf("expected restored cursor=2 at ArtistLevel, got %d", m.browse.artists.cursor)
	}
}

// TestArtistsViewRenderOutput verifies that switching to the Artists view
// produces non-empty render output containing artist names.
func TestArtistsViewRenderOutput(t *testing.T) {
	m := newModelWithTracks()
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
	if m.browse.artists.cursor != 0 {
		t.Fatalf("expected initial cursor=0, got %d", m.browse.artists.cursor)
	}

	m = pressKey(m, "j")
	if m.browse.artists.cursor != 1 {
		t.Errorf("expected cursor=1 after j, got %d", m.browse.artists.cursor)
	}

	m = pressKey(m, "k")
	if m.browse.artists.cursor != 0 {
		t.Errorf("expected cursor=0 after k, got %d", m.browse.artists.cursor)
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
	if !strings.Contains(v.Content, "No artists") {
		t.Error("expected empty artists hint in artists view")
	}
}
