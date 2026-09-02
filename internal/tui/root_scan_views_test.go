package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/sorting"
)

// TestScanBatchRefreshesArtistAlbumLevel verifies that when the artists view
// is drilled into AlbumLevel, a ScanBatchMsg re-derives the album list so
// newly scanned albums appear.
func TestScanBatchRefreshesArtistAlbumLevel(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Artist: "Art", Title: "T1", Path: "/a", Hash: "h1", Album: "Album1"})
	// Drill into "Art" albums.
	m.activeView = viewBrowse
	m.browse.mode = BrowseModeArtists
	m.browse.artists.artists = []string{"Art"}
	m.browse.artists.selectedArtist = "Art"
	m.browse.artists = m.browse.artists.openAlbums([]string{"Album1"})

	// Scan batch adds a second track with a new album.
	next, _ := m.Update(library.ScanBatchMsg{
		Tracks: []library.Track{
			{Artist: "Art", Title: "T2", Path: "/b", Hash: "h2", Album: "Album2"},
		},
	})
	nm := next.(Model)

	if len(nm.browse.artists.albums) != 2 {
		t.Errorf("expected 2 albums after scan, got %d: %v", len(nm.browse.artists.albums), nm.browse.artists.albums)
	}
}

// TestScanBatchRefreshesArtistTrackLevel verifies that when the artists view
// is drilled into TrackLevel, a ScanBatchMsg re-derives the track list.
func TestScanBatchRefreshesArtistTrackLevel(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Artist: "Art", Title: "T1", Path: "/a", Hash: "h1", Album: "Alb"})
	// Drill into "Art" > "Alb" tracks.
	m.activeView = viewBrowse
	m.browse.mode = BrowseModeArtists
	m.browse.artists.artists = []string{"Art"}
	m.browse.artists.selectedArtist = "Art"
	m.browse.artists = m.browse.artists.openAlbums([]string{"Alb"})
	m.browse.artists.selectedAlbum = "Alb"
	m.browse.artists = m.browse.artists.openTracks([]library.Track{
		{Artist: "Art", Title: "T1", Path: "/a", Hash: "h1", Album: "Alb"},
	})

	// Scan batch adds a new track under the same artist+album.
	next, _ := m.Update(library.ScanBatchMsg{
		Tracks: []library.Track{
			{Artist: "Art", Title: "T2", Path: "/b", Hash: "h2", Album: "Alb"},
		},
	})
	nm := next.(Model)

	if len(nm.browse.artists.tracks) != 2 {
		t.Errorf("expected 2 tracks after scan, got %d", len(nm.browse.artists.tracks))
	}
}

// TestScanBatchRefreshesAlbumTrackLevel verifies that when the albums view
// is drilled into TrackLevel, a ScanBatchMsg re-derives the track list.
func TestScanBatchRefreshesAlbumTrackLevel(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Artist: "Art", Title: "T1", Path: "/a", Hash: "h1", Album: "Alb"})
	// Drill into "Alb" tracks.
	m.activeView = viewBrowse
	m.browse.mode = BrowseModeAlbums
	m.browse.albums.albums = []string{"Alb"}
	m.browse.albums.selectedAlbum = "Alb"
	m.browse.albums = m.browse.albums.openTracks([]library.Track{
		{Artist: "Art", Title: "T1", Path: "/a", Hash: "h1", Album: "Alb"},
	})

	// Scan batch adds a new track under the same album.
	next, _ := m.Update(library.ScanBatchMsg{
		Tracks: []library.Track{
			{Artist: "Art2", Title: "T2", Path: "/b", Hash: "h2", Album: "Alb"},
		},
	})
	nm := next.(Model)

	if len(nm.browse.albums.tracks) != 2 {
		t.Errorf("expected 2 tracks after scan, got %d", len(nm.browse.albums.tracks))
	}
}

