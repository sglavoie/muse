package artists

import "testing"

func TestGoToTopResetsCursorAndScroll(t *testing.T) {
	v := View{
		Level:        ArtistLevel,
		Cursor:       5,
		ScrollOffset: 3,
		Artists:      make([]string, 10),
	}

	v = v.GoToTop()
	if v.Cursor != 0 {
		t.Errorf("GoToTop: expected Cursor=0, got %d", v.Cursor)
	}
	if v.ScrollOffset != 0 {
		t.Errorf("GoToTop: expected ScrollOffset=0, got %d", v.ScrollOffset)
	}
}

func TestGoToBottomMovesCursorToEnd(t *testing.T) {
	v := View{
		Level:   ArtistLevel,
		Cursor:  0,
		Artists: make([]string, 10),
	}

	v = v.GoToBottom()
	if v.Cursor != 9 {
		t.Errorf("GoToBottom: expected Cursor=9, got %d", v.Cursor)
	}
}

func TestGoToBottomEmptyList(t *testing.T) {
	v := View{Level: ArtistLevel, Cursor: 0}
	v = v.GoToBottom()
	if v.Cursor != 0 {
		t.Errorf("GoToBottom empty: expected Cursor=0, got %d", v.Cursor)
	}
}

func TestGoToBottomClampsScrollOffsetWhenListShrank(t *testing.T) {
	v := View{
		Level:        ArtistLevel,
		Artists:      []string{"A", "B", "C"},
		Cursor:       0,
		ScrollOffset: 80,
		Height:       20,
	}

	v = v.GoToBottom()
	if v.Cursor != 2 {
		t.Errorf("expected Cursor=2, got %d", v.Cursor)
	}
	if v.ScrollOffset > v.Cursor {
		t.Errorf("expected ScrollOffset<=Cursor (%d), got ScrollOffset=%d", v.Cursor, v.ScrollOffset)
	}
	if v.ScrollOffset < 0 {
		t.Errorf("ScrollOffset must not be negative, got %d", v.ScrollOffset)
	}
}

func TestHalfPageDownMovesByHalfVisibleRows(t *testing.T) {
	v := View{
		Level:   ArtistLevel,
		Cursor:  0,
		Height:  13,
		Artists: make([]string, 20),
	}

	v = v.HalfPageDown()
	if v.Cursor != 6 {
		t.Errorf("HalfPageDown: expected Cursor=6, got %d", v.Cursor)
	}
}

func TestFullPageDownMovesByVisibleRows(t *testing.T) {
	v := View{
		Level:   ArtistLevel,
		Cursor:  0,
		Height:  13,
		Artists: make([]string, 20),
	}

	v = v.FullPageDown()
	if v.Cursor != 12 {
		t.Errorf("FullPageDown: expected Cursor=12, got %d", v.Cursor)
	}
}

func TestPageMovementDoesNotAdvanceScrollWhenNoRowsVisible(t *testing.T) {
	v := View{
		Level:        AlbumLevel,
		Cursor:       1,
		ScrollOffset: 0,
		Height:       1,
		Albums:       []string{"Alpha", "Beta", "Gamma"},
	}
	if rows := v.visibleRows(); rows != 0 {
		t.Fatalf("precondition: expected no visible rows, got %d", rows)
	}

	v = v.FullPageDown()
	if v.Cursor != 1 {
		t.Errorf("FullPageDown: expected Cursor=1, got %d", v.Cursor)
	}
	if v.ScrollOffset > v.Cursor {
		t.Errorf("FullPageDown: expected ScrollOffset<=Cursor, got %d > %d", v.ScrollOffset, v.Cursor)
	}
}
