package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/library"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
	"github.com/sglavoie/muse/internal/tui/sorting"
)

// TestResetLibrarySortRebuildFilterCache verifies that resetting the library
// sort when a filter is active rebuilds the filter cache so that TrackIndex
// values reference the unsorted track list instead of the stale sorted order.
func TestResetLibrarySortRebuildFilterCache(t *testing.T) {
	m := New(context.Background())
	m.library.Fuzzy = library.FakeFuzzySearcher{}

	m.library.Index.Insert(library.Track{Hash: "h1", Title: "Charlie", Artist: "A"})
	m.library.Index.Insert(library.Track{Hash: "h2", Title: "Alpha", Artist: "B"})
	m.library.Index.Insert(library.Track{Hash: "h3", Title: "Beta", Artist: "C"})

	m.library.SortState = sorting.SortState{Field: sorting.SortTitle, Dir: sorting.SortAsc}
	m.library = m.library.RecomputeSortedTracks()

	m.library.Filter.Text = "alpha"
	m.library = m.library.RebuildFilterCache()
	if len(m.library.FilteredMatches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(m.library.FilteredMatches))
	}
	sortedIdx := m.library.FilteredMatches[0].TrackIndex
	if m.library.SortedTracks[sortedIdx].Hash != "h2" {
		t.Fatalf("sorted match should point to h2 (Alpha), got %s", m.library.SortedTracks[sortedIdx].Hash)
	}

	m = m.resetLibrarySort()

	if m.library.SortedTracks != nil {
		t.Fatal("sortedTracks should be nil after reset")
	}

	if len(m.library.FilteredMatches) != 1 {
		t.Fatalf("expected 1 filtered match after sort reset, got %d", len(m.library.FilteredMatches))
	}
	unsortedIdx := m.library.FilteredMatches[0].TrackIndex
	tracks := m.library.Index.Tracks()
	if unsortedIdx < 0 || unsortedIdx >= len(tracks) {
		t.Fatalf("TrackIndex %d out of bounds (len=%d)", unsortedIdx, len(tracks))
	}
	if tracks[unsortedIdx].Hash != "h2" {
		t.Fatalf("unsorted match should point to h2 (Alpha), got %s", tracks[unsortedIdx].Hash)
	}
}

// TestResetArtistsSortRederivesData verifies that resetting a non-default
// artists sort restores the original (index-derived) artist order instead of
// leaving the in-place-sorted slice behind.
func TestResetArtistsSortRederivesData(t *testing.T) {
	m := New(context.Background())
	// LibraryIndex sorts alphabetically: Alpha, Beta, Charlie.
	m.library.Index.Insert(library.Track{Hash: "h1", Artist: "Charlie", Title: "T1"})
	m.library.Index.Insert(library.Track{Hash: "h2", Artist: "Alpha", Title: "T2"})
	m.library.Index.Insert(library.Track{Hash: "h3", Artist: "Beta", Title: "T3"})

	m.browse.Artists.Level = arttui.ArtistLevel
	m.browse.Artists.Artists = arttui.DeriveArtists(m.library.Index.Tracks())
	if m.browse.Artists.Artists[0] != "Alpha" {
		t.Fatalf("precondition: first artist should be Alpha, got %q", m.browse.Artists.Artists[0])
	}

	// Sort descending: Charlie, Beta, Alpha.
	m.browse.Artists.SortState = sorting.SortState{Field: sorting.SortTitle, Dir: sorting.SortDesc}
	m.browse.Artists = m.browse.Artists.RecomputeArtistsSort()
	if m.browse.Artists.Artists[0] != "Charlie" {
		t.Fatalf("after sort desc: first artist should be Charlie, got %q", m.browse.Artists.Artists[0])
	}

	// Reset should restore original alphabetical order.
	m = m.resetArtistsSort()
	if m.browse.Artists.SortState.Field != sorting.SortDefault {
		t.Fatal("sortState should be SortDefault after reset")
	}
	if m.browse.Artists.Artists[0] != "Alpha" {
		t.Errorf("after reset: first artist should be Alpha (original order), got %q", m.browse.Artists.Artists[0])
	}
}

// TestResetAlbumsSortRederivesData verifies that resetting a non-default
// albums sort restores the original album order (alphabetical by default).
func TestResetAlbumsSortRederivesData(t *testing.T) {
	m := New(context.Background())
	m.library.Index.Insert(library.Track{Hash: "h1", Artist: "A", Album: "Alpha", Title: "T1"})
	m.library.Index.Insert(library.Track{Hash: "h2", Artist: "A", Album: "Zebra", Title: "T2"})

	m.browse.Albums.Level = albtui.AlbumLevel
	m.browse.Albums.Albums = albtui.DeriveAllAlbums(m.library.Index.Tracks())
	// DeriveAllAlbums sorts alphabetically: Alpha, Zebra.
	if m.browse.Albums.Albums[0] != "Alpha" {
		t.Fatalf("precondition: first album should be Alpha, got %q", m.browse.Albums.Albums[0])
	}

	// Sort descending: Zebra, Alpha.
	m.browse.Albums.SortState = sorting.SortState{Field: sorting.SortTitle, Dir: sorting.SortDesc}
	m.browse.Albums = m.browse.Albums.RecomputeAlbumsSort()
	if m.browse.Albums.Albums[0] != "Zebra" {
		t.Fatalf("after sort desc: first album should be Zebra, got %q", m.browse.Albums.Albums[0])
	}

	// Reset should restore default alphabetical order.
	m = m.resetAlbumsSort()
	if m.browse.Albums.Albums[0] != "Alpha" {
		t.Errorf("after reset: first album should be Alpha (original order), got %q", m.browse.Albums.Albums[0])
	}
}

// TestResetGenresSortRederivesData verifies that resetting a non-default
// genres sort restores the original genre order.
func TestResetGenresSortRederivesData(t *testing.T) {
	m := New(context.Background())
	// Tracks sorted by (artist, title): both "A", so T1 before T2.
	// DeriveGenres first-occurrence order: Rock, Jazz.
	m.library.Index.Insert(library.Track{Hash: "h1", Artist: "A", Genre: "Rock", Title: "T1"})
	m.library.Index.Insert(library.Track{Hash: "h2", Artist: "A", Genre: "Jazz", Title: "T2"})

	m.browse.Genres.Level = gentui.GenreLevel
	m.browse.Genres.Genres = gentui.DeriveGenres(m.library.Index.Tracks())
	if len(m.browse.Genres.Genres) < 2 {
		t.Fatalf("precondition: expected 2 genres, got %d", len(m.browse.Genres.Genres))
	}
	origFirst := m.browse.Genres.Genres[0]

	// Sort ascending: Jazz, Rock.
	m.browse.Genres.SortState = sorting.SortState{Field: sorting.SortTitle, Dir: sorting.SortAsc}
	m.browse.Genres = m.browse.Genres.RecomputeGenresSort()
	if m.browse.Genres.Genres[0] != "Jazz" {
		t.Fatalf("after sort asc: first genre should be Jazz, got %q", m.browse.Genres.Genres[0])
	}

	// Reset should restore original derive order.
	m = m.resetGenresSort()
	if m.browse.Genres.Genres[0] != origFirst {
		t.Errorf("after reset: first genre should be %q (original order), got %q", origFirst, m.browse.Genres.Genres[0])
	}
}