// TestScanBatchPreservesArtistLevelSort verifies that an active sort on the
// artists view at ArtistLevel is preserved across scan batches.
func TestScanBatchPreservesArtistLevelSort(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "Zara", Title: "T1"})
	m.library.index.Insert(library.Track{Hash: "h2", Path: "/b.flac", Artist: "Alice", Title: "T2"})
	m.activeView = viewBrowse
	m.browse.mode = BrowseModeArtists
	m.browse.artists.artists = []string{"Zara", "Alice"}
	m.browse.artists.sortState = sorting.SortState{Field: sorting.SortName, Dir: sorting.SortAsc}
	m.browse.artists = m.browse.artists.recomputeArtistsSort()

	// Scan batch adds a new artist.
	next, _ := m.Update(library.ScanBatchMsg{
		Tracks: []library.Track{
			{Hash: "h3", Path: "/c.flac", Artist: "Bob", Title: "T3"},
		},
	})
	nm := next.(Model)

	if nm.browse.artists.sortState.Field != sorting.SortName {
		t.Fatalf("sort field reset; got %v", nm.browse.artists.sortState.Field)
	}
	if len(nm.browse.artists.artists) < 3 {
		t.Fatalf("expected at least 3 artists, got %d", len(nm.browse.artists.artists))
	}
	// With ascending name sort: Alice, Bob, Zara.
	if nm.browse.artists.artists[0] != "Alice" || nm.browse.artists.artists[1] != "Bob" || nm.browse.artists.artists[2] != "Zara" {
		t.Errorf("unexpected artist order: %v", nm.browse.artists.artists)
	}
}

// TestScanBatchPreservesAlbumLevelSort verifies that an active sort on the
// albums view at AlbumLevel is preserved across scan batches.
func TestScanBatchPreservesAlbumLevelSort(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "A", Album: "Zebra", Title: "T1"})
	m.library.index.Insert(library.Track{Hash: "h2", Path: "/b.flac", Artist: "A", Album: "Alpha", Title: "T2"})
	m.activeView = viewBrowse
	m.browse.mode = BrowseModeAlbums
	m.browse.albums.albums = []string{"Zebra", "Alpha"}
	m.browse.albums.sortState = sorting.SortState{Field: sorting.SortName, Dir: sorting.SortAsc}
	m.browse.albums = m.browse.albums.recomputeAlbumsSort()

	next, _ := m.Update(library.ScanBatchMsg{
		Tracks: []library.Track{
			{Hash: "h3", Path: "/c.flac", Artist: "A", Album: "Middle", Title: "T3"},
		},
	})
	nm := next.(Model)

	if nm.browse.albums.sortState.Field != sorting.SortName {
		t.Fatalf("sort field reset; got %v", nm.browse.albums.sortState.Field)
	}
	if len(nm.browse.albums.albums) < 3 {
		t.Fatalf("expected at least 3 albums, got %d", len(nm.browse.albums.albums))
	}
	// Ascending: Alpha, Middle, Zebra.
	if nm.browse.albums.albums[0] != "Alpha" || nm.browse.albums.albums[1] != "Middle" || nm.browse.albums.albums[2] != "Zebra" {
		t.Errorf("unexpected album order: %v", nm.browse.albums.albums)
	}
}

// TestScanBatchPreservesGenreLevelSort verifies that an active sort on the
// genres view at GenreLevel is preserved across scan batches.
func TestScanBatchPreservesGenreLevelSort(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Genre: "Rock", Title: "T1"})
	m.library.index.Insert(library.Track{Hash: "h2", Path: "/b.flac", Genre: "Jazz", Title: "T2"})
	m.activeView = viewBrowse
	m.browse.mode = BrowseModeGenres
	m.browse.genres.genres = []string{"Rock", "Jazz"}
	m.browse.genres.sortState = sorting.SortState{Field: sorting.SortName, Dir: sorting.SortAsc}
	m.browse.genres = m.browse.genres.recomputeGenresSort()

	next, _ := m.Update(library.ScanBatchMsg{
		Tracks: []library.Track{
			{Hash: "h3", Path: "/c.flac", Genre: "Pop", Title: "T3"},
		},
	})
	nm := next.(Model)

	if nm.browse.genres.sortState.Field != sorting.SortName {
		t.Fatalf("sort field reset; got %v", nm.browse.genres.sortState.Field)
	}
	if len(nm.browse.genres.genres) < 3 {
		t.Fatalf("expected at least 3 genres, got %d", len(nm.browse.genres.genres))
	}
	// Ascending: Jazz, Pop, Rock.
	if nm.browse.genres.genres[0] != "Jazz" || nm.browse.genres.genres[1] != "Pop" || nm.browse.genres.genres[2] != "Rock" {
		t.Errorf("unexpected genre order: %v", nm.browse.genres.genres)
	}
}
