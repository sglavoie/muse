package tags

import (
	"testing"

	"github.com/sglavoie/muse/internal/db"
)

// TestTagsViewDisplayTracksStaleSortOrderLength verifies that displayTracks
// does not return zero-padded rows when sortedTrackOrder has fewer entries
// than tracks (e.g. after a track was deleted but the sort permutation was
// not yet recomputed). The view must safely fall back to natural order.
func TestTagsViewDisplayTracksStaleSortOrderLength(t *testing.T) {
	v := TagsView{
		Level: TrackLevel,
		Tracks: []db.PlaylistTrackRow{
			{ID: 1, Title: "A", Hash: "h1"},
			{ID: 2, Title: "B", Hash: "h2"},
			{ID: 3, Title: "C", Hash: "h3"},
		},
		// Stale: only 2 entries even though tracks has 3.
		SortedTrackOrder: []int{2, 0},
	}
	got := v.DisplayTracks()
	if len(got) != 3 {
		t.Fatalf("expected 3 tracks (natural order fallback), got %d", len(got))
	}
	// Confirm none are zero-filled (Title would be empty).
	for i, r := range got {
		if r.Title == "" {
			t.Errorf("track %d: empty title indicates zero-filled fallback row", i)
		}
	}
}

// TestTagsViewDisplayTracksStaleSortOrderIndices verifies that displayTracks
// falls back to natural order when sortedTrackOrder contains an index that
// is out of range for the tracks slice. This guards against panics when a
// track is removed but the sort permutation is not yet refreshed.
func TestTagsViewDisplayTracksStaleSortOrderIndices(t *testing.T) {
	v := TagsView{
		Level: TrackLevel,
		Tracks: []db.PlaylistTrackRow{
			{ID: 1, Title: "A", Hash: "h1"},
			{ID: 2, Title: "B", Hash: "h2"},
		},
		// Length matches but indices are out of range.
		SortedTrackOrder: []int{1, 5},
	}
	got := v.DisplayTracks()
	if len(got) != 2 {
		t.Fatalf("expected 2 tracks (natural fallback), got %d", len(got))
	}
	if got[0].Title != "A" || got[1].Title != "B" {
		t.Errorf("expected natural order [A B], got [%s %s]", got[0].Title, got[1].Title)
	}
}

// TestTagsViewDisplayTracksValidSortOrderRespected verifies that a valid
// sortedTrackOrder still reorders tracks as expected (no false negative
// from the new defensive guard).
func TestTagsViewDisplayTracksValidSortOrderRespected(t *testing.T) {
	v := TagsView{
		Level: TrackLevel,
		Tracks: []db.PlaylistTrackRow{
			{ID: 1, Title: "A", Hash: "h1"},
			{ID: 2, Title: "B", Hash: "h2"},
			{ID: 3, Title: "C", Hash: "h3"},
		},
		SortedTrackOrder: []int{2, 0, 1},
	}
	got := v.DisplayTracks()
	if len(got) != 3 {
		t.Fatalf("expected 3 tracks, got %d", len(got))
	}
	if got[0].Title != "C" || got[1].Title != "A" || got[2].Title != "B" {
		t.Errorf("expected order [C A B], got [%s %s %s]", got[0].Title, got[1].Title, got[2].Title)
	}
}
