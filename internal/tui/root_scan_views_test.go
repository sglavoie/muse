package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/browse"
	"github.com/sglavoie/muse/internal/tui/sorting"
)

// TestScanBatchRefreshesArtistAlbumLevel verifies that when the artists view
// is drilled into AlbumLevel, a ScanBatchMsg re-derives the album list so
// newly scanned albums appear.
func TestScanBatchRefreshesArtistAlbumLevel(t *testing.T) {
	m := New(context.Background())
	m.library.Index.Insert(library.Track{Artist: "Art", Title: "T1", Path: "/a", Hash: "h1", Album: "Album1"})
	// Drill into "Art" albums.
	m.ActiveView = viewBrowse
	m.browse.Mode = browse.BrowseModeArtists
	m.browse.Artists.Artists = []string{"Art"}
	m.browse.Artists.SelectedArtist = "Art"
	m.browse.Artists = m.browse.Artists.OpenAlbums([]string{"Album1"})

	// Scan batch adds a second track with a new album.
	next, _ := m.Update(library.ScanBatchMsg{
		Tracks: []library.Track{
			{Artist: "Art", Title: "T2", Path: "/b", Hash: "h2", Album: "Album2"},
		},
	})
	nm := next.(Model)

	if len(nm.browse.Artists.Albums) != 2 {
		t.Errorf("expected 2 albums after scan, got %d: %v", len(nm.browse.Artists.Albums), nm.browse.Artists.Albums)
	}
}

// TestScanBatchRefreshesArtistTrackLevel verifies that when the artists view
// is drilled into TrackLevel, a ScanBatchMsg re-derives the track list.
func TestScanBatchRefreshesArtistTrackLevel(t *testing.T) {
	m := New(context.Background())
	m.library.Index.Insert(library.Track{Artist: "Art", Title: "T1", Path: "/a", Hash: "h1", Album: "Alb"})
	// Drill into "Art" > "Alb" tracks.
	m.ActiveView = viewBrowse
	m.browse.Mode = browse.BrowseModeArtists
	m.browse.Artists.Artists = []string{"Art"}
	m.browse.Artists.SelectedArtist = "Art"
	m.browse.Artists = m.browse.Artists.OpenAlbums([]string{"Alb"})
	m.browse.Artists.SelectedAlbum = "Alb"
	m.browse.Artists = m.browse.Artists.OpenTracks([]library.Track{
		{Artist: "Art", Title: "T1", Path: "/a", Hash: "h1", Album: "Alb"},
	})

	// Scan batch adds a new track under the same artist+album.
	next, _ := m.Update(library.ScanBatchMsg{
		Tracks: []library.Track{
			{Artist: "Art", Title: "T2", Path: "/b", Hash: "h2", Album: "Alb"},
		},
	})
	nm := next.(Model)

	if len(nm.browse.Artists.Tracks) != 2 {
		t.Errorf("expected 2 tracks after scan, got %d", len(nm.browse.Artists.Tracks))
	}
}

// TestScanBatchRefreshesAlbumTrackLevel verifies that when the albums view
// is drilled into TrackLevel, a ScanBatchMsg re-derives the track list.
func TestScanBatchRefreshesAlbumTrackLevel(t *testing.T) {
	m := New(context.Background())
	m.library.Index.Insert(library.Track{Artist: "Art", Title: "T1", Path: "/a", Hash: "h1", Album: "Alb"})
	// Drill into "Alb" tracks.
	m.ActiveView = viewBrowse
	m.browse.Mode = browse.BrowseModeAlbums
	m.browse.Albums.Albums = []string{"Alb"}
	m.browse.Albums.SelectedAlbum = "Alb"
	m.browse.Albums = m.browse.Albums.OpenTracks([]library.Track{
		{Artist: "Art", Title: "T1", Path: "/a", Hash: "h1", Album: "Alb"},
	})

	// Scan batch adds a new track under the same album.
	next, _ := m.Update(library.ScanBatchMsg{
		Tracks: []library.Track{
			{Artist: "Art2", Title: "T2", Path: "/b", Hash: "h2", Album: "Alb"},
		},
	})
	nm := next.(Model)

	if len(nm.browse.Albums.Tracks) != 2 {
		t.Errorf("expected 2 tracks after scan, got %d", len(nm.browse.Albums.Tracks))
	}
}

