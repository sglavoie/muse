package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

// TestWatchBatchPreservesQueueSort verifies that a watcher metadata update
// recomputes the queue display order when a sort is active.
func TestWatchBatchPreservesQueueSort(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "Zara", Title: "T1"})
	m.library.index.Insert(library.Track{Hash: "h2", Path: "/b.flac", Artist: "Alice", Title: "T2"})
	m.queue.entries = []QueueEntry{
		{ID: 1, Hash: "h1", Path: "/a.flac", Artist: "Zara", Title: "T1"},
		{ID: 2, Hash: "h2", Path: "/b.flac", Artist: "Alice", Title: "T2"},
	}
	m.queue.sortState = SortState{Field: SortArtist, Dir: SortAsc}
	m.queue = m.queue.recomputeDisplayOrder()

	// Watcher renames h1 to "Aaron" - should cause re-sort.
	next, _ := m.Update(watchBatchMsg{
		tracks: []library.Track{
			{Hash: "h1", Path: "/a.flac", Artist: "Aaron", Title: "T1"},
		},
	})
	nm := next.(Model)

	if nm.queue.displayOrder == nil {
		t.Fatal("expected displayOrder to be non-nil after watcher update with active sort")
	}
	// Aaron(0), Alice(1) -> displayOrder = [0, 1]
	if nm.queue.displayOrder[0] != 0 {
		t.Errorf("expected displayOrder[0]=0 (Aaron), got %d", nm.queue.displayOrder[0])
	}
}

// TestWatchBatchPreservesPlaylistSort verifies that a watcher metadata update
// recomputes the playlist sort when at track level with a sort active.
func TestWatchBatchPreservesPlaylistSort(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "Zara", Title: "T1"})
	m.playlists.level = pltui.TrackLevel
	m.playlists.tracks = []db.PlaylistTrackRow{
		{ID: 1, Hash: "h1", Path: "/a.flac", Artist: "Zara", Title: "T1"},
		{ID: 2, Hash: "h2", Path: "/b.flac", Artist: "Alice", Title: "T2"},
	}
	m.playlists.sortState = SortState{Field: SortArtist, Dir: SortAsc}
	m.playlists = m.playlists.recomputePlaylistSort()

	next, _ := m.Update(watchBatchMsg{
		tracks: []library.Track{
			{Hash: "h1", Path: "/a.flac", Artist: "Aaron", Title: "T1"},
		},
	})
	nm := next.(Model)

	if nm.playlists.sortedTrackOrder == nil {
		t.Fatal("expected sortedTrackOrder to be non-nil after watcher update with active playlist sort")
	}
}

// TestWatchBatchPreservesFavoritesSort verifies that a watcher metadata update
// recomputes the favorites sort when tracks are displayed with a sort active.
func TestWatchBatchPreservesFavoritesSort(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "Zara", Title: "T1"})
	m.favorites.level = favtui.ListLevel
	m.favorites.category = favtui.CategoryTracks
	m.favorites.tracks = []library.Track{
		{Hash: "h1", Path: "/a.flac", Artist: "Zara", Title: "T1"},
		{Hash: "h2", Path: "/b.flac", Artist: "Alice", Title: "T2"},
	}
	m.favorites.sortState = SortState{Field: SortArtist, Dir: SortAsc}
	m.favorites = m.favorites.recomputeFavoritesSort()

	next, _ := m.Update(watchBatchMsg{
		tracks: []library.Track{
			{Hash: "h1", Path: "/a.flac", Artist: "Aaron", Title: "T1"},
		},
	})
	nm := next.(Model)

	// After re-sort: Aaron should come before Alice.
	if nm.favorites.tracks[0].Artist != "Aaron" {
		t.Errorf("expected first track artist=%q, got %q", "Aaron", nm.favorites.tracks[0].Artist)
	}
}

// TestWatchBatchPreservesTagsSort verifies that a watcher metadata update
// recomputes the tags sort when at track level with a sort active.
func TestWatchBatchPreservesTagsSort(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "Zara", Title: "T1"})
	m.tagsView.level = tagtui.TrackLevel
	m.tagsView.tracks = []db.PlaylistTrackRow{
		{ID: 1, Hash: "h1", Path: "/a.flac", Artist: "Zara", Title: "T1"},
		{ID: 2, Hash: "h2", Path: "/b.flac", Artist: "Alice", Title: "T2"},
	}
	m.tagsView.sortState = SortState{Field: SortArtist, Dir: SortAsc}
	m.tagsView = m.tagsView.recomputeTagsSort()

	next, _ := m.Update(watchBatchMsg{
		tracks: []library.Track{
			{Hash: "h1", Path: "/a.flac", Artist: "Aaron", Title: "T1"},
		},
	})
	nm := next.(Model)

	if nm.tagsView.sortedTrackOrder == nil {
		t.Fatal("expected sortedTrackOrder to be non-nil after watcher update with active tags sort")
	}
}

