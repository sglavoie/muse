package favorites

import "testing"

func TestItemCount_CategoryLevel(t *testing.T) {
	v := View{Level: CategoryLevel}
	if got := v.itemCount(); got != 3 {
		t.Errorf("expected 3, got %d", got)
	}
}

func TestItemCount_ListLevel_Artists(t *testing.T) {
	v := View{
		Level:    ListLevel,
		Category: CategoryArtists,
		Artists:  []string{"A", "B", "C"},
	}
	if got := v.itemCount(); got != 3 {
		t.Errorf("expected 3, got %d", got)
	}
}

func TestItemCount_ListLevel_ArtistsWithFilter(t *testing.T) {
	v := View{
		Level:      ListLevel,
		Category:   CategoryArtists,
		Artists:    []string{"Alice", "Bob", "Carol"},
		FilterText: "ali",
	}
	if got := v.itemCount(); got != 1 {
		t.Errorf("expected 1, got %d", got)
	}
}

func TestItemCount_ListLevel_Tracks(t *testing.T) {
	v := View{
		Level:    ListLevel,
		Category: CategoryTracks,
		Tracks:   sampleTracks(5),
	}
	if got := v.itemCount(); got != 5 {
		t.Errorf("expected 5, got %d", got)
	}
}

func TestItemCount_TrackLevel(t *testing.T) {
	v := View{
		Level:           TrackLevel,
		DrilldownTracks: sampleTracks(7),
	}
	if got := v.itemCount(); got != 7 {
		t.Errorf("expected 7, got %d", got)
	}
}

func TestEffectiveItems_Artists(t *testing.T) {
	v := View{
		Category: CategoryArtists,
		Artists:  []string{"Alice", "Bob", "Carol"},
	}
	got := v.EffectiveItems()
	if len(got) != 3 {
		t.Fatalf("expected 3, got %d", len(got))
	}
}

func TestEffectiveItems_ArtistsFiltered(t *testing.T) {
	v := View{
		Category:   CategoryArtists,
		Artists:    []string{"Alice", "Bob", "Carol"},
		FilterText: "bob",
	}
	got := v.EffectiveItems()
	if len(got) != 1 || got[0] != "Bob" {
		t.Errorf("expected [Bob], got %v", got)
	}
}

func TestEffectiveItems_Albums(t *testing.T) {
	v := View{
		Category: CategoryAlbums,
		Albums:   []string{"Album1", "Album2"},
	}
	got := v.EffectiveItems()
	if len(got) != 2 {
		t.Fatalf("expected 2, got %d", len(got))
	}
}

func TestEffectiveItems_Tracks_ReturnsNil(t *testing.T) {
	v := View{
		Category: CategoryTracks,
		Tracks:   sampleTracks(5),
	}
	got := v.EffectiveItems()
	if got != nil {
		t.Errorf("expected nil for CategoryTracks, got %v", got)
	}
}

func TestEffectiveTracks_AtTrackLevel(t *testing.T) {
	v := View{
		Level:           TrackLevel,
		DrilldownTracks: sampleTracks(4),
	}
	got := v.EffectiveTracks()
	if len(got) != 4 {
		t.Errorf("expected 4, got %d", len(got))
	}
}

func TestEffectiveTracks_AtListLevel(t *testing.T) {
	v := View{
		Level:    ListLevel,
		Category: CategoryTracks,
		Tracks:   sampleTracks(3),
	}
	got := v.EffectiveTracks()
	if len(got) != 3 {
		t.Errorf("expected 3, got %d", len(got))
	}
}

func TestVisibleWindow_ShiftsStaleLowScrollToCursor(t *testing.T) {
	v := View{
		Level:        ListLevel,
		Category:     CategoryArtists,
		Cursor:       4,
		ScrollOffset: 0,
		Height:       4, // visibleRows = 2
	}
	start, end := v.visibleWindow(5)
	if start != 3 || end != 5 {
		t.Fatalf("expected window [3,5) to keep cursor visible, got [%d,%d)", start, end)
	}
}

func TestVisibleWindow_KeepsCursorVisibleWithHighScroll(t *testing.T) {
	v := View{
		Level:        ListLevel,
		Category:     CategoryArtists,
		Cursor:       1,
		ScrollOffset: 4,
		Height:       4, // visibleRows = 2
	}
	start, end := v.visibleWindow(5)
	if start != 1 || end != 3 {
		t.Fatalf("expected window [1,3) to keep cursor visible, got [%d,%d)", start, end)
	}
}

func TestVisibleWindow_ZeroVisibleRowsReturnsEmptyWindow(t *testing.T) {
	v := View{
		Level:        ListLevel,
		Category:     CategoryArtists,
		Cursor:       2,
		ScrollOffset: 1,
		Height:       2, // visibleRows = 0
	}
	start, end := v.visibleWindow(5)
	if start != 0 || end != 0 {
		t.Fatalf("expected empty window [0,0), got [%d,%d)", start, end)
	}
}
