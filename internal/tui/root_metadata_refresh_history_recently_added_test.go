package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/library"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	"github.com/sglavoie/muse/internal/tui/sorting"
)

// TestRefreshDerivedViewsUpdatesHistoryTracks verifies that
// refreshDerivedViews updates history source track metadata.
func TestRefreshDerivedViewsUpdatesHistoryTracks(t *testing.T) {
	m := New(context.Background())
	m.library.Index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "OldArt", Title: "OldTitle", Album: "OldAlbum", Genre: "OldGenre"})
	m.library.Mode = libtui.LibraryModeHistory
	m.library.SourceTracks = []library.Track{
		{Hash: "h1", Path: "/a.flac", Artist: "OldArt", Title: "OldTitle", Album: "OldAlbum", Genre: "OldGenre"},
	}

	m.library.Index.UpdateMetadata("h1", "NewTitle", "NewArt", "NewAlbum", "NewGenre")
	m = m.refreshDerivedViews()

	if m.library.SourceTracks[0].Title != "NewTitle" {
		t.Errorf("history track title = %q, want %q", m.library.SourceTracks[0].Title, "NewTitle")
	}
	if m.library.SourceTracks[0].Artist != "NewArt" {
		t.Errorf("history track artist = %q, want %q", m.library.SourceTracks[0].Artist, "NewArt")
	}
	if m.library.SourceTracks[0].Album != "NewAlbum" {
		t.Errorf("history track album = %q, want %q", m.library.SourceTracks[0].Album, "NewAlbum")
	}
	if m.library.SourceTracks[0].Genre != "NewGenre" {
		t.Errorf("history track genre = %q, want %q", m.library.SourceTracks[0].Genre, "NewGenre")
	}
}

// TestRefreshDerivedViewsRebuildsHistorySortCache verifies that
// refreshDerivedViews recomputes the history sort cache when sorting is active.
func TestRefreshDerivedViewsRebuildsHistorySortCache(t *testing.T) {
	m := New(context.Background())
	m.library.Index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "Art", Title: "Bravo", Album: "Alb"})
	m.library.Index.Insert(library.Track{Hash: "h2", Path: "/b.flac", Artist: "Art", Title: "Alpha", Album: "Alb"})
	m.library.Mode = libtui.LibraryModeHistory
	m.library.SourceTracks = []library.Track{
		{Hash: "h1", Path: "/a.flac", Artist: "Art", Title: "Bravo", Album: "Alb"},
		{Hash: "h2", Path: "/b.flac", Artist: "Art", Title: "Alpha", Album: "Alb"},
	}
	m.library.SortState = sorting.SortState{Field: sorting.SortTitle, Dir: sorting.SortAsc}
	m.library = m.library.RecomputeSortedTracks()

	// Rename h1 so it sorts last.
	m.library.Index.UpdateMetadata("h1", "Zulu", "Art", "Alb", "")
	m = m.refreshDerivedViews()

	if m.library.SortedTracks == nil {
		t.Fatal("expected sortedTracks to be non-nil after refreshDerivedViews")
	}
	last := m.library.SortedTracks[len(m.library.SortedTracks)-1]
	if last.Hash != "h1" || last.Title != "Zulu" {
		t.Errorf("expected renamed track at end of sorted list, got hash=%q title=%q", last.Hash, last.Title)
	}
}

// TestRefreshDerivedViewsRebuildsHistoryFilterCache verifies that
// refreshDerivedViews recomputes the history filter cache when a filter is
// active.
func TestRefreshDerivedViewsRebuildsHistoryFilterCache(t *testing.T) {
	m := New(context.Background())
	m.library.Index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "Art", Title: "OldTitle", Album: "Alb"})
	m.library.Mode = libtui.LibraryModeHistory
	m.library.SourceTracks = []library.Track{
		{Hash: "h1", Path: "/a.flac", Artist: "Art", Title: "OldTitle", Album: "Alb"},
	}
	m.library.Filter.Text = "Unique"
	m.library = m.library.RebuildFilterCache()
	if len(m.library.FilteredMatches) != 0 {
		t.Fatalf("expected 0 filter matches before rename, got %d", len(m.library.FilteredMatches))
	}

	m.library.Index.UpdateMetadata("h1", "Unique Title", "Art", "Alb", "")
	m = m.refreshDerivedViews()

	if len(m.library.FilteredMatches) == 0 {
		t.Error("expected filter matches after renaming track to 'Unique Title', got 0")
	}
}

// TestRefreshDerivedViewsSkipsHistoryWhenEmpty verifies that
// refreshDerivedViews does not panic when history has no source tracks.
func TestRefreshDerivedViewsSkipsHistoryWhenEmpty(t *testing.T) {
	m := New(context.Background())
	m.library.Mode = libtui.LibraryModeHistory
	m.library.SourceTracks = nil
	m = m.refreshDerivedViews()
}