// TestWatchBatchPreservesArtistLevelSort verifies that a watcher event
// preserves a user-applied sort at ArtistLevel instead of resetting it.
func TestWatchBatchPreservesArtistLevelSort(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "Zara", Title: "T1"})
	m.library.index.Insert(library.Track{Hash: "h2", Path: "/b.flac", Artist: "Alice", Title: "T2"})
	m.activeView = viewBrowse
	m.browse.mode = BrowseModeArtists
	m.browse.artists.artists = []string{"Zara", "Alice"}
	// Simulate user sorting artists ascending.
	m.browse.artists.sortState = SortState{Field: SortName, Dir: SortAsc}
	m.browse.artists = m.browse.artists.recomputeArtistsSort()

	if m.browse.artists.artists[0] != "Alice" {
		t.Fatalf("setup: expected first artist=%q after sort, got %q", "Alice", m.browse.artists.artists[0])
	}

	// Watcher adds a new track - sort state should be preserved.
	next, _ := m.Update(watchBatchMsg{
		tracks: []library.Track{
			{Hash: "h3", Path: "/c.flac", Artist: "Bob", Title: "T3"},
		},
	})
	nm := next.(Model)

	if nm.browse.artists.sortState.Field != SortName {
		t.Errorf("sort field = %v, want SortName", nm.browse.artists.sortState.Field)
	}
	// After re-derive + re-sort: Alice, Bob, Zara.
	if len(nm.browse.artists.artists) != 3 {
		t.Fatalf("expected 3 artists, got %d", len(nm.browse.artists.artists))
	}
	if nm.browse.artists.artists[0] != "Alice" {
		t.Errorf("expected first artist=%q, got %q", "Alice", nm.browse.artists.artists[0])
	}
	if nm.browse.artists.artists[1] != "Bob" {
		t.Errorf("expected second artist=%q, got %q", "Bob", nm.browse.artists.artists[1])
	}
}

// TestWatchBatchPreservesAlbumLevelSort verifies that a watcher event
// preserves a user-applied sort at AlbumLevel (Albums view) instead of
// resetting it.
func TestWatchBatchPreservesAlbumLevelSort(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "Art", Title: "T1", Album: "Zeta"})
	m.library.index.Insert(library.Track{Hash: "h2", Path: "/b.flac", Artist: "Art", Title: "T2", Album: "Alpha"})
	m.activeView = viewBrowse
	m.browse.mode = BrowseModeAlbums
	m.browse.albums.albums = []string{"Zeta", "Alpha"}
	m.browse.albums.sortState = SortState{Field: SortName, Dir: SortAsc}
	m.browse.albums = m.browse.albums.recomputeAlbumsSort()

	if m.browse.albums.albums[0] != "Alpha" {
		t.Fatalf("setup: expected first album=%q after sort, got %q", "Alpha", m.browse.albums.albums[0])
	}

	next, _ := m.Update(watchBatchMsg{
		tracks: []library.Track{
			{Hash: "h3", Path: "/c.flac", Artist: "Art", Title: "T3", Album: "Beta"},
		},
	})
	nm := next.(Model)

	if nm.browse.albums.sortState.Field != SortName {
		t.Errorf("sort field = %v, want SortName", nm.browse.albums.sortState.Field)
	}
	if len(nm.browse.albums.albums) != 3 {
		t.Fatalf("expected 3 albums, got %d", len(nm.browse.albums.albums))
	}
	if nm.browse.albums.albums[0] != "Alpha" {
		t.Errorf("expected first album=%q, got %q", "Alpha", nm.browse.albums.albums[0])
	}
	if nm.browse.albums.albums[1] != "Beta" {
		t.Errorf("expected second album=%q, got %q", "Beta", nm.browse.albums.albums[1])
	}
}

// TestWatchBatchPreservesGenreLevelSort verifies that a watcher event
// preserves a user-applied sort at GenreLevel instead of resetting it.
func TestWatchBatchPreservesGenreLevelSort(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "Art", Title: "T1", Genre: "Zydeco"})
	m.library.index.Insert(library.Track{Hash: "h2", Path: "/b.flac", Artist: "Art", Title: "T2", Genre: "Alternative"})
	m.activeView = viewBrowse
	m.browse.mode = BrowseModeGenres
	m.browse.genres.genres = []string{"Zydeco", "Alternative"}
	m.browse.genres.sortState = SortState{Field: SortName, Dir: SortAsc}
	m.browse.genres = m.browse.genres.recomputeGenresSort()

	if m.browse.genres.genres[0] != "Alternative" {
		t.Fatalf("setup: expected first genre=%q after sort, got %q", "Alternative", m.browse.genres.genres[0])
	}

	next, _ := m.Update(watchBatchMsg{
		tracks: []library.Track{
			{Hash: "h3", Path: "/c.flac", Artist: "Art", Title: "T3", Genre: "Blues"},
		},
	})
	nm := next.(Model)

	if nm.browse.genres.sortState.Field != SortName {
		t.Errorf("sort field = %v, want SortName", nm.browse.genres.sortState.Field)
	}
	if len(nm.browse.genres.genres) != 3 {
		t.Fatalf("expected 3 genres, got %d", len(nm.browse.genres.genres))
	}
	if nm.browse.genres.genres[0] != "Alternative" {
		t.Errorf("expected first genre=%q, got %q", "Alternative", nm.browse.genres.genres[0])
	}
	if nm.browse.genres.genres[1] != "Blues" {
		t.Errorf("expected second genre=%q, got %q", "Blues", nm.browse.genres.genres[1])
	}
}
