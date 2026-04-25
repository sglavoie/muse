package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

// TestFocusedTrackMeta_GenresArtistLevel verifies that focusedTrackMeta returns
// the focused artist name when the Genres view is at ArtistLevel.
func TestFocusedTrackMeta_GenresArtistLevel(t *testing.T) {
	m := newModelWithTracks()
	m.activeView = viewGenres
	m.genres = GenresView{
		level:   gentui.ArtistLevel,
		artists: []string{"Alice", "Bob"},
		cursor:  1,
	}
	artist, album, found := m.focusedTrackMeta()
	if !found {
		t.Fatal("expected found=true at Genres ArtistLevel")
	}
	if artist != "Bob" {
		t.Errorf("artist: want %q, got %q", "Bob", artist)
	}
	if album != "" {
		t.Errorf("album: want empty, got %q", album)
	}
}

// TestFocusedTrackMeta_GenresTrackLevel verifies that focusedTrackMeta returns
// artist and album from the focused track when the Genres view is at TrackLevel.
func TestFocusedTrackMeta_GenresTrackLevel(t *testing.T) {
	m := newModelWithTracks()
	m.activeView = viewGenres
	m.genres = GenresView{
		level: gentui.TrackLevel,
		tracks: []library.Track{
			{Artist: "Carol", Album: "Delta", Title: "Song", Hash: "h1"},
		},
		cursor: 0,
	}
	artist, album, found := m.focusedTrackMeta()
	if !found {
		t.Fatal("expected found=true at Genres TrackLevel")
	}
	if artist != "Carol" {
		t.Errorf("artist: want %q, got %q", "Carol", artist)
	}
	if album != "Delta" {
		t.Errorf("album: want %q, got %q", "Delta", album)
	}
}

// TestFocusedTrackMeta_GenresGenreLevel verifies that focusedTrackMeta returns
// not-found at GenreLevel since genres are not artists or albums.
func TestFocusedTrackMeta_GenresGenreLevel(t *testing.T) {
	m := newModelWithTracks()
	m.activeView = viewGenres
	m.genres = GenresView{
		level:  gentui.GenreLevel,
		genres: []string{"Rock", "Jazz"},
		cursor: 0,
	}
	_, _, found := m.focusedTrackMeta()
	if found {
		t.Error("expected found=false at Genres GenreLevel")
	}
}

// TestFocusedTrackMeta_TagsTrackLevel verifies that focusedTrackMeta returns
// the artist and album of the focused track when the Tags view is at TrackLevel.
func TestFocusedTrackMeta_TagsTrackLevel(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewTags
	m.tagsView = TagsView{
		level: tagtui.TrackLevel,
		tracks: []db.PlaylistTrackRow{
			{Hash: "h1", Artist: "TagArtist", Album: "TagAlbum", Title: "TagSong"},
		},
		cursor: 0,
	}

	artist, album, found := m.focusedTrackMeta()
	if !found {
		t.Fatal("expected found=true at Tags TrackLevel")
	}
	if artist != "TagArtist" {
		t.Errorf("artist: want %q, got %q", "TagArtist", artist)
	}
	if album != "TagAlbum" {
		t.Errorf("album: want %q, got %q", "TagAlbum", album)
	}
}

// TestFocusedTrackMeta_TagsTagLevel verifies that focusedTrackMeta returns
// not-found at TagLevel since tags are not tracks.
func TestFocusedTrackMeta_TagsTagLevel(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewTags
	m.tagsView = TagsView{
		level: tagtui.TagLevel,
		tags:  []db.TagRow{{ID: 1, Name: "MyTag"}},
	}

	_, _, found := m.focusedTrackMeta()
	if found {
		t.Error("expected found=false at Tags TagLevel")
	}
}
