package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/browse"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
)

func TestJumpToGenre(t *testing.T) {
	tracks := []library.Track{
		{Artist: "Alice", Album: "Alpha", Genre: "Rock", Title: "T1", Hash: "h1", Path: "/a.flac"},
		{Artist: "Bob", Album: "Beta", Genre: "Jazz", Title: "T2", Hash: "h2", Path: "/b.flac"},
	}
	m := New(context.Background(), WithLibraryTracks(tracks))
	m = m.jumpToGenre("Jazz")

	if m.ActiveView != viewBrowse {
		t.Errorf("activeView: want viewBrowse(%d), got %d", viewBrowse, m.ActiveView)
	}
	if m.browse.Mode != browse.BrowseModeGenres {
		t.Errorf("browse.mode: want BrowseModeGenres, got %v", m.browse.Mode)
	}
	if m.browse.Genres.Level != gentui.GenreLevel {
		t.Errorf("level: want GenreLevel, got %v", m.browse.Genres.Level)
	}
	// Genres are sorted alphabetically: Jazz, Rock
	if m.browse.Genres.Cursor != 0 {
		t.Errorf("cursor: want 0 (Jazz), got %d", m.browse.Genres.Cursor)
	}
}

func TestJumpToGenre_NotFound(t *testing.T) {
	m := newModelWithTracks()
	original := m.ActiveView
	m = m.jumpToGenre("Nonexistent")
	if m.ActiveView != original {
		t.Errorf("no-op expected: activeView changed to %d", m.ActiveView)
	}
}

func TestJumpToGenre_UnknownGenre(t *testing.T) {
	tracks := []library.Track{
		{Artist: "Alice", Album: "Alpha", Genre: "", Title: "T1", Hash: "h1", Path: "/a.flac"},
		{Artist: "Bob", Album: "Beta", Genre: "Rock", Title: "T2", Hash: "h2", Path: "/b.flac"},
	}
	m := New(context.Background(), WithLibraryTracks(tracks))
	m = m.jumpToGenre("")

	if m.ActiveView != viewBrowse {
		t.Errorf("activeView: want viewBrowse, got %d", m.ActiveView)
	}
	if m.browse.Mode != browse.BrowseModeGenres {
		t.Errorf("browse.mode: want BrowseModeGenres, got %v", m.browse.Mode)
	}
	// "(unknown genre)" sorts after "Rock"
	if m.browse.Genres.Cursor != 1 {
		t.Errorf("cursor: want 1 (unknown genre), got %d", m.browse.Genres.Cursor)
	}
}

func TestJumpToPlaylist(t *testing.T) {
	m := newModelWithTracks()
	m.playlists.Playlists = []db.PlaylistRow{
		{ID: 1, Name: "Alpha"},
		{ID: 2, Name: "Beta"},
		{ID: 3, Name: "Gamma"},
	}
	m = m.jumpToPlaylist("Beta")

	if m.ActiveView != viewPlaylists {
		t.Errorf("activeView: want viewPlaylists(%d), got %d", viewPlaylists, m.ActiveView)
	}
	if m.playlists.Level != pltui.ListLevel {
		t.Errorf("level: want ListLevel, got %v", m.playlists.Level)
	}
	if m.playlists.Cursor != 1 {
		t.Errorf("cursor: want 1 (Beta), got %d", m.playlists.Cursor)
	}
}

func TestJumpToPlaylist_NotFound(t *testing.T) {
	m := newModelWithTracks()
	m.playlists.Playlists = []db.PlaylistRow{{ID: 1, Name: "Alpha"}}
	original := m.ActiveView
	m = m.jumpToPlaylist("Nonexistent")
	if m.ActiveView != original {
		t.Errorf("no-op expected: activeView changed to %d", m.ActiveView)
	}
}
