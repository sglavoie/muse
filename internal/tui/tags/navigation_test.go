package tags

import (
	"testing"

	"github.com/sglavoie/muse/internal/db"
)

// TestBack_ClampsCursorWhenParentShrank verifies that Back() clamps the saved
// tag cursor against the (possibly shrunk) Tags slice — e.g. when a tag was
// deleted while the user was viewing its tracks.
func TestBack_ClampsCursorWhenParentShrank(t *testing.T) {
	tags := tagRows("a", "b", "c", "d")
	v := View{
		Level:           TrackLevel,
		Cursor:          0,
		TagCursor:       3,
		TagScrollOffset: 2,
		Tags:            tags,
		TagCounts: map[int64]int{
			tags[0].ID: 1,
			tags[1].ID: 1,
		},
		Tracks: trackRows("track"),
	}

	// Simulate tag deletion: now only two tags exist with non-zero counts.
	v.Tags = tags[:2]
	v = v.Back()
	if v.Level != TagLevel {
		t.Fatalf("expected TagLevel, got %v", v.Level)
	}
	if v.Cursor != 1 {
		t.Errorf("Back: expected clamped Cursor=1, got %d", v.Cursor)
	}
	if v.ScrollOffset > v.Cursor {
		t.Errorf("Back: ScrollOffset(%d) > Cursor(%d)", v.ScrollOffset, v.Cursor)
	}
}

// TestBack_ClampsCursorWhenParentEmpty resets cursor/scroll to 0 when every
// tag was removed (or all counts hit zero) during the drill-down.
func TestBack_ClampsCursorWhenParentEmpty(t *testing.T) {
	v := View{
		Level:           TrackLevel,
		Cursor:          0,
		TagCursor:       2,
		TagScrollOffset: 1,
		Tags:            []db.TagRow{},
	}

	v = v.Back()
	if v.Cursor != 0 || v.ScrollOffset != 0 {
		t.Errorf("Back with empty parent: expected cursor=0 scroll=0, got cursor=%d scroll=%d", v.Cursor, v.ScrollOffset)
	}
}

// TestBack_SkipsClampWhenTagsNil verifies the escape hatch used by outer
// wrappers that defer parent-list clamping: when Tags is nil, the restored
// cursor/scroll are passed through unchanged.
func TestBack_SkipsClampWhenTagsNil(t *testing.T) {
	v := View{
		Level:           TrackLevel,
		Cursor:          0,
		TagCursor:       7,
		TagScrollOffset: 4,
		// Tags deliberately left nil.
	}

	v = v.Back()
	if v.Cursor != 7 {
		t.Errorf("expected pass-through Cursor=7, got %d", v.Cursor)
	}
	if v.ScrollOffset != 4 {
		t.Errorf("expected pass-through ScrollOffset=4, got %d", v.ScrollOffset)
	}
}

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

func TestBackWithNoVisibleRowsKeepsScrollOnCursor(t *testing.T) {
	tags := tagRows("a", "b", "c")
	v := View{
		Level:           TrackLevel,
		Cursor:          0,
		TagCursor:       2,
		TagScrollOffset: 0,
		Height:          1,
		FilterActive:    true,
		SortIndicator:   "Sort: Name",
		Tags:            tags,
		TagCounts:       tagCountsFor(tags, 1),
		Tracks:          trackRows("track"),
	}

	v = v.Back()

	if v.Cursor != 2 {
		t.Fatalf("expected Cursor=2, got %d", v.Cursor)
	}
	if v.ScrollOffset != v.Cursor {
		t.Fatalf("expected ScrollOffset=%d, got %d", v.Cursor, v.ScrollOffset)
	}
}

func TestOpenTracksClearsSelectedHashes(t *testing.T) {
	tag := tagRows("tag")[0]
	v := View{
		Level:          TagLevel,
		SelectedHashes: map[string]struct{}{"stale": {}},
	}

	v = v.OpenTracks(tag, trackRows("next"))

	if v.SelectedHashes != nil {
		t.Fatalf("OpenTracks should clear stale selected hashes, got %v", v.SelectedHashes)
	}
}

func TestBackClearsSelectedHashes(t *testing.T) {
	tags := tagRows("tag")
	v := View{
		Level:          TrackLevel,
		SelectedHashes: map[string]struct{}{"stale": {}},
		Tags:           tags,
		TagCounts:      tagCountsFor(tags, 1),
		Tracks:         trackRows("track"),
	}

	v = v.Back()

	if v.SelectedHashes != nil {
		t.Fatalf("Back should clear selected hashes outside track level, got %v", v.SelectedHashes)
	}
}

// TestBack_KeepsCursorVisibleAfterResize verifies that when the saved
// TagScrollOffset trails far behind a still-valid TagCursor (e.g. because the
// terminal was resized smaller while the user was drilled into TrackLevel),
// the clamp pushes ScrollOffset forward so the cursor is rendered inside the
// visible window after Back().
func TestBack_KeepsCursorVisibleAfterResize(t *testing.T) {
	tags := tagRows("a", "b", "c", "d", "e", "f", "g", "h", "i", "j")
	v := View{
		Level:           TrackLevel,
		Cursor:          0,
		TagCursor:       8,
		TagScrollOffset: 0,
		Tags:            tags,
		TagCounts:       tagCountsFor(tags, 1),
		Tracks:          trackRows("track"),
		Height:          5, // TagLevel visibleRows = 5
	}

	v = v.Back()
	if v.Level != TagLevel {
		t.Fatalf("expected TagLevel, got %v", v.Level)
	}
	if v.Cursor != 8 {
		t.Fatalf("expected cursor=8, got %d", v.Cursor)
	}
	maxRows := v.visibleRows()
	if v.Cursor < v.ScrollOffset || v.Cursor >= v.ScrollOffset+maxRows {
		t.Errorf("cursor=%d not visible in window [%d, %d)", v.Cursor, v.ScrollOffset, v.ScrollOffset+maxRows)
	}
}

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
