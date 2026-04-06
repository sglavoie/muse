package favorites

import (
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/library"
)

// --- helpers ---

func sampleTracks(n int) []library.Track {
	out := make([]library.Track, n)
	for i := range out {
		out[i] = library.Track{
			Hash:   "h" + string(rune('a'+i)),
			Title:  "t" + string(rune('0'+i)),
			Artist: "a" + string(rune('0'+i)),
			Album:  "al" + string(rune('0'+i)),
		}
	}
	return out
}

// --- Level transitions ---

func TestOpenList_TransitionsToCategoryLevel(t *testing.T) {
	v := View{
		Level:  CategoryLevel,
		Cursor: 1, // Albums
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

func TestBack_RoundTrip(t *testing.T) {
	v := View{
		Level:  CategoryLevel,
		Cursor: 2, // Tracks
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

// --- itemCount ---

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

// --- Cursor movement ---

func TestCursorUp_AtTop(t *testing.T) {
	v := View{Level: CategoryLevel, Cursor: 0}
	v2 := v.CursorUp()
	if v2.Cursor != 0 {
		t.Errorf("expected 0, got %d", v2.Cursor)
	}
}

func TestCursorUp_Normal(t *testing.T) {
	v := View{Level: CategoryLevel, Cursor: 2}
	v2 := v.CursorUp()
	if v2.Cursor != 1 {
		t.Errorf("expected 1, got %d", v2.Cursor)
	}
}

func TestCursorDown_AtBottom(t *testing.T) {
	v := View{Level: CategoryLevel, Cursor: 2, Height: 20}
	v2 := v.CursorDown()
	if v2.Cursor != 2 {
		t.Errorf("expected 2, got %d", v2.Cursor)
	}
}

func TestCursorDown_Normal(t *testing.T) {
	v := View{Level: CategoryLevel, Cursor: 0, Height: 20}
	v2 := v.CursorDown()
	if v2.Cursor != 1 {
		t.Errorf("expected 1, got %d", v2.Cursor)
	}
}

func TestGoToTop(t *testing.T) {
	v := View{
		Level:        ListLevel,
		Category:     CategoryArtists,
		Artists:      []string{"A", "B", "C", "D"},
		Cursor:       3,
		ScrollOffset: 2,
	}
	v2 := v.GoToTop()
	if v2.Cursor != 0 || v2.ScrollOffset != 0 {
		t.Errorf("expected cursor=0 scroll=0, got cursor=%d scroll=%d", v2.Cursor, v2.ScrollOffset)
	}
}

func TestGoToBottom(t *testing.T) {
	v := View{
		Level:    ListLevel,
		Category: CategoryArtists,
		Artists:  []string{"A", "B", "C", "D"},
		Cursor:   0,
		Height:   3,
	}
	v2 := v.GoToBottom()
	if v2.Cursor != 3 {
		t.Errorf("expected cursor=3, got %d", v2.Cursor)
	}
}

func TestMoveCursor_EmptyList(t *testing.T) {
	v := View{
		Level:    ListLevel,
		Category: CategoryArtists,
		Artists:  nil,
		Height:   20,
	}
	v2 := v.moveCursor(5)
	if v2.Cursor != 0 {
		t.Errorf("expected cursor=0 for empty list, got %d", v2.Cursor)
	}
}

func TestMoveCursor_ClampToEnd(t *testing.T) {
	v := View{
		Level:    ListLevel,
		Category: CategoryArtists,
		Artists:  []string{"A", "B", "C"},
		Cursor:   1,
		Height:   20,
	}
	v2 := v.moveCursor(100)
	if v2.Cursor != 2 {
		t.Errorf("expected cursor=2 (clamped), got %d", v2.Cursor)
	}
}

func TestMoveCursor_ClampToStart(t *testing.T) {
	v := View{
		Level:    ListLevel,
		Category: CategoryArtists,
		Artists:  []string{"A", "B", "C"},
		Cursor:   1,
		Height:   20,
	}
	v2 := v.moveCursor(-100)
	if v2.Cursor != 0 {
		t.Errorf("expected cursor=0 (clamped), got %d", v2.Cursor)
	}
}

// --- EffectiveItems ---

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

// --- EffectiveTracks ---

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

// --- visibleRows ---

func TestVisibleRows_CategoryLevel(t *testing.T) {
	v := View{Level: CategoryLevel, Height: 20}
	got := v.visibleRows()
	if got != 20 {
		t.Errorf("expected 20, got %d", got)
	}
}

func TestVisibleRows_ListLevel_ArtistsNoFilter(t *testing.T) {
	// ListLevel with non-track category: no column header reserve
	v := View{Level: ListLevel, Category: CategoryArtists, Height: 20}
	// reserve = 0 (no column header for artist list, no filter)
	if got := v.visibleRows(); got != 20 {
		t.Errorf("expected 20, got %d", got)
	}
}

func TestVisibleRows_ListLevel_TracksCategory(t *testing.T) {
	v := View{Level: ListLevel, Category: CategoryTracks, Height: 20}
	// reserve = 1 (column header for tracks)
	if got := v.visibleRows(); got != 19 {
		t.Errorf("expected 19, got %d", got)
	}
}

func TestVisibleRows_TrackLevel(t *testing.T) {
	v := View{Level: TrackLevel, Height: 20}
	// reserve = 3 (heading + separator + column header)
	if got := v.visibleRows(); got != 17 {
		t.Errorf("expected 17, got %d", got)
	}
}

func TestVisibleRows_WithSortIndicator(t *testing.T) {
	v := View{Level: CategoryLevel, Height: 20, SortIndicator: "Sort: Name ▲"}
	if got := v.visibleRows(); got != 19 {
		t.Errorf("expected 19, got %d", got)
	}
}

func TestVisibleRows_ZeroHeight(t *testing.T) {
	v := View{Level: CategoryLevel, Height: 0}
	got := v.visibleRows()
	if got != 50 {
		t.Errorf("expected default 50, got %d", got)
	}
}

// --- ComputeTrack columns ---

func TestComputeTrack_NarrowWidth(t *testing.T) {
	cols := ComputeTrack(10)
	if cols.Title != 1 {
		t.Errorf("expected Title=1 for narrow width, got %d", cols.Title)
	}
	if cols.Artist != 0 {
		t.Errorf("expected Artist=0 for narrow width, got %d", cols.Artist)
	}
}

func TestComputeTrack_NormalWidth(t *testing.T) {
	cols := ComputeTrack(120)
	if cols.Title <= 0 || cols.Artist <= 0 || cols.Album <= 0 {
		t.Errorf("expected positive flex columns, got title=%d artist=%d album=%d",
			cols.Title, cols.Artist, cols.Album)
	}
	total := cols.Marker + cols.Position + cols.Title + cols.Artist + cols.Album +
		cols.Duration + cols.PlayCount + trackSepCount
	if total != 120 {
		t.Errorf("columns should sum to total width 120, got %d", total)
	}
}

// --- HalfPage / FullPage ---

func TestHalfPageDown(t *testing.T) {
	v := View{
		Level:    ListLevel,
		Category: CategoryArtists,
		Artists:  []string{"A", "B", "C", "D", "E", "F", "G", "H", "I", "J"},
		Cursor:   0,
		Height:   6,
	}
	v2 := v.HalfPageDown()
	if v2.Cursor != 3 {
		t.Errorf("expected cursor=3 (half of 6), got %d", v2.Cursor)
	}
}

func TestFullPageUp(t *testing.T) {
	v := View{
		Level:        ListLevel,
		Category:     CategoryArtists,
		Artists:      []string{"A", "B", "C", "D", "E", "F", "G", "H", "I", "J"},
		Cursor:       9,
		ScrollOffset: 4,
		Height:       6,
	}
	v2 := v.FullPageUp()
	if v2.Cursor != 3 {
		t.Errorf("expected cursor=3 (9-6), got %d", v2.Cursor)
	}
}

// --- CursorDown scroll adjustment ---

func TestCursorDown_AdjustsScroll(t *testing.T) {
	v := View{
		Level:        ListLevel,
		Category:     CategoryArtists,
		Artists:      []string{"A", "B", "C", "D"},
		Cursor:       1,
		ScrollOffset: 0,
		Height:       2,
	}
	v2 := v.CursorDown()
	if v2.Cursor != 2 {
		t.Errorf("expected cursor=2, got %d", v2.Cursor)
	}
	if v2.ScrollOffset < 1 {
		t.Errorf("expected scrollOffset >= 1, got %d", v2.ScrollOffset)
	}
}

// --- DrilldownName heading ---

// TestRenderTracks_UsesDrilldownNameForHeading verifies that the TrackLevel
// heading uses DrilldownName rather than reconstructing it from the cursor
// position, which would be incorrect when the drill-down was initiated from
// a filtered list.
func TestRenderTracks_UsesDrilldownNameForHeading(t *testing.T) {
	v := View{
		Level:           TrackLevel,
		Category:        CategoryArtists,
		Width:           80,
		Height:          20,
		DrilldownTracks: sampleTracks(3),
		DrilldownName:   "Correct Artist",
		Artists:         []string{"Alice", "Bob", "Carol", "Correct Artist"},
		ListCursor:      0, // points to "Alice" in the unfiltered list
	}
	out := v.Render()
	if !strings.Contains(out, "Correct Artist") {
		t.Errorf("expected heading to contain DrilldownName 'Correct Artist', got:\n%s", out)
	}
}

// TestRenderTracks_EmptyDrilldownNameFallback verifies that when DrilldownName
// is empty the heading renders without panicking.
func TestRenderTracks_EmptyDrilldownNameFallback(t *testing.T) {
	v := View{
		Level:           TrackLevel,
		Category:        CategoryArtists,
		Width:           80,
		Height:          20,
		DrilldownTracks: sampleTracks(2),
	}
	// Should not panic and should produce output.
	out := v.Render()
	if out == "" {
		t.Error("expected non-empty render output")
	}
}

// --- CursorUp scroll adjustment ---

func TestCursorUp_AdjustsScroll(t *testing.T) {
	v := View{
		Level:        ListLevel,
		Category:     CategoryArtists,
		Artists:      []string{"A", "B", "C", "D"},
		Cursor:       2,
		ScrollOffset: 2,
		Height:       2,
	}
	v2 := v.CursorUp()
	if v2.Cursor != 1 {
		t.Errorf("expected cursor=1, got %d", v2.Cursor)
	}
	if v2.ScrollOffset != 1 {
		t.Errorf("expected scrollOffset=1, got %d", v2.ScrollOffset)
	}
}
