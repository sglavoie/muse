package tags

import (
	"testing"
)

// TestCursorUp_ClampsStaleCursorAfterShrink guards a regression where
// CursorUp simply decremented a stale Cursor (e.g. left over from before a
// filter/delete shrank the list) instead of clamping it to the new last
// index. The cursor must land inside [0, n-1] after a single up press.
func TestCursorUp_ClampsStaleCursorAfterShrink(t *testing.T) {
	tags := tagRows("a", "b", "c")
	v := View{
		Level:     TagLevel,
		Cursor:    10,
		Tags:      tags,
		TagCounts: tagCountsFor(tags, 1),
		Height:    20,
	}

	v = v.CursorUp()
	if v.Cursor != 2 {
		t.Errorf("CursorUp with stale Cursor=10 against itemCount=3: want Cursor=2, got %d", v.Cursor)
	}
}

// TestCursorDown_ClampsStaleCursorAfterShrink guards the same regression for
// CursorDown: a stale Cursor above the list end must be brought back inside
// [0, n-1] rather than incremented further out of range.
func TestCursorDown_ClampsStaleCursorAfterShrink(t *testing.T) {
	tags := tagRows("a", "b", "c")
	v := View{
		Level:     TagLevel,
		Cursor:    10,
		Tags:      tags,
		TagCounts: tagCountsFor(tags, 1),
		Height:    20,
	}

	v = v.CursorDown()
	if v.Cursor != 2 {
		t.Errorf("CursorDown with stale Cursor=10 against itemCount=3: want Cursor=2, got %d", v.Cursor)
	}
}

// TestCursorUp_ClampsNegativeCursor verifies that a negative stale Cursor is
// brought to 0 rather than left negative or decremented further.
func TestCursorUp_ClampsNegativeCursor(t *testing.T) {
	tags := tagRows("a", "b")
	v := View{
		Level:     TagLevel,
		Cursor:    -3,
		Tags:      tags,
		TagCounts: tagCountsFor(tags, 1),
		Height:    20,
	}

	v = v.CursorUp()
	if v.Cursor < 0 {
		t.Errorf("CursorUp with negative Cursor: want >=0, got %d", v.Cursor)
	}
}

// TestCursorDown_ClampsStaleScrollOffsetWhenListShrank guards a regression
// where CursorDown left a stale ScrollOffset above the new clamped Cursor
// after the list shrank (e.g. via filter/delete), leaving the cursor rendering
// outside the visible window until the next render pass clamped it.
func TestCursorDown_ClampsStaleScrollOffsetWhenListShrank(t *testing.T) {
	tags := tagRows("a", "b", "c")
	v := View{
		Level:        TagLevel,
		Cursor:       10,
		ScrollOffset: 50,
		Tags:         tags,
		TagCounts:    tagCountsFor(tags, 1),
		Height:       20,
	}

	v = v.CursorDown()
	if v.Cursor != 2 {
		t.Errorf("CursorDown: expected clamped Cursor=2, got %d", v.Cursor)
	}
	if v.ScrollOffset > v.Cursor {
		t.Errorf("CursorDown: expected ScrollOffset<=Cursor (%d), got ScrollOffset=%d", v.Cursor, v.ScrollOffset)
	}
}

// TestGoToBottom_ClampsScrollOffsetWhenListShrank guards a regression where
// GoToBottom kept a stale ScrollOffset above the new last index after a
// filter/delete shrank the list, leaving the cursor rendering above the
// visible window.
func TestGoToBottom_ClampsScrollOffsetWhenListShrank(t *testing.T) {
	tags := tagRows("a", "b", "c")
	v := View{
		Level:        TagLevel,
		Tags:         tags,
		TagCounts:    tagCountsFor(tags, 1),
		Cursor:       0,
		ScrollOffset: 50,
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
