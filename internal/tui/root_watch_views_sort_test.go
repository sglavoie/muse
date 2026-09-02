package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/browse"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
	"github.com/sglavoie/muse/internal/tui/queue"
	"github.com/sglavoie/muse/internal/tui/sorting"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

// TestWatchBatchPreservesQueueSort verifies that a watcher metadata update
// recomputes the queue display order when a sort is active.
func TestWatchBatchPreservesQueueSort(t *testing.T) {
	m := New(context.Background())
	m.library.Index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "Zara", Title: "T1"})
	m.library.Index.Insert(library.Track{Hash: "h2", Path: "/b.flac", Artist: "Alice", Title: "T2"})
	m.queue.Entries = []queue.Entry{
		{ID: 1, Hash: "h1", Path: "/a.flac", Artist: "Zara", Title: "T1"},
		{ID: 2, Hash: "h2", Path: "/b.flac", Artist: "Alice", Title: "T2"},
	}
	m.queue.SortState = sorting.SortState{Field: sorting.SortArtist, Dir: sorting.SortAsc}
	m.queue = m.queue.RecomputeDisplayOrder()

	// Watcher renames h1 to "Aaron" - should cause re-sort.
	next, _ := m.Update(watchBatchMsg{
		tracks: []library.Track{
			{Hash: "h1", Path: "/a.flac", Artist: "Aaron", Title: "T1"},
		},
	})
	nm := next.(Model)

	if nm.queue.DisplayOrder == nil {
		t.Fatal("expected displayOrder to be non-nil after watcher update with active sort")
	}
	// Aaron(0), Alice(1) -> displayOrder = [0, 1]
	if nm.queue.DisplayOrder[0] != 0 {
		t.Errorf("expected displayOrder[0]=0 (Aaron), got %d", nm.queue.DisplayOrder[0])
	}
}

// TestWatchBatchPreservesPlaylistSort verifies that a watcher metadata update
// recomputes the playlist sort when at track level with a sort active.
func TestWatchBatchPreservesPlaylistSort(t *testing.T) {
	m := New(context.Background())
	m.library.Index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "Zara", Title: "T1"})
	m.playlists.Level = pltui.TrackLevel
	m.playlists.Tracks = []db.PlaylistTrackRow{
		{ID: 1, Hash: "h1", Path: "/a.flac", Artist: "Zara", Title: "T1"},
		{ID: 2, Hash: "h2", Path: "/b.flac", Artist: "Alice", Title: "T2"},
	}
	m.playlists.SortState = sorting.SortState{Field: sorting.SortArtist, Dir: sorting.SortAsc}
	m.playlists = m.playlists.RecomputePlaylistSort()

	next, _ := m.Update(watchBatchMsg{
		tracks: []library.Track{
			{Hash: "h1", Path: "/a.flac", Artist: "Aaron", Title: "T1"},
		},
	})
	nm := next.(Model)

	if nm.playlists.SortedTrackOrder == nil {
		t.Fatal("expected sortedTrackOrder to be non-nil after watcher update with active playlist sort")
	}
}

// TestWatchBatchPreservesFavoritesSort verifies that a watcher metadata update
// recomputes the favorites sort when tracks are displayed with a sort active.
func TestWatchBatchPreservesFavoritesSort(t *testing.T) {
	m := New(context.Background())
	m.library.Index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "Zara", Title: "T1"})
	m.favorites.Level = favtui.ListLevel
	m.favorites.Category = favtui.CategoryTracks
	m.favorites.Tracks = []library.Track{
		{Hash: "h1", Path: "/a.flac", Artist: "Zara", Title: "T1"},
		{Hash: "h2", Path: "/b.flac", Artist: "Alice", Title: "T2"},
	}
	m.favorites.SortState = sorting.SortState{Field: sorting.SortArtist, Dir: sorting.SortAsc}
	m.favorites = m.favorites.RecomputeFavoritesSort()

	next, _ := m.Update(watchBatchMsg{
		tracks: []library.Track{
			{Hash: "h1", Path: "/a.flac", Artist: "Aaron", Title: "T1"},
		},
	})
	nm := next.(Model)

	// After re-sort: Aaron should come before Alice.
	if nm.favorites.Tracks[0].Artist != "Aaron" {
		t.Errorf("expected first track artist=%q, got %q", "Aaron", nm.favorites.Tracks[0].Artist)
	}
}

