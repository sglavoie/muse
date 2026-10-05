package tui

import (
	"context"
	"fmt"
	"testing"

	"github.com/sglavoie/muse/internal/library"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	"github.com/sglavoie/muse/internal/tui/browse"
)

// TestJumpToAlbum verifies that jumpToAlbum switches to the Albums view with
// the cursor pre-positioned on the named album.
func TestJumpToAlbum(t *testing.T) {
	m := newModelWithTracks()
	// DeriveAllAlbums (sorted): ["Alpha"(0), "Beta"(1), "Delta"(2), "Gamma"(3)]
	m = m.jumpToAlbum("Gamma")

	if m.ActiveView != viewBrowse {
		t.Errorf("activeView: want viewBrowse(%d), got %d", viewBrowse, m.ActiveView)
	}
	if m.browse.Mode != browse.BrowseModeAlbums {
		t.Errorf("browse.mode: want BrowseModeAlbums, got %v", m.browse.Mode)
	}
	if m.browse.Albums.Level != albtui.AlbumLevel {
		t.Errorf("level: want AlbumLevel, got %v", m.browse.Albums.Level)
	}
	if m.browse.Albums.Cursor != 3 {
		t.Errorf("cursor: want 3 (Gamma), got %d", m.browse.Albums.Cursor)
	}
	if len(m.browse.Albums.Albums) != 4 {
		t.Errorf("albums list: want 4, got %d", len(m.browse.Albums.Albums))
	}
	if library.AlbumTitle(m.browse.Albums.Albums[3]) != "Gamma" {
		t.Errorf("albums[3]: want %q, got %q", "Gamma", m.browse.Albums.Albums[3])
	}
}

// TestJumpToAlbum_First verifies cursor positioning at the start of the list.
func TestJumpToAlbum_First(t *testing.T) {
	m := newModelWithTracks()
	m = m.jumpToAlbum("Alpha")

	if m.browse.Albums.Cursor != 0 {
		t.Errorf("cursor: want 0 (Alpha), got %d", m.browse.Albums.Cursor)
	}
}

// TestJumpToAlbum_NotFound verifies that jumpToAlbum is a no-op when the
// album does not exist in the library.
func TestJumpToAlbum_NotFound(t *testing.T) {
	m := newModelWithTracks()
	original := m.ActiveView
	m = m.jumpToAlbum("Nonexistent")

	if m.ActiveView != original {
		t.Errorf("no-op expected: activeView changed to %d", m.ActiveView)
	}
}

// TestJumpToAlbum_UnknownAlbum verifies that an empty album string is mapped
// to the UnknownAlbum sentinel so it can be located in the derived list.
func TestJumpToAlbum_UnknownAlbum(t *testing.T) {
	tracks := []library.Track{
		{Artist: "Alice", Album: "", Title: "No Album Track", Hash: "h1", Path: "/a.flac"},
		{Artist: "Bob", Album: "Gamma", Title: "Bob Track", Hash: "h2", Path: "/b.flac"},
	}
	m := New(context.Background(), WithLibraryTracks(tracks))
	// DeriveAllAlbums sorts unknown albums after named ones: ["Gamma", "(unknown album)"]
	m = m.jumpToAlbum("")

	if m.ActiveView != viewBrowse {
		t.Errorf("activeView: want viewBrowse, got %d", m.ActiveView)
	}
	if m.browse.Mode != browse.BrowseModeAlbums {
		t.Errorf("browse.mode: want BrowseModeAlbums, got %v", m.browse.Mode)
	}
	if m.browse.Albums.Cursor != 1 {
		t.Errorf("cursor: want 1 (%s), got %d", arttui.UnknownAlbum, m.browse.Albums.Cursor)
	}
	if len(m.browse.Albums.Albums) < 2 || library.AlbumTitle(m.browse.Albums.Albums[1]) != "" {
		t.Errorf("albums[1]: want %q, got %q", arttui.UnknownAlbum, m.browse.Albums.Albums[1])
	}
}

// TestJumpToAlbum_CentersScroll verifies the scroll offset centers the row.
func TestJumpToAlbum_CentersScroll(t *testing.T) {
	tracks := make([]library.Track, 20)
	for i := range tracks {
		tracks[i] = library.Track{
			Artist: "Artist",
			Album:  fmt.Sprintf("Album%02d", i),
			Title:  fmt.Sprintf("Track%d", i),
			Hash:   fmt.Sprintf("h%d", i),
			Path:   fmt.Sprintf("/t%d.flac", i),
		}
	}
	m := New(context.Background(), WithLibraryTracks(tracks))
	m.browse.Albums.Height = 10

	m = m.jumpToAlbum("Album15")

	if m.browse.Albums.Cursor != 15 {
		t.Errorf("cursor: want 15, got %d", m.browse.Albums.Cursor)
	}
	wantScroll := max(0, 15-m.browse.Albums.Height/2)
	if m.browse.Albums.ScrollOffset != wantScroll {
		t.Errorf("scrollOffset: want %d, got %d", wantScroll, m.browse.Albums.ScrollOffset)
	}
}

// TestJumpToAlbum_EmptyAlbumViaAction verifies that pressing the jump key on
// a track with an empty album field jumps to the "(unknown album)" row in the
// Albums view rather than being a no-op.
func TestJumpToAlbum_EmptyAlbumViaAction(t *testing.T) {
	tracks := []library.Track{
		{Artist: "Alice", Album: "", Title: "No Album", Hash: "h1", Path: "/a.flac"},
		{Artist: "Bob", Album: "Gamma", Title: "Bob Track", Hash: "h2", Path: "/b.flac"},
	}
	m := New(context.Background(), WithLibraryTracks(tracks))
	// Library index sorts unknown albums after named ones within the same
	// artist. "Alice" (with empty album) sorts before "Bob" alphabetically,
	// so Alice's track is at index 0.
	m.library.Cursor = 0

	ctx := m.focusedGoToContext()
	if !ctx.found {
		t.Fatal("expected focused track to be found")
	}
	if library.AlbumTitle(ctx.album) != "" {
		t.Fatalf("expected empty album, got %q", ctx.album)
	}
	m = m.jumpToAlbum(ctx.album)

	if m.ActiveView != viewBrowse {
		t.Errorf("activeView: want viewBrowse, got %d", m.ActiveView)
	}
	if m.browse.Mode != browse.BrowseModeAlbums {
		t.Errorf("browse.mode: want BrowseModeAlbums, got %v", m.browse.Mode)
	}
	if library.AlbumTitle(m.browse.Albums.Albums[m.browse.Albums.Cursor]) != "" {
		t.Errorf("cursor on %q, want %q", m.browse.Albums.Albums[m.browse.Albums.Cursor], arttui.UnknownAlbum)
	}
}
