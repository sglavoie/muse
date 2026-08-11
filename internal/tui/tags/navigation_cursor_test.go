package tags

import (
	"testing"
)

func TestGoToBottom_EmptyListResetsCursorAndScroll(t *testing.T) {
	v := View{
		Level:        TagLevel,
		Cursor:       7,
		ScrollOffset: 3,
	}

	v = v.GoToBottom()

	if v.Cursor != 0 {
		t.Errorf("expected Cursor=0 for empty list, got %d", v.Cursor)
	}
	if v.ScrollOffset != 0 {
		t.Errorf("expected ScrollOffset=0 for empty list, got %d", v.ScrollOffset)
	}
}

func TestCursorMovement_EmptyListResetsCursorAndScroll(t *testing.T) {
	tests := []struct {
		name string
		move func(View) View
	}{
		{name: "up", move: View.CursorUp},
		{name: "down", move: View.CursorDown},
		{name: "half-page-up", move: View.HalfPageUp},
		{name: "half-page-down", move: View.HalfPageDown},
		{name: "full-page-up", move: View.FullPageUp},
		{name: "full-page-down", move: View.FullPageDown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := View{
				Level:        TagLevel,
				Cursor:       7,
				ScrollOffset: 3,
			}

			v = tt.move(v)

			if v.Cursor != 0 {
				t.Errorf("expected Cursor=0 for empty list, got %d", v.Cursor)
			}
			if v.ScrollOffset != 0 {
				t.Errorf("expected ScrollOffset=0 for empty list, got %d", v.ScrollOffset)
			}
		})
	}
}

func TestVisibleWindowKeepsCursorVisibleWhenScrollTrails(t *testing.T) {
	v := View{
		Cursor:       8,
		ScrollOffset: 0,
	}

	start, end := v.visibleWindow(20, 5)

	if start != 4 || end != 9 {
		t.Fatalf("visibleWindow = [%d,%d), want [4,9)", start, end)
	}
}

func TestVisibleWindowKeepsCursorVisibleWhenScrollLeads(t *testing.T) {
	v := View{
		Cursor:       2,
		ScrollOffset: 10,
	}

	start, end := v.visibleWindow(20, 5)

	if start != 2 || end != 7 {
		t.Fatalf("visibleWindow = [%d,%d), want [2,7)", start, end)
	}
}

func TestVisibleWindowWithNoRowsReturnsEmptyWindow(t *testing.T) {
	v := View{
		Cursor:       2,
		ScrollOffset: 10,
	}

	start, end := v.visibleWindow(20, 0)

	if start != 0 || end != 0 {
		t.Fatalf("visibleWindow = [%d,%d), want [0,0)", start, end)
	}
}

func TestCursorDownWithNoVisibleRowsKeepsScrollOnCursor(t *testing.T) {
	tags := tagRows("a", "b")
	v := View{
		Level:         TagLevel,
		Cursor:        0,
		Height:        1,
		FilterActive:  true,
		SortIndicator: "Sort: Name",
		Tags:          tags,
		TagCounts:     tagCountsFor(tags, 1),
	}

	v = v.CursorDown()

	if v.Cursor != 1 {
		t.Fatalf("expected Cursor=1, got %d", v.Cursor)
	}
	if v.ScrollOffset != v.Cursor {
		t.Fatalf("expected ScrollOffset=%d, got %d", v.Cursor, v.ScrollOffset)
	}
}

func TestCursorUpWithNoVisibleRowsKeepsScrollOnCursor(t *testing.T) {
	tags := tagRows("a", "b", "c")
	v := View{
		Level:         TagLevel,
		Cursor:        2,
		ScrollOffset:  0,
		Height:        1,
		FilterActive:  true,
		SortIndicator: "Sort: Name",
		Tags:          tags,
		TagCounts:     tagCountsFor(tags, 1),
	}

	v = v.CursorUp()

	if v.Cursor != 1 {
		t.Fatalf("expected Cursor=1, got %d", v.Cursor)
	}
	if v.ScrollOffset != v.Cursor {
		t.Fatalf("expected ScrollOffset=%d, got %d", v.Cursor, v.ScrollOffset)
	}
}

func TestPageMovementWithNoVisibleRowsKeepsScrollOnCursor(t *testing.T) {
	v := View{
		Level:        TrackLevel,
		Cursor:       0,
		ScrollOffset: 0,
		Height:       2, // TrackLevel reserves 3 rows, leaving no data rows.
		Tracks:       trackRows("one", "two", "three"),
	}

	v = v.moveCursor(2)

	if v.Cursor != 2 {
		t.Fatalf("expected Cursor=2, got %d", v.Cursor)
	}
	if v.ScrollOffset != v.Cursor {
		t.Fatalf("expected ScrollOffset=%d, got %d", v.Cursor, v.ScrollOffset)
	}
}

func TestHalfPageDownMovesAtLeastOneVisibleRow(t *testing.T) {
	tags := tagRows("a", "b", "c")
	v := View{
		Level:     TagLevel,
		Cursor:    0,
		Height:    1,
		Tags:      tags,
		TagCounts: tagCountsFor(tags, 1),
	}

	v = v.HalfPageDown()

	if v.Cursor != 1 {
		t.Fatalf("HalfPageDown with one visible row: expected Cursor=1, got %d", v.Cursor)
	}
	if v.ScrollOffset != 1 {
		t.Fatalf("HalfPageDown with one visible row: expected ScrollOffset=1, got %d", v.ScrollOffset)
	}
}

func TestHalfPageUpMovesAtLeastOneVisibleRow(t *testing.T) {
	tags := tagRows("a", "b", "c")
	v := View{
		Level:        TagLevel,
		Cursor:       2,
		ScrollOffset: 2,
		Height:       1,
		Tags:         tags,
		TagCounts:    tagCountsFor(tags, 1),
	}

	v = v.HalfPageUp()

	if v.Cursor != 1 {
		t.Fatalf("HalfPageUp with one visible row: expected Cursor=1, got %d", v.Cursor)
	}
	if v.ScrollOffset != 1 {
		t.Fatalf("HalfPageUp with one visible row: expected ScrollOffset=1, got %d", v.ScrollOffset)
	}
}

func TestFullPageMovementWithNoVisibleRowsMovesAtLeastOneRow(t *testing.T) {
	tracks := trackRows("one", "two", "three")

	down := View{
		Level:  TrackLevel,
		Cursor: 0,
		Height: 2, // TrackLevel reserves 3 rows, leaving no data rows.
		Tracks: tracks,
	}.FullPageDown()
	if down.Cursor != 1 {
		t.Fatalf("FullPageDown with no visible rows: expected Cursor=1, got %d", down.Cursor)
	}
	if down.ScrollOffset != down.Cursor {
		t.Fatalf("FullPageDown with no visible rows: expected ScrollOffset=%d, got %d", down.Cursor, down.ScrollOffset)
	}

	up := View{
		Level:        TrackLevel,
		Cursor:       2,
		ScrollOffset: 2,
		Height:       2, // TrackLevel reserves 3 rows, leaving no data rows.
		Tracks:       tracks,
	}.FullPageUp()
	if up.Cursor != 1 {
		t.Fatalf("FullPageUp with no visible rows: expected Cursor=1, got %d", up.Cursor)
	}
	if up.ScrollOffset != up.Cursor {
		t.Fatalf("FullPageUp with no visible rows: expected ScrollOffset=%d, got %d", up.Cursor, up.ScrollOffset)
	}
}
