package player

import (
	"errors"
	"sync"
	"testing"
)

// shuffledRefreshManager returns a manager playing row 1 of a five-row queue
// under shuffle, with the unseen pool fixed at rows 5, 4, 3, 2 by the reversed
// permutation. It is the starting point for the substitution tests below.
func shuffledRefreshManager(t *testing.T, name string) (*Manager, []QueueTrack) {
	t.Helper()
	var mu sync.Mutex
	var received []ipcTestCommand
	m := newConnectedManager(t, name, &mu, &received)

	q := refreshQueue(t, 1, 2, 3, 4, 5)
	m.mu.Lock()
	m.queue = q
	m.currentIndex = 0
	m.mu.Unlock()
	m.SetMissingMarker(&countingMarker{})
	m.SetPermuteFunc(reversePermute)
	if err := m.SetShuffle(true); err != nil {
		t.Fatalf("SetShuffle(true): %v", err)
	}
	assertEligible(t, m, 5, 4, 3, 2)
	return m, q
}

// TestApplyQueueRefresh_SubstitutesShuffleSlotsInPlace is the core guarantee: a
// successor inherits the turn the cycle had already given its predecessor.
//
// Reconciling the same change through SetQueue would drop rows 4 and 2 and
// append rows 10 and 11 behind everything still scheduled, silently reordering a
// cycle the refresh never claimed. Here the pool keeps its shape.
func TestApplyQueueRefresh_SubstitutesShuffleSlotsInPlace(t *testing.T) {
	m, q := shuffledRefreshManager(t, "ref-sub")
	fresh := refreshQueue(t, 10, 11)

	snap := m.CaptureQueueRefresh()
	plan := buildRefresh(snap, q, []refreshSwap{swap(4, fresh[0]), swap(2, fresh[1])})
	res, err := m.ApplyQueueRefresh(plan)
	if err != nil {
		t.Fatalf("ApplyQueueRefresh: %v", err)
	}
	if res.Replaced != 2 || res.Retired != 0 || res.Added != 0 {
		t.Fatalf("result = %+v, want 2 replaced, 0 retired, 0 added", res)
	}
	if !res.PrefetchInvalidated {
		t.Fatal("a refresh that changed the upcoming rows must invalidate the prefetch")
	}

	// Rows 5 and 3 keep their turn; the successors take the slots 4 and 2 held.
	assertEligible(t, m, 5, 10, 3, 11)
	if got := currentQueueID(m); got != 1 {
		t.Fatalf("current row = %d, want 1 (the refresh must not move playback)", got)
	}
	if res.CurrentIndex != 0 {
		t.Fatalf("result current index = %d, want 0", res.CurrentIndex)
	}

	// The traversal actually visits the substituted order.
	for _, want := range []int64{5, 10, 3, 11} {
		if err := m.Next(); err != nil {
			t.Fatalf("Next: %v", err)
		}
		if got := currentQueueID(m); got != want {
			t.Fatalf("Next played row %d, want %d", got, want)
		}
	}
	if err := m.Next(); !errors.Is(err, ErrPlaybackFailed) {
		t.Fatalf("Next after the refreshed cycle: want ErrPlaybackFailed, got %v", err)
	}
}

// TestApplyQueueRefresh_PreservesPlayedAndForwardHistory verifies that a refresh
// taken mid-cycle leaves both directions of history intact: the rows already
// heard stay played, and the forward history a Previous left behind is still
// replayed by the next Next, ahead of anything the refresh introduced.
func TestApplyQueueRefresh_PreservesPlayedAndForwardHistory(t *testing.T) {
	m, q := shuffledRefreshManager(t, "ref-hist")
	fresh := refreshQueue(t, 10)

	for range 2 { // -> 5, -> 4
		if err := m.Next(); err != nil {
			t.Fatalf("Next: %v", err)
		}
	}
	if err := m.Previous(); err != nil { // back to 5, leaving 4 ahead
		t.Fatalf("Previous: %v", err)
	}
	snap := m.CaptureQueueRefresh()
	assertSnapshot(t, snap, 5, []int64{1}, []int64{4}, []int64{3, 2}, nil)

	plan := buildRefresh(snap, q, []refreshSwap{swap(2, fresh[0])})
	if _, err := m.ApplyQueueRefresh(plan); err != nil {
		t.Fatalf("ApplyQueueRefresh: %v", err)
	}
	after := m.CaptureQueueRefresh()
	assertSnapshot(t, after, 5, []int64{1}, []int64{4}, []int64{3, 10}, nil)

	// The forward history is replayed first, exactly as it would have been.
	for _, want := range []int64{4, 3, 10} {
		if err := m.Next(); err != nil {
			t.Fatalf("Next: %v", err)
		}
		if got := currentQueueID(m); got != want {
			t.Fatalf("Next played row %d, want %d", got, want)
		}
	}
	// Previous still walks the whole history back, refresh included.
	for _, want := range []int64{3, 4, 5, 1} {
		if err := m.Previous(); err != nil {
			t.Fatalf("Previous: %v", err)
		}
		if got := currentQueueID(m); got != want {
			t.Fatalf("Previous played row %d, want %d", got, want)
		}
	}
}