// TestWatchBatchPreservesTagsSort verifies that a watcher metadata update
// recomputes the tags sort when at track level with a sort active.
func TestWatchBatchPreservesTagsSort(t *testing.T) {
	m := New(context.Background())
	m.library.Index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "Zara", Title: "T1"})
	m.tagsView.Level = tagtui.TrackLevel
	m.tagsView.Tracks = []db.PlaylistTrackRow{
		{ID: 1, Hash: "h1", Path: "/a.flac", Artist: "Zara", Title: "T1"},
		{ID: 2, Hash: "h2", Path: "/b.flac", Artist: "Alice", Title: "T2"},
	}
	m.tagsView.SortState = sorting.SortState{Field: sorting.SortArtist, Dir: sorting.SortAsc}
	m.tagsView = m.tagsView.RecomputeTagsSort()

	next, _ := m.Update(watchBatchMsg{
		tracks: []library.Track{
			{Hash: "h1", Path: "/a.flac", Artist: "Aaron", Title: "T1"},
		},
	})
	nm := next.(Model)

	if nm.tagsView.SortedTrackOrder == nil {
		t.Fatal("expected sortedTrackOrder to be non-nil after watcher update with active tags sort")
	}
}

// TestWatchBatchPreservesArtistLevelSort verifies that a watcher event
// preserves a user-applied sort at ArtistLevel instead of resetting it.
func TestWatchBatchPreservesArtistLevelSort(t *testing.T) {
	m := New(context.Background())
	m.library.Index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "Zara", Title: "T1"})
	m.library.Index.Insert(library.Track{Hash: "h2", Path: "/b.flac", Artist: "Alice", Title: "T2"})
	m.ActiveView = viewBrowse
	m.browse.Mode = browse.BrowseModeArtists
	m.browse.Artists.Artists = []string{"Zara", "Alice"}
	// Simulate user sorting artists ascending.
	m.browse.Artists.SortState = sorting.SortState{Field: sorting.SortName, Dir: sorting.SortAsc}
	m.browse.Artists = m.browse.Artists.RecomputeArtistsSort()

	if m.browse.Artists.Artists[0] != "Alice" {
		t.Fatalf("setup: expected first artist=%q after sort, got %q", "Alice", m.browse.Artists.Artists[0])
	}

	// Watcher adds a new track - sort state should be preserved.
	next, _ := m.Update(watchBatchMsg{
		tracks: []library.Track{
			{Hash: "h3", Path: "/c.flac", Artist: "Bob", Title: "T3"},
		},
	})
	nm := next.(Model)

	if nm.browse.Artists.SortState.Field != sorting.SortName {
		t.Errorf("sort field = %v, want SortName", nm.browse.Artists.SortState.Field)
	}
	// After re-derive + re-sort: Alice, Bob, Zara.
	if len(nm.browse.Artists.Artists) != 3 {
		t.Fatalf("expected 3 artists, got %d", len(nm.browse.Artists.Artists))
	}
	if nm.browse.Artists.Artists[0] != "Alice" {
		t.Errorf("expected first artist=%q, got %q", "Alice", nm.browse.Artists.Artists[0])
	}
	if nm.browse.Artists.Artists[1] != "Bob" {
		t.Errorf("expected second artist=%q, got %q", "Bob", nm.browse.Artists.Artists[1])
	}
}

