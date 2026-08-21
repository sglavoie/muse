package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/library"
)

// drilledInsights returns a model with an open Insights drill over the given
// tracks, mirroring what openDrill builds at runtime.
func drilledInsights(t *testing.T, tracks []library.Track) Model {
	t.Helper()
	m := New(context.Background())
	for _, tr := range tracks {
		m.library.index.Insert(tr)
	}
	m.activeView = viewInsights
	m.insights.setSize(100, 30)
	m.insights.dormant = tracks
	m.insights.openDrill("No album", m.insightsDrillTracks(insightsRow{drill: drillDormant}))
	return m
}

// TestRefreshDerivedViewsUpdatesInsightsDrillTracks covers the reported bug: an
// import edits the library index while an Insights drill is open, and the drill
// keeps rendering the snapshot openDrill froze.
func TestRefreshDerivedViewsUpdatesInsightsDrillTracks(t *testing.T) {
	m := drilledInsights(t, []library.Track{
		{Hash: "h1", Path: "/a.flac", Artist: "OldArt", Title: "OldTitle", Album: "", Genre: "OldGenre", Duration: 120},
	})

	m.library.index.UpdateMetadata("h1", "NewTitle", "NewArt", "NewAlbum", "NewGenre")
	m.library.index.UpdateDuration("h1", 200)
	m.library.index.UpdateTrackPosition("h1", 7, 2)
	m = m.refreshDerivedViews()

	got := m.insights.trackView.sourceTracks[0]
	if got.Title != "NewTitle" || got.Artist != "NewArt" || got.Album != "NewAlbum" || got.Genre != "NewGenre" {
		t.Errorf("drill track metadata not refreshed: %+v", got)
	}
	if got.Duration != 200 {
		t.Errorf("drill track duration = %v, want 200", got.Duration)
	}
	if got.TrackNumber != 7 || got.DiscNumber != 2 {
		t.Errorf("drill track position = %d/%d, want 7/2", got.TrackNumber, got.DiscNumber)
	}
}

// TestRefreshDerivedViewsPreservesInsightsSelectionAndSort verifies the refresh
// is a value update: the hash-keyed bulk selection survives and an active sort
// is recomputed rather than dropped.
func TestRefreshDerivedViewsPreservesInsightsSelectionAndSort(t *testing.T) {
	m := drilledInsights(t, []library.Track{
		{Hash: "h1", Path: "/a.flac", Artist: "A", Title: "Zulu"},
		{Hash: "h2", Path: "/b.flac", Artist: "B", Title: "Alpha"},
	})
	m.insights.trackView.selectedHashes = map[string]struct{}{"h2": {}}
	m.insights.trackView.sortState = SortState{Field: SortTitle, Dir: SortAsc}
	m.insights.trackView = m.insights.trackView.refreshView()

	m.library.index.UpdateMetadata("h1", "Bravo", "A", "", "")
	m = m.refreshDerivedViews()

	if _, ok := m.insights.trackView.selectedHashes["h2"]; !ok {
		t.Error("the hash-keyed selection must survive an in-place value edit")
	}
	sorted := m.insights.trackView.sortedTracks
	if len(sorted) != 2 || sorted[0].Title != "Alpha" || sorted[1].Title != "Bravo" {
		t.Errorf("sorted cache not recomputed against the new titles: %+v", sorted)
	}
}

// TestRefreshDerivedViewsClampsInsightsCursorWhenAFilterNarrows covers the
// clampCursor call: with an active fuzzy filter, an edit that stops matching
// shrinks filteredMatches and would leave the cursor out of range.
func TestRefreshDerivedViewsClampsInsightsCursorWhenAFilterNarrows(t *testing.T) {
	m := drilledInsights(t, []library.Track{
		{Hash: "h1", Path: "/a.flac", Artist: "A", Title: "zebra one"},
		{Hash: "h2", Path: "/b.flac", Artist: "B", Title: "zebra two"},
	})
	m.insights.trackView.filter.Text = "zebra"
	m.insights.trackView = m.insights.trackView.refreshView()
	m.insights.trackView.cursor = 1
	if len(m.insights.trackView.filteredMatches) != 2 {
		t.Fatalf("expected both tracks to match, got %d", len(m.insights.trackView.filteredMatches))
	}

	m.library.index.UpdateMetadata("h2", "quiet", "B", "", "")
	m = m.refreshDerivedViews()

	if n := len(m.insights.trackView.filteredMatches); n != 1 {
		t.Fatalf("expected the filter to narrow to one match, got %d", n)
	}
	if m.insights.trackView.cursor != 0 {
		t.Errorf("cursor = %d, want it clamped to 0", m.insights.trackView.cursor)
	}
}

// TestRefreshDerivedViewsSkipsClosedInsightsDrill verifies the briefing level is
// a no-op, so a stale snapshot left behind by a previous drill is not resurrected.
func TestRefreshDerivedViewsSkipsClosedInsightsDrill(t *testing.T) {
	m := drilledInsights(t, []library.Track{
		{Hash: "h1", Path: "/a.flac", Artist: "OldArt", Title: "OldTitle"},
	})
	m.insights.back()

	m.library.index.UpdateMetadata("h1", "NewTitle", "NewArt", "", "")
	m = m.refreshDerivedViews()

	// back() drops trackView, so the aliased dormant snapshot is what would
	// carry a stray write.
	if got := m.insights.dormant[0].Title; got != "OldTitle" {
		t.Errorf("a closed drill must be left alone, got title %q", got)
	}
	if m.insights.trackView.sourceTracks != nil {
		t.Error("refreshDerivedViews must not repopulate a closed drill")
	}
}
