package tui

import (
	"testing"

	"github.com/sglavoie/muse/internal/tui/sorting"
)

// TestRefreshDerivedViewsRebuildsLibrarySortCache verifies that
// refreshDerivedViews recomputes the library sort cache when sorting is active.
func TestRefreshDerivedViewsRebuildsLibrarySortCache(t *testing.T) {
	m := newModelWithTracks()
	m.library.SortState = sorting.SortState{Field: sorting.SortTitle, Dir: sorting.SortAsc}
	m.library = m.library.RecomputeSortedTracks()
	if m.library.SortedTracks == nil {
		t.Fatal("expected sortedTracks to be non-nil after recomputeSortedTracks")
	}

	m.library.Index.UpdateMetadata("h1", "ZZZ Last", "Alice", "Alpha", "")
	m = m.refreshDerivedViews()

	if m.library.SortedTracks == nil {
		t.Fatal("expected sortedTracks to remain non-nil after refreshDerivedViews")
	}
	last := m.library.SortedTracks[len(m.library.SortedTracks)-1]
	if last.Hash != "h1" || last.Title != "ZZZ Last" {
		t.Errorf("expected renamed track at end of sorted list, got hash=%q title=%q", last.Hash, last.Title)
	}
}

// TestRefreshDerivedViewsRebuildsLibraryFilterCache verifies that
// refreshDerivedViews recomputes the library filter cache when a filter is
// active.
func TestRefreshDerivedViewsRebuildsLibraryFilterCache(t *testing.T) {
	m := newModelWithTracks()
	m.library.Filter.Text = "Unique"
	m.library = m.library.RebuildFilterCache()
	if len(m.library.FilteredMatches) != 0 {
		t.Fatalf("expected 0 filter matches before rename, got %d", len(m.library.FilteredMatches))
	}

	m.library.Index.UpdateMetadata("h1", "Unique Title", "Alice", "Alpha", "")
	m = m.refreshDerivedViews()

	if len(m.library.FilteredMatches) == 0 {
		t.Error("expected filter matches after renaming track to 'Unique Title', got 0")
	}
}
