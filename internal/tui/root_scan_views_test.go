package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/library"
)

// TestScanBatchRefreshesArtistAlbumLevel verifies that when the artists view
// is drilled into AlbumLevel, a ScanBatchMsg re-derives the album list so
// newly scanned albums appear.
func TestScanBatchRefreshesArtistAlbumLevel(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Artist: "Art", Title: "T1", Path: "/a", Hash: "h1", Album: "Album1"})
	// Drill into "Art" albums.
	m.activeView = viewArtists
	m.artists.artists = []string{"Art"}
	m.artists.selectedArtist = "Art"
	m.artists = m.artists.openAlbums([]string{"Album1"})

	// Scan batch adds a second track with a new album.
	next, _ := m.Update(library.ScanBatchMsg{
		Tracks: []library.Track{
			{Artist: "Art", Title: "T2", Path: "/b", Hash: "h2", Album: "Album2"},
		},
	})
	nm := next.(Model)

	if len(nm.artists.albums) != 2 {
		t.Errorf("expected 2 albums after scan, got %d: %v", len(nm.artists.albums), nm.artists.albums)
	}
}

// TestScanBatchRefreshesArtistTrackLevel verifies that when the artists view
// is drilled into TrackLevel, a ScanBatchMsg re-derives the track list.
func TestScanBatchRefreshesArtistTrackLevel(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Artist: "Art", Title: "T1", Path: "/a", Hash: "h1", Album: "Alb"})
	// Drill into "Art" > "Alb" tracks.
	m.activeView = viewArtists
	m.artists.artists = []string{"Art"}
	m.artists.selectedArtist = "Art"
	m.artists = m.artists.openAlbums([]string{"Alb"})
	m.artists.selectedAlbum = "Alb"
	m.artists = m.artists.openTracks([]library.Track{
		{Artist: "Art", Title: "T1", Path: "/a", Hash: "h1", Album: "Alb"},
	})

	// Scan batch adds a new track under the same artist+album.
	next, _ := m.Update(library.ScanBatchMsg{
		Tracks: []library.Track{
			{Artist: "Art", Title: "T2", Path: "/b", Hash: "h2", Album: "Alb"},
		},
	})
	nm := next.(Model)

	if len(nm.artists.tracks) != 2 {
		t.Errorf("expected 2 tracks after scan, got %d", len(nm.artists.tracks))
	}
}

// TestScanBatchRefreshesAlbumTrackLevel verifies that when the albums view
// is drilled into TrackLevel, a ScanBatchMsg re-derives the track list.
func TestScanBatchRefreshesAlbumTrackLevel(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Artist: "Art", Title: "T1", Path: "/a", Hash: "h1", Album: "Alb"})
	// Drill into "Alb" tracks.
	m.activeView = viewAlbums
	m.albums.albums = []string{"Alb"}
	m.albums.selectedAlbum = "Alb"
	m.albums = m.albums.openTracks([]library.Track{
		{Artist: "Art", Title: "T1", Path: "/a", Hash: "h1", Album: "Alb"},
	})

	// Scan batch adds a new track under the same album.
	next, _ := m.Update(library.ScanBatchMsg{
		Tracks: []library.Track{
			{Artist: "Art2", Title: "T2", Path: "/b", Hash: "h2", Album: "Alb"},
		},
	})
	nm := next.(Model)

	if len(nm.albums.tracks) != 2 {
		t.Errorf("expected 2 tracks after scan, got %d", len(nm.albums.tracks))
	}
}

