package playlists

import (
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/db"
)

// --- bulk-selection state ---

func TestToggleBulkSelectionToggles(t *testing.T) {
	v := View{}

	v = v.ToggleBulkSelection(42)
	if _, ok := v.SelectedTrackIDs[42]; !ok {
		t.Fatal("expected ID 42 to be selected after first toggle")
	}

	v = v.ToggleBulkSelection(42)
	if _, ok := v.SelectedTrackIDs[42]; ok {
		t.Fatal("expected ID 42 to be deselected after second toggle")
	}
	if len(v.SelectedTrackIDs) != 0 {
		t.Fatalf("expected empty selection set, got %v", v.SelectedTrackIDs)
	}
}

func TestToggleBulkSelection_CopyOnWriteAdd(t *testing.T) {
	original := View{SelectedTrackIDs: map[int64]struct{}{1: {}}}

	updated := original.ToggleBulkSelection(2)

	if _, ok := original.SelectedTrackIDs[2]; ok {
		t.Fatal("original view selection must remain unchanged")
	}
	if _, ok := updated.SelectedTrackIDs[2]; !ok {
		t.Fatal("updated view should contain the toggled ID")
	}
}

func TestToggleBulkSelection_CopyOnWriteRemove(t *testing.T) {
	original := View{SelectedTrackIDs: map[int64]struct{}{1: {}, 2: {}}}

	updated := original.ToggleBulkSelection(2)

	if _, ok := original.SelectedTrackIDs[2]; !ok {
		t.Fatal("original view selection must remain unchanged")
	}
	if _, ok := updated.SelectedTrackIDs[2]; ok {
		t.Fatal("updated view should remove the toggled ID")
	}
}

func TestToggleBulkSelectionZeroIDIsNoOp(t *testing.T) {
	v := View{}
	v = v.ToggleBulkSelection(0)
	if len(v.SelectedTrackIDs) != 0 {
		t.Fatalf("ToggleBulkSelection(0) must be a no-op; got %v", v.SelectedTrackIDs)
	}
}

func TestClearBulkSelectionEmptiesSet(t *testing.T) {
	v := View{}
	v = v.ToggleBulkSelection(1)
	v = v.ToggleBulkSelection(2)
	v = v.ClearBulkSelection()
	if len(v.SelectedTrackIDs) != 0 {
		t.Fatalf("expected empty selection after ClearBulkSelection, got %v", v.SelectedTrackIDs)
	}
}

func TestRenderTracksShowsBulletOnlyForSelected(t *testing.T) {
	tracks := []db.PlaylistTrackRow{
		{ID: 10, Position: 1, Title: "Selected Song", Artist: "A", DurationMs: 60000},
		{ID: 20, Position: 2, Title: "Unselected Song", Artist: "B", DurationMs: 60000},
	}
	v := View{
		Level:            TrackLevel,
		Width:            80,
		Tracks:           tracks,
		SelectedTrackIDs: map[int64]struct{}{10: {}},
	}
	out := v.Render()
	if !strings.Contains(out, "●") {
		t.Fatal("expected '●' marker in output for selected track")
	}
	lines := strings.Split(out, "\n")
	var selectedLine, unselectedLine string
	for _, l := range lines {
		if strings.Contains(l, "Selected Song") {
			selectedLine = l
		}
		if strings.Contains(l, "Unselected Song") {
			unselectedLine = l
		}
	}
	if !strings.Contains(selectedLine, "●") {
		t.Errorf("selected track line must contain '●'; got: %q", selectedLine)
	}
	if strings.Contains(unselectedLine, "●") {
		t.Errorf("unselected track line must not contain '●'; got: %q", unselectedLine)
	}
}

// --- 'x' bulk selection toggle at TrackLevel ---

func TestBulkSelectionToggleAtTrackLevelAddsID(t *testing.T) {
	tracks := []db.PlaylistTrackRow{
		{ID: 10, Position: 1, Title: "Song A"},
		{ID: 20, Position: 2, Title: "Song B"},
	}
	v := View{
		Level:  TrackLevel,
		Cursor: 0,
		Tracks: tracks,
	}

	v = v.ToggleBulkSelection(tracks[0].ID)
	if _, ok := v.SelectedTrackIDs[10]; !ok {
		t.Error("ToggleBulkSelection: expected ID 10 in SelectedTrackIDs")
	}
}

