package player

import (
	"errors"
	"os"
	"slices"
	"sync"
	"testing"
)

// assertTraversal compares a snapshot against the timeline the traversal state
// should describe.
func assertTraversal(t *testing.T, got QueueTraversal, order []int, pos, remaining int, shuffled bool) {
	t.Helper()
	if len(got.RemainingIndices) != got.Remaining {
		t.Fatalf("remaining indices disagree with count: %+v", got)
	}
	if !slices.Equal(got.Order, order) || got.CurrentPosition != pos ||
		got.Remaining != remaining || got.Shuffled != shuffled {
		t.Fatalf("traversal = {Order:%v CurrentPosition:%d Remaining:%d Shuffled:%v}, want {Order:%v CurrentPosition:%d Remaining:%d Shuffled:%v}",
			got.Order, got.CurrentPosition, got.Remaining, got.Shuffled,
			order, pos, remaining, shuffled)
	}
}

// assertTraversalEqual fails the test unless two snapshots describe the same
// timeline.
func assertTraversalEqual(t *testing.T, got, want QueueTraversal) {
	t.Helper()
	assertTraversal(t, got, want.Order, want.CurrentPosition, want.Remaining, want.Shuffled)
}

// TestQueueTraversal_SequentialSnapshot verifies the shuffle-off snapshot:
// visible queue order, the current index as the position, and everything after
// it as remaining — including the startup case where nothing plays yet.
func TestQueueTraversal_SequentialSnapshot(t *testing.T) {
	var mu sync.Mutex
	var received []ipcTestCommand
	m := newConnectedManager(t, "trav-seq", &mu, &received)

	// Empty queue before anything is loaded.
	assertTraversal(t, m.QueueTraversal(), []int{}, -1, 0, false)

	m.mu.Lock()
	m.queue = shuffleTestQueue(t, 4)
	m.currentIndex = -1
	m.mu.Unlock()

	// Startup: nothing is current yet, so the whole queue is still ahead.
	assertTraversal(t, m.QueueTraversal(), []int{0, 1, 2, 3}, -1, 4, false)

	m.SetCurrentIndex(1)
	assertTraversal(t, m.QueueTraversal(), []int{0, 1, 2, 3}, 1, 2, false)

	m.SetCurrentIndex(3)
	assertTraversal(t, m.QueueTraversal(), []int{0, 1, 2, 3}, 3, 0, false)
}

// TestQueueTraversal_ShuffleFollowsTraversal walks a shuffle cycle through
// anchoring, Next, Previous, forward-history replay, and exhaustion, asserting
// the snapshot matches the timeline at every step. Order stays fixed while the
// cursor moves: Previous does not unschedule the forward history it left behind.
func TestQueueTraversal_ShuffleFollowsTraversal(t *testing.T) {
	var mu sync.Mutex
	var received []ipcTestCommand
	m := newConnectedManager(t, "trav-shuf", &mu, &received)

	m.mu.Lock()
	m.queue = shuffleTestQueue(t, 4)
	m.currentIndex = 0
	m.mu.Unlock()

	m.SetPermuteFunc(reversePermute) // unseen over [1,2,3] -> 3,2,1
	if err := m.SetShuffle(true); err != nil {
		t.Fatalf("SetShuffle(true): %v", err)
	}

	timeline := []int{0, 3, 2, 1}
	assertTraversal(t, m.QueueTraversal(), timeline, 0, 3, true)

	mustNext := func() {
		t.Helper()
		if err := m.Next(); err != nil {
			t.Fatalf("Next: %v", err)
		}
	}
	mustNext() // -> 3
	assertTraversal(t, m.QueueTraversal(), timeline, 1, 2, true)
	mustNext() // -> 2
	assertTraversal(t, m.QueueTraversal(), timeline, 2, 1, true)

	if err := m.Previous(); err != nil {
		t.Fatalf("Previous: %v", err)
	}
	// Index 2 is now forward history: still scheduled, still counted.
	assertTraversal(t, m.QueueTraversal(), timeline, 1, 2, true)

	mustNext() // replays forward history -> 2
	assertTraversal(t, m.QueueTraversal(), timeline, 2, 1, true)
	mustNext() // consumes the last unseen entry -> 1
	assertTraversal(t, m.QueueTraversal(), timeline, 3, 0, true)

	if err := m.Next(); !errors.Is(err, ErrPlaybackFailed) {
		t.Fatalf("Next after exhaustion: want ErrPlaybackFailed, got %v", err)
	}
	// An exhausted cycle leaves the timeline intact with nothing remaining.
	assertTraversal(t, m.QueueTraversal(), timeline, 3, 0, true)

	if err := m.SetShuffle(false); err != nil {
		t.Fatalf("SetShuffle(false): %v", err)
	}
	assertTraversal(t, m.QueueTraversal(), []int{0, 1, 2, 3}, 1, 2, false)
}