// TestApplyQueueRefresh_RetiredSlotsExhaustTheCycle verifies that a plan which
// came back with fewer tracks than it removed drops those slots outright, and
// that a cycle left with nothing upcoming reports exhaustion rather than
// replaying history.
func TestApplyQueueRefresh_RetiredSlotsExhaustTheCycle(t *testing.T) {
	m, q := shuffledRefreshManager(t, "ref-retire")

	snap := m.CaptureQueueRefresh()
	plan := buildRefresh(snap, q, []refreshSwap{retire(5), retire(4), retire(3), retire(2)})
	res, err := m.ApplyQueueRefresh(plan)
	if err != nil {
		t.Fatalf("ApplyQueueRefresh: %v", err)
	}
	if res.Replaced != 0 || res.Retired != 4 {
		t.Fatalf("result = %+v, want 0 replaced, 4 retired", res)
	}
	assertSnapshot(t, m.CaptureQueueRefresh(), 1, nil, nil, nil, nil)
	if got := len(managerQueue(m)); got != 1 {
		t.Fatalf("queue length = %d, want 1", got)
	}
	if err := m.Next(); !errors.Is(err, ErrPlaybackFailed) {
		t.Fatalf("Next on an emptied cycle: want ErrPlaybackFailed, got %v", err)
	}
	// The row that is playing survived the emptying and is still current.
	if got := currentQueueID(m); got != 1 {
		t.Fatalf("current row = %d, want 1", got)
	}
}

// TestApplyQueueRefresh_DuplicateHashesDistinctRowIDs verifies that a refresh
// keyed by row identity replaces exactly one occurrence of a track the queue
// holds twice, leaving the other copy where the cycle put it.
func TestApplyQueueRefresh_DuplicateHashesDistinctRowIDs(t *testing.T) {
	var mu sync.Mutex
	var received []ipcTestCommand
	m := newConnectedManager(t, "ref-dupapply", &mu, &received)

	base := refreshQueue(t, 1, 2, 3)
	dupA := QueueTrack{Path: base[1].Path, Hash: base[1].Hash, QueueID: 2}
	dupB := QueueTrack{Path: base[1].Path, Hash: base[1].Hash, QueueID: 4}
	q := []QueueTrack{base[0], dupA, base[2], dupB}
	m.mu.Lock()
	m.queue = q
	m.currentIndex = 0
	m.mu.Unlock()
	m.SetMissingMarker(&countingMarker{})
	m.SetPermuteFunc(reversePermute)
	if err := m.SetShuffle(true); err != nil {
		t.Fatalf("SetShuffle(true): %v", err)
	}
	assertEligible(t, m, 4, 3, 2)

	fresh := refreshQueue(t, 10)
	snap := m.CaptureQueueRefresh()
	plan := buildRefresh(snap, q, []refreshSwap{swap(4, fresh[0])})
	if _, err := m.ApplyQueueRefresh(plan); err != nil {
		t.Fatalf("ApplyQueueRefresh: %v", err)
	}
	// Only the row-4 occurrence went; the row-2 copy of the same track stayed in
	// its own slot.
	assertEligible(t, m, 10, 3, 2)
	for _, row := range managerQueue(m) {
		if row.QueueID == 4 {
			t.Fatal("the replaced occurrence is still in the queue")
		}
	}
}

// TestApplyQueueRefresh_SequentialKeepsManualInsertsAndOrder verifies the
// shuffle-off half: a row pasted behind an exhausted cursor stays first in line
// after a refresh, the refreshed rows land behind it, and the manual rows keep
// their visible order.
func TestApplyQueueRefresh_SequentialKeepsManualInsertsAndOrder(t *testing.T) {
	var mu sync.Mutex
	var received []ipcTestCommand
	m := newConnectedManager(t, "ref-seqapply", &mu, &received)

	q := refreshQueue(t, 1, 2, 3, 9)
	m.SetMissingMarker(&countingMarker{})
	m.SetQueue(q[:3])
	if err := m.PlayFromIndex(2); err != nil {
		t.Fatalf("PlayFromIndex(2): %v", err)
	}
	// Paste row 9 in front of the row that finished, then refresh.
	pasted := []QueueTrack{q[0], q[1], q[3], q[2]}
	m.SetQueue(pasted)
	snap := m.CaptureQueueRefresh()
	assertSnapshot(t, snap, 3, []int64{1, 2}, nil, []int64{9}, nil)

	fresh := refreshQueue(t, 10, 11)
	res, err := m.ApplyQueueRefresh(buildRefresh(snap, pasted, nil, fresh...))
	if err != nil {
		t.Fatalf("ApplyQueueRefresh: %v", err)
	}
	if res.Added != 2 || res.Replaced != 0 {
		t.Fatalf("result = %+v, want 2 added, 0 replaced", res)
	}
	// The pasted row is still owed to the traversal and still comes first.
	assertSnapshot(t, m.CaptureQueueRefresh(), 3, []int64{1, 2}, nil, []int64{9, 10, 11}, nil)

	idx, err := m.ResumeAfterQueueExtension()
	if err != nil {
		t.Fatalf("ResumeAfterQueueExtension: %v", err)
	}
	if got := managerQueue(m)[idx].QueueID; got != 9 {
		t.Fatalf("resume played row %d, want the pasted row 9", got)
	}
	// Having played the insertion, only the refreshed rows are left.
	assertSnapshot(t, m.CaptureQueueRefresh(), 9, []int64{1, 2, 3}, nil, []int64{10, 11}, nil)
}

