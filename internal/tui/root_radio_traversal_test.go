package tui

import (
	"testing"

	"github.com/sglavoie/muse/internal/tui/filter"
	"github.com/sglavoie/muse/internal/tui/queue"
	"github.com/sglavoie/muse/internal/tui/sorting"
)

// radioTraversalModel returns a radio-enabled model over a six-row queue whose
// player traverses it in order, sitting at pos. visibleIdx is the persisted row
// the TUI believes is playing — the number the old low-water arithmetic used.
func radioTraversalModel(order []int, pos, visibleIdx int) (Model, *fakeTraversalPlayer) {
	p := newFakeTraversalPlayer(order, pos)
	m := newModelWithTracks()
	m.player = p
	m.batchEnqueuer = &stubBatchEnqueuer{}
	m.radio.enabled = true
	m.queue.Entries = make([]queue.Entry, len(order))
	for i := range m.queue.Entries {
		m.queue.Entries[i] = queue.Entry{ID: int64(i + 1), Hash: "h" + string(rune('1'+i)), Path: "/a.flac"}
	}
	m.playback.currentIndex = visibleIdx
	return m, p
}

// TestRadioLowWaterFollowsTraversalNotPersistedPosition drives the low-water
// decision from shuffle cursors parked at both ends of the persisted queue while
// the traversal's real remaining count sits above, at, and below the mark. The
// first two cases are the ones the visible-index arithmetic got wrong in
// opposite directions: a cursor near the head hides an almost-finished cycle, and
// a cursor near the tail invents one.
func TestRadioLowWaterFollowsTraversalNotPersistedPosition(t *testing.T) {
	tests := []struct {
		name         string
		order        []int
		pos          int
		visibleIdx   int
		wantRemain   int
		wantEnqueue  bool
		visibleWould bool // what the persisted-index arithmetic would have decided
	}{
		{
			name: "cursor at persisted head with one row left in the cycle",
			// The cycle ends on row 0, the row the user sees first.
			order: []int{1, 2, 3, 4, 0, 5}, pos: 4, visibleIdx: 0,
			wantRemain: 1, wantEnqueue: true, visibleWould: false,
		},
		{
			name: "cursor at persisted tail with most of the cycle left",
			// The cycle visits row 5 second, so five rows in the queue are behind
			// it in position but four are still ahead in traversal.
			order: []int{0, 5, 1, 2, 3, 4}, pos: 1, visibleIdx: 5,
			wantRemain: 4, wantEnqueue: false, visibleWould: true,
		},
		{
			name:  "remaining exactly at the low-water mark",
			order: []int{0, 1, 2, 5, 3, 4}, pos: 3, visibleIdx: 5,
			wantRemain: radioLowWaterMark, wantEnqueue: true, visibleWould: true,
		},
		{
			name:  "remaining one above the low-water mark",
			order: []int{1, 2, 0, 3, 4, 5}, pos: 2, visibleIdx: 0,
			wantRemain: radioLowWaterMark + 1, wantEnqueue: false, visibleWould: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m, _ := radioTraversalModel(tc.order, tc.pos, tc.visibleIdx)

			if got := m.radioRemainingCount(); got != tc.wantRemain {
				t.Fatalf("radioRemainingCount() = %d, want %d", got, tc.wantRemain)
			}
			gotEnqueue := m.radioMaybeEnqueueCmd() != nil
			if gotEnqueue != tc.wantEnqueue {
				t.Errorf("enqueue = %v, want %v", gotEnqueue, tc.wantEnqueue)
			}
			// The visible-index arithmetic is what this replaces; where the two
			// disagree, the traversal answer is the one that must win.
			visible := len(m.queue.Entries) - tc.visibleIdx - 1
			if (visible <= radioLowWaterMark) != tc.visibleWould {
				t.Fatalf("test setup: persisted-index arithmetic gives %d remaining, expected it to decide %v",
					visible, tc.visibleWould)
			}
		})
	}
}

// TestRadioRemainingIgnoresQueueViewState proves the count is a property of the
// player, not of the queue view: sorting and filtering reorder and hide rows
// without changing how much of the cycle is left.
func TestRadioRemainingIgnoresQueueViewState(t *testing.T) {
	m, _ := radioTraversalModel([]int{3, 1, 4, 0, 5, 2}, 2, 4)
	before := m.radioRemainingCount()
	if before != 3 {
		t.Fatalf("radioRemainingCount() = %d, want 3", before)
	}

	m.queue.SortState = sorting.SortState{Field: sorting.SortTitle, Dir: sorting.SortAsc}
	m.queue = m.queue.RecomputeDisplayOrder()
	m.queue.Filter = filter.State{Active: true, Text: "no-such-track"}

	if got := m.radioRemainingCount(); got != before {
		t.Errorf("radioRemainingCount() = %d after sorting and filtering, want %d", got, before)
	}
	if len(m.queue.FilteredEntries()) == len(m.queue.Entries) {
		t.Fatal("test setup: the filter did not hide anything")
	}
}

// TestRadioRemainingSequentialTraversal proves the same wiring carries the
// shuffle-off count: it still comes from the player, which skips rows the
// sequential walk gave up on.
func TestRadioRemainingSequentialTraversal(t *testing.T) {
	m, p := radioTraversalModel([]int{0, 1, 2, 3, 4, 5}, 2, 2)
	p.shuffled = false
	p.missing[4] = true // given up on: not something left to play

	if got := m.radioRemainingCount(); got != 2 {
		t.Errorf("radioRemainingCount() = %d, want 2 (rows 3 and 5)", got)
	}
	if m.radioMaybeEnqueueCmd() == nil {
		t.Error("expected an enqueue at the low-water mark with shuffle off")
	}
}

// TestRadioRemainingFallsBackToVisibleIndex proves players without the traversal
// snapshot keep the persisted-index arithmetic, including its out-of-range guard.
func TestRadioRemainingFallsBackToVisibleIndex(t *testing.T) {
	m := newModelWithTracks()
	m.player = &stubPlayer{}
	m.queue.Entries = make([]queue.Entry, 6)
	for i := range m.queue.Entries {
		m.queue.Entries[i] = queue.Entry{ID: int64(i + 1)}
	}

	cases := map[int]int{0: 5, 4: 1, 5: 0, -1: 6, 99: 6}
	for idx, want := range cases {
		m.playback.currentIndex = idx
		if got := m.radioRemainingCount(); got != want {
			t.Errorf("currentIndex %d: radioRemainingCount() = %d, want %d", idx, got, want)
		}
	}
}
