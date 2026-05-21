package tags

import (
	"testing"

	"github.com/sglavoie/muse/internal/db"
)

func TestEffectiveTags_ExcludesZeroCount(t *testing.T) {
	v := View{
		Tags: []db.TagRow{
			{ID: 1, Name: "has-tracks"},
			{ID: 2, Name: "empty"},
			{ID: 3, Name: "also-has-tracks"},
		},
		TagCounts: map[int64]int{1: 5, 2: 0, 3: 2},
	}

	got := v.EffectiveTags()
	if len(got) != 2 {
		t.Fatalf("got %d tags, want 2", len(got))
	}
	if got[0].Name != "has-tracks" || got[1].Name != "also-has-tracks" {
		t.Errorf("unexpected tags: %+v", got)
	}
}

func TestEffectiveTags_ExcludesZeroCountWithFilter(t *testing.T) {
	v := View{
		Tags: []db.TagRow{
			{ID: 1, Name: "workout"},
			{ID: 2, Name: "work"},
			{ID: 3, Name: "chill"},
		},
		TagCounts:  map[int64]int{1: 3, 2: 0, 3: 1},
		FilterText: "work",
	}

	got := v.EffectiveTags()
	if len(got) != 1 {
		t.Fatalf("got %d tags, want 1", len(got))
	}
	if got[0].Name != "workout" {
		t.Errorf("got %q, want %q", got[0].Name, "workout")
	}
}

// TestBack_ClampsCursorWhenParentShrank verifies that Back() clamps the saved
// tag cursor against the (possibly shrunk) Tags slice — e.g. when a tag was
// deleted while the user was viewing its tracks.
func TestBack_ClampsCursorWhenParentShrank(t *testing.T) {
	v := View{
		Level:           TrackLevel,
		Cursor:          0,
		TagCursor:       3,
		TagScrollOffset: 2,
		Tags: []db.TagRow{
			{ID: 1, Name: "a"}, {ID: 2, Name: "b"}, {ID: 3, Name: "c"}, {ID: 4, Name: "d"},
		},
		TagCounts: map[int64]int{1: 1, 2: 1},
		Tracks:    []db.PlaylistTrackRow{{ID: 1}},
	}
	// Simulate tag deletion: now only two tags exist with non-zero counts.
	v.Tags = []db.TagRow{{ID: 1, Name: "a"}, {ID: 2, Name: "b"}}
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

func TestEffectiveTags_NilCountsShowsAll(t *testing.T) {
	v := View{
		Tags: []db.TagRow{
			{ID: 1, Name: "a"},
			{ID: 2, Name: "b"},
		},
	}

	got := v.EffectiveTags()
	if len(got) != 2 {
		t.Fatalf("got %d tags, want 2 (nil counts should show all)", len(got))
	}
}
