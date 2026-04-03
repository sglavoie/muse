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
	m = pressKey(m, "4")

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

	m = pressEnter(m)
	if m.artists.level != arttui.AlbumLevel {
		t.Fatalf("expected AlbumLevel, got %v", m.artists.level)
	}
	if m.artists.selectedArtist != "Alice" {
		t.Fatalf("expected selectedArtist='Alice', got %q", m.artists.selectedArtist)
	}

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
	m = pressEnter(m)
	m = pressEnter(m)
	if m.artists.level != arttui.TrackLevel {
		t.Fatalf("expected TrackLevel, got %v", m.artists.level)
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
	if m.artists.level != arttui.TrackLevel {
		t.Fatalf("expected TrackLevel, got %v", m.artists.level)
	}

	m = pressEsc(m)
	if m.artists.level != arttui.AlbumLevel {
		t.Errorf("expected AlbumLevel after first Esc, got %v", m.artists.level)
	}
	if len(m.artists.albums) == 0 {
		t.Error("expected albums to be preserved after back from TrackLevel")
	}

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
	m = pressKey(m, "j")
	m = pressKey(m, "j")
	if m.artists.cursor != 2 {
		t.Fatalf("expected cursor=2, got %d", m.artists.cursor)
	}

	m = pressEnter(m)
	m = pressEsc(m)
	if m.artists.cursor != 2 {
		t.Errorf("expected restored cursor=2 at ArtistLevel, got %d", m.artists.cursor)
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
