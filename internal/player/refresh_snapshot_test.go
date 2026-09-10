package player

import (
	"os"
	"sync"
	"testing"
)

// TestCaptureQueueRefresh_SequentialClassification verifies the shuffle-off
// classification at the three points a queue passes through: nothing playing
// yet, playback part-way, and an exhausted traversal with nothing left to give
// up.
func TestCaptureQueueRefresh_SequentialClassification(t *testing.T) {
	var mu sync.Mutex
	var received []ipcTestCommand
	m := newConnectedManager(t, "ref-seq", &mu, &received)

	q := refreshQueue(t, 1, 2, 3, 4)
	m.mu.Lock()
	m.queue = q
	m.currentIndex = -1
	m.mu.Unlock()

	// Startup: the whole queue is still ahead, so every row is replaceable.
	assertSnapshot(t, m.CaptureQueueRefresh(), 0, nil, nil, []int64{1, 2, 3, 4}, nil)

	if err := m.PlayFromIndex(1); err != nil {
		t.Fatalf("PlayFromIndex(1): %v", err)
	}
	assertSnapshot(t, m.CaptureQueueRefresh(), 2, []int64{1}, nil, []int64{3, 4}, nil)

	// Exhausted: the row that finished is still current, and nothing is left to
	// refresh.
	if err := m.PlayFromIndex(3); err != nil {
		t.Fatalf("PlayFromIndex(3): %v", err)
	}
	assertSnapshot(t, m.CaptureQueueRefresh(), 4, []int64{1, 2, 3}, nil, nil, nil)

	if snap := m.CaptureQueueRefresh(); snap.Shuffled {
		t.Fatal("snapshot reports shuffle with shuffle off")
	}
}

// TestCaptureQueueRefresh_ShuffleHistoryAndForwardHistory walks a shuffle cycle
// through Next and Previous and pins the classification at every step. The
// forward history a Previous leaves behind is upcoming but must never be
// eligible: it is a row the user navigated away from, not a slot the cycle is
// free to hand over.
func TestCaptureQueueRefresh_ShuffleHistoryAndForwardHistory(t *testing.T) {
	var mu sync.Mutex
	var received []ipcTestCommand
	m := newConnectedManager(t, "ref-shuf", &mu, &received)

	q := refreshQueue(t, 1, 2, 3, 4)
	m.SetQueue(q)
	m.SetCurrentIndex(0)
	m.SetPermuteFunc(reversePermute) // unseen over rows 2,3,4 -> 4,3,2
	if err := m.SetShuffle(true); err != nil {
		t.Fatalf("SetShuffle(true): %v", err)
	}

	snap := m.CaptureQueueRefresh()
	if !snap.Shuffled {
		t.Fatal("snapshot does not report shuffle traversal")
	}
	assertSnapshot(t, snap, 1, nil, nil, []int64{4, 3, 2}, nil)

	mustNext := func() {
		t.Helper()
		if err := m.Next(); err != nil {
			t.Fatalf("Next: %v", err)
		}
	}
	mustNext() // -> 4
	assertSnapshot(t, m.CaptureQueueRefresh(), 4, []int64{1}, nil, []int64{3, 2}, nil)
	mustNext() // -> 3
	assertSnapshot(t, m.CaptureQueueRefresh(), 3, []int64{1, 4}, nil, []int64{2}, nil)

	if err := m.Previous(); err != nil {
		t.Fatalf("Previous: %v", err)
	}
	// Row 3 is now forward history: ahead of the cursor, but protected.
	assertSnapshot(t, m.CaptureQueueRefresh(), 4, []int64{1}, []int64{3}, []int64{2}, nil)

	mustNext() // replays the forward history -> 3
	assertSnapshot(t, m.CaptureQueueRefresh(), 3, []int64{1, 4}, nil, []int64{2}, nil)
	mustNext() // consumes the last unseen row -> 2
	assertSnapshot(t, m.CaptureQueueRefresh(), 2, []int64{1, 4, 3}, nil, nil, nil)
}