func TestBulkSelectionToggleAtTrackLevelRemovesID(t *testing.T) {
	v := View{
		Level:            TrackLevel,
		SelectedTrackIDs: map[int64]struct{}{10: {}},
	}
	v = v.ToggleBulkSelection(10)
	if _, ok := v.SelectedTrackIDs[10]; ok {
		t.Error("second ToggleBulkSelection must deselect the track")
	}
}

func TestBulkSelectionToggleMultipleIDs(t *testing.T) {
	v := View{Level: TrackLevel}
	v = v.ToggleBulkSelection(10)
	v = v.ToggleBulkSelection(20)
	v = v.ToggleBulkSelection(30)

	for _, id := range []int64{10, 20, 30} {
		if _, ok := v.SelectedTrackIDs[id]; !ok {
			t.Errorf("expected ID %d in SelectedTrackIDs", id)
		}
	}
	if len(v.SelectedTrackIDs) != 3 {
		t.Errorf("expected 3 selected IDs, got %d", len(v.SelectedTrackIDs))
	}
}

// --- 'a' bulk remove at TrackLevel ---

func TestBulkRemoveTracksAtTrackLevel(t *testing.T) {
	tracks := []db.PlaylistTrackRow{
		{ID: 10, Position: 1, Title: "Keep"},
		{ID: 20, Position: 2, Title: "Remove A"},
		{ID: 30, Position: 3, Title: "Remove B"},
		{ID: 40, Position: 4, Title: "Keep Too"},
	}
	v := View{
		Level:  TrackLevel,
		Cursor: 3,
		Tracks: tracks,
		SelectedTrackIDs: map[int64]struct{}{
			20: {},
			30: {},
		},
	}

	var remaining []db.PlaylistTrackRow
	for _, tr := range v.Tracks {
		if _, ok := v.SelectedTrackIDs[tr.ID]; !ok {
			remaining = append(remaining, tr)
		}
	}
	v.Tracks = remaining
	if v.Cursor >= len(v.Tracks) && v.Cursor > 0 {
		v.Cursor = len(v.Tracks) - 1
	}
	v = v.ClearBulkSelection()

	if len(v.Tracks) != 2 {
		t.Errorf("expected 2 tracks after bulk remove, got %d", len(v.Tracks))
	}
	for _, tr := range v.Tracks {
		if tr.Title == "Remove A" || tr.Title == "Remove B" {
			t.Errorf("bulk remove must eliminate %q", tr.Title)
		}
	}
	if v.Cursor != 1 {
		t.Errorf("expected cursor clamped to 1 (new last index), got %d", v.Cursor)
	}
	if len(v.SelectedTrackIDs) != 0 {
		t.Errorf("expected selection cleared after bulk remove, got %v", v.SelectedTrackIDs)
	}
}

func TestBulkRemoveSelectAllClearsTrackList(t *testing.T) {
	v := View{
		Level:  TrackLevel,
		Cursor: 1,
		Tracks: []db.PlaylistTrackRow{
			{ID: 1, Position: 1, Title: "A"},
			{ID: 2, Position: 2, Title: "B"},
		},
		SelectedTrackIDs: map[int64]struct{}{1: {}, 2: {}},
	}

	var remaining []db.PlaylistTrackRow
	for _, tr := range v.Tracks {
		if _, ok := v.SelectedTrackIDs[tr.ID]; !ok {
			remaining = append(remaining, tr)
		}
	}
	v.Tracks = remaining
	if v.Cursor >= len(v.Tracks) && v.Cursor > 0 {
		v.Cursor = len(v.Tracks) - 1
	}
	v = v.ClearBulkSelection()

	if len(v.Tracks) != 0 {
		t.Errorf("expected empty track list after removing all, got %d", len(v.Tracks))
	}
	if v.Cursor != -1 {
		t.Errorf("expected cursor=-1 after removing all tracks (len-1), got %d", v.Cursor)
	}
	if v.SelectedTrackIDs != nil {
		t.Errorf("expected SelectedTrackIDs=nil after ClearBulkSelection, got %v", v.SelectedTrackIDs)
	}
}
