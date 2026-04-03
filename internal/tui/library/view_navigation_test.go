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

func TestGoToBottomAdjustsScrollOffset(t *testing.T) {
	tracks := make([]musiclib.Track, 20)
	for i := range tracks {
		tracks[i] = musiclib.Track{Artist: "A", Title: strings.Repeat("T", i+1), Hash: strings.Repeat("h", i+1)}
	}
	idx := buildIndex(tracks)
	v := View{Index: idx, Width: 80, Height: 6, Cursor: 0} // visibleRows = 5
	v = v.GoToBottom()
	if v.Cursor != 19 {
		t.Errorf("GoToBottom: expected Cursor=19, got %d", v.Cursor)
	}
	if v.ScrollOffset != 15 {
		t.Errorf("GoToBottom: expected ScrollOffset=15, got %d", v.ScrollOffset)
	}
}

func TestHalfPageDownMovesByHalfVisibleRows(t *testing.T) {
	tracks := make([]musiclib.Track, 20)
	for i := range tracks {
		tracks[i] = musiclib.Track{Artist: "A", Title: strings.Repeat("T", i+1), Hash: strings.Repeat("h", i+1)}
	}
	idx := buildIndex(tracks)
	v := View{Index: idx, Width: 80, Height: 13, Cursor: 0} // visibleRows = 12
	v = v.HalfPageDown()
	if v.Cursor != 6 {
		t.Errorf("HalfPageDown: expected Cursor=6, got %d", v.Cursor)
	}
}

func TestHalfPageUpMovesByHalfVisibleRows(t *testing.T) {
	tracks := make([]musiclib.Track, 20)
	for i := range tracks {
		tracks[i] = musiclib.Track{Artist: "A", Title: strings.Repeat("T", i+1), Hash: strings.Repeat("h", i+1)}
	}
	idx := buildIndex(tracks)
	v := View{Index: idx, Width: 80, Height: 13, Cursor: 12} // visibleRows = 12
	v = v.HalfPageUp()
	if v.Cursor != 6 {
		t.Errorf("HalfPageUp: expected Cursor=6, got %d", v.Cursor)
	}
}

func TestFullPageDownMovesByVisibleRows(t *testing.T) {
	tracks := make([]musiclib.Track, 20)
	for i := range tracks {
		tracks[i] = musiclib.Track{Artist: "A", Title: strings.Repeat("T", i+1), Hash: strings.Repeat("h", i+1)}
	}
	idx := buildIndex(tracks)
	v := View{Index: idx, Width: 80, Height: 13, Cursor: 0} // visibleRows = 12
	v = v.FullPageDown()
	if v.Cursor != 12 {
		t.Errorf("FullPageDown: expected Cursor=12, got %d", v.Cursor)
	}
}

func TestFullPageUpMovesByVisibleRows(t *testing.T) {
	tracks := make([]musiclib.Track, 20)
	for i := range tracks {
		tracks[i] = musiclib.Track{Artist: "A", Title: strings.Repeat("T", i+1), Hash: strings.Repeat("h", i+1)}
	}
	idx := buildIndex(tracks)
	v := View{Index: idx, Width: 80, Height: 13, Cursor: 15} // visibleRows = 12
	v = v.FullPageUp()
	if v.Cursor != 3 {
		t.Errorf("FullPageUp: expected Cursor=3, got %d", v.Cursor)
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
	v := View{Width: 80, Height: 24}
	v2 := v.moveCursor(5)
	if v2.Cursor != 0 {
		t.Errorf("moveCursor empty: expected Cursor=0, got %d", v2.Cursor)
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
	v := View{Height: 4}
	if v.visibleRows() != 3 {
		t.Errorf("visibleRows Height=4: expected 3, got %d", v.visibleRows())
	}
}

// TestVisibleRowsSameWithAndWithoutFilter verifies that visibleRows reserves
// one extra row for the filter bar when a filter is active or has text.
func TestVisibleRowsSameWithAndWithoutFilter(t *testing.T) {
	h := 24
	noFilter := View{Height: h}
	withFilter := View{Height: h, Filter: filter.State{Text: "abc"}}
	withActiveFilter := View{Height: h, Filter: filter.State{Active: true, Text: "abc"}}

	base := noFilter.visibleRows()
	// When filter has text or is active, the filter bar consumes one extra row.
	wantFiltered := base - 1
	if got := withFilter.visibleRows(); got != wantFiltered {
		t.Errorf("visibleRows with committed filter: got %d, want %d (base-1 for filter bar)", got, wantFiltered)
	}
	if got := withActiveFilter.visibleRows(); got != wantFiltered {
		t.Errorf("visibleRows with active filter: got %d, want %d (base-1 for filter bar)", got, wantFiltered)
	}
}
