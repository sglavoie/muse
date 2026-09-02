package tui

import (
	"testing"

	"github.com/sglavoie/muse/internal/library"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	"github.com/sglavoie/muse/internal/tui/sorting"
)

// TestRederiveFavoritesDataArtists verifies that rederiveFavoritesData at
// ListLevel/CategoryArtists re-reads the original artist list from the
// database, restoring the created_at DESC order after an in-place sort.
func TestRederiveFavoritesDataArtists(t *testing.T) {
	// Simulate DB returning artists in created_at DESC order.
	reader := &stubFavoriteReader{
		favorites: map[string][]string{
			"artist": {"Charlie", "Bob", "Alice"},
		},
	}

	m := Model{}
	m.favoriteReader = reader
	m.favorites.Level = favtui.ListLevel
	m.favorites.Category = favtui.CategoryArtists
	// Simulate an in-place sort that scrambled the original order.
	m.favorites.Artists = []string{"Alice", "Bob", "Charlie"}

	m = m.rederiveFavoritesData()

	want := []string{"Charlie", "Bob", "Alice"}
	if len(m.favorites.Artists) != len(want) {
		t.Fatalf("got %d artists, want %d", len(m.favorites.Artists), len(want))
	}
	for i, w := range want {
		if m.favorites.Artists[i] != w {
			t.Errorf("artists[%d] = %q, want %q", i, m.favorites.Artists[i], w)
		}
	}
}

// TestRederiveFavoritesDataAlbums verifies that rederiveFavoritesData at
// ListLevel/CategoryAlbums re-reads the original album list from the database.
func TestRederiveFavoritesDataAlbums(t *testing.T) {
	reader := &stubFavoriteReader{
		favorites: map[string][]string{
			"album": {"Zebra", "Middle", "Alpha"},
		},
	}

	m := Model{}
	m.favoriteReader = reader
	m.favorites.Level = favtui.ListLevel
	m.favorites.Category = favtui.CategoryAlbums
	m.favorites.Albums = []string{"Alpha", "Middle", "Zebra"}

	m = m.rederiveFavoritesData()

	want := []string{"Zebra", "Middle", "Alpha"}
	if len(m.favorites.Albums) != len(want) {
		t.Fatalf("got %d albums, want %d", len(m.favorites.Albums), len(want))
	}
	for i, w := range want {
		if m.favorites.Albums[i] != w {
			t.Errorf("albums[%d] = %q, want %q", i, m.favorites.Albums[i], w)
		}
	}
}

// TestRederiveFavoritesDataArtistsNilReader verifies that rederiveFavoritesData
// does not panic when favoriteReader is nil.
func TestRederiveFavoritesDataArtistsNilReader(t *testing.T) {
	m := Model{}
	m.favorites.Level = favtui.ListLevel
	m.favorites.Category = favtui.CategoryArtists
	m.favorites.Artists = []string{"Sorted"}

	m = m.rederiveFavoritesData()

	// Should remain unchanged when no reader is available.
	if len(m.favorites.Artists) != 1 || m.favorites.Artists[0] != "Sorted" {
		t.Errorf("artists should be unchanged without favoriteReader, got %v", m.favorites.Artists)
	}
}

// TestRederiveFavoritesDataTracks verifies that rederiveFavoritesData at
// ListLevel/CategoryTracks still works as before (regression guard).
func TestRederiveFavoritesDataTracks(t *testing.T) {
	m := Model{}
	m.library.Index.Insert(library.Track{Artist: "A", Title: "T1", Path: "/a", Hash: "ha"})
	m.library.Index.Insert(library.Track{Artist: "B", Title: "T2", Path: "/b", Hash: "hb"})

	m.favorites.Level = favtui.ListLevel
	m.favorites.Category = favtui.CategoryTracks
	// Pre-populate favorites.tracks with known hashes so rebuild can find them.
	allTracks := m.library.Index.Tracks()
	m.favorites.Tracks = make([]library.Track, len(allTracks))
	copy(m.favorites.Tracks, allTracks)

	m = m.rederiveFavoritesData()

	if len(m.favorites.Tracks) != 2 {
		t.Fatalf("expected 2 tracks, got %d", len(m.favorites.Tracks))
	}
}

// TestResetFavoritesSortRederivesArtists verifies the full flow: sorting
// favorites artists, then resetting sort, restores the DB order.
func TestResetFavoritesSortRederivesArtists(t *testing.T) {
	reader := &stubFavoriteReader{
		favorites: map[string][]string{
			"artist": {"Charlie", "Bob", "Alice"},
		},
	}

	m := Model{}
	m.favoriteReader = reader
	m.activeView = viewFavorites
	m.favorites.Level = favtui.ListLevel
	m.favorites.Category = favtui.CategoryArtists
	m.favorites.Artists = []string{"Charlie", "Bob", "Alice"}

	// Simulate that a sort was active (in-place sort scrambled the slice).
	m.favorites.SortState = sorting.SortState{Field: sorting.SortName, Dir: sorting.SortAsc}
	m.favorites.Artists = []string{"Alice", "Bob", "Charlie"} // sorted alphabetically

	m = m.resetFavoritesSort()

	if m.favorites.SortState.Field != sorting.SortDefault {
		t.Errorf("expected SortDefault, got %v", m.favorites.SortState.Field)
	}

	want := []string{"Charlie", "Bob", "Alice"}
	for i, w := range want {
		if i >= len(m.favorites.Artists) {
			t.Fatalf("artists list too short at index %d", i)
		}
		if m.favorites.Artists[i] != w {
			t.Errorf("artists[%d] = %q, want %q", i, m.favorites.Artists[i], w)
		}
	}
}

// TestSortCycleToDefaultRederivesFavoritesArtists verifies that cycling sort
// back to default on favorites/artists view restores the original DB order.
func TestSortCycleToDefaultRederivesFavoritesArtists(t *testing.T) {
	reader := &stubFavoriteReader{
		favorites: map[string][]string{
			"artist": {"Charlie", "Bob", "Alice"},
		},
	}

	m := Model{}
	m.favoriteReader = reader
	m.activeView = viewFavorites
	m.favorites.Level = favtui.ListLevel
	m.favorites.Category = favtui.CategoryArtists
	m.favorites.Artists = []string{"Alice", "Bob", "Charlie"} // sorted in-place

	// Set sort to SortName so that cycling once reaches SortDefault.
	m.favorites.SortState = sorting.SortState{Field: sorting.SortName, Dir: sorting.SortAsc}

	result, _, _ := m.handleSortCycle()
	m = result.(Model)

	if m.favorites.SortState.Field != sorting.SortDefault {
		t.Fatalf("expected SortDefault, got %v", m.favorites.SortState.Field)
	}

	want := []string{"Charlie", "Bob", "Alice"}
	for i, w := range want {
		if i >= len(m.favorites.Artists) {
			t.Fatalf("artists list too short at index %d", i)
		}
		if m.favorites.Artists[i] != w {
			t.Errorf("artists[%d] = %q, want %q", i, m.favorites.Artists[i], w)
		}
	}
}
