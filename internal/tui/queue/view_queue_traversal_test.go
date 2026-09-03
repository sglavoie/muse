package queue

import (
	"fmt"
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/tui/keys"
	"github.com/sglavoie/muse/internal/tui/sorting"
	"github.com/sglavoie/muse/internal/tui/statusbar"
)

// TestTraversalOrdersRowsAndKeepsIdentity verifies that an active shuffled
// timeline decides the display order while every row still carries the index of
// the queue row it came from.
func TestTraversalOrdersRowsAndKeepsIdentity(t *testing.T) {
	v := makeQueue(120, entriesTitled("A", "B", "C", "D"))
	v.Traversal = shuffledTimeline(1, 2, 0, 3, 1)

	got := v.FilteredEntries()
	if want := []string{"C", "A", "D", "B"}; !slices.Equal(entryTitles(got), want) {
		t.Errorf("display order = %v, want %v", entryTitles(got), want)
	}
	if want := []int{2, 0, 3, 1}; !slices.Equal(entryOrigIdxs(got), want) {
		t.Errorf("OrigIdx = %v, want %v", entryOrigIdxs(got), want)
	}
}

// TestTraversalIgnoredWhenNotShuffled verifies that a sequential timeline is
// never allowed to reorder anything: with shuffle off the persisted order is
// already the traversal, and honouring the snapshot could only introduce drift.
func TestTraversalIgnoredWhenNotShuffled(t *testing.T) {
	v := makeQueue(120, entriesTitled("A", "B", "C"))
	// A hostile snapshot: sequential, yet claiming a reordered timeline.
	v.Traversal = fixedTraversal{Traversal{Order: []int{2, 1, 0}, CurrentPosition: 0}}

	got := entryTitles(v.FilteredEntries())
	if want := []string{"A", "B", "C"}; !slices.Equal(got, want) {
		t.Errorf("shuffle-off order = %v, want persisted %v", got, want)
	}
}

// TestTraversalRejectedWhenStaleOrMalformed verifies that a snapshot that is
// not a true permutation of the live queue falls back to persisted order rather
// than dropping or duplicating rows.
func TestTraversalRejectedWhenStaleOrMalformed(t *testing.T) {
	cases := []struct {
		name  string
		order []int
	}{
		{"too short", []int{1, 0}},
		{"too long", []int{0, 1, 2, 0}},
		{"index out of range", []int{0, 3, 1}},
		{"negative index", []int{0, -1, 1}},
		{"duplicate index", []int{0, 1, 1}},
		{"empty", []int{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := makeQueue(120, entriesTitled("A", "B", "C"))
			v.Traversal = shuffledTimeline(0, tc.order...)

			got := v.FilteredEntries()
			if want := []string{"A", "B", "C"}; !slices.Equal(entryTitles(got), want) {
				t.Errorf("order = %v, want persisted %v", entryTitles(got), want)
			}
			if want := []int{0, 1, 2}; !slices.Equal(entryOrigIdxs(got), want) {
				t.Errorf("OrigIdx = %v, want %v", entryOrigIdxs(got), want)
			}
		})
	}
}

// TestNoTraversalSourceKeepsPersistedOrder verifies that a player without the
// timeline capability leaves the queue exactly as it was before this existed.
func TestNoTraversalSourceKeepsPersistedOrder(t *testing.T) {
	v := makeQueue(120, entriesTitled("A", "B", "C"))
	got := v.FilteredEntries()
	if want := []string{"A", "B", "C"}; !slices.Equal(entryTitles(got), want) {
		t.Errorf("order = %v, want %v", entryTitles(got), want)
	}
}

// TestExplicitSortOverridesTraversalAndDefaultRestoresIt verifies the display
// order precedence: an explicit metadata sort wins, and cycling back to Default
// hands the panel back to the live timeline without touching playback.
func TestExplicitSortOverridesTraversalAndDefaultRestoresIt(t *testing.T) {
	v := makeQueue(120, entriesTitled("C", "A", "B"))
	v.Traversal = shuffledTimeline(0, 2, 0, 1)

	v.SortState = sorting.SortState{Field: sorting.SortTitle, Dir: sorting.SortAsc}
	v = v.RecomputeDisplayOrder()
	sorted := entryTitles(v.FilteredEntries())
	if want := []string{"A", "B", "C"}; !slices.Equal(sorted, want) {
		t.Errorf("explicit sort must win: got %v, want %v", sorted, want)
	}

	v.SortState = sorting.SortState{}
	v = v.RecomputeDisplayOrder()
	if v.DisplayOrder != nil {
		t.Fatalf("cycling to Default must clear DisplayOrder, got %v", v.DisplayOrder)
	}
	restored := entryTitles(v.FilteredEntries())
	if want := []string{"B", "C", "A"}; !slices.Equal(restored, want) {
		t.Errorf("Default must restore the timeline: got %v, want %v", restored, want)
	}
}