// TestRefreshDerivedViewsUpdatesRecentlyAddedTracks verifies that
// refreshDerivedViews updates recentlyAdded source track metadata.
func TestRefreshDerivedViewsUpdatesRecentlyAddedTracks(t *testing.T) {
	m := New(context.Background())
	m.library.Index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "OldArt", Title: "OldTitle", Album: "OldAlbum", Genre: "OldGenre"})
	m.library.Mode = libtui.LibraryModeRecent
	m.library.SourceTracks = []library.Track{
		{Hash: "h1", Path: "/a.flac", Artist: "OldArt", Title: "OldTitle", Album: "OldAlbum", Genre: "OldGenre"},
	}

	m.library.Index.UpdateMetadata("h1", "NewTitle", "NewArt", "NewAlbum", "NewGenre")
	m = m.refreshDerivedViews()

	if m.library.SourceTracks[0].Title != "NewTitle" {
		t.Errorf("recentlyAdded track title = %q, want %q", m.library.SourceTracks[0].Title, "NewTitle")
	}
	if m.library.SourceTracks[0].Artist != "NewArt" {
		t.Errorf("recentlyAdded track artist = %q, want %q", m.library.SourceTracks[0].Artist, "NewArt")
	}
	if m.library.SourceTracks[0].Album != "NewAlbum" {
		t.Errorf("recentlyAdded track album = %q, want %q", m.library.SourceTracks[0].Album, "NewAlbum")
	}
	if m.library.SourceTracks[0].Genre != "NewGenre" {
		t.Errorf("recentlyAdded track genre = %q, want %q", m.library.SourceTracks[0].Genre, "NewGenre")
	}
}

// TestRefreshDerivedViewsRebuildsRecentlyAddedSortCache verifies that
// refreshDerivedViews recomputes the recentlyAdded sort cache when sorting is active.
func TestRefreshDerivedViewsRebuildsRecentlyAddedSortCache(t *testing.T) {
	m := New(context.Background())
	m.library.Index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "Art", Title: "Bravo", Album: "Alb"})
	m.library.Index.Insert(library.Track{Hash: "h2", Path: "/b.flac", Artist: "Art", Title: "Alpha", Album: "Alb"})
	m.library.Mode = libtui.LibraryModeRecent
	m.library.SourceTracks = []library.Track{
		{Hash: "h1", Path: "/a.flac", Artist: "Art", Title: "Bravo", Album: "Alb"},
		{Hash: "h2", Path: "/b.flac", Artist: "Art", Title: "Alpha", Album: "Alb"},
	}
	m.library.SortState = sorting.SortState{Field: sorting.SortTitle, Dir: sorting.SortAsc}
	m.library = m.library.RecomputeSortedTracks()

	m.library.Index.UpdateMetadata("h1", "Zulu", "Art", "Alb", "")
	m = m.refreshDerivedViews()

	if m.library.SortedTracks == nil {
		t.Fatal("expected sortedTracks to be non-nil after refreshDerivedViews")
	}
	last := m.library.SortedTracks[len(m.library.SortedTracks)-1]
	if last.Hash != "h1" || last.Title != "Zulu" {
		t.Errorf("expected renamed track at end of sorted list, got hash=%q title=%q", last.Hash, last.Title)
	}
}

// TestRefreshDerivedViewsRebuildsRecentlyAddedFilterCache verifies that
// refreshDerivedViews recomputes the recentlyAdded filter cache when a filter
// is active.
func TestRefreshDerivedViewsRebuildsRecentlyAddedFilterCache(t *testing.T) {
	m := New(context.Background())
	m.library.Index.Insert(library.Track{Hash: "h1", Path: "/a.flac", Artist: "Art", Title: "OldTitle", Album: "Alb"})
	m.library.Mode = libtui.LibraryModeRecent
	m.library.SourceTracks = []library.Track{
		{Hash: "h1", Path: "/a.flac", Artist: "Art", Title: "OldTitle", Album: "Alb"},
	}
	m.library.Fuzzy = library.SahilmFuzzySearcher{}
	m.library.Filter.Text = "Unique"
	m.library = m.library.RebuildFilterCache()
	if len(m.library.FilteredMatches) != 0 {
		t.Fatalf("expected 0 filter matches before rename, got %d", len(m.library.FilteredMatches))
	}

	m.library.Index.UpdateMetadata("h1", "Unique Title", "Art", "Alb", "")
	m = m.refreshDerivedViews()

	if len(m.library.FilteredMatches) == 0 {
		t.Error("expected filter matches after renaming track to 'Unique Title', got 0")
	}
}