// TestScanBatchPreservesArtistLevelSort verifies that an active sort on the
// artists view at ArtistLevel is preserved across scan batches.
func TestScanBatchPreservesArtistLevelSort(t *testing.T) {
	m := New(context.Background())
	m.library.Index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "Zara", Title: "T1"})
	m.library.Index.Insert(library.Track{Hash: "h2", Path: "/b.flac", Artist: "Alice", Title: "T2"})
	m.ActiveView = viewBrowse
	m.browse.Mode = browse.BrowseModeArtists
	m.browse.Artists.Artists = []string{"Zara", "Alice"}
	m.browse.Artists.SortState = sorting.SortState{Field: sorting.SortName, Dir: sorting.SortAsc}
	m.browse.Artists = m.browse.Artists.RecomputeArtistsSort()

	// Scan batch adds a new artist.
	next, _ := m.Update(library.ScanBatchMsg{
		Tracks: []library.Track{
			{Hash: "h3", Path: "/c.flac", Artist: "Bob", Title: "T3"},
		},
	})
	nm := next.(Model)

	if nm.browse.Artists.SortState.Field != sorting.SortName {
		t.Fatalf("sort field reset; got %v", nm.browse.Artists.SortState.Field)
	}
	if len(nm.browse.Artists.Artists) < 3 {
		t.Fatalf("expected at least 3 artists, got %d", len(nm.browse.Artists.Artists))
	}
	// With ascending name sort: Alice, Bob, Zara.
	if nm.browse.Artists.Artists[0] != "Alice" || nm.browse.Artists.Artists[1] != "Bob" || nm.browse.Artists.Artists[2] != "Zara" {
		t.Errorf("unexpected artist order: %v", nm.browse.Artists.Artists)
	}
}

// TestScanBatchPreservesAlbumLevelSort verifies that an active sort on the
// albums view at AlbumLevel is preserved across scan batches.
func TestScanBatchPreservesAlbumLevelSort(t *testing.T) {
	m := New(context.Background())
	m.library.Index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "A", Album: "Zebra", Title: "T1"})
	m.library.Index.Insert(library.Track{Hash: "h2", Path: "/b.flac", Artist: "A", Album: "Alpha", Title: "T2"})
	m.ActiveView = viewBrowse
	m.browse.Mode = browse.BrowseModeAlbums
	m.browse.Albums.Albums = []string{"Zebra", "Alpha"}
	m.browse.Albums.SortState = sorting.SortState{Field: sorting.SortName, Dir: sorting.SortAsc}
	m.browse.Albums = m.browse.Albums.RecomputeAlbumsSort()

	next, _ := m.Update(library.ScanBatchMsg{
		Tracks: []library.Track{
			{Hash: "h3", Path: "/c.flac", Artist: "A", Album: "Middle", Title: "T3"},
		},
	})
	nm := next.(Model)

	if nm.browse.Albums.SortState.Field != sorting.SortName {
		t.Fatalf("sort field reset; got %v", nm.browse.Albums.SortState.Field)
	}
	if len(nm.browse.Albums.Albums) < 3 {
		t.Fatalf("expected at least 3 albums, got %d", len(nm.browse.Albums.Albums))
	}
	// Ascending: Alpha, Middle, Zebra.
	if library.AlbumTitle(nm.browse.Albums.Albums[0]) != "Alpha" || library.AlbumTitle(nm.browse.Albums.Albums[1]) != "Middle" || library.AlbumTitle(nm.browse.Albums.Albums[2]) != "Zebra" {
		t.Errorf("unexpected album order: %v", nm.browse.Albums.Albums)
	}
}

// TestScanBatchPreservesGenreLevelSort verifies that an active sort on the
// genres view at GenreLevel is preserved across scan batches.
func TestScanBatchPreservesGenreLevelSort(t *testing.T) {
	m := New(context.Background())
	m.library.Index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Genre: "Rock", Title: "T1"})
	m.library.Index.Insert(library.Track{Hash: "h2", Path: "/b.flac", Genre: "Jazz", Title: "T2"})
	m.ActiveView = viewBrowse
	m.browse.Mode = browse.BrowseModeGenres
	m.browse.Genres.Genres = []string{"Rock", "Jazz"}
	m.browse.Genres.SortState = sorting.SortState{Field: sorting.SortName, Dir: sorting.SortAsc}
	m.browse.Genres = m.browse.Genres.RecomputeGenresSort()

	next, _ := m.Update(library.ScanBatchMsg{
		Tracks: []library.Track{
			{Hash: "h3", Path: "/c.flac", Genre: "Pop", Title: "T3"},
		},
	})
	nm := next.(Model)

	if nm.browse.Genres.SortState.Field != sorting.SortName {
		t.Fatalf("sort field reset; got %v", nm.browse.Genres.SortState.Field)
	}
	if len(nm.browse.Genres.Genres) < 3 {
		t.Fatalf("expected at least 3 genres, got %d", len(nm.browse.Genres.Genres))
	}
	// Ascending: Jazz, Pop, Rock.
	if nm.browse.Genres.Genres[0] != "Jazz" || nm.browse.Genres.Genres[1] != "Pop" || nm.browse.Genres.Genres[2] != "Rock" {
		t.Errorf("unexpected genre order: %v", nm.browse.Genres.Genres)
	}
}