// TestCaptureQueueRefresh_DuplicateHashesDistinctRows verifies that two queue
// rows holding the same track are classified independently: anchoring on the
// second copy leaves the first replaceable, and the identities that name them
// are distinct.
func TestCaptureQueueRefresh_DuplicateHashesDistinctRows(t *testing.T) {
	var mu sync.Mutex
	var received []ipcTestCommand
	m := newConnectedManager(t, "ref-dup", &mu, &received)

	dir := t.TempDir()
	dup := dir + "/dup.flac"
	other := dir + "/other.flac"
	for _, p := range []string{dup, other} {
		if err := os.WriteFile(p, []byte{}, 0o600); err != nil {
			t.Fatalf("write temp file: %v", err)
		}
	}
	m.SetQueue([]QueueTrack{
		{Path: dup, Hash: "dup", QueueID: 1},
		{Path: other, Hash: "other", QueueID: 2},
		{Path: dup, Hash: "dup", QueueID: 3}, // same track, distinct queue row
	})
	m.SetCurrentIndex(2)
	if err := m.SetShuffle(true); err != nil {
		t.Fatalf("SetShuffle(true): %v", err)
	}

	snap := m.CaptureQueueRefresh()
	assertSnapshot(t, snap, 3, nil, nil, []int64{1, 2}, nil)
	if snap.Current.Identity == snap.Eligible[0].Identity {
		t.Fatal("the two copies of the duplicate track share an identity")
	}
	if snap.Current.Hash != snap.Eligible[0].Hash {
		t.Fatalf("the duplicate rows should share a hash, got %q and %q",
			snap.Current.Hash, snap.Eligible[0].Hash)
	}
	if got := QueueRowIdentity(3); got != snap.Current.Identity {
		t.Fatalf("QueueRowIdentity(3) = %q, want %q", got, snap.Current.Identity)
	}
	if QueueRowIdentity(0) != "" {
		t.Fatal("an unpersisted row must have no reconstructable identity")
	}
}

// TestCaptureQueueRefresh_ManualInsertBehindExhaustedCursor verifies that a row
// pasted behind an exhausted shuffle-off cursor is reported as eligible. The
// positional scan cannot reach it, so a classification derived from the cursor
// alone would call the queue finished and hide a row the resume is going to
// play.
func TestCaptureQueueRefresh_ManualInsertBehindExhaustedCursor(t *testing.T) {
	var mu sync.Mutex
	var received []ipcTestCommand
	m := newConnectedManager(t, "ref-insert", &mu, &received)

	q := refreshQueue(t, 1, 2, 3, 9)
	m.SetQueue(q[:3])
	if err := m.PlayFromIndex(2); err != nil {
		t.Fatalf("PlayFromIndex(2): %v", err)
	}
	assertSnapshot(t, m.CaptureQueueRefresh(), 3, []int64{1, 2}, nil, nil, nil)

	// Paste row 9 in front of the row that finished.
	m.SetQueue([]QueueTrack{q[0], q[1], q[3], q[2]})
	assertSnapshot(t, m.CaptureQueueRefresh(), 3, []int64{1, 2}, nil, []int64{9}, nil)
}

// TestCaptureQueueRefresh_ParksRowsTheCycleGaveUp verifies that entries skipped
// as missing on disk are reported as unscheduled rather than eligible. The cycle
// is not going to visit them, so a refresh that swapped one in would schedule a
// slot nothing reaches.
func TestCaptureQueueRefresh_ParksRowsTheCycleGaveUp(t *testing.T) {
	var mu sync.Mutex
	var received []ipcTestCommand
	m := newConnectedManager(t, "ref-missing", &mu, &received)

	dir := t.TempDir()
	anchor := dir + "/anchor.flac"
	playable := dir + "/playable.flac"
	for _, p := range []string{anchor, playable} {
		if err := os.WriteFile(p, []byte{}, 0o600); err != nil {
			t.Fatalf("write temp file: %v", err)
		}
	}
	m.SetQueue([]QueueTrack{
		{Path: anchor, Hash: "h1", QueueID: 1},
		{Path: dir + "/gone.flac", Hash: "h2", QueueID: 2},
		{Path: playable, Hash: "h3", QueueID: 3},
	})
	m.SetCurrentIndex(0)
	m.SetMissingMarker(&countingMarker{})
	if err := m.SetShuffle(true); err != nil {
		t.Fatalf("SetShuffle(true): %v", err)
	}
	if err := m.Next(); err != nil { // skips row 2, plays row 3
		t.Fatalf("Next: %v", err)
	}
	assertSnapshot(t, m.CaptureQueueRefresh(), 3, []int64{1}, nil, nil, []int64{2})
}
