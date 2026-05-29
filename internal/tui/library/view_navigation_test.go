package library

import (
	"strings"
	"testing"

	musiclib "github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/filter"
)

// TestCursorDownAdvances verifies that CursorDown (j / arrow-down) increments
// the cursor index by one.
func TestCursorDownAdvances(t *testing.T) {
	idx := buildIndex(sampleTracks)
	v := View{Index: idx, Width: 80, Height: 0, Cursor: 0}
	v = v.CursorDown()
	if v.Cursor != 1 {
		t.Errorf("CursorDown: expected cursor 1, got %d", v.Cursor)
	}
}

// TestCursorUpAdvances verifies that CursorUp (k / arrow-up) decrements the
// cursor index by one.
func TestCursorUpAdvances(t *testing.T) {
	idx := buildIndex(sampleTracks)
	v := View{Index: idx, Width: 80, Height: 0, Cursor: 2}
	v = v.CursorUp()
	if v.Cursor != 1 {
		t.Errorf("CursorUp: expected cursor 1, got %d", v.Cursor)
	}
}

// TestCursorClampAtBottom verifies that CursorDown stops at the last track
// without wrapping.
func TestCursorClampAtBottom(t *testing.T) {
	idx := buildIndex(sampleTracks)
	last := len(sampleTracks) - 1
	v := View{Index: idx, Width: 80, Height: 0, Cursor: last}
	v = v.CursorDown()
	if v.Cursor != last {
		t.Errorf("CursorDown at last: expected cursor %d (clamped), got %d", last, v.Cursor)
	}
}

func TestCursorDownClampsStaleScrollOffsetWhenListShrank(t *testing.T) {
	idx := buildIndex(sampleTracks)
	v := View{Index: idx, Width: 80, Height: 24, Cursor: 0, ScrollOffset: 80}

	v = v.CursorDown()

	if v.Cursor != 1 {
		t.Errorf("CursorDown: expected Cursor=1, got %d", v.Cursor)
	}
	if v.ScrollOffset != v.Cursor {
		t.Errorf("CursorDown: expected stale ScrollOffset to clamp to cursor %d, got %d", v.Cursor, v.ScrollOffset)
	}
}

func TestCursorMovementClampsNegativeStaleState(t *testing.T) {
	idx := buildIndex(sampleTracks)
	v := View{Index: idx, Width: 80, Height: 24, Cursor: 1, ScrollOffset: -4}

	v = v.CursorUp()

	if v.Cursor != 0 {
		t.Errorf("CursorUp: expected Cursor=0 after moving from row 1, got %d", v.Cursor)
	}
	if v.ScrollOffset != 0 {
		t.Errorf("CursorUp: expected negative ScrollOffset to clamp to 0, got %d", v.ScrollOffset)
	}
}

func TestCursorDownFromNegativeCursorStartsAtFirstRow(t *testing.T) {
	idx := buildIndex(sampleTracks)
	v := View{Index: idx, Width: 80, Height: 24, Cursor: -5, ScrollOffset: -2}

	v = v.CursorDown()

	if v.Cursor != 0 {
		t.Errorf("CursorDown: expected negative Cursor to clamp to first row, got %d", v.Cursor)
	}
	if v.ScrollOffset != 0 {
		t.Errorf("CursorDown: expected negative ScrollOffset to clamp to 0, got %d", v.ScrollOffset)
	}
}

func TestCursorDownWithNoVisibleRowsKeepsCursorVisibleForFutureResize(t *testing.T) {
	idx := buildIndex(sampleTracks)
	v := View{Index: idx, Width: 80, Height: 1, Cursor: 0, ScrollOffset: 0}

	v = v.CursorDown()

	if v.Cursor != 1 {
		t.Errorf("CursorDown: expected Cursor=1, got %d", v.Cursor)
	}
	if v.ScrollOffset != 1 {
		t.Errorf("CursorDown: expected ScrollOffset=1 when no rows fit, got %d", v.ScrollOffset)
	}
}

