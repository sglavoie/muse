package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
)

// TestJumpToArtist_ExitsVisualMode verifies that visual mode is cleared when
// jumping to an artist so that stale visual state from the source view does
// not bleed into the target view.
func TestJumpToArtist_ExitsVisualMode(t *testing.T) {
	m := newModelWithTracks()
	m.ActiveView = viewLibrary
	m = enterVisualMode(m)
	if !m.visual.active {
		t.Fatal("precondition: visual mode should be active")
	}

	m = m.jumpToArtist("Bob")

	if m.visual.active {
		t.Error("visual mode should be cleared after jumpToArtist")
	}
}

// TestJumpToAlbum_ExitsVisualMode verifies that visual mode is cleared when
// jumping to an album.
func TestJumpToAlbum_ExitsVisualMode(t *testing.T) {
	m := newModelWithTracks()
	m.ActiveView = viewLibrary
	m = enterVisualMode(m)
	if !m.visual.active {
		t.Fatal("precondition: visual mode should be active")
	}

	m = m.jumpToAlbum("Alpha")

	if m.visual.active {
		t.Error("visual mode should be cleared after jumpToAlbum")
	}
}

// TestJumpToGenre_ExitsVisualMode verifies that visual mode is cleared when
// jumping to a genre.
func TestJumpToGenre_ExitsVisualMode(t *testing.T) {
	tracks := []library.Track{
		{Artist: "Alice", Album: "Alpha", Genre: "Rock", Title: "T1", Hash: "h1", Path: "/a.flac"},
		{Artist: "Bob", Album: "Beta", Genre: "Jazz", Title: "T2", Hash: "h2", Path: "/b.flac"},
	}
	m := New(context.Background(), WithLibraryTracks(tracks))
	m.ActiveView = viewLibrary
	m = enterVisualMode(m)
	if !m.visual.active {
		t.Fatal("precondition: visual mode should be active")
	}

	m = m.jumpToGenre("Jazz")

	if m.visual.active {
		t.Error("visual mode should be cleared after jumpToGenre")
	}
}

// TestJumpToPlaylist_ExitsVisualMode verifies that visual mode is cleared when
// jumping to a playlist.
func TestJumpToPlaylist_ExitsVisualMode(t *testing.T) {
	m := newModelWithTracks()
	m.playlists.Playlists = []db.PlaylistRow{
		{ID: 1, Name: "Alpha"},
		{ID: 2, Name: "Beta"},
	}
	m.ActiveView = viewLibrary
	m = enterVisualMode(m)
	if !m.visual.active {
		t.Fatal("precondition: visual mode should be active")
	}

	m = m.jumpToPlaylist("Beta")

	if m.visual.active {
		t.Error("visual mode should be cleared after jumpToPlaylist")
	}
}
