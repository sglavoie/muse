package queue

import (
	"testing"

	"github.com/sglavoie/muse/internal/tui/text"
)

// TestFormatQueueDuration covers the duration formatting used by the queue panel.
func TestFormatQueueDuration(t *testing.T) {
	cases := []struct {
		ms   int64
		want string
	}{
		{0, "--:--"},
		{-1, "--:--"},
		{1_000, "0:01"},
		{60_000, "1:00"},
		{65_000, "1:05"},
		{3_600_000, "1:00:00"},
		{3_661_000, "1:01:01"},
		{5_999_000, "1:39:59"},
	}
	for _, tc := range cases {
		got := text.FormatDurationMs(tc.ms)
		if got != tc.want {
			t.Errorf("text.FormatDurationMs(%d) = %q, want %q", tc.ms, got, tc.want)
		}
	}
}

func TestQueueMoveQueueCursorEmpty(t *testing.T) {
	q := View{Cursor: 0}
	q2 := q.MoveQueueCursor(5)
	if q2.Cursor != 0 {
		t.Errorf("moveQueueCursor empty: expected 0, got %d", q2.Cursor)
	}
}

func TestQueueVisibleRowsZeroHeight(t *testing.T) {
	q := View{Height: 0}
	if q.VisibleRows() != 50 {
		t.Errorf("visibleRows height=0: expected 50, got %d", q.VisibleRows())
	}
}

func TestQueueVisibleRowsSmallHeight(t *testing.T) {
	q := View{Height: 5}
	// reserve = 3 (strip + column header + bottom margin)
	if q.VisibleRows() != 2 {
		t.Errorf("visibleRows height=5: expected 2, got %d", q.VisibleRows())
	}
}

// TestFilteredEntriesStaleDisplayOrder verifies that filteredEntries does not
// panic when displayOrder has a different length than entries (stale sort).
func TestFilteredEntriesStaleDisplayOrder(t *testing.T) {
	v := View{
		Entries: []Entry{
			{Title: "A", Artist: "X"},
			{Title: "B", Artist: "Y"},
			{Title: "C", Artist: "Z"},
		},
		DisplayOrder: []int{1, 0},
	}
	got := v.FilteredEntries()
	if len(got) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(got))
	}
	for i, e := range got {
		if e.OrigIdx != i {
			t.Errorf("entry %d: origIdx = %d, want %d", i, e.OrigIdx, i)
		}
	}
}

// TestFilteredEntriesInvalidDisplayOrder verifies that filteredEntries falls
// back to insertion order rather than panicking when a stale displayOrder has
// the right length but contains an invalid entry index.
func TestFilteredEntriesInvalidDisplayOrder(t *testing.T) {
	v := View{
		Entries: []Entry{
			{Title: "A", Artist: "X"},
			{Title: "B", Artist: "Y"},
			{Title: "C", Artist: "Z"},
		},
		DisplayOrder: []int{2, 3, 0},
	}
	got := v.FilteredEntries()
	if len(got) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(got))
	}
	for i, e := range got {
		if e.OrigIdx != i {
			t.Errorf("entry %d: origIdx = %d, want %d", i, e.OrigIdx, i)
		}
	}
}

// TestFilteredEntriesOrigIdx verifies that origIdx is set correctly by
// filteredEntries with and without a displayOrder.
func TestFilteredEntriesOrigIdx(t *testing.T) {
	v := View{
		Entries: []Entry{
			{Title: "A", Artist: "X"},
			{Title: "B", Artist: "Y"},
			{Title: "C", Artist: "Z"},
		},
	}
	got := v.FilteredEntries()
	for i, e := range got {
		if e.OrigIdx != i {
			t.Errorf("unsorted entry %d: origIdx = %d, want %d", i, e.OrigIdx, i)
		}
	}

	v.DisplayOrder = []int{2, 1, 0}
	got = v.FilteredEntries()
	if got[0].OrigIdx != 2 || got[1].OrigIdx != 1 || got[2].OrigIdx != 0 {
		t.Errorf("sorted origIdx: got %d,%d,%d; want 2,1,0",
			got[0].OrigIdx, got[1].OrigIdx, got[2].OrigIdx)
	}
}

// TestQueueVisibleRowsAbsorbsFilter verifies that the state strip absorbs the
// filter chunk, so visibleRows does not change with filter state.
func TestQueueVisibleRowsAbsorbsFilter(t *testing.T) {
	q := View{Height: 20}
	baseRows := q.VisibleRows()

	q.Filter.Active = true
	if got := q.VisibleRows(); got != baseRows {
		t.Errorf("active filter: expected %d (strip absorbs filter), got %d", baseRows, got)
	}

	q.Filter.Active = false
	q.Filter.Text = "hello"
	if got := q.VisibleRows(); got != baseRows {
		t.Errorf("committed filter text: expected %d (strip absorbs filter), got %d", baseRows, got)
	}
}

// TestQueueVisibleRowsAccountsForSaveInput verifies that visibleRows reserves
// an extra line when the save-as-playlist input is active.
func TestQueueVisibleRowsAccountsForSaveInput(t *testing.T) {
	q := View{Height: 20}
	baseRows := q.VisibleRows()

	q.SavingAsPlaylist = true
	if got := q.VisibleRows(); got != baseRows-1 {
		t.Errorf("save input active: expected %d, got %d", baseRows-1, got)
	}
}

// TestQueueVisibleRowsAccountsForBothFilterAndSave verifies that when both
// filter and the save-as-playlist input are active, only the save input affects
// visibleRows (the filter is absorbed by the state strip).
func TestQueueVisibleRowsAccountsForBothFilterAndSave(t *testing.T) {
	q := View{Height: 20}
	baseRows := q.VisibleRows()

	q.Filter.Active = true
	q.SavingAsPlaylist = true
	if got := q.VisibleRows(); got != baseRows-1 {
		t.Errorf("both filter and save: expected %d (only save reserves), got %d", baseRows-1, got)
	}
}
