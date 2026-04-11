package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/library"
)

// TestSortDirNoopWhenDefault verifies that handleSortDir returns false (not
// consumed) for the recently added and history views when no sort field has
// been selected. Without the SortDefault guard, pressing the asc/desc key
// would consume the event and show a misleading sort notification.
func TestSortDirNoopWhenDefault(t *testing.T) {
	tracks := []library.Track{
		{Hash: "1", Artist: "A", Title: "T1"},
		{Hash: "2", Artist: "B", Title: "T2"},
	}

	tests := []struct {
		name string
		view viewID
		dir  SortDir
	}{
		{"RecentlyAdded/Asc", viewRecentlyAdded, SortAsc},
		{"History/Asc", viewHistory, SortAsc},
		{"RecentlyAdded/Desc", viewRecentlyAdded, SortDesc},
		{"History/Desc", viewHistory, SortDesc},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New(context.Background(), WithLibraryTracks(tracks))
			m.activeView = tt.view

			switch tt.view {
			case viewRecentlyAdded:
				m.recentlyAdded.sortState = SortState{Field: SortDefault, Dir: tt.dir}
			case viewHistory:
				m.history.sortState = SortState{Field: SortDefault, Dir: tt.dir}
			}

			_, _, consumed := m.handleSortDir(tt.dir)
			if consumed {
				t.Errorf("handleSortDir(%v) should not consume the key when sort field is SortDefault", tt.dir)
			}
		})
	}
}
