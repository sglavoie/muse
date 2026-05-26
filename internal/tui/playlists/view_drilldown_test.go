package playlists

import (
	"testing"

	"github.com/sglavoie/muse/internal/db"
)

// --- drill-down Enter / Esc transitions ---

func TestOpenTracksTransitionsToTrackLevel(t *testing.T) {
	tracks := []db.PlaylistTrackRow{
		{ID: 1, Position: 1, Title: "Song A"},
		{ID: 2, Position: 2, Title: "Song B"},
	}
	v := View{
		Level:  ListLevel,
		Cursor: 2,
		Playlists: []db.PlaylistRow{
			{ID: 1, Name: "Alpha"},
			{ID: 2, Name: "Beta"},
			{ID: 3, Name: "Gamma"},
		},
	}
	v2 := v.OpenTracks(tracks)

	if v2.Level != TrackLevel {
		t.Errorf("OpenTracks: expected Level=TrackLevel, got %v", v2.Level)
	}
	if v2.Cursor != 0 {
		t.Errorf("OpenTracks: expected Cursor=0, got %d", v2.Cursor)
	}
	if v2.ListCursor != 2 {
		t.Errorf("OpenTracks: expected ListCursor=2 (saved), got %d", v2.ListCursor)
	}
	if len(v2.Tracks) != 2 {
		t.Errorf("OpenTracks: expected 2 tracks, got %d", len(v2.Tracks))
	}
}

func TestOpenTracksResetsScrollOffset(t *testing.T) {
	tracks := []db.PlaylistTrackRow{
		{ID: 1, Position: 1, Title: "Song A"},
		{ID: 2, Position: 2, Title: "Song B"},
	}
	v := View{
		Level:        ListLevel,
		Cursor:       5,
		ScrollOffset: 3,
		Playlists:    make([]db.PlaylistRow, 10),
	}

	v = v.OpenTracks(tracks)

	if v.ScrollOffset != 0 {
		t.Errorf("OpenTracks: expected ScrollOffset=0, got %d", v.ScrollOffset)
	}
}

func TestOpenTracksClearsBulkSelection(t *testing.T) {
	v := View{
		Level:            ListLevel,
		Cursor:           1,
		ScrollOffset:     1,
		SelectedTrackIDs: map[int64]struct{}{10: {}, 20: {}},
	}

	v = v.OpenTracks([]db.PlaylistTrackRow{{ID: 1}})

	if v.SelectedTrackIDs != nil {
		t.Errorf("OpenTracks: expected SelectedTrackIDs=nil, got %v", v.SelectedTrackIDs)
	}
}

func TestBackToListRestoresCursorAndLevel(t *testing.T) {
	tracks := []db.PlaylistTrackRow{
		{ID: 1, Position: 1, Title: "Song A"},
		{ID: 2, Position: 2, Title: "Song B"},
	}
	v := View{
		Level:  ListLevel,
		Cursor: 3,
		Playlists: []db.PlaylistRow{
			{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4},
		},
	}

	v = v.OpenTracks(tracks)
	if v.Cursor != 0 {
		t.Fatalf("precondition: expected track cursor=0, got %d", v.Cursor)
	}

	v = v.BackToList()
	if v.Level != ListLevel {
		t.Errorf("BackToList: expected Level=ListLevel, got %v", v.Level)
	}
	if v.Cursor != 3 {
		t.Errorf("BackToList: expected Cursor=3 (restored), got %d", v.Cursor)
	}
	if v.Tracks != nil {
		t.Errorf("BackToList: expected Tracks=nil, got %v", v.Tracks)
	}
}

func TestBackToListClearsBulkSelection(t *testing.T) {
	v := View{
		Level:            TrackLevel,
		ListCursor:       1,
		ListScrollOffset: 0,
		Tracks:           []db.PlaylistTrackRow{{ID: 1}},
		SelectedTrackIDs: map[int64]struct{}{10: {}, 20: {}},
		Playlists:        []db.PlaylistRow{{ID: 1}, {ID: 2}},
	}

	v = v.BackToList()

	if v.SelectedTrackIDs != nil {
		t.Errorf("BackToList: expected SelectedTrackIDs=nil, got %v", v.SelectedTrackIDs)
	}
}

