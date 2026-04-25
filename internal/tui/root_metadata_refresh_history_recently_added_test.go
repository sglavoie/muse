package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/library"
)

// TestRefreshDerivedViewsUpdatesHistoryTracks verifies that
// refreshDerivedViews updates history source track metadata.
func TestRefreshDerivedViewsUpdatesHistoryTracks(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "OldArt", Title: "OldTitle", Album: "OldAlbum", Genre: "OldGenre"})
	m.history.sourceTracks = []library.Track{
		{Hash: "h1", Path: "/a.flac", Artist: "OldArt", Title: "OldTitle", Album: "OldAlbum", Genre: "OldGenre"},
	}

	m.library.index.UpdateMetadata("h1", "NewTitle", "NewArt", "NewAlbum", "NewGenre")
	m = m.refreshDerivedViews()

	if m.history.sourceTracks[0].Title != "NewTitle" {
		t.Errorf("history track title = %q, want %q", m.history.sourceTracks[0].Title, "NewTitle")
	}
	if m.history.sourceTracks[0].Artist != "NewArt" {
		t.Errorf("history track artist = %q, want %q", m.history.sourceTracks[0].Artist, "NewArt")
	}
	if m.history.sourceTracks[0].Album != "NewAlbum" {
		t.Errorf("history track album = %q, want %q", m.history.sourceTracks[0].Album, "NewAlbum")
	}
	if m.history.sourceTracks[0].Genre != "NewGenre" {
		t.Errorf("history track genre = %q, want %q", m.history.sourceTracks[0].Genre, "NewGenre")
	}
}

// TestRefreshDerivedViewsRebuildsHistorySortCache verifies that
// refreshDerivedViews recomputes the history sort cache when sorting is active.
func TestRefreshDerivedViewsRebuildsHistorySortCache(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "Art", Title: "Bravo", Album: "Alb"})
	m.library.index.Insert(library.Track{Hash: "h2", Path: "/b.flac", Artist: "Art", Title: "Alpha", Album: "Alb"})
	m.history.sourceTracks = []library.Track{
		{Hash: "h1", Path: "/a.flac", Artist: "Art", Title: "Bravo", Album: "Alb"},
		{Hash: "h2", Path: "/b.flac", Artist: "Art", Title: "Alpha", Album: "Alb"},
	}
	m.history.sortState = SortState{Field: SortTitle, Dir: SortAsc}
	m.history = m.history.recomputeSortedTracks()

	// Rename h1 so it sorts last.
	m.library.index.UpdateMetadata("h1", "Zulu", "Art", "Alb", "")
	m = m.refreshDerivedViews()

	if m.history.sortedTracks == nil {
		t.Fatal("expected sortedTracks to be non-nil after refreshDerivedViews")
	}
	last := m.history.sortedTracks[len(m.history.sortedTracks)-1]
	if last.Hash != "h1" || last.Title != "Zulu" {
		t.Errorf("expected renamed track at end of sorted list, got hash=%q title=%q", last.Hash, last.Title)
	}
}

// TestRefreshDerivedViewsRebuildsHistoryFilterCache verifies that
// refreshDerivedViews recomputes the history filter cache when a filter is
// active.
func TestRefreshDerivedViewsRebuildsHistoryFilterCache(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "Art", Title: "OldTitle", Album: "Alb"})
	m.history.sourceTracks = []library.Track{
		{Hash: "h1", Path: "/a.flac", Artist: "Art", Title: "OldTitle", Album: "Alb"},
	}
	m.history.filter.Text = "Unique"
	m.history = m.history.rebuildFilterCache()
	if len(m.history.filteredMatches) != 0 {
		t.Fatalf("expected 0 filter matches before rename, got %d", len(m.history.filteredMatches))
	}

	m.library.index.UpdateMetadata("h1", "Unique Title", "Art", "Alb", "")
	m = m.refreshDerivedViews()

	if len(m.history.filteredMatches) == 0 {
		t.Error("expected filter matches after renaming track to 'Unique Title', got 0")
	}
}

// TestRefreshDerivedViewsSkipsHistoryWhenEmpty verifies that
// refreshDerivedViews does not panic when history has no source tracks.
func TestRefreshDerivedViewsSkipsHistoryWhenEmpty(t *testing.T) {
	m := New(context.Background())
	m.history.sourceTracks = nil
	m = m.refreshDerivedViews()
}

