package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/library"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
)

// TestResetLibrarySortRebuildFilterCache verifies that resetting the library
// sort when a filter is active rebuilds the filter cache so that TrackIndex
// values reference the unsorted track list instead of the stale sorted order.
func TestResetLibrarySortRebuildFilterCache(t *testing.T) {
	m := New(context.Background())
	m.library.fuzzy = library.FakeFuzzySearcher{}

	m.library.index.Insert(library.Track{Hash: "h1", Title: "Charlie", Artist: "A"})
	m.library.index.Insert(library.Track{Hash: "h2", Title: "Alpha", Artist: "B"})
	m.library.index.Insert(library.Track{Hash: "h3", Title: "Beta", Artist: "C"})

	m.library.sortState = SortState{Field: SortTitle, Dir: SortAsc}
	m.library = m.library.recomputeSortedTracks()

	m.library.filter.Text = "alpha"
	m.library = m.library.rebuildFilterCache()
	if len(m.library.filteredMatches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(m.library.filteredMatches))
	}
	sortedIdx := m.library.filteredMatches[0].TrackIndex
	if m.library.sortedTracks[sortedIdx].Hash != "h2" {
		t.Fatalf("sorted match should point to h2 (Alpha), got %s", m.library.sortedTracks[sortedIdx].Hash)
	}

	m = m.resetLibrarySort()

	if m.library.sortedTracks != nil {
		t.Fatal("sortedTracks should be nil after reset")
	}

	if len(m.library.filteredMatches) != 1 {
		t.Fatalf("expected 1 filtered match after sort reset, got %d", len(m.library.filteredMatches))
	}
	unsortedIdx := m.library.filteredMatches[0].TrackIndex
	tracks := m.library.index.Tracks()
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
	m.library.index.Insert(library.Track{Hash: "h1", Artist: "Charlie", Title: "T1"})
	m.library.index.Insert(library.Track{Hash: "h2", Artist: "Alpha", Title: "T2"})
	m.library.index.Insert(library.Track{Hash: "h3", Artist: "Beta", Title: "T3"})

	m.artists.level = arttui.ArtistLevel
	m.artists.artists = arttui.DeriveArtists(m.library.index.Tracks())
	if m.artists.artists[0] != "Alpha" {
		t.Fatalf("precondition: first artist should be Alpha, got %q", m.artists.artists[0])
	}

	// Sort descending: Charlie, Beta, Alpha.
	m.artists.sortState = SortState{Field: SortTitle, Dir: SortDesc}
	m.artists = m.artists.recomputeArtistsSort()
	if m.artists.artists[0] != "Charlie" {
		t.Fatalf("after sort desc: first artist should be Charlie, got %q", m.artists.artists[0])
	}

	// Reset should restore original alphabetical order.
	m = m.resetArtistsSort()
	if m.artists.sortState.Field != SortDefault {
		t.Fatal("sortState should be SortDefault after reset")
	}
	if m.artists.artists[0] != "Alpha" {
		t.Errorf("after reset: first artist should be Alpha (original order), got %q", m.artists.artists[0])
	}
}

// TestResetAlbumsSortRederivesData verifies that resetting a non-default
// albums sort restores the original album order (alphabetical by default).
func TestResetAlbumsSortRederivesData(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Artist: "A", Album: "Alpha", Title: "T1"})
	m.library.index.Insert(library.Track{Hash: "h2", Artist: "A", Album: "Zebra", Title: "T2"})

	m.albums.level = albtui.AlbumLevel
	m.albums.albums = albtui.DeriveAllAlbums(m.library.index.Tracks())
	// DeriveAllAlbums sorts alphabetically: Alpha, Zebra.
	if m.albums.albums[0] != "Alpha" {
		t.Fatalf("precondition: first album should be Alpha, got %q", m.albums.albums[0])
	}

	// Sort descending: Zebra, Alpha.
	m.albums.sortState = SortState{Field: SortTitle, Dir: SortDesc}
	m.albums = m.albums.recomputeAlbumsSort()
	if m.albums.albums[0] != "Zebra" {
		t.Fatalf("after sort desc: first album should be Zebra, got %q", m.albums.albums[0])
	}

	// Reset should restore default alphabetical order.
	m = m.resetAlbumsSort()
	if m.albums.albums[0] != "Alpha" {
		t.Errorf("after reset: first album should be Alpha (original order), got %q", m.albums.albums[0])
	}
}

// TestResetGenresSortRederivesData verifies that resetting a non-default
// genres sort restores the original genre order.
func TestResetGenresSortRederivesData(t *testing.T) {
	m := New(context.Background())
	// Tracks sorted by (artist, title): both "A", so T1 before T2.
	// DeriveGenres first-occurrence order: Rock, Jazz.
	m.library.index.Insert(library.Track{Hash: "h1", Artist: "A", Genre: "Rock", Title: "T1"})
	m.library.index.Insert(library.Track{Hash: "h2", Artist: "A", Genre: "Jazz", Title: "T2"})

	m.genres.level = gentui.GenreLevel
	m.genres.genres = gentui.DeriveGenres(m.library.index.Tracks())
	if len(m.genres.genres) < 2 {
		t.Fatalf("precondition: expected 2 genres, got %d", len(m.genres.genres))
	}
	origFirst := m.genres.genres[0]

	// Sort ascending: Jazz, Rock.
	m.genres.sortState = SortState{Field: SortTitle, Dir: SortAsc}
	m.genres = m.genres.recomputeGenresSort()
	if m.genres.genres[0] != "Jazz" {
		t.Fatalf("after sort asc: first genre should be Jazz, got %q", m.genres.genres[0])
	}

	// Reset should restore original derive order.
	m = m.resetGenresSort()
	if m.genres.genres[0] != origFirst {
		t.Errorf("after reset: first genre should be %q (original order), got %q", origFirst, m.genres.genres[0])
	}
}
