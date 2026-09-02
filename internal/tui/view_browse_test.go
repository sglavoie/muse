package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/library"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	"github.com/sglavoie/muse/internal/tui/browse"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
)

// TestBrowseModeNext_Cycles verifies that handleBrowseModeNext cycles through
// Artists → Albums → Genres → Artists.
func TestBrowseModeNext_Cycles(t *testing.T) {
	m := New(context.Background())
	m.ActiveView = viewBrowse
	m.browse.Mode = browse.BrowseModeArtists

	// Artists → Albums
	m, _ = m.handleBrowseModeNext()
	if m.browse.Mode != browse.BrowseModeAlbums {
		t.Errorf("want BrowseModeAlbums after first next, got %v", m.browse.Mode)
	}

	// Albums → Genres
	m, _ = m.handleBrowseModeNext()
	if m.browse.Mode != browse.BrowseModeGenres {
		t.Errorf("want BrowseModeGenres after second next, got %v", m.browse.Mode)
	}

	// Genres → Artists (wrap around)
	m, _ = m.handleBrowseModeNext()
	if m.browse.Mode != browse.BrowseModeArtists {
		t.Errorf("want BrowseModeArtists after third next (wrap), got %v", m.browse.Mode)
	}
}

// TestBrowseModePrev_Cycles verifies that handleBrowseModePrev cycles backward:
// Artists → Genres → Albums → Artists.
func TestBrowseModePrev_Cycles(t *testing.T) {
	m := New(context.Background())
	m.ActiveView = viewBrowse
	m.browse.Mode = browse.BrowseModeArtists

	// Artists → Genres (backward)
	m, _ = m.handleBrowseModePrev()
	if m.browse.Mode != browse.BrowseModeGenres {
		t.Errorf("want BrowseModeGenres after first prev, got %v", m.browse.Mode)
	}

	// Genres → Albums (backward)
	m, _ = m.handleBrowseModePrev()
	if m.browse.Mode != browse.BrowseModeAlbums {
		t.Errorf("want BrowseModeAlbums after second prev, got %v", m.browse.Mode)
	}

	// Albums → Artists (backward)
	m, _ = m.handleBrowseModePrev()
	if m.browse.Mode != browse.BrowseModeArtists {
		t.Errorf("want BrowseModeArtists after third prev (wrap), got %v", m.browse.Mode)
	}
}

// TestBrowseModeNext_ActiveViewRemainsBrowse verifies that activeView stays
// viewBrowse in all modes after cycling.
func TestBrowseModeNext_ActiveViewRemainsBrowse(t *testing.T) {
	m := New(context.Background())
	m.ActiveView = viewBrowse
	m.browse.Mode = browse.BrowseModeArtists

	for i := 0; i < 3; i++ {
		m, _ = m.handleBrowseModeNext()
		if m.ActiveView != viewBrowse {
			t.Errorf("cycle %d: activeView should remain viewBrowse, got %v", i, m.ActiveView)
		}
	}
}

// TestBrowseModePrev_ActiveViewRemainsBrowse verifies that activeView stays
// viewBrowse in all modes when cycling backward.
func TestBrowseModePrev_ActiveViewRemainsBrowse(t *testing.T) {
	m := New(context.Background())
	m.ActiveView = viewBrowse
	m.browse.Mode = browse.BrowseModeArtists

	for i := 0; i < 3; i++ {
		m, _ = m.handleBrowseModePrev()
		if m.ActiveView != viewBrowse {
			t.Errorf("cycle %d: activeView should remain viewBrowse, got %v", i, m.ActiveView)
		}
	}
}

// TestBrowseMode_ViewDisplayName verifies that viewDisplayName returns the
// correct mode-qualified string for each Browse sub-mode.
func TestBrowseMode_ViewDisplayName(t *testing.T) {
	m := New(context.Background())
	m.ActiveView = viewBrowse

	cases := []struct {
		mode browse.BrowseMode
		want string
	}{
		{browse.BrowseModeArtists, "Browse (Artists)"},
		{browse.BrowseModeAlbums, "Browse (Albums)"},
		{browse.BrowseModeGenres, "Browse (Genres)"},
	}
	for _, tc := range cases {
		m.browse.Mode = tc.mode
		got := m.viewDisplayName(viewBrowse)
		if got != tc.want {
			t.Errorf("mode %v: want %q, got %q", tc.mode, tc.want, got)
		}
	}
}