// TestScanBatchPreservesArtistLevelSort verifies that an active sort on the
// artists view at ArtistLevel is preserved across scan batches.
func TestScanBatchPreservesArtistLevelSort(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "Zara", Title: "T1"})
	m.library.index.Insert(library.Track{Hash: "h2", Path: "/b.flac", Artist: "Alice", Title: "T2"})
	m.activeView = viewArtists
	m.artists.artists = []string{"Zara", "Alice"}
	m.artists.sortState = SortState{Field: SortName, Dir: SortAsc}
	m.artists = m.artists.recomputeArtistsSort()

	// Scan batch adds a new artist.
	next, _ := m.Update(library.ScanBatchMsg{
		Tracks: []library.Track{
			{Hash: "h3", Path: "/c.flac", Artist: "Bob", Title: "T3"},
		},
	})
	nm := next.(Model)

	if nm.artists.sortState.Field != SortName {
		t.Fatalf("sort field reset; got %v", nm.artists.sortState.Field)
	}
	if len(nm.artists.artists) < 3 {
		t.Fatalf("expected at least 3 artists, got %d", len(nm.artists.artists))
	}
	// With ascending name sort: Alice, Bob, Zara.
	if nm.artists.artists[0] != "Alice" || nm.artists.artists[1] != "Bob" || nm.artists.artists[2] != "Zara" {
		t.Errorf("unexpected artist order: %v", nm.artists.artists)
	}
}

// TestScanBatchPreservesAlbumLevelSort verifies that an active sort on the
// albums view at AlbumLevel is preserved across scan batches.
func TestScanBatchPreservesAlbumLevelSort(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "A", Album: "Zebra", Title: "T1"})
	m.library.index.Insert(library.Track{Hash: "h2", Path: "/b.flac", Artist: "A", Album: "Alpha", Title: "T2"})
	m.activeView = viewAlbums
	m.albums.albums = []string{"Zebra", "Alpha"}
	m.albums.sortState = SortState{Field: SortName, Dir: SortAsc}
	m.albums = m.albums.recomputeAlbumsSort()

	next, _ := m.Update(library.ScanBatchMsg{
		Tracks: []library.Track{
			{Hash: "h3", Path: "/c.flac", Artist: "A", Album: "Middle", Title: "T3"},
		},
	})
	nm := next.(Model)

	if nm.albums.sortState.Field != SortName {
		t.Fatalf("sort field reset; got %v", nm.albums.sortState.Field)
	}
	if len(nm.albums.albums) < 3 {
		t.Fatalf("expected at least 3 albums, got %d", len(nm.albums.albums))
	}
	// Ascending: Alpha, Middle, Zebra.
	if nm.albums.albums[0] != "Alpha" || nm.albums.albums[1] != "Middle" || nm.albums.albums[2] != "Zebra" {
		t.Errorf("unexpected album order: %v", nm.albums.albums)
	}
}

// TestScanBatchPreservesGenreLevelSort verifies that an active sort on the
// genres view at GenreLevel is preserved across scan batches.
func TestScanBatchPreservesGenreLevelSort(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Genre: "Rock", Title: "T1"})
	m.library.index.Insert(library.Track{Hash: "h2", Path: "/b.flac", Genre: "Jazz", Title: "T2"})
	m.activeView = viewGenres
	m.genres.genres = []string{"Rock", "Jazz"}
	m.genres.sortState = SortState{Field: SortName, Dir: SortAsc}
	m.genres = m.genres.recomputeGenresSort()

	next, _ := m.Update(library.ScanBatchMsg{
		Tracks: []library.Track{
			{Hash: "h3", Path: "/c.flac", Genre: "Pop", Title: "T3"},
		},
	})
	nm := next.(Model)

	if nm.genres.sortState.Field != SortName {
		t.Fatalf("sort field reset; got %v", nm.genres.sortState.Field)
	}
	if len(nm.genres.genres) < 3 {
		t.Fatalf("expected at least 3 genres, got %d", len(nm.genres.genres))
	}
	// Ascending: Jazz, Pop, Rock.
	if nm.genres.genres[0] != "Jazz" || nm.genres.genres[1] != "Pop" || nm.genres.genres[2] != "Rock" {
		t.Errorf("unexpected genre order: %v", nm.genres.genres)
	}
}