// TestStaleExplicitSortFallsBackToPersistedOrder verifies that a DisplayOrder
// that no longer lines up with the queue drops to persisted order rather than
// to the timeline, so rows never contradict the "sort" chunk on the strip.
func TestStaleExplicitSortFallsBackToPersistedOrder(t *testing.T) {
	v := makeQueue(120, entriesTitled("A", "B", "C"))
	v.Traversal = shuffledTimeline(0, 2, 0, 1)
	v.SortState = sorting.SortState{Field: sorting.SortTitle, Dir: sorting.SortAsc}
	v.DisplayOrder = []int{1, 0} // stale: the queue grew after the sort

	got := entryTitles(v.FilteredEntries())
	if want := []string{"A", "B", "C"}; !slices.Equal(got, want) {
		t.Errorf("stale sort order = %v, want persisted %v", got, want)
	}
}

// TestTraversalPreservedThroughTextFilter verifies that filtering removes rows
// without resequencing the ones it keeps.
func TestTraversalPreservedThroughTextFilter(t *testing.T) {
	v := makeQueue(120, entriesTitled("keep1", "drop", "keep2", "keep3"))
	v.Traversal = shuffledTimeline(0, 3, 1, 0, 2)
	v.Filter.Text = "keep"

	got := v.FilteredEntries()
	if want := []string{"keep3", "keep1", "keep2"}; !slices.Equal(entryTitles(got), want) {
		t.Errorf("filtered order = %v, want %v", entryTitles(got), want)
	}
	if want := []int{3, 0, 2}; !slices.Equal(entryOrigIdxs(got), want) {
		t.Errorf("filtered OrigIdx = %v, want %v", entryOrigIdxs(got), want)
	}
}

// TestTraversalPreservedThroughTagFilter verifies the same for the cross-view
// tag filter, which runs before the text filter.
func TestTraversalPreservedThroughTagFilter(t *testing.T) {
	v := makeQueue(120, entriesTitled("A", "B", "C", "D"))
	v.Traversal = shuffledTimeline(0, 3, 1, 0, 2)
	v.TagFilterHashes = map[string]struct{}{"hash-A": {}, "hash-D": {}}

	got := v.FilteredEntries()
	if want := []string{"D", "A"}; !slices.Equal(entryTitles(got), want) {
		t.Errorf("tag-filtered order = %v, want %v", entryTitles(got), want)
	}
	if want := []int{3, 0}; !slices.Equal(entryOrigIdxs(got), want) {
		t.Errorf("tag-filtered OrigIdx = %v, want %v", entryOrigIdxs(got), want)
	}
}

// TestTraversalKeepsDuplicateRowsDistinct verifies that two queue rows for the
// same track keep their own identities when the timeline separates them.
func TestTraversalKeepsDuplicateRowsDistinct(t *testing.T) {
	entries := entriesTitled("Dup", "Other", "Dup")
	entries[2].Hash = entries[0].Hash
	entries[2].Path = entries[0].Path
	v := makeQueue(120, entries)
	v.Traversal = shuffledTimeline(1, 2, 0, 1)

	got := v.FilteredEntries()
	if want := []int{2, 0, 1}; !slices.Equal(entryOrigIdxs(got), want) {
		t.Fatalf("duplicate rows must stay distinct: OrigIdx = %v, want %v", entryOrigIdxs(got), want)
	}
	v.Cursor = 0
	if idx := v.FocusedEntryIndex(); idx != 2 {
		t.Errorf("focused entry = %d, want the second occurrence (2)", idx)
	}
}

// TestFocusNowPlayingLocatesTimelineRow verifies that focusing the current
// track lands on its row in the effective order, not on its queue index.
func TestFocusNowPlayingLocatesTimelineRow(t *testing.T) {
	v := makeQueue(120, entriesTitled("A", "B", "C", "D"))
	v.Traversal = shuffledTimeline(2, 3, 1, 0, 2)

	v = v.FocusNowPlaying(0)
	if v.Cursor != 2 {
		t.Errorf("cursor = %d, want timeline row 2 for queue index 0", v.Cursor)
	}
	if idx := v.FocusedEntryIndex(); idx != 0 {
		t.Errorf("focused entry index = %d, want 0", idx)
	}
}