// TestCursorClampAtTop verifies that CursorUp stops at the first track without
// wrapping.
func TestCursorClampAtTop(t *testing.T) {
	idx := buildIndex(sampleTracks)
	v := View{Index: idx, Width: 80, Height: 0, Cursor: 0}
	v = v.CursorUp()
	if v.Cursor != 0 {
		t.Errorf("CursorUp at first: expected cursor 0 (clamped), got %d", v.Cursor)
	}
}

// TestCursorStoredOnModel verifies that the cursor index is retained on the
// View value after multiple moves.
func TestCursorStoredOnModel(t *testing.T) {
	idx := buildIndex(sampleTracks)
	v := View{Index: idx, Width: 80, Height: 0, Cursor: 0}
	v = v.CursorDown()
	v = v.CursorDown()
	if v.Cursor != 2 {
		t.Errorf("cursor after two CursorDown calls: expected 2, got %d", v.Cursor)
	}
}

func TestGoToTopResetsCursorAndScroll(t *testing.T) {
	idx := buildIndex(sampleTracks)
	v := View{Index: idx, Width: 80, Height: 24, Cursor: 2, ScrollOffset: 1}
	v = v.GoToTop()
	if v.Cursor != 0 {
		t.Errorf("GoToTop: expected Cursor=0, got %d", v.Cursor)
	}
	if v.ScrollOffset != 0 {
		t.Errorf("GoToTop: expected ScrollOffset=0, got %d", v.ScrollOffset)
	}
}

func TestGoToBottomMovesCursorToLast(t *testing.T) {
	idx := buildIndex(sampleTracks)
	v := View{Index: idx, Width: 80, Height: 24, Cursor: 0}
	v = v.GoToBottom()
	want := len(sampleTracks) - 1
	if v.Cursor != want {
		t.Errorf("GoToBottom: expected Cursor=%d, got %d", want, v.Cursor)
	}
}

func TestGoToBottomEmptyLibrary(t *testing.T) {
	v := View{Width: 80, Height: 24, Cursor: 0}
	v = v.GoToBottom()
	if v.Cursor != 0 {
		t.Errorf("GoToBottom empty: expected Cursor=0, got %d", v.Cursor)
	}
}

func TestCursorMovementsResetStaleStateWhenLibraryEmpty(t *testing.T) {
	base := View{Width: 80, Height: 24, Cursor: 7, ScrollOffset: 5}
	tests := []struct {
		name string
		move func(View) View
	}{
		{"CursorUp", View.CursorUp},
		{"CursorDown", View.CursorDown},
		{"HalfPageUp", View.HalfPageUp},
		{"HalfPageDown", View.HalfPageDown},
		{"FullPageUp", View.FullPageUp},
		{"FullPageDown", View.FullPageDown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := tt.move(base)
			if v.Cursor != 0 || v.ScrollOffset != 0 {
				t.Errorf("expected cursor=0 and scroll=0 for empty library, got cursor=%d scroll=%d", v.Cursor, v.ScrollOffset)
			}
		})
	}
}

func TestGoToBottomAdjustsScrollOffset(t *testing.T) {
	tracks := make([]musiclib.Track, 20)
	for i := range tracks {
		tracks[i] = musiclib.Track{Artist: "A", Title: strings.Repeat("T", i+1), Hash: strings.Repeat("h", i+1)}
	}
	idx := buildIndex(tracks)
	v := View{Index: idx, Width: 80, Height: 6, Cursor: 0} // visibleRows = 4
	v = v.GoToBottom()
	if v.Cursor != 19 {
		t.Errorf("GoToBottom: expected Cursor=19, got %d", v.Cursor)
	}
	if v.ScrollOffset != 16 {
		t.Errorf("GoToBottom: expected ScrollOffset=16, got %d", v.ScrollOffset)
	}
}

