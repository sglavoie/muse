package tui

import (
	"context"
	"fmt"
	"testing"

	"github.com/sglavoie/muse/internal/library"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
)

// TestJumpToArtist verifies that jumpToArtist switches to the Artists view
// with the cursor pre-positioned on the named artist.
func TestJumpToArtist(t *testing.T) {
	m := newModelWithTracks()
	// DeriveArtists: ["Alice"(0), "Bob"(1), "Charlie"(2)]
	m = m.jumpToArtist("Bob")

	if m.activeView != viewBrowse {
		t.Errorf("activeView: want viewBrowse(%d), got %d", viewBrowse, m.activeView)
	}
	if m.browse.mode != BrowseModeArtists {
		t.Errorf("browse.mode: want BrowseModeArtists, got %v", m.browse.mode)
	}
	if m.browse.artists.level != arttui.ArtistLevel {
		t.Errorf("level: want ArtistLevel, got %v", m.browse.artists.level)
	}
	if m.browse.artists.cursor != 1 {
		t.Errorf("cursor: want 1 (Bob), got %d", m.browse.artists.cursor)
	}
	if len(m.browse.artists.artists) != 3 {
		t.Errorf("artists list: want 3, got %d", len(m.browse.artists.artists))
	}
	if m.browse.artists.artists[1] != "Bob" {
		t.Errorf("artists[1]: want %q, got %q", "Bob", m.browse.artists.artists[1])
	}
}

// TestJumpToArtist_First verifies cursor positioning at the start of the list.
func TestJumpToArtist_First(t *testing.T) {
	m := newModelWithTracks()
	m = m.jumpToArtist("Alice")

	if m.browse.artists.cursor != 0 {
		t.Errorf("cursor: want 0 (Alice), got %d", m.browse.artists.cursor)
	}
}

// TestJumpToArtist_NotFound verifies that jumpToArtist is a no-op when the
// artist does not exist in the library.
func TestJumpToArtist_NotFound(t *testing.T) {
	m := newModelWithTracks()
	original := m.activeView
	m = m.jumpToArtist("Nobody")

	if m.activeView != original {
		t.Errorf("no-op expected: activeView changed to %d", m.activeView)
	}
}

// TestJumpToArtist_UnknownArtist verifies that an empty artist string is mapped
// to the UnknownArtist sentinel so it can still be located in the derived list.
func TestJumpToArtist_UnknownArtist(t *testing.T) {
	tracks := []library.Track{
		{Artist: "", Album: "Mystery", Title: "Anonymous Track", Hash: "h1", Path: "/a.flac"},
		{Artist: "Bob", Album: "Gamma", Title: "Bob Track", Hash: "h2", Path: "/b.flac"},
	}
	m := New(context.Background(), WithLibraryTracks(tracks))
	// LibraryIndex sorts unknown artists after named artists.
	// DeriveArtists preserves that order: ["Bob", "(unknown artist)"]
	m = m.jumpToArtist("")

	if m.activeView != viewBrowse {
		t.Errorf("activeView: want viewBrowse, got %d", m.activeView)
	}
	if m.browse.mode != BrowseModeArtists {
		t.Errorf("browse.mode: want BrowseModeArtists, got %v", m.browse.mode)
	}
	if m.browse.artists.cursor != 1 {
		t.Errorf("cursor: want 1 (%s), got %d", arttui.UnknownArtist, m.browse.artists.cursor)
	}
	if len(m.browse.artists.artists) < 2 || m.browse.artists.artists[1] != arttui.UnknownArtist {
		t.Errorf("artists[1]: want %q, got %q", arttui.UnknownArtist, m.browse.artists.artists[1])
	}
}

// TestJumpToArtist_CentersScroll verifies that the scroll offset is computed
// to center the target row within the visible area.
func TestJumpToArtist_CentersScroll(t *testing.T) {
	tracks := make([]library.Track, 20)
	for i := range tracks {
		tracks[i] = library.Track{
			Artist: fmt.Sprintf("Artist%02d", i),
			Album:  "Album",
			Title:  fmt.Sprintf("Track%d", i),
			Hash:   fmt.Sprintf("h%d", i),
			Path:   fmt.Sprintf("/t%d.flac", i),
		}
	}
	m := New(context.Background(), WithLibraryTracks(tracks))
	m.browse.artists.height = 10

	m = m.jumpToArtist("Artist15")

	if m.browse.artists.cursor != 15 {
		t.Errorf("cursor: want 15, got %d", m.browse.artists.cursor)
	}
	wantScroll := max(0, 15-m.browse.artists.height/2)
	if m.browse.artists.scrollOffset != wantScroll {
		t.Errorf("scrollOffset: want %d, got %d", wantScroll, m.browse.artists.scrollOffset)
	}
}

// TestJumpToArtist_EmptyArtistViaAction verifies that pressing the jump key on
// a track with an empty artist field jumps to the "(unknown artist)" row in the
// Artists view rather than being a no-op.
func TestJumpToArtist_EmptyArtistViaAction(t *testing.T) {
	tracks := []library.Track{
		{Artist: "", Album: "Mystery", Title: "Anonymous", Hash: "h1", Path: "/a.flac"},
		{Artist: "Bob", Album: "Gamma", Title: "Bob Track", Hash: "h2", Path: "/b.flac"},
	}
	m := New(context.Background(), WithLibraryTracks(tracks))
	// Library index sorts unknown artists after named ones, so cursor 1 is
	// the track with Artist=="".
	m.library.Cursor = 1

	ctx := m.focusedGoToContext()
	if !ctx.found {
		t.Fatal("expected focused track to be found")
	}
	if ctx.artist != "" {
		t.Fatalf("expected empty artist, got %q", ctx.artist)
	}
	m = m.jumpToArtist(ctx.artist)

	if m.activeView != viewBrowse {
		t.Errorf("activeView: want viewBrowse, got %d", m.activeView)
	}
	if m.browse.mode != BrowseModeArtists {
		t.Errorf("browse.mode: want BrowseModeArtists, got %v", m.browse.mode)
	}
	if m.browse.artists.artists[m.browse.artists.cursor] != arttui.UnknownArtist {
		t.Errorf("cursor on %q, want %q", m.browse.artists.artists[m.browse.artists.cursor], arttui.UnknownArtist)
	}
}
