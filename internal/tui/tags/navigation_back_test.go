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