// TestGoToBottomClampsScrollOffsetWhenListShrank guards a regression where
// GoToBottom failed to bring ScrollOffset back down after a filter/delete
// reduced the effective track count: the saved ScrollOffset stayed above
// the new last index, leaving the cursor rendering above the visible window.
func TestGoToBottomClampsScrollOffsetWhenListShrank(t *testing.T) {
	tracks := make([]musiclib.Track, 3)
	for i := range tracks {
		tracks[i] = musiclib.Track{Artist: "A", Title: strings.Repeat("T", i+1), Hash: strings.Repeat("h", i+1)}
	}
	idx := buildIndex(tracks)
	// Simulate a state where the user had scrolled far down inside a larger
	// list (ScrollOffset=80) before a filter shrank the list to 3 tracks.
	v := View{Index: idx, Width: 80, Height: 20, Cursor: 0, ScrollOffset: 80}
	v = v.GoToBottom()
	if v.Cursor != 2 {
		t.Errorf("GoToBottom: expected Cursor=2, got %d", v.Cursor)
	}
	if v.ScrollOffset > v.Cursor {
		t.Errorf("GoToBottom: expected ScrollOffset<=Cursor (%d), got ScrollOffset=%d", v.Cursor, v.ScrollOffset)
	}
	if v.ScrollOffset < 0 {
		t.Errorf("GoToBottom: ScrollOffset must not be negative, got %d", v.ScrollOffset)
	}
}

func TestHalfPageDownMovesByHalfVisibleRows(t *testing.T) {
	tracks := make([]musiclib.Track, 20)
	for i := range tracks {
		tracks[i] = musiclib.Track{Artist: "A", Title: strings.Repeat("T", i+1), Hash: strings.Repeat("h", i+1)}
	}
	idx := buildIndex(tracks)
	v := View{Index: idx, Width: 80, Height: 13, Cursor: 0} // visibleRows = 11
	v = v.HalfPageDown()
	if v.Cursor != 5 {
		t.Errorf("HalfPageDown: expected Cursor=5, got %d", v.Cursor)
	}
}

func TestHalfPageDownMovesAtLeastOneVisibleRow(t *testing.T) {
	idx := buildIndex(sampleTracks)
	v := View{Index: idx, Width: 80, Height: 3, Cursor: 0} // visibleRows = 1
	v = v.HalfPageDown()
	if v.Cursor != 1 {
		t.Errorf("HalfPageDown with one visible row: expected Cursor=1, got %d", v.Cursor)
	}
}

func TestHalfPageUpMovesByHalfVisibleRows(t *testing.T) {
	tracks := make([]musiclib.Track, 20)
	for i := range tracks {
		tracks[i] = musiclib.Track{Artist: "A", Title: strings.Repeat("T", i+1), Hash: strings.Repeat("h", i+1)}
	}
	idx := buildIndex(tracks)
	v := View{Index: idx, Width: 80, Height: 13, Cursor: 12} // visibleRows = 11
	v = v.HalfPageUp()
	if v.Cursor != 7 {
		t.Errorf("HalfPageUp: expected Cursor=7, got %d", v.Cursor)
	}
}

func TestHalfPageUpMovesAtLeastOneVisibleRow(t *testing.T) {
	idx := buildIndex(sampleTracks)
	v := View{Index: idx, Width: 80, Height: 3, Cursor: 1} // visibleRows = 1
	v = v.HalfPageUp()
	if v.Cursor != 0 {
		t.Errorf("HalfPageUp with one visible row: expected Cursor=0, got %d", v.Cursor)
	}
}

func TestFullPageDownMovesByVisibleRows(t *testing.T) {
	tracks := make([]musiclib.Track, 20)
	for i := range tracks {
		tracks[i] = musiclib.Track{Artist: "A", Title: strings.Repeat("T", i+1), Hash: strings.Repeat("h", i+1)}
	}
	idx := buildIndex(tracks)
	v := View{Index: idx, Width: 80, Height: 13, Cursor: 0} // visibleRows = 11
	v = v.FullPageDown()
	if v.Cursor != 11 {
		t.Errorf("FullPageDown: expected Cursor=11, got %d", v.Cursor)
	}
}

