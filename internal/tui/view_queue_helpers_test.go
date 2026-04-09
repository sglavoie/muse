package tui

import "testing"

// TestFormatQueueDuration covers the duration formatting helper.
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
		got := formatQueueDuration(tc.ms)
		if got != tc.want {
			t.Errorf("formatQueueDuration(%d) = %q, want %q", tc.ms, got, tc.want)
		}
	}
}

func TestFormatDurationSecs(t *testing.T) {
	cases := []struct {
		secs float64
		want string
	}{
		{0, "--:--"},
		{-5, "--:--"},
		{65, "1:05"},
		{185, "3:05"},
		{3661, "1:01:01"},
	}
	for _, tc := range cases {
		got := FormatDurationSecs(tc.secs)
		if got != tc.want {
			t.Errorf("FormatDurationSecs(%v) = %q, want %q", tc.secs, got, tc.want)
		}
	}
}

func TestQueueMoveQueueCursorEmpty(t *testing.T) {
	q := QueueView{cursor: 0}
	q2 := q.moveQueueCursor(5)
	if q2.cursor != 0 {
		t.Errorf("moveQueueCursor empty: expected 0, got %d", q2.cursor)
	}
}

func TestQueueVisibleRowsZeroHeight(t *testing.T) {
	q := QueueView{height: 0}
	if q.visibleRows() != 50 {
		t.Errorf("visibleRows height=0: expected 50, got %d", q.visibleRows())
	}
}

func TestQueueVisibleRowsSmallHeight(t *testing.T) {
	q := QueueView{height: 5}
	if q.visibleRows() != 3 {
		t.Errorf("visibleRows height=5: expected 3, got %d", q.visibleRows())
	}
}

// TestFilteredEntriesStaleDisplayOrder verifies that filteredEntries does not
// panic when displayOrder has a different length than entries (stale sort).
func TestFilteredEntriesStaleDisplayOrder(t *testing.T) {
	v := QueueView{
		entries: []QueueEntry{
			{Title: "A", Artist: "X"},
			{Title: "B", Artist: "Y"},
			{Title: "C", Artist: "Z"},
		},
		displayOrder: []int{1, 0},
	}
	got := v.filteredEntries()
	if len(got) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(got))
	}
	for i, e := range got {
		if e.origIdx != i {
			t.Errorf("entry %d: origIdx = %d, want %d", i, e.origIdx, i)
		}
	}
}

// TestFilteredEntriesOrigIdx verifies that origIdx is set correctly by
// filteredEntries with and without a displayOrder.
func TestFilteredEntriesOrigIdx(t *testing.T) {
	v := QueueView{
		entries: []QueueEntry{
			{Title: "A", Artist: "X"},
			{Title: "B", Artist: "Y"},
			{Title: "C", Artist: "Z"},
		},
	}
	got := v.filteredEntries()
	for i, e := range got {
		if e.origIdx != i {
			t.Errorf("unsorted entry %d: origIdx = %d, want %d", i, e.origIdx, i)
		}
	}

	v.displayOrder = []int{2, 1, 0}
	got = v.filteredEntries()
	if got[0].origIdx != 2 || got[1].origIdx != 1 || got[2].origIdx != 0 {
		t.Errorf("sorted origIdx: got %d,%d,%d; want 2,1,0",
			got[0].origIdx, got[1].origIdx, got[2].origIdx)
	}
}

// TestQueueVisibleRowsAccountsForFilter verifies that visibleRows reserves an
// extra line when the filter bar is active or has committed (non-empty) text.
func TestQueueVisibleRowsAccountsForFilter(t *testing.T) {
	q := QueueView{height: 20}
	baseRows := q.visibleRows()

	q.filter.Active = true
	if got := q.visibleRows(); got != baseRows-1 {
		t.Errorf("active filter: expected %d, got %d", baseRows-1, got)
	}

	q.filter.Active = false
	q.filter.Text = "hello"
	if got := q.visibleRows(); got != baseRows-1 {
		t.Errorf("committed filter text: expected %d, got %d", baseRows-1, got)
	}
}

// TestQueueVisibleRowsAccountsForSaveInput verifies that visibleRows reserves
// an extra line when the save-as-playlist input is active.
func TestQueueVisibleRowsAccountsForSaveInput(t *testing.T) {
	q := QueueView{height: 20}
	baseRows := q.visibleRows()

	q.savingAsPlaylist = true
	if got := q.visibleRows(); got != baseRows-1 {
		t.Errorf("save input active: expected %d, got %d", baseRows-1, got)
	}
}

// TestQueueVisibleRowsAccountsForBothFilterAndSave verifies that visibleRows
// reserves lines for both filter and save input simultaneously.
func TestQueueVisibleRowsAccountsForBothFilterAndSave(t *testing.T) {
	q := QueueView{height: 20}
	baseRows := q.visibleRows()

	q.filter.Active = true
	q.savingAsPlaylist = true
	if got := q.visibleRows(); got != baseRows-2 {
		t.Errorf("both filter and save: expected %d, got %d", baseRows-2, got)
	}
}