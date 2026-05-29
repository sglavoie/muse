package genres

import "testing"

func TestGoToBottom_WithScroll(t *testing.T) {
	v := View{
		Level:  GenreLevel,
		Genres: []string{"A", "B", "C", "D", "E"},
		Cursor: 0,
		Height: 3,
	}
	v2 := v.GoToBottom()
	if v2.Cursor != 4 {
		t.Errorf("expected cursor=4, got %d", v2.Cursor)
	}
	if v2.ScrollOffset < 2 {
		t.Errorf("expected scrollOffset >= 2, got %d", v2.ScrollOffset)
	}
}

func TestGoToTop_ResetsScroll(t *testing.T) {
	v := View{
		Level:        GenreLevel,
		Genres:       []string{"A", "B", "C", "D"},
		Cursor:       3,
		ScrollOffset: 2,
	}
	v2 := v.GoToTop()
	if v2.Cursor != 0 || v2.ScrollOffset != 0 {
		t.Errorf("expected cursor=0 scroll=0, got cursor=%d scroll=%d", v2.Cursor, v2.ScrollOffset)
	}
}

func TestHalfPageDown(t *testing.T) {
	v := View{
		Level:  GenreLevel,
		Genres: []string{"A", "B", "C", "D", "E", "F", "G", "H", "I", "J"},
		Cursor: 0,
		Height: 6,
	}
	v2 := v.HalfPageDown()
	// visibleRows = height(6) - reserve(1, strip) = 5; HalfPageDown = 5/2 = 2
	if v2.Cursor != 2 {
		t.Errorf("expected cursor=2, got %d", v2.Cursor)
	}
}

func TestHalfPageMovementMovesOneRowWhenOnlyOneRowVisible(t *testing.T) {
	v := View{
		Level:  GenreLevel,
		Genres: []string{"A", "B", "C"},
		Cursor: 1,
		Height: 2,
	}
	if rows := v.visibleRows(); rows != 1 {
		t.Fatalf("precondition: expected one visible row, got %d", rows)
	}

	if down := v.HalfPageDown(); down.Cursor != 2 {
		t.Errorf("HalfPageDown: expected cursor=2, got %d", down.Cursor)
	}
	if up := v.HalfPageUp(); up.Cursor != 0 {
		t.Errorf("HalfPageUp: expected cursor=0, got %d", up.Cursor)
	}
}

func TestPageMovementDoesNotAdvanceScrollWhenNoRowsVisible(t *testing.T) {
	v := View{
		Level:        GenreLevel,
		Genres:       []string{"A", "B", "C"},
		Cursor:       1,
		ScrollOffset: 0,
		Height:       1, // GenreLevel reserves one row, leaving no content rows.
	}
	if rows := v.visibleRows(); rows != 0 {
		t.Fatalf("precondition: expected no visible rows, got %d", rows)
	}

	v = v.FullPageDown()
	if v.Cursor != 1 {
		t.Errorf("expected Cursor=1, got %d", v.Cursor)
	}
	if v.ScrollOffset > v.Cursor {
		t.Errorf("expected ScrollOffset<=Cursor, got %d > %d", v.ScrollOffset, v.Cursor)
	}
}

func TestPageMovementClampsNegativeScrollOffset(t *testing.T) {
	v := View{
		Level:        GenreLevel,
		Genres:       []string{"A", "B", "C"},
		Cursor:       0,
		ScrollOffset: -5,
		Height:       6,
	}

	v = v.FullPageUp()
	if v.Cursor != 0 {
		t.Errorf("expected Cursor=0 at top, got %d", v.Cursor)
	}
	if v.ScrollOffset != 0 {
		t.Errorf("expected ScrollOffset clamped to 0, got %d", v.ScrollOffset)
	}
}

func TestPageMovementClampsHugeStaleCursorWithoutOverflow(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	v := View{
		Level:  GenreLevel,
		Genres: []string{"A", "B", "C"},
		Cursor: maxInt,
		Height: 20,
	}

	v = v.FullPageDown()
	if v.Cursor != 2 {
		t.Errorf("expected Cursor clamped to last row, got %d", v.Cursor)
	}
	if v.ScrollOffset > v.Cursor {
		t.Errorf("expected ScrollOffset<=Cursor, got %d > %d", v.ScrollOffset, v.Cursor)
	}
}

// TestGoToBottomClampsScrollOffsetWhenListShrank guards a regression where
// GoToBottom kept a stale ScrollOffset above the new last index after a
// filter/delete shrank the list, leaving the cursor rendering above the
// visible window.
func TestGoToBottomClampsScrollOffsetWhenListShrank(t *testing.T) {
	v := View{
		Level:        GenreLevel,
		Genres:       []string{"Rock", "Jazz", "Folk"},
		Cursor:       0,
		ScrollOffset: 80,
		Height:       20,
	}
	v = v.GoToBottom()
	if v.Cursor != 2 {
		t.Errorf("expected Cursor=2, got %d", v.Cursor)
	}
	if v.ScrollOffset != 0 {
		t.Errorf("expected ScrollOffset=0 when list fits on screen, got %d", v.ScrollOffset)
	}
}

func TestGoToBottomAnchorsScrollToCursorWhenNoRowsVisible(t *testing.T) {
	v := View{
		Level:  GenreLevel,
		Genres: []string{"Rock", "Jazz", "Folk"},
		Height: 1,
	}
	if rows := v.visibleRows(); rows != 0 {
		t.Fatalf("precondition: expected no visible rows, got %d", rows)
	}

	v = v.GoToBottom()
	if v.Cursor != 2 {
		t.Errorf("expected Cursor=2, got %d", v.Cursor)
	}
	if v.ScrollOffset != v.Cursor {
		t.Errorf("expected ScrollOffset anchored to cursor when no rows fit, got scroll=%d cursor=%d", v.ScrollOffset, v.Cursor)
	}
}