// TestQueueTraversal_NoCurrentEntryUnderShuffle verifies that a shuffle cycle
// built with nothing playing reports no position and counts every scheduled row
// as remaining.
func TestQueueTraversal_NoCurrentEntryUnderShuffle(t *testing.T) {
	var mu sync.Mutex
	var received []ipcTestCommand
	m := newConnectedManager(t, "trav-nocur", &mu, &received)

	m.mu.Lock()
	m.queue = shuffleTestQueue(t, 3)
	m.currentIndex = -1
	m.mu.Unlock()

	if err := m.SetShuffle(true); err != nil {
		t.Fatalf("SetShuffle(true): %v", err)
	}
	assertTraversal(t, m.QueueTraversal(), []int{0, 1, 2}, -1, 3, true)
}

// TestQueueTraversal_SurvivesQueueMutations verifies that appends, removals, and
// visible reordering are reflected as index changes without disturbing the
// traversal itself: identities keep their place in the timeline and the cursor
// keeps naming the audible entry even when its visible index moves.
func TestQueueTraversal_SurvivesQueueMutations(t *testing.T) {
	var mu sync.Mutex
	var received []ipcTestCommand
	m := newConnectedManager(t, "trav-mutate", &mu, &received)

	q := shuffleTestQueue(t, 5)
	m.mu.Lock()
	m.queue = append([]QueueTrack(nil), q[:4]...)
	m.currentIndex = 0
	m.mu.Unlock()

	m.SetPermuteFunc(reversePermute) // unseen over [1,2,3] -> 3,2,1
	if err := m.SetShuffle(true); err != nil {
		t.Fatalf("SetShuffle(true): %v", err)
	}
	if err := m.Next(); err != nil { // -> 3
		t.Fatalf("Next: %v", err)
	}
	assertTraversal(t, m.QueueTraversal(), []int{0, 3, 2, 1}, 1, 2, true)

	// Append: the new identity joins the unseen pool at the end.
	m.AppendToQueue(q[4])
	assertTraversal(t, m.QueueTraversal(), []int{0, 3, 2, 1, 4}, 1, 3, true)

	// Remove the unplayed row at visible index 2; the surviving identities keep
	// their traversal order at their new indices.
	if err := m.RemoveFromQueue(2); err != nil {
		t.Fatalf("RemoveFromQueue(2): %v", err)
	}
	assertTraversal(t, m.QueueTraversal(), []int{0, 2, 1, 3}, 1, 2, true)

	// Reorder the visible queue (reverse it). The timeline is unchanged; only
	// the indices it resolves to move, and the cursor still names the audible
	// identity even though currentIndex was not updated by the reorder.
	m.mu.RLock()
	reversed := make([]QueueTrack, 0, len(m.queue))
	for i := len(m.queue) - 1; i >= 0; i-- {
		reversed = append(reversed, m.queue[i])
	}
	m.mu.RUnlock()
	m.SetQueue(reversed)
	assertTraversal(t, m.QueueTraversal(), []int{3, 1, 2, 0}, 1, 2, true)
}

