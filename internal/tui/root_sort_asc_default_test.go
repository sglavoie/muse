package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/library"
)

// TestSortAscNoopWhenDefault verifies that handleSortAsc returns false (not
// consumed) for the recently added and history views when no sort field has
// been selected. Without the SortDefault guard, pressing the asc key would
// consume the event and show a misleading sort notification.
func TestSortAscNoopWhenDefault(t *testing.T) {
	tracks := []library.Track{
		{Hash: "1", Artist: "A", Title: "T1"},
		{Hash: "2", Artist: "B", Title: "T2"},
	}

	tests := []struct {
		name string
		view viewID
	}{
		{"RecentlyAdded", viewRecentlyAdded},
		{"History", viewHistory},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New(context.Background(), WithLibraryTracks(tracks))
			m.activeView = tt.view

			// Ensure the sort state is at default.
			switch tt.view {
			case viewRecentlyAdded:
				m.recentlyAdded.sortState = SortState{Field: SortDefault, Dir: SortAsc}
			case viewHistory:
				m.history.sortState = SortState{Field: SortDefault, Dir: SortAsc}
			}

			_, _, consumed := m.handleSortAsc()
			if consumed {
				t.Error("handleSortAsc should not consume the key when sort field is SortDefault")
			}
		})
	}
}

// TestSortDescNoopWhenDefault verifies the same guard exists for handleSortDesc.
func TestSortDescNoopWhenDefault(t *testing.T) {
	tracks := []library.Track{
		{Hash: "1", Artist: "A", Title: "T1"},
		{Hash: "2", Artist: "B", Title: "T2"},
	}

	tests := []struct {
		name string
		view viewID
	}{
		{"RecentlyAdded", viewRecentlyAdded},
		{"History", viewHistory},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New(context.Background(), WithLibraryTracks(tracks))
			m.activeView = tt.view

			switch tt.view {
			case viewRecentlyAdded:
				m.recentlyAdded.sortState = SortState{Field: SortDefault, Dir: SortDesc}
			case viewHistory:
				m.history.sortState = SortState{Field: SortDefault, Dir: SortDesc}
			}

			_, _, consumed := m.handleSortDesc()
			if consumed {
				t.Error("handleSortDesc should not consume the key when sort field is SortDefault")
			}
		})
	}
}
