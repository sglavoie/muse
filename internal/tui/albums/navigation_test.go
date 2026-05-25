package albums

import "testing"

func TestCursorUpDown(t *testing.T) {
	v := View{
		Level:  AlbumLevel,
		Albums: []string{"A", "B", "C"},
		Width:  80,
		Height: 20,
	}

	v = v.CursorDown()
	if v.Cursor != 1 {
		t.Errorf("expected cursor=1, got %d", v.Cursor)
	}
	v = v.CursorDown()
	if v.Cursor != 2 {
		t.Errorf("expected cursor=2, got %d", v.Cursor)
	}
	v = v.CursorDown()
	if v.Cursor != 2 {
		t.Errorf("expected cursor=2 (clamped), got %d", v.Cursor)
	}
	v = v.CursorUp()
	if v.Cursor != 1 {
		t.Errorf("expected cursor=1, got %d", v.Cursor)
	}
}

func TestGoToTopBottom(t *testing.T) {
	v := View{
		Level:  AlbumLevel,
		Albums: []string{"A", "B", "C", "D", "E"},
		Width:  80,
		Height: 20,
	}

	v = v.GoToBottom()
	if v.Cursor != 4 {
		t.Errorf("expected cursor=4, got %d", v.Cursor)
	}
	v = v.GoToTop()
	if v.Cursor != 0 {
		t.Errorf("expected cursor=0, got %d", v.Cursor)
	}
}

func TestGoToBottomResetsStateWhenFilteredListIsEmpty(t *testing.T) {
	v := View{
		Level:        AlbumLevel,
		Albums:       []string{"A", "B", "C"},
		Cursor:       2,
		ScrollOffset: 1,
		Width:        80,
		Height:       20,
		FilterText:   "zzz",
	}

	v = v.GoToBottom()
	if v.Cursor != 0 || v.ScrollOffset != 0 {
		t.Errorf("expected cursor=0 and scroll=0 for empty filtered list, got cursor=%d scroll=%d", v.Cursor, v.ScrollOffset)
	}
}

func TestGoToBottomClampsScrollOffsetWhenListShrank(t *testing.T) {
	v := View{
		Level:        AlbumLevel,
		Albums:       []string{"A", "B", "C"},
		Cursor:       0,
		ScrollOffset: 80,
		Width:        80,
		Height:       20,
	}

	v = v.GoToBottom()
	if v.Cursor != 2 {
		t.Errorf("expected cursor=2, got %d", v.Cursor)
	}
	if v.ScrollOffset > v.Cursor {
		t.Errorf("expected scroll<=cursor (%d), got scroll=%d", v.Cursor, v.ScrollOffset)
	}
	if v.ScrollOffset < 0 {
		t.Errorf("expected non-negative scroll, got %d", v.ScrollOffset)
	}
}

func TestCursorMovementResetsStateWhenFilteredListIsEmpty(t *testing.T) {
	base := View{
		Level:        AlbumLevel,
		Albums:       []string{"A", "B", "C"},
		Cursor:       2,
		ScrollOffset: 1,
		Width:        80,
		Height:       20,
		FilterText:   "zzz",
	}
	tests := []struct {
		name string
		move func(View) View
	}{
		{name: "CursorUp", move: View.CursorUp},
		{name: "CursorDown", move: View.CursorDown},
		{name: "HalfPageUp", move: View.HalfPageUp},
		{name: "HalfPageDown", move: View.HalfPageDown},
		{name: "FullPageUp", move: View.FullPageUp},
		{name: "FullPageDown", move: View.FullPageDown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := tt.move(base)
			if v.Cursor != 0 || v.ScrollOffset != 0 {
				t.Errorf("expected cursor=0 and scroll=0 for empty filtered list, got cursor=%d scroll=%d", v.Cursor, v.ScrollOffset)
			}
		})
	}
}

func TestCursorMovementWithNoVisibleRowsDoesNotScrollPastCursor(t *testing.T) {
	base := View{
		Level:  AlbumLevel,
		Albums: []string{"A", "B", "C"},
		Width:  80,
		Height: 1,
	}
	tests := []struct {
		name string
		move func(View) View
	}{
		{name: "CursorDown", move: View.CursorDown},
		{name: "HalfPageDown", move: View.HalfPageDown},
		{name: "FullPageDown", move: View.FullPageDown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := tt.move(base)
			if v.ScrollOffset > v.Cursor {
				t.Fatalf("scroll offset advanced past cursor: cursor=%d scroll=%d", v.Cursor, v.ScrollOffset)
			}
			if v.ScrollOffset < 0 {
				t.Fatalf("scroll offset must not be negative, got %d", v.ScrollOffset)
			}
		})
	}
}
