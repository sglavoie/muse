package favorites

import "testing"

func TestOpenList_TransitionsToCategoryLevel(t *testing.T) {
	v := View{
		Level:  CategoryLevel,
		Cursor: 1,
	}
	v2 := v.OpenList()
	if v2.Level != ListLevel {
		t.Fatalf("expected ListLevel, got %v", v2.Level)
	}
	if v2.Category != CategoryAlbums {
		t.Errorf("expected CategoryAlbums (%d), got %d", CategoryAlbums, v2.Category)
	}
	if v2.CategoryCursor != 1 {
		t.Errorf("expected CategoryCursor=1, got %d", v2.CategoryCursor)
	}
	if v2.Cursor != 0 || v2.ScrollOffset != 0 {
		t.Errorf("expected cursor/scroll reset, got cursor=%d scroll=%d", v2.Cursor, v2.ScrollOffset)
	}
}

func TestOpenTracks_TransitionsToTrackLevel(t *testing.T) {
	v := View{
		Level:        ListLevel,
		Category:     CategoryArtists,
		Cursor:       2,
		ScrollOffset: 1,
	}
	tracks := sampleTracks(3)
	v2 := v.OpenTracks(tracks)
	if v2.Level != TrackLevel {
		t.Fatalf("expected TrackLevel, got %v", v2.Level)
	}
	if v2.ListCursor != 2 {
		t.Errorf("expected ListCursor=2, got %d", v2.ListCursor)
	}
	if v2.ListScrollOffset != 1 {
		t.Errorf("expected ListScrollOffset=1, got %d", v2.ListScrollOffset)
	}
	if v2.Cursor != 0 || v2.ScrollOffset != 0 {
		t.Errorf("expected cursor/scroll reset, got cursor=%d scroll=%d", v2.Cursor, v2.ScrollOffset)
	}
	if v2.SelectedItems != nil {
		t.Error("expected SelectedItems cleared")
	}
}

func TestBack_FromListLevel(t *testing.T) {
	v := View{
		Level:          ListLevel,
		Category:       CategoryArtists,
		Artists:        []string{"A", "B"},
		CategoryCursor: 0,
		SelectedItems:  map[int]struct{}{0: {}},
	}
	v2 := v.Back()
	if v2.Level != CategoryLevel {
		t.Fatalf("expected CategoryLevel, got %v", v2.Level)
	}
	if v2.Cursor != 0 {
		t.Errorf("expected cursor=0, got %d", v2.Cursor)
	}
	if v2.Artists != nil {
		t.Error("expected Artists cleared")
	}
	if v2.SelectedItems != nil {
		t.Error("expected SelectedItems cleared")
	}
}

func TestBack_FromTrackLevel(t *testing.T) {
	v := View{
		Level:            TrackLevel,
		Category:         CategoryArtists,
		Artists:          []string{"A", "B", "C", "D", "E"},
		DrilldownTracks:  sampleTracks(2),
		ListCursor:       3,
		ListScrollOffset: 1,
		SelectedItems:    map[int]struct{}{1: {}},
	}
	v2 := v.Back()
	if v2.Level != ListLevel {
		t.Fatalf("expected ListLevel, got %v", v2.Level)
	}
	if v2.Cursor != 3 {
		t.Errorf("expected cursor=3, got %d", v2.Cursor)
	}
	if v2.ScrollOffset != 1 {
		t.Errorf("expected scrollOffset=1, got %d", v2.ScrollOffset)
	}
	if v2.DrilldownTracks != nil {
		t.Error("expected DrilldownTracks cleared")
	}
}

