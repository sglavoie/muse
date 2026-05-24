package tags

import (
	"strings"
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

func TestCursorDownWithNoVisibleRowsKeepsScrollOnCursor(t *testing.T) {
	v := View{
		Level:         TagLevel,
		Cursor:        0,
		Height:        1,
		FilterActive:  true,
		SortIndicator: "Sort: Name",
		Tags:          []db.TagRow{{ID: 1, Name: "a"}, {ID: 2, Name: "b"}},
		TagCounts:     map[int64]int{1: 1, 2: 1},
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
	v := View{
		Level:         TagLevel,
		Cursor:        2,
		ScrollOffset:  0,
		Height:        1,
		FilterActive:  true,
		SortIndicator: "Sort: Name",
		Tags: []db.TagRow{
			{ID: 1, Name: "a"},
			{ID: 2, Name: "b"},
			{ID: 3, Name: "c"},
		},
		TagCounts: map[int64]int{1: 1, 2: 1, 3: 1},
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
		Tracks: []db.PlaylistTrackRow{
			{ID: 1, Title: "one"},
			{ID: 2, Title: "two"},
			{ID: 3, Title: "three"},
		},
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
	v := View{
		Level:           TrackLevel,
		Cursor:          0,
		TagCursor:       2,
		TagScrollOffset: 0,
		Height:          1,
		FilterActive:    true,
		SortIndicator:   "Sort: Name",
		Tags: []db.TagRow{
			{ID: 1, Name: "a"},
			{ID: 2, Name: "b"},
			{ID: 3, Name: "c"},
		},
		TagCounts: map[int64]int{1: 1, 2: 1, 3: 1},
		Tracks:    []db.PlaylistTrackRow{{ID: 1}},
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
	v := View{
		Level:          TagLevel,
		SelectedHashes: map[string]struct{}{"stale": {}},
	}

	v = v.OpenTracks(db.TagRow{ID: 1, Name: "tag"}, []db.PlaylistTrackRow{{Hash: "next"}})

	if v.SelectedHashes != nil {
		t.Fatalf("OpenTracks should clear stale selected hashes, got %v", v.SelectedHashes)
	}
}

func TestBackClearsSelectedHashes(t *testing.T) {
	v := View{
		Level:          TrackLevel,
		SelectedHashes: map[string]struct{}{"stale": {}},
		Tags:           []db.TagRow{{ID: 1, Name: "tag"}},
		TagCounts:      map[int64]int{1: 1},
		Tracks:         []db.PlaylistTrackRow{{Hash: "track"}},
	}

	v = v.Back()

	if v.SelectedHashes != nil {
		t.Fatalf("Back should clear selected hashes outside track level, got %v", v.SelectedHashes)
	}
}

func TestRenderTagsAllZeroCountsShowsEmptyState(t *testing.T) {
	v := View{
		Level: TagLevel,
		Width: 40,
		Tags: []db.TagRow{
			{ID: 1, Name: "unused"},
			{ID: 2, Name: "empty"},
		},
		TagCounts: map[int64]int{1: 0, 2: 0},
	}

	out := v.Render()

	if strings.Contains(out, "(no matches)") {
		t.Fatalf("zero-count tags without a filter should render an empty state, got:\n%s", out)
	}
	if !strings.Contains(out, "No tagged tracks yet") {
		t.Fatalf("expected empty tagged-tracks message, got:\n%s", out)
	}
}

func TestRenderTagsFilterNoMatchesStillShowsNoMatches(t *testing.T) {
	v := View{
		Level:      TagLevel,
		Width:      40,
		FilterText: "zzz",
		Tags:       []db.TagRow{{ID: 1, Name: "alpha"}},
		TagCounts:  map[int64]int{1: 1},
	}

	out := v.Render()

	if !strings.Contains(out, "(no matches)") {
		t.Fatalf("expected filter miss to render no matches, got:\n%s", out)
	}
}

func TestRenderTagsCreatingWithNoVisibleRowsDoesNotPanic(t *testing.T) {
	v := View{
		Level:         TagLevel,
		Width:         20,
		Height:        1,
		FilterActive:  true,
		SortIndicator: "Sort: Name",
		Tags:          []db.TagRow{{ID: 1, Name: "a"}},
		TagCounts:     map[int64]int{1: 1},
		Creating:      true,
		CreateInput:   "new",
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Render panicked with no visible rows while creating: %v", r)
		}
	}()

	_ = v.Render()
}

func TestRenderTagsWithNegativeScrollOffsetDoesNotPanic(t *testing.T) {
	v := View{
		Level:        TagLevel,
		Width:        20,
		Height:       10,
		ScrollOffset: -3,
		Tags:         []db.TagRow{{ID: 1, Name: "alpha"}},
		TagCounts:    map[int64]int{1: 1},
	}

	out := v.Render()

	if !strings.Contains(out, "alpha") {
		t.Fatalf("expected render to clamp negative scroll and include first tag, got:\n%s", out)
	}
}

func TestRenderTagsWithHighScrollOffsetShowsLastWindow(t *testing.T) {
	v := View{
		Level:        TagLevel,
		Width:        20,
		Height:       2,
		ScrollOffset: 50,
		Tags: []db.TagRow{
			{ID: 1, Name: "alpha"},
			{ID: 2, Name: "beta"},
			{ID: 3, Name: "gamma"},
		},
		TagCounts: map[int64]int{1: 1, 2: 1, 3: 1},
	}

	out := v.Render()

	if !strings.Contains(out, "gamma") {
		t.Fatalf("expected render to clamp high scroll and include last tag, got:\n%s", out)
	}
}

func TestRenderTracksWithNegativeScrollOffsetDoesNotPanic(t *testing.T) {
	v := View{
		Level:        TrackLevel,
		Width:        80,
		Height:       10,
		ScrollOffset: -2,
		Tracks: []db.PlaylistTrackRow{
			{ID: 1, Title: "first", Artist: "artist", Album: "album"},
		},
	}

	out := v.Render()

	if !strings.Contains(out, "first") {
		t.Fatalf("expected render to clamp negative scroll and include first track, got:\n%s", out)
	}
}

func TestRenderTracksWithHighScrollOffsetShowsLastWindow(t *testing.T) {
	v := View{
		Level:        TrackLevel,
		Width:        80,
		Height:       5,
		ScrollOffset: 50,
		Tracks: []db.PlaylistTrackRow{
			{ID: 1, Title: "first", Artist: "artist", Album: "album"},
			{ID: 2, Title: "second", Artist: "artist", Album: "album"},
			{ID: 3, Title: "third", Artist: "artist", Album: "album"},
		},
	}

	out := v.Render()

	if !strings.Contains(out, "third") {
		t.Fatalf("expected render to clamp high scroll and include last track, got:\n%s", out)
	}
}

// TestBack_KeepsCursorVisibleAfterResize verifies that when the saved
// TagScrollOffset trails far behind a still-valid TagCursor (e.g. because the
// terminal was resized smaller while the user was drilled into TrackLevel),
// the clamp pushes ScrollOffset forward so the cursor is rendered inside the
// visible window after Back().
func TestBack_KeepsCursorVisibleAfterResize(t *testing.T) {
	tags := []db.TagRow{
		{ID: 1, Name: "a"}, {ID: 2, Name: "b"}, {ID: 3, Name: "c"},
		{ID: 4, Name: "d"}, {ID: 5, Name: "e"}, {ID: 6, Name: "f"},
		{ID: 7, Name: "g"}, {ID: 8, Name: "h"}, {ID: 9, Name: "i"},
		{ID: 10, Name: "j"},
	}
	counts := map[int64]int{1: 1, 2: 1, 3: 1, 4: 1, 5: 1, 6: 1, 7: 1, 8: 1, 9: 1, 10: 1}
	v := View{
		Level:           TrackLevel,
		Cursor:          0,
		TagCursor:       8,
		TagScrollOffset: 0,
		Tags:            tags,
		TagCounts:       counts,
		Tracks:          []db.PlaylistTrackRow{{ID: 1}},
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

// TestGoToBottom_ClampsScrollOffsetWhenListShrank guards a regression where
// GoToBottom kept a stale ScrollOffset above the new last index after a
// filter/delete shrank the list, leaving the cursor rendering above the
// visible window.
func TestGoToBottom_ClampsScrollOffsetWhenListShrank(t *testing.T) {
	v := View{
		Level: TagLevel,
		Tags: []db.TagRow{
			{ID: 1, Name: "a"},
			{ID: 2, Name: "b"},
			{ID: 3, Name: "c"},
		},
		TagCounts:    map[int64]int{1: 1, 2: 1, 3: 1},
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