// TestBackToListClampsCursorWhenParentShrank verifies that BackToList clamps
// the saved list cursor against the (possibly shrunk) Playlists slice — e.g.
// when a playlist was deleted while the user was viewing its tracks.
func TestBackToListClampsCursorWhenParentShrank(t *testing.T) {
	v := View{
		Level:  ListLevel,
		Cursor: 4,
		Playlists: []db.PlaylistRow{
			{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4}, {ID: 5},
		},
		ScrollOffset: 2,
	}
	v = v.OpenTracks([]db.PlaylistTrackRow{{ID: 1}})
	// Simulate playlist deletion while drilled into the previously selected
	// playlist's tracks.
	v.Playlists = []db.PlaylistRow{{ID: 1}, {ID: 2}}
	v = v.BackToList()
	if v.Level != ListLevel {
		t.Fatalf("expected ListLevel, got %v", v.Level)
	}
	if v.Cursor != 1 {
		t.Errorf("BackToList: expected clamped Cursor=1, got %d", v.Cursor)
	}
	if v.ScrollOffset > v.Cursor {
		t.Errorf("BackToList: ScrollOffset(%d) > Cursor(%d)", v.ScrollOffset, v.Cursor)
	}
}

// TestBackToListClampsCursorWhenParentEmpty resets cursor/scroll to 0 when
// every playlist was removed while drilled into TrackLevel.
func TestBackToListClampsCursorWhenParentEmpty(t *testing.T) {
	v := View{
		Level:        ListLevel,
		Cursor:       2,
		ScrollOffset: 1,
		Playlists:    []db.PlaylistRow{{ID: 1}, {ID: 2}, {ID: 3}},
	}
	v = v.OpenTracks([]db.PlaylistTrackRow{{ID: 1}})
	// Empty (non-nil) slice signals that the caller knows the parent list is
	// drained — distinct from nil which means "do not clamp here".
	v.Playlists = []db.PlaylistRow{}
	v = v.BackToList()
	if v.Cursor != 0 || v.ScrollOffset != 0 {
		t.Errorf("BackToList with empty parent: expected cursor=0 scroll=0, got cursor=%d scroll=%d", v.Cursor, v.ScrollOffset)
	}
}

// TestBackToListKeepsCursorVisibleAfterResize verifies that BackToList scrolls
// the visible window forward when the saved ListScrollOffset trailed behind a
// still-valid ListCursor (e.g. the terminal was resized smaller during the
// drill-down) so the focused row is rendered inside the visible window.
func TestBackToListKeepsCursorVisibleAfterResize(t *testing.T) {
	v := View{
		Level:            TrackLevel,
		Cursor:           0,
		Tracks:           []db.PlaylistTrackRow{{ID: 1}},
		ListCursor:       8,
		ListScrollOffset: 0,
		Playlists: []db.PlaylistRow{
			{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4}, {ID: 5},
			{ID: 6}, {ID: 7}, {ID: 8}, {ID: 9}, {ID: 10},
		},
		Height: 6, // ListLevel visibleRows = 5 (reserve = 1)
	}
	v = v.BackToList()
	if v.Level != ListLevel {
		t.Fatalf("expected ListLevel, got %v", v.Level)
	}
	if v.Cursor != 8 {
		t.Fatalf("expected cursor=8, got %d", v.Cursor)
	}
	maxRows := v.visibleRows()
	if v.Cursor < v.ScrollOffset || v.Cursor >= v.ScrollOffset+maxRows {
		t.Errorf("cursor=%d not visible in window [%d, %d)", v.Cursor, v.ScrollOffset, v.ScrollOffset+maxRows)
	}
}

// TestBackToListSkipsClampWhenPlaylistsNil verifies the escape hatch used by
// outer wrappers that defer parent-list clamping: when Playlists is nil, the
// restored cursor/scroll are passed through unchanged.
func TestBackToListSkipsClampWhenPlaylistsNil(t *testing.T) {
	v := View{
		Level:            TrackLevel,
		Cursor:           0,
		Tracks:           []db.PlaylistTrackRow{{ID: 1}},
		ListCursor:       7,
		ListScrollOffset: 4,
		// Playlists deliberately left nil.
	}
	v = v.BackToList()
	if v.Cursor != 7 {
		t.Errorf("expected pass-through Cursor=7, got %d", v.Cursor)
	}
	if v.ScrollOffset != 4 {
		t.Errorf("expected pass-through ScrollOffset=4, got %d", v.ScrollOffset)
	}
}

func TestEscAtListLevelIsNoOp(t *testing.T) {
	v := View{
		Level:  ListLevel,
		Cursor: 1,
		Playlists: []db.PlaylistRow{
			{ID: 1, Name: "A"},
			{ID: 2, Name: "B"},
		},
	}

	if v.Level != ListLevel {
		v = v.BackToList()
	}

	if v.Level != ListLevel {
		t.Errorf("Esc at ListLevel: expected Level=ListLevel, got %v", v.Level)
	}
	if v.Cursor != 1 {
		t.Errorf("Esc at ListLevel: expected Cursor=1 (unchanged), got %d", v.Cursor)
	}
}