// TestBack_FromTrackLevelClampsCursorWhenParentShrank verifies that Back()
// clamps the saved list cursor against the (possibly shrunk) favorites list —
// e.g. when an artist was unfavorited while drilled into its tracks.
func TestBack_FromTrackLevelClampsCursorWhenParentShrank(t *testing.T) {
	v := View{
		Level:            TrackLevel,
		Category:         CategoryArtists,
		Artists:          []string{"A", "B"},
		DrilldownTracks:  sampleTracks(2),
		ListCursor:       4,
		ListScrollOffset: 2,
	}
	v2 := v.Back()
	if v2.Level != ListLevel {
		t.Fatalf("expected ListLevel, got %v", v2.Level)
	}
	if v2.Cursor != 1 {
		t.Errorf("expected clamped cursor=1, got %d", v2.Cursor)
	}
	if v2.ScrollOffset > v2.Cursor {
		t.Errorf("ScrollOffset(%d) > Cursor(%d)", v2.ScrollOffset, v2.Cursor)
	}
}

// TestBack_FromTrackLevelClampsCursorWhenParentEmpty resets cursor/scroll to 0
// when the parent list was fully drained during the drill-down.
func TestBack_FromTrackLevelClampsCursorWhenParentEmpty(t *testing.T) {
	v := View{
		Level:            TrackLevel,
		Category:         CategoryArtists,
		Artists:          nil,
		DrilldownTracks:  sampleTracks(2),
		ListCursor:       2,
		ListScrollOffset: 1,
	}
	v2 := v.Back()
	if v2.Cursor != 0 || v2.ScrollOffset != 0 {
		t.Errorf("expected cursor=0 scroll=0, got cursor=%d scroll=%d", v2.Cursor, v2.ScrollOffset)
	}
}

func TestBack_FromTrackLevelKeepsCursorVisibleAfterResize(t *testing.T) {
	v := View{
		Level:            TrackLevel,
		Category:         CategoryArtists,
		Artists:          []string{"A", "B", "C", "D", "E", "F", "G", "H", "I", "J"},
		DrilldownTracks:  sampleTracks(2),
		ListCursor:       8,
		ListScrollOffset: 4,
		Height:           5, // visibleRows at ListLevel is 3
	}
	v2 := v.Back()
	if v2.Level != ListLevel {
		t.Fatalf("expected ListLevel, got %v", v2.Level)
	}
	if v2.Cursor != 8 {
		t.Fatalf("expected cursor=8, got %d", v2.Cursor)
	}
	if v2.ScrollOffset != 6 {
		t.Errorf("expected scrollOffset=6 to keep cursor visible, got %d", v2.ScrollOffset)
	}
}

func TestBack_FromTrackLevelZeroVisibleRowsAnchorsScrollToCursor(t *testing.T) {
	v := View{
		Level:            TrackLevel,
		Category:         CategoryArtists,
		Artists:          []string{"A", "B", "C"},
		DrilldownTracks:  sampleTracks(2),
		ListCursor:       2,
		ListScrollOffset: 0,
		Height:           2, // visibleRows at ListLevel is 0
	}
	v2 := v.Back()
	if v2.Level != ListLevel {
		t.Fatalf("expected ListLevel, got %v", v2.Level)
	}
	if v2.Cursor != 2 {
		t.Fatalf("expected cursor=2, got %d", v2.Cursor)
	}
	if v2.ScrollOffset != v2.Cursor {
		t.Errorf("expected scrollOffset anchored to cursor, got cursor=%d scroll=%d", v2.Cursor, v2.ScrollOffset)
	}
}

func TestBack_RoundTrip(t *testing.T) {
	v := View{
		Level:  CategoryLevel,
		Cursor: 2,
	}
	v = v.OpenList()
	v.Tracks = sampleTracks(5)
	v = v.OpenTracks(sampleTracks(3))
	v = v.Back()
	if v.Level != ListLevel {
		t.Fatalf("expected ListLevel, got %v", v.Level)
	}
	v = v.Back()
	if v.Level != CategoryLevel {
		t.Fatalf("expected CategoryLevel, got %v", v.Level)
	}
	if v.Cursor != 2 {
		t.Errorf("expected cursor=2, got %d", v.Cursor)
	}
}
