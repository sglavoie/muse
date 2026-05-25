package genres

import "testing"

func TestCursorDown_AtBottom(t *testing.T) {
	v := View{
		Level:  GenreLevel,
		Genres: []string{"A", "B", "C"},
		Cursor: 2,
		Height: 20,
	}
	v2 := v.CursorDown()
	if v2.Cursor != 2 {
		t.Errorf("expected cursor=2 at bottom, got %d", v2.Cursor)
	}
}

func TestCursorUp_AtTop(t *testing.T) {
	v := View{
		Level:  GenreLevel,
		Genres: []string{"A", "B"},
		Cursor: 0,
	}
	v2 := v.CursorUp()
	if v2.Cursor != 0 {
		t.Errorf("expected cursor=0 at top, got %d", v2.Cursor)
	}
}

func TestCursorUp_EmptyGenresResetsStaleState(t *testing.T) {
	v := View{
		Level:        GenreLevel,
		Genres:       nil,
		Cursor:       3,
		ScrollOffset: 2,
	}
	v2 := v.CursorUp()
	if v2.Cursor != 0 || v2.ScrollOffset != 0 {
		t.Errorf("expected cursor=0 scroll=0 for empty, got cursor=%d scroll=%d", v2.Cursor, v2.ScrollOffset)
	}
}

func TestCursorDown_EmptyGenresResetsStaleState(t *testing.T) {
	v := View{
		Level:        GenreLevel,
		Genres:       nil,
		Cursor:       3,
		ScrollOffset: 2,
	}
	v2 := v.CursorDown()
	if v2.Cursor != 0 || v2.ScrollOffset != 0 {
		t.Errorf("expected cursor=0 scroll=0 for empty, got cursor=%d scroll=%d", v2.Cursor, v2.ScrollOffset)
	}
}

func TestCursorDownDoesNotAdvanceScrollWhenNoRowsVisible(t *testing.T) {
	v := View{
		Level:        TrackLevel,
		Tracks:       tracks(struct{ genre, artist, title string }{"Rock", "Alice", "t1"}, struct{ genre, artist, title string }{"Rock", "Alice", "t2"}),
		Cursor:       0,
		ScrollOffset: 0,
		Height:       1, // TrackLevel reserves two rows, leaving no content rows.
	}
	if rows := v.visibleRows(); rows != 0 {
		t.Fatalf("precondition: expected no visible rows, got %d", rows)
	}

	v = v.CursorDown()
	if v.Cursor != 1 {
		t.Errorf("expected Cursor=1, got %d", v.Cursor)
	}
	if v.ScrollOffset > v.Cursor {
		t.Errorf("expected ScrollOffset<=Cursor, got %d > %d", v.ScrollOffset, v.Cursor)
	}
}

func TestMoveCursor_EmptyGenres(t *testing.T) {
	v := View{Level: GenreLevel, Genres: nil, Cursor: 2, ScrollOffset: 1, Height: 20}
	v2 := v.moveCursor(5)
	if v2.Cursor != 0 || v2.ScrollOffset != 0 {
		t.Errorf("expected cursor=0 scroll=0 for empty, got cursor=%d scroll=%d", v2.Cursor, v2.ScrollOffset)
	}
}

func TestCursorUp_ClampsStaleCursorPastEnd(t *testing.T) {
	v := View{
		Level:        GenreLevel,
		Genres:       []string{"A", "B", "C"},
		Cursor:       9,
		ScrollOffset: 9,
		Height:       20,
	}
	v = v.CursorUp()
	if v.Cursor != 2 {
		t.Fatalf("CursorUp: expected stale cursor clamped to 2, got %d", v.Cursor)
	}
	if v.ScrollOffset > v.Cursor {
		t.Fatalf("CursorUp: expected ScrollOffset<=Cursor, got %d > %d", v.ScrollOffset, v.Cursor)
	}
}

func TestCursorDown_ClampsStaleCursorPastEnd(t *testing.T) {
	v := View{
		Level:        GenreLevel,
		Genres:       []string{"A", "B", "C"},
		Cursor:       9,
		ScrollOffset: 9,
		Height:       20,
	}
	v = v.CursorDown()
	if v.Cursor != 2 {
		t.Fatalf("CursorDown: expected stale cursor clamped to 2, got %d", v.Cursor)
	}
	if v.ScrollOffset > v.Cursor {
		t.Fatalf("CursorDown: expected ScrollOffset<=Cursor, got %d > %d", v.ScrollOffset, v.Cursor)
	}
}

func TestCursorDown_ClampsNegativeCursorToFirstRow(t *testing.T) {
	v := View{
		Level:  GenreLevel,
		Genres: []string{"A", "B", "C"},
		Cursor: -3,
	}
	v = v.CursorDown()
	if v.Cursor != 0 {
		t.Fatalf("CursorDown: expected negative cursor clamped to 0, got %d", v.Cursor)
	}
}

func TestCursorUp_ClampsNegativeCursorToFirstRow(t *testing.T) {
	v := View{
		Level:  GenreLevel,
		Genres: []string{"A", "B", "C"},
		Cursor: -3,
	}
	v = v.CursorUp()
	if v.Cursor != 0 {
		t.Fatalf("CursorUp: expected negative cursor clamped to 0, got %d", v.Cursor)
	}
}