// TestBrowseMode_StatePreservedOnSwitch verifies that cursor and scroll offset
// are preserved in a sub-view when switching to another mode and switching back.
func TestBrowseMode_StatePreservedOnSwitch(t *testing.T) {
	tracks := []library.Track{
		{Artist: "Alice", Album: "Alpha", Title: "T1", Hash: "h1", Path: "/a.flac"},
		{Artist: "Bob", Album: "Beta", Title: "T2", Hash: "h2", Path: "/b.flac"},
		{Artist: "Charlie", Album: "Gamma", Title: "T3", Hash: "h3", Path: "/c.flac"},
	}
	m := New(context.Background(), WithLibraryTracks(tracks))
	m.ActiveView = viewBrowse
	m.browse.Mode = browse.BrowseModeArtists
	m.browse.Artists.Artists = arttui.DeriveArtists(m.library.Index.Tracks())
	m.browse.Artists.Cursor = 2
	m.browse.Artists.ScrollOffset = 1

	// Switch to Albums
	m, _ = m.handleBrowseModeNext()
	if m.browse.Mode != browse.BrowseModeAlbums {
		t.Fatalf("expected BrowseModeAlbums, got %v", m.browse.Mode)
	}

	// Artists state should be unchanged
	if m.browse.Artists.Cursor != 2 {
		t.Errorf("artists cursor should be preserved: want 2, got %d", m.browse.Artists.Cursor)
	}
	if m.browse.Artists.ScrollOffset != 1 {
		t.Errorf("artists scrollOffset should be preserved: want 1, got %d", m.browse.Artists.ScrollOffset)
	}

	// Switch back to Artists (go forward twice: Albums→Genres→Artists)
	m, _ = m.handleBrowseModeNext()
	m, _ = m.handleBrowseModeNext()
	if m.browse.Mode != browse.BrowseModeArtists {
		t.Fatalf("expected BrowseModeArtists after cycling back, got %v", m.browse.Mode)
	}

	// Artists cursor and scroll should still be at the saved values
	if m.browse.Artists.Cursor != 2 {
		t.Errorf("artists cursor after return: want 2, got %d", m.browse.Artists.Cursor)
	}
	if m.browse.Artists.ScrollOffset != 1 {
		t.Errorf("artists scrollOffset after return: want 1, got %d", m.browse.Artists.ScrollOffset)
	}
}

// TestBrowseMode_AlbumsCursorPreservedOnSwitch verifies album cursor state is
// preserved when switching away from Albums mode and returning.
func TestBrowseMode_AlbumsCursorPreservedOnSwitch(t *testing.T) {
	tracks := []library.Track{
		{Artist: "Alice", Album: "Alpha", Title: "T1", Hash: "h1", Path: "/a.flac"},
		{Artist: "Bob", Album: "Beta", Title: "T2", Hash: "h2", Path: "/b.flac"},
	}
	m := New(context.Background(), WithLibraryTracks(tracks))
	m.ActiveView = viewBrowse
	m.browse.Mode = browse.BrowseModeAlbums
	m.browse.Albums.Albums = albtui.DeriveAllAlbums(m.library.Index.Tracks())
	m.browse.Albums.Cursor = 1
	m.browse.Albums.ScrollOffset = 0

	// Switch away to Genres
	m, _ = m.handleBrowseModeNext()
	if m.browse.Mode != browse.BrowseModeGenres {
		t.Fatalf("expected BrowseModeGenres, got %v", m.browse.Mode)
	}

	// Albums cursor should be preserved
	if m.browse.Albums.Cursor != 1 {
		t.Errorf("albums cursor preserved: want 1, got %d", m.browse.Albums.Cursor)
	}
}

// TestBrowseMode_GenresCursorPreservedOnSwitch verifies genre cursor state is
// preserved when switching away from Genres mode and returning.
func TestBrowseMode_GenresCursorPreservedOnSwitch(t *testing.T) {
	tracks := []library.Track{
		{Artist: "Alice", Album: "Alpha", Genre: "Rock", Title: "T1", Hash: "h1", Path: "/a.flac"},
		{Artist: "Bob", Album: "Beta", Genre: "Jazz", Title: "T2", Hash: "h2", Path: "/b.flac"},
	}
	m := New(context.Background(), WithLibraryTracks(tracks))
	m.ActiveView = viewBrowse
	m.browse.Mode = browse.BrowseModeGenres
	m.browse.Genres.Genres = gentui.DeriveGenres(m.library.Index.Tracks())
	m.browse.Genres.Cursor = 1
	m.browse.Genres.ScrollOffset = 0

	// Switch away to Artists
	m, _ = m.handleBrowseModeNext()
	if m.browse.Mode != browse.BrowseModeArtists {
		t.Fatalf("expected BrowseModeArtists, got %v", m.browse.Mode)
	}

	// Genres cursor should be preserved
	if m.browse.Genres.Cursor != 1 {
		t.Errorf("genres cursor preserved: want 1, got %d", m.browse.Genres.Cursor)
	}
}
