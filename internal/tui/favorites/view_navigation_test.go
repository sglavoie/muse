package favorites

import "testing"

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

func TestHalfPageDown(t *testing.T) {
	v := View{
		Level:    ListLevel,
		Category: CategoryArtists,
		Artists:  []string{"A", "B", "C", "D", "E", "F", "G", "H", "I", "J"},
		Cursor:   0,
		Height:   6,
	}
	v2 := v.HalfPageDown()
	// visibleRows = 6-2 = 4, half = 2
	if v2.Cursor != 2 {
		t.Errorf("expected cursor=2 (half of 4), got %d", v2.Cursor)
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
	// visibleRows = 6-2 = 4, cursor = 9-4 = 5
	if v2.Cursor != 5 {
		t.Errorf("expected cursor=5 (9-4), got %d", v2.Cursor)
	}
}

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
