package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
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

	if m.activeView != viewBrowse {
		t.Errorf("activeView: want viewBrowse(%d), got %d", viewBrowse, m.activeView)
	}
	if m.browse.mode != BrowseModeGenres {
		t.Errorf("browse.mode: want BrowseModeGenres, got %v", m.browse.mode)
	}
	if m.browse.genres.level != gentui.GenreLevel {
		t.Errorf("level: want GenreLevel, got %v", m.browse.genres.level)
	}
	// Genres are sorted alphabetically: Jazz, Rock
	if m.browse.genres.cursor != 0 {
		t.Errorf("cursor: want 0 (Jazz), got %d", m.browse.genres.cursor)
	}
}

func TestJumpToGenre_NotFound(t *testing.T) {
	m := newModelWithTracks()
	original := m.activeView
	m = m.jumpToGenre("Nonexistent")
	if m.activeView != original {
		t.Errorf("no-op expected: activeView changed to %d", m.activeView)
	}
}

func TestJumpToGenre_UnknownGenre(t *testing.T) {
	tracks := []library.Track{
		{Artist: "Alice", Album: "Alpha", Genre: "", Title: "T1", Hash: "h1", Path: "/a.flac"},
		{Artist: "Bob", Album: "Beta", Genre: "Rock", Title: "T2", Hash: "h2", Path: "/b.flac"},
	}
	m := New(context.Background(), WithLibraryTracks(tracks))
	m = m.jumpToGenre("")

	if m.activeView != viewBrowse {
		t.Errorf("activeView: want viewBrowse, got %d", m.activeView)
	}
	if m.browse.mode != BrowseModeGenres {
		t.Errorf("browse.mode: want BrowseModeGenres, got %v", m.browse.mode)
	}
	// "(unknown genre)" sorts after "Rock"
	if m.browse.genres.cursor != 1 {
		t.Errorf("cursor: want 1 (unknown genre), got %d", m.browse.genres.cursor)
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

	if m.activeView != viewPlaylists {
		t.Errorf("activeView: want viewPlaylists(%d), got %d", viewPlaylists, m.activeView)
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
	original := m.activeView
	m = m.jumpToPlaylist("Nonexistent")
	if m.activeView != original {
		t.Errorf("no-op expected: activeView changed to %d", m.activeView)
	}
}