// TestScrollToNowPlayingUsesTimelineRow verifies the follow-scroll centres the
// current row by its timeline position.
func TestScrollToNowPlayingUsesTimelineRow(t *testing.T) {
	const n = 100
	titles := make([]string, n)
	order := make([]int, n)
	for i := range titles {
		titles[i] = fmt.Sprintf("t%02d", i)
		order[i] = n - 1 - i // the timeline runs the queue backwards
	}
	v := makeQueue(120, entriesTitled(titles...))
	v.Height = 20 // 17 visible rows
	v.Traversal = shuffledTimeline(50, order...)

	// Queue index 49 sits at timeline row 50.
	v = v.ScrollToNowPlaying(49)
	if want := 50 - v.VisibleRows()/3; v.ScrollOffset != want {
		t.Errorf("scroll offset = %d, want %d", v.ScrollOffset, want)
	}
}

// TestEntryIndexTranslationFollowsTimeline verifies the display↔underlying
// translation helpers round-trip through the effective order.
func TestEntryIndexTranslationFollowsTimeline(t *testing.T) {
	v := makeQueue(120, entriesTitled("A", "B", "C", "D"))
	order := []int{2, 0, 3, 1}
	v.Traversal = shuffledTimeline(0, order...)

	for displayIdx, entryIdx := range order {
		if got := v.EntryIndex(displayIdx); got != entryIdx {
			t.Errorf("EntryIndex(%d) = %d, want %d", displayIdx, got, entryIdx)
		}
		if got := v.FindEntryDisplayPosition(entryIdx); got != displayIdx {
			t.Errorf("FindEntryDisplayPosition(%d) = %d, want %d", entryIdx, got, displayIdx)
		}
		if got := v.FindEntryFilteredPosition(entryIdx); got != displayIdx {
			t.Errorf("FindEntryFilteredPosition(%d) = %d, want %d", entryIdx, got, displayIdx)
		}
	}
}

// TestSelectAndDequeueKeyedByOrigIdxUnderTimeline verifies that acting on a row
// addresses the underlying queue row, never the display position.
func TestSelectAndDequeueKeyedByOrigIdxUnderTimeline(t *testing.T) {
	v := makeQueue(120, entriesTitled("A", "B", "C", "D"))
	v.Traversal = shuffledTimeline(0, 3, 1, 0, 2)

	v.Cursor = 0
	_, cmd := v.HandleAction(keys.ActionDequeue)
	msg, ok := cmd().(DequeueTrackMsg)
	if !ok {
		t.Fatalf("expected DequeueTrackMsg, got %T", cmd())
	}
	if msg.Index != 3 {
		t.Errorf("dequeue index = %d, want underlying row 3", msg.Index)
	}

	v.Cursor = 2
	_, cmd = v.HandleAction(keys.ActionBulkSelectionToggle)
	sel, ok := cmd().(BulkSelectionToggleMsg)
	if !ok {
		t.Fatalf("expected BulkSelectionToggleMsg, got %T", cmd())
	}
	if sel.Index != 0 {
		t.Errorf("selection index = %d, want underlying row 0", sel.Index)
	}
}

// TestReorderRefusedWhileTimelineActive verifies that position-based edits
// refuse while display rows no longer line up with queue rows.
func TestReorderRefusedWhileTimelineActive(t *testing.T) {
	v := makeQueue(120, entriesTitled("A", "B", "C"))
	v.Traversal = shuffledTimeline(0, 2, 0, 1)
	v.Cursor = 1

	if !v.ReorderBlocked() {
		t.Error("ReorderBlocked must be true while the timeline orders the rows")
	}
	_, cmd := v.HandleAction(keys.ActionMoveTrackUp)
	if _, ok := cmd().(statusbar.TransientNotifyMsg); !ok {
		t.Errorf("move up must be refused, got %T", cmd())
	}
}

// TestReorderAllowedWithoutTimeline verifies the guard does not fire for a
// player without shuffle traversal, keeping reordering available as before.
func TestReorderAllowedWithoutTimeline(t *testing.T) {
	v := makeQueue(120, entriesTitled("A", "B", "C"))
	v.Traversal = sequentialTimeline(0, 3)
	v.Cursor = 1

	if v.ReorderBlocked() {
		t.Fatal("ReorderBlocked must be false with shuffle off")
	}
	var cmd tea.Cmd
	if _, cmd = v.HandleAction(keys.ActionMoveTrackUp); cmd == nil {
		t.Fatal("move up must produce a command")
	}
	msg, ok := cmd().(MoveTrackUpMsg)
	if !ok {
		t.Fatalf("expected MoveTrackUpMsg, got %T", cmd())
	}
	if msg.Index != 1 {
		t.Errorf("move index = %d, want 1", msg.Index)
	}
}