// TestWatchBatchPreservesAlbumLevelSort verifies that a watcher event
// preserves a user-applied sort at AlbumLevel (Albums view) instead of
// resetting it.
func TestWatchBatchPreservesAlbumLevelSort(t *testing.T) {
	m := New(context.Background())
	m.library.Index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "Art", Title: "T1", Album: "Zeta"})
	m.library.Index.Insert(library.Track{Hash: "h2", Path: "/b.flac", Artist: "Art", Title: "T2", Album: "Alpha"})
	m.ActiveView = viewBrowse
	m.browse.Mode = browse.BrowseModeAlbums
	m.browse.Albums.Albums = []string{"Zeta", "Alpha"}
	m.browse.Albums.SortState = sorting.SortState{Field: sorting.SortName, Dir: sorting.SortAsc}
	m.browse.Albums = m.browse.Albums.RecomputeAlbumsSort()

	if m.browse.Albums.Albums[0] != "Alpha" {
		t.Fatalf("setup: expected first album=%q after sort, got %q", "Alpha", m.browse.Albums.Albums[0])
	}

	next, _ := m.Update(watchBatchMsg{
		tracks: []library.Track{
			{Hash: "h3", Path: "/c.flac", Artist: "Art", Title: "T3", Album: "Beta"},
		},
	})
	nm := next.(Model)

	if nm.browse.Albums.SortState.Field != sorting.SortName {
		t.Errorf("sort field = %v, want SortName", nm.browse.Albums.SortState.Field)
	}
	if len(nm.browse.Albums.Albums) != 3 {
		t.Fatalf("expected 3 albums, got %d", len(nm.browse.Albums.Albums))
	}
	if nm.browse.Albums.Albums[0] != "Alpha" {
		t.Errorf("expected first album=%q, got %q", "Alpha", nm.browse.Albums.Albums[0])
	}
	if nm.browse.Albums.Albums[1] != "Beta" {
		t.Errorf("expected second album=%q, got %q", "Beta", nm.browse.Albums.Albums[1])
	}
}

// TestWatchBatchPreservesGenreLevelSort verifies that a watcher event
// preserves a user-applied sort at GenreLevel instead of resetting it.
func TestWatchBatchPreservesGenreLevelSort(t *testing.T) {
	m := New(context.Background())
	m.library.Index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "Art", Title: "T1", Genre: "Zydeco"})
	m.library.Index.Insert(library.Track{Hash: "h2", Path: "/b.flac", Artist: "Art", Title: "T2", Genre: "Alternative"})
	m.ActiveView = viewBrowse
	m.browse.Mode = browse.BrowseModeGenres
	m.browse.Genres.Genres = []string{"Zydeco", "Alternative"}
	m.browse.Genres.SortState = sorting.SortState{Field: sorting.SortName, Dir: sorting.SortAsc}
	m.browse.Genres = m.browse.Genres.RecomputeGenresSort()

	if m.browse.Genres.Genres[0] != "Alternative" {
		t.Fatalf("setup: expected first genre=%q after sort, got %q", "Alternative", m.browse.Genres.Genres[0])
	}

	next, _ := m.Update(watchBatchMsg{
		tracks: []library.Track{
			{Hash: "h3", Path: "/c.flac", Artist: "Art", Title: "T3", Genre: "Blues"},
		},
	})
	nm := next.(Model)

	if nm.browse.Genres.SortState.Field != sorting.SortName {
		t.Errorf("sort field = %v, want SortName", nm.browse.Genres.SortState.Field)
	}
	if len(nm.browse.Genres.Genres) != 3 {
		t.Fatalf("expected 3 genres, got %d", len(nm.browse.Genres.Genres))
	}
	if nm.browse.Genres.Genres[0] != "Alternative" {
		t.Errorf("expected first genre=%q, got %q", "Alternative", nm.browse.Genres.Genres[0])
	}
	if nm.browse.Genres.Genres[1] != "Blues" {
		t.Errorf("expected second genre=%q, got %q", "Blues", nm.browse.Genres.Genres[1])
	}
}