// TestQueueTraversal_ExcludesSkippedMissingRows verifies that entries the cycle
// skipped as missing stay visible at the end of the order without being counted
// as remaining, that an unrelated append does not reschedule them, and that a
// fresh cycle gives them another chance.
func TestQueueTraversal_ExcludesSkippedMissingRows(t *testing.T) {
	var mu sync.Mutex
	var received []ipcTestCommand
	m := newConnectedManager(t, "trav-missing", &mu, &received)

	dir := t.TempDir()
	anchor := dir + "/anchor.flac"
	playable := dir + "/playable.flac"
	added := dir + "/added.flac"
	for _, p := range []string{anchor, playable, added} {
		if err := os.WriteFile(p, []byte{}, 0o600); err != nil {
			t.Fatalf("write temp file: %v", err)
		}
	}

	m.mu.Lock()
	m.queue = []QueueTrack{
		{Path: anchor, Hash: "h0", QueueID: 1},
		{Path: dir + "/gone-1.flac", Hash: "h1", QueueID: 2},
		{Path: dir + "/gone-2.flac", Hash: "h2", QueueID: 3},
		{Path: playable, Hash: "h3", QueueID: 4},
	}
	m.currentIndex = 0
	m.mu.Unlock()
	m.SetMissingMarker(&countingMarker{})

	if err := m.SetShuffle(true); err != nil { // identity order: unseen [1,2,3]
		t.Fatalf("SetShuffle(true): %v", err)
	}
	if err := m.Next(); err != nil { // skips 1 and 2, plays 3
		t.Fatalf("Next: %v", err)
	}
	// The two skipped rows are parked at the end: visible, not remaining.
	assertTraversal(t, m.QueueTraversal(), []int{0, 3, 1, 2}, 1, 0, true)

	m.mu.RLock()
	excluded := len(m.shuffleExcluded)
	m.mu.RUnlock()
	if excluded != 2 {
		t.Fatalf("excluded identities = %d, want 2", excluded)
	}

	// An unrelated append must not resurrect the skipped rows: only the new
	// identity becomes eligible.
	m.AppendToQueue(QueueTrack{Path: added, Hash: "h4", QueueID: 5})
	assertTraversal(t, m.QueueTraversal(), []int{0, 3, 4, 1, 2}, 1, 1, true)

	// A genuinely fresh cycle re-permutes everything, exclusions included: a
	// file that was missing may well be back.
	if err := m.RestartQueueCycle(); err != nil {
		t.Fatalf("RestartQueueCycle: %v", err)
	}
	assertTraversal(t, m.QueueTraversal(), []int{0, 1, 2, 3, 4}, 0, 4, true)
	m.mu.RLock()
	excluded = len(m.shuffleExcluded)
	m.mu.RUnlock()
	if excluded != 0 {
		t.Fatalf("after a cycle restart, excluded identities = %d, want 0", excluded)
	}
}

// TestQueueTraversal_DuplicateAndZeroIDIdentities verifies that rows sharing a
// hash and path resolve to their own occurrence: duplicates with distinct queue
// IDs and zero-ID rows (tests, and rows not yet persisted) each hold one slot in
// the order.
func TestQueueTraversal_DuplicateAndZeroIDIdentities(t *testing.T) {
	var mu sync.Mutex
	var received []ipcTestCommand
	m := newConnectedManager(t, "trav-dup", &mu, &received)

	dir := t.TempDir()
	dup := dir + "/dup.flac"
	other := dir + "/other.flac"
	zero := dir + "/zero.flac"
	for _, p := range []string{dup, other, zero} {
		if err := os.WriteFile(p, []byte{}, 0o600); err != nil {
			t.Fatalf("write temp file: %v", err)
		}
	}

	m.mu.Lock()
	m.queue = []QueueTrack{
		{Path: dup, Hash: "dup", QueueID: 1},
		{Path: other, Hash: "other", QueueID: 2},
		{Path: dup, Hash: "dup", QueueID: 3}, // same track, distinct queue row
		{Path: zero, Hash: "zero"},           // zero-ID fallback, occurrence 0
		{Path: zero, Hash: "zero"},           // zero-ID fallback, occurrence 1
	}
	// Anchor on the *second* copy of the duplicate track.
	m.currentIndex = 2
	m.mu.Unlock()

	if err := m.SetShuffle(true); err != nil {
		t.Fatalf("SetShuffle(true): %v", err)
	}
	// The anchor resolves to index 2, not to the first row with the same
	// hash/path, and every other row is scheduled exactly once.
	assertTraversal(t, m.QueueTraversal(), []int{2, 0, 1, 3, 4}, 0, 4, true)

	visited := map[int]int{currentIdx(m): 1}
	for range 4 {
		if err := m.Next(); err != nil {
			t.Fatalf("Next: %v", err)
		}
		visited[currentIdx(m)]++
	}
	if len(visited) != 5 {
		t.Fatalf("visited %d distinct rows, want 5: %v", len(visited), visited)
	}
	assertTraversal(t, m.QueueTraversal(), []int{2, 0, 1, 3, 4}, 4, 0, true)
}