// TestApplyQueueRefresh_SequentialReplacesUpcomingOnly verifies that a
// shuffle-off refresh takes over the rows the positional scan has not reached
// and leaves the played prefix and the current row alone.
func TestApplyQueueRefresh_SequentialReplacesUpcomingOnly(t *testing.T) {
	var mu sync.Mutex
	var received []ipcTestCommand
	m := newConnectedManager(t, "ref-sequp", &mu, &received)

	q := refreshQueue(t, 1, 2, 3, 4)
	m.SetMissingMarker(&countingMarker{})
	m.SetQueue(q)
	if err := m.PlayFromIndex(1); err != nil {
		t.Fatalf("PlayFromIndex(1): %v", err)
	}
	snap := m.CaptureQueueRefresh()
	assertSnapshot(t, snap, 2, []int64{1}, nil, []int64{3, 4}, nil)

	fresh := refreshQueue(t, 10)
	plan := buildRefresh(snap, q, []refreshSwap{swap(3, fresh[0]), retire(4)})
	if _, err := m.ApplyQueueRefresh(plan); err != nil {
		t.Fatalf("ApplyQueueRefresh: %v", err)
	}
	assertSnapshot(t, m.CaptureQueueRefresh(), 2, []int64{1}, nil, []int64{10}, nil)
	if err := m.Next(); err != nil {
		t.Fatalf("Next: %v", err)
	}
	if got := currentQueueID(m); got != 10 {
		t.Fatalf("Next played row %d, want the refreshed row 10", got)
	}
}

// TestApplyQueueRefresh_RejectsStaleSnapshot covers the guard: anything that
// moves the queue or playback between the capture and the apply invalidates the
// plan, because it was chosen against upcoming rows that are no longer upcoming.
func TestApplyQueueRefresh_RejectsStaleSnapshot(t *testing.T) {
	tests := []struct {
		name    string
		disturb func(t *testing.T, m *Manager, q []QueueTrack)
	}{
		{"playback advanced", func(t *testing.T, m *Manager, _ []QueueTrack) {
			if err := m.Next(); err != nil {
				t.Fatalf("Next: %v", err)
			}
		}},
		{"row appended", func(_ *testing.T, m *Manager, _ []QueueTrack) {
			m.AppendToQueue(QueueTrack{Path: "/dev/null", Hash: "hx", QueueID: 99})
		}},
		{"row removed", func(t *testing.T, m *Manager, _ []QueueTrack) {
			if err := m.RemoveFromQueue(3); err != nil {
				t.Fatalf("RemoveFromQueue(3): %v", err)
			}
		}},
		{"queue reordered", func(_ *testing.T, m *Manager, q []QueueTrack) {
			m.SetQueue([]QueueTrack{q[0], q[2], q[1], q[3], q[4]})
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m, q := shuffledRefreshManager(t, "ref-stale")
			fresh := refreshQueue(t, 10)
			snap := m.CaptureQueueRefresh()
			plan := buildRefresh(snap, q, []refreshSwap{swap(2, fresh[0])})

			tc.disturb(t, m, q)
			before := m.CaptureQueueRefresh()

			if _, err := m.ApplyQueueRefresh(plan); !errors.Is(err, ErrRefreshStale) {
				t.Fatalf("ApplyQueueRefresh on a stale snapshot: want ErrRefreshStale, got %v", err)
			}
			// A rejected plan publishes nothing.
			assertRefreshClass(t, "eligible after rejection", m.CaptureQueueRefresh().Eligible, rowIDs(before.Eligible))
			for _, row := range managerQueue(m) {
				if row.QueueID == 10 {
					t.Fatal("a rejected refresh published one of its rows")
				}
			}
		})
	}
}
