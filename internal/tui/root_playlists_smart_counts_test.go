package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
)

// TestSmartPlaylistCountsUpdatesSortedCopy verifies that when smart playlist
// track counts are updated, the sorted playlist copy is recomputed so that
// sort-by-TrackCount reflects the new values and order.
func TestSmartPlaylistCountsUpdatesSortedCopy(t *testing.T) {
	playlists := []db.PlaylistRow{
		{ID: 1, Name: "Rock", TrackCount: 10},
		{ID: 2, Name: "Smart", TrackCount: 0, IsSmart: true, Query: `{"field":"artist","op":"eq","value":"X"}`},
	}
	m := New(context.Background(), WithPlaylists(playlists))
	m.activeView = viewPlaylists
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = next.(Model)

	// Sort by track count descending: Rock(10) before Smart(0).
	m.playlists.sortState = SortState{Field: SortTrackCount, Dir: SortDesc}
	m.playlists = m.playlists.recomputePlaylistSort()

	effective := m.playlists.effectivePlaylists()
	if effective[0].ID != 1 {
		t.Fatalf("before update: first should be Rock(ID=1), got ID=%d", effective[0].ID)
	}

	// Simulate smart playlist count update: Smart now has 20 tracks.
	next, _ = m.Update(smartPlaylistCountsUpdatedMsg{counts: map[int64]int{2: 20}})
	m = next.(Model)

	effective = m.playlists.effectivePlaylists()
	if effective[0].ID != 2 {
		t.Errorf("after count update: first should be Smart(ID=2, count=20), got ID=%d (count=%d)",
			effective[0].ID, effective[0].TrackCount)
	}
	if effective[0].TrackCount != 20 {
		t.Errorf("Smart playlist track count should be 20, got %d", effective[0].TrackCount)
	}
}
