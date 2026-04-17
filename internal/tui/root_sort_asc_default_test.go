package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
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

// TestSortCycleTagsTrackLevel verifies that handleSortCycle works for the
// Tags view at TrackLevel, cycling through the expected sort fields.
func TestSortCycleTagsTrackLevel(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewTags
	m.tagsView.level = tagtui.TrackLevel
	m.tagsView.tracks = []db.PlaylistTrackRow{
		{Hash: "h1", Title: "Zulu", Artist: "C", Album: "Red", DurationMs: 300000},
		{Hash: "h2", Title: "Alpha", Artist: "A", Album: "Blue", DurationMs: 100000},
	}
	m.tagsView.width = 80
	m.tagsView.height = 40

	// Cycle: Default -> Title -> Artist -> Album -> Duration -> Default
	expected := []SortField{SortTitle, SortArtist, SortAlbum, SortDuration, SortDefault}
	for _, want := range expected {
		next, _, consumed := m.handleSortCycle()
		if !consumed {
			t.Fatalf("handleSortCycle should be consumed for Tags TrackLevel (want field %d)", want)
		}
		m = next.(Model)
		if m.tagsView.sortState.Field != want {
			t.Errorf("expected sort field %d, got %d", want, m.tagsView.sortState.Field)
		}
	}
}

// TestSortCycleTagsTagLevel verifies that handleSortCycle is a no-op at TagLevel.
func TestSortCycleTagsTagLevel(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewTags
	m.tagsView.level = tagtui.TagLevel

	_, _, consumed := m.handleSortCycle()
	if consumed {
		t.Error("handleSortCycle should not be consumed at Tags TagLevel")
	}
}

// TestSortDirTagsTrackLevel verifies that handleSortDir works for Tags at
// TrackLevel when a sort field is active.
func TestSortDirTagsTrackLevel(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewTags
	m.tagsView.level = tagtui.TrackLevel
	m.tagsView.tracks = []db.PlaylistTrackRow{
		{Hash: "h1", Title: "Zulu", Artist: "C"},
		{Hash: "h2", Title: "Alpha", Artist: "A"},
	}
	m.tagsView.sortState = SortState{Field: SortTitle, Dir: SortAsc}
	m.tagsView = m.tagsView.recomputeTagsSort()
	m.tagsView.width = 80
	m.tagsView.height = 40

	next, _, consumed := m.handleSortDir(SortDesc)
	if !consumed {
		t.Fatal("handleSortDir should be consumed for Tags with active sort field")
	}
	m = next.(Model)
	if m.tagsView.sortState.Dir != SortDesc {
		t.Errorf("expected SortDesc, got %v", m.tagsView.sortState.Dir)
	}
	display := m.tagsView.displayTracks()
	if display[0].Title != "Zulu" {
		t.Errorf("first track in desc order should be Zulu, got %q", display[0].Title)
	}
}

// TestSortDirTagsNoopWhenDefault verifies that handleSortDir returns false
// when Tags sort field is SortDefault.
func TestSortDirTagsNoopWhenDefault(t *testing.T) {
	m := New(context.Background())
	m.activeView = viewTags
	m.tagsView.level = tagtui.TrackLevel
	m.tagsView.sortState = SortState{Field: SortDefault}

	_, _, consumed := m.handleSortDir(SortAsc)
	if consumed {
		t.Error("handleSortDir should not be consumed when sort field is SortDefault")
	}
}
