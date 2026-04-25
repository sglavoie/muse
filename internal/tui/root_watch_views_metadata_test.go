package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
)

// TestWatchBatchMsgUpdatesPlaylistTracks verifies that a watcher metadata
// update refreshes in-memory playlist track metadata when the playlists
// view is at TrackLevel.
func TestWatchBatchMsgUpdatesPlaylistTracks(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "OldArt", Title: "OldTitle", Album: "OldAlbum"})
	m.playlists.level = pltui.TrackLevel
	m.playlists.tracks = []db.PlaylistTrackRow{
		{ID: 1, Hash: "h1", Path: "/a.flac", Artist: "OldArt", Title: "OldTitle", Album: "OldAlbum"},
	}

	next, _ := m.Update(watchBatchMsg{
		tracks: []library.Track{
			{Hash: "h1", Path: "/a.flac", Artist: "NewArt", Title: "NewTitle", Album: "NewAlbum"},
		},
	})
	nm := next.(Model)

	if nm.playlists.tracks[0].Title != "NewTitle" {
		t.Errorf("playlist track title = %q, want %q", nm.playlists.tracks[0].Title, "NewTitle")
	}
	if nm.playlists.tracks[0].Artist != "NewArt" {
		t.Errorf("playlist track artist = %q, want %q", nm.playlists.tracks[0].Artist, "NewArt")
	}
	if nm.playlists.tracks[0].Album != "NewAlbum" {
		t.Errorf("playlist track album = %q, want %q", nm.playlists.tracks[0].Album, "NewAlbum")
	}
}

// TestWatchBatchMsgUpdatesFavoritesTracks verifies that a watcher metadata
// update refreshes in-memory favorites track metadata when the favorites
// view is at ListLevel with CategoryTracks.
func TestWatchBatchMsgUpdatesFavoritesTracks(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "OldArt", Title: "OldTitle", Album: "OldAlbum"})
	m.favorites.level = favtui.ListLevel
	m.favorites.category = favtui.CategoryTracks
	m.favorites.tracks = []library.Track{
		{Hash: "h1", Path: "/a.flac", Artist: "OldArt", Title: "OldTitle", Album: "OldAlbum"},
	}

	next, _ := m.Update(watchBatchMsg{
		tracks: []library.Track{
			{Hash: "h1", Path: "/a.flac", Artist: "NewArt", Title: "NewTitle", Album: "NewAlbum"},
		},
	})
	nm := next.(Model)

	if nm.favorites.tracks[0].Title != "NewTitle" {
		t.Errorf("favorites track title = %q, want %q", nm.favorites.tracks[0].Title, "NewTitle")
	}
	if nm.favorites.tracks[0].Artist != "NewArt" {
		t.Errorf("favorites track artist = %q, want %q", nm.favorites.tracks[0].Artist, "NewArt")
	}
}

// TestWatchBatchMsgUpdatesFavoritesDrilldownTracks verifies that a watcher
// metadata update refreshes in-memory favorites drilldown track metadata
// when the favorites view is at TrackLevel.
func TestWatchBatchMsgUpdatesFavoritesDrilldownTracks(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "OldArt", Title: "OldTitle", Album: "OldAlbum"})
	m.favorites.level = favtui.TrackLevel
	m.favorites.drilldownTracks = []library.Track{
		{Hash: "h1", Path: "/a.flac", Artist: "OldArt", Title: "OldTitle", Album: "OldAlbum"},
	}

	next, _ := m.Update(watchBatchMsg{
		tracks: []library.Track{
			{Hash: "h1", Path: "/a.flac", Artist: "NewArt", Title: "NewTitle", Album: "NewAlbum"},
		},
	})
	nm := next.(Model)

	if nm.favorites.drilldownTracks[0].Title != "NewTitle" {
		t.Errorf("drilldown track title = %q, want %q", nm.favorites.drilldownTracks[0].Title, "NewTitle")
	}
	if nm.favorites.drilldownTracks[0].Artist != "NewArt" {
		t.Errorf("drilldown track artist = %q, want %q", nm.favorites.drilldownTracks[0].Artist, "NewArt")
	}
}

// TestWatchBatchUpdatesHistoryTracks verifies that a watcher metadata update
// refreshes in-memory history track metadata.
func TestWatchBatchUpdatesHistoryTracks(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "OldArt", Title: "OldTitle", Album: "OldAlbum", Genre: "OldGenre"})
	m.history.sourceTracks = []library.Track{
		{Hash: "h1", Path: "/a.flac", Artist: "OldArt", Title: "OldTitle", Album: "OldAlbum", Genre: "OldGenre"},
	}
	m.history.sortState = SortState{Field: SortTitle, Dir: SortAsc}
	m.history = m.history.recomputeSortedTracks()

	next, _ := m.Update(watchBatchMsg{
		tracks: []library.Track{
			{Hash: "h1", Path: "/a.flac", Artist: "NewArt", Title: "NewTitle", Album: "NewAlbum", Genre: "NewGenre"},
		},
	})
	nm := next.(Model)

	if nm.history.sourceTracks[0].Title != "NewTitle" {
		t.Errorf("history track title = %q, want %q", nm.history.sourceTracks[0].Title, "NewTitle")
	}
	if nm.history.sourceTracks[0].Artist != "NewArt" {
		t.Errorf("history track artist = %q, want %q", nm.history.sourceTracks[0].Artist, "NewArt")
	}
	if nm.history.sourceTracks[0].Genre != "NewGenre" {
		t.Errorf("history track genre = %q, want %q", nm.history.sourceTracks[0].Genre, "NewGenre")
	}
}