// TestRefreshDerivedViewsUpdatesRecentlyAddedTracks verifies that
// refreshDerivedViews updates recentlyAdded source track metadata.
func TestRefreshDerivedViewsUpdatesRecentlyAddedTracks(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "OldArt", Title: "OldTitle", Album: "OldAlbum", Genre: "OldGenre"})
	m.recentlyAdded.sourceTracks = []library.Track{
		{Hash: "h1", Path: "/a.flac", Artist: "OldArt", Title: "OldTitle", Album: "OldAlbum", Genre: "OldGenre"},
	}

	m.library.index.UpdateMetadata("h1", "NewTitle", "NewArt", "NewAlbum", "NewGenre")
	m = m.refreshDerivedViews()

	if m.recentlyAdded.sourceTracks[0].Title != "NewTitle" {
		t.Errorf("recentlyAdded track title = %q, want %q", m.recentlyAdded.sourceTracks[0].Title, "NewTitle")
	}
	if m.recentlyAdded.sourceTracks[0].Artist != "NewArt" {
		t.Errorf("recentlyAdded track artist = %q, want %q", m.recentlyAdded.sourceTracks[0].Artist, "NewArt")
	}
	if m.recentlyAdded.sourceTracks[0].Album != "NewAlbum" {
		t.Errorf("recentlyAdded track album = %q, want %q", m.recentlyAdded.sourceTracks[0].Album, "NewAlbum")
	}
	if m.recentlyAdded.sourceTracks[0].Genre != "NewGenre" {
		t.Errorf("recentlyAdded track genre = %q, want %q", m.recentlyAdded.sourceTracks[0].Genre, "NewGenre")
	}
}

// TestRefreshDerivedViewsRebuildsRecentlyAddedSortCache verifies that
// refreshDerivedViews recomputes the recentlyAdded sort cache when sorting is active.
func TestRefreshDerivedViewsRebuildsRecentlyAddedSortCache(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "Art", Title: "Bravo", Album: "Alb"})
	m.library.index.Insert(library.Track{Hash: "h2", Path: "/b.flac", Artist: "Art", Title: "Alpha", Album: "Alb"})
	m.recentlyAdded.sourceTracks = []library.Track{
		{Hash: "h1", Path: "/a.flac", Artist: "Art", Title: "Bravo", Album: "Alb"},
		{Hash: "h2", Path: "/b.flac", Artist: "Art", Title: "Alpha", Album: "Alb"},
	}
	m.recentlyAdded.sortState = SortState{Field: SortTitle, Dir: SortAsc}
	m.recentlyAdded = m.recentlyAdded.recomputeSortedTracks()

	m.library.index.UpdateMetadata("h1", "Zulu", "Art", "Alb", "")
	m = m.refreshDerivedViews()

	if m.recentlyAdded.sortedTracks == nil {
		t.Fatal("expected sortedTracks to be non-nil after refreshDerivedViews")
	}
	last := m.recentlyAdded.sortedTracks[len(m.recentlyAdded.sortedTracks)-1]
	if last.Hash != "h1" || last.Title != "Zulu" {
		t.Errorf("expected renamed track at end of sorted list, got hash=%q title=%q", last.Hash, last.Title)
	}
}

// TestRefreshDerivedViewsRebuildsRecentlyAddedFilterCache verifies that
// refreshDerivedViews recomputes the recentlyAdded filter cache when a filter
// is active.
func TestRefreshDerivedViewsRebuildsRecentlyAddedFilterCache(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "Art", Title: "OldTitle", Album: "Alb"})
	m.recentlyAdded.sourceTracks = []library.Track{
		{Hash: "h1", Path: "/a.flac", Artist: "Art", Title: "OldTitle", Album: "Alb"},
	}
	m.recentlyAdded.fuzzy = library.SahilmFuzzySearcher{}
	m.recentlyAdded.filter.Text = "Unique"
	m.recentlyAdded = m.recentlyAdded.rebuildFilterCache()
	if len(m.recentlyAdded.filteredMatches) != 0 {
		t.Fatalf("expected 0 filter matches before rename, got %d", len(m.recentlyAdded.filteredMatches))
	}

	m.library.index.UpdateMetadata("h1", "Unique Title", "Art", "Alb", "")
	m = m.refreshDerivedViews()

	if len(m.recentlyAdded.filteredMatches) == 0 {
		t.Error("expected filter matches after renaming track to 'Unique Title', got 0")
	}
}