func TestFullPageUpMovesByVisibleRows(t *testing.T) {
	tracks := make([]musiclib.Track, 20)
	for i := range tracks {
		tracks[i] = musiclib.Track{Artist: "A", Title: strings.Repeat("T", i+1), Hash: strings.Repeat("h", i+1)}
	}
	idx := buildIndex(tracks)
	v := View{Index: idx, Width: 80, Height: 13, Cursor: 15} // visibleRows = 11
	v = v.FullPageUp()
	if v.Cursor != 4 {
		t.Errorf("FullPageUp: expected Cursor=4, got %d", v.Cursor)
	}
}

func TestPageMovementsClampsToEnd(t *testing.T) {
	idx := buildIndex(sampleTracks) // 3 tracks
	v := View{Index: idx, Width: 80, Height: 13, Cursor: 2}
	v = v.FullPageDown()
	if v.Cursor != 2 {
		t.Errorf("FullPageDown clamp: expected Cursor=2, got %d", v.Cursor)
	}
}

func TestPageMovementsClampsToStart(t *testing.T) {
	idx := buildIndex(sampleTracks)
	v := View{Index: idx, Width: 80, Height: 13, Cursor: 1}
	v = v.FullPageUp()
	if v.Cursor != 0 {
		t.Errorf("FullPageUp clamp: expected Cursor=0, got %d", v.Cursor)
	}
}

func TestMoveCursorEmptyIsNoOp(t *testing.T) {
	v := View{Width: 80, Height: 24, Cursor: 4, ScrollOffset: 2}
	v2 := v.moveCursor(5)
	if v2.Cursor != 0 || v2.ScrollOffset != 0 {
		t.Errorf("moveCursor empty: expected cursor=0 scroll=0, got cursor=%d scroll=%d", v2.Cursor, v2.ScrollOffset)
	}
}

// TestVisibleRowsZeroHeight verifies the default fallback height.
func TestVisibleRowsZeroHeight(t *testing.T) {
	v := View{Height: 0}
	if v.visibleRows() != 50 {
		t.Errorf("visibleRows Height=0: expected 50, got %d", v.visibleRows())
	}
}

func TestVisibleRowsSmallHeight(t *testing.T) {
	v := View{Width: 80, Height: 4}
	if v.visibleRows() != 2 {
		t.Errorf("visibleRows Height=4: expected 2, got %d", v.visibleRows())
	}
}

// TestVisibleRowsSameWithAndWithoutFilter verifies that visibleRows reserves
// the state strip and column header. Filter state is rendered inside the
// strip, so it does not consume another row in columnar mode.
func TestVisibleRowsSameWithAndWithoutFilter(t *testing.T) {
	h := 24
	noFilter := View{Width: 80, Height: h}
	withFilter := View{Width: 80, Height: h, Filter: filter.State{Text: "abc"}}
	withActiveFilter := View{Width: 80, Height: h, Filter: filter.State{Active: true, Text: "abc"}}

	base := noFilter.visibleRows()
	if got := withFilter.visibleRows(); got != base {
		t.Errorf("visibleRows with committed filter: got %d, want %d", got, base)
	}
	if got := withActiveFilter.visibleRows(); got != base {
		t.Errorf("visibleRows with active filter: got %d, want %d", got, base)
	}
}

func TestVisibleRowsZeroWidthAccountsForFallbackFilterLine(t *testing.T) {
	noFilter := View{Width: 0, Height: 4}
	withFilter := View{Width: 0, Height: 4, Filter: filter.State{Text: "abc"}}

	if got := noFilter.visibleRows(); got != 4 {
		t.Errorf("zero-width visibleRows without filter: got %d, want 4", got)
	}
	if got := withFilter.visibleRows(); got != 3 {
		t.Errorf("zero-width visibleRows with filter: got %d, want 3", got)
	}
}
