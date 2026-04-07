package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/library"
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
