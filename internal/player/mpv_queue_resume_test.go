package player

import (
	"errors"
	"slices"
	"sync"
	"testing"
)

// exhaustedShuffleCycle returns a connected manager whose four-entry shuffle
// cycle has been played to exhaustion, together with two further playable tracks
// a test can append to it.
func exhaustedShuffleCycle(t *testing.T, name string, mu *sync.Mutex, received *[]ipcTestCommand) (*Manager, []QueueTrack) {
	t.Helper()
	m := newConnectedManager(t, name, mu, received)

	q := shuffleTestQueue(t, 6)
	m.mu.Lock()
	m.queue = append([]QueueTrack(nil), q[:4]...)
	m.currentIndex = 0
	m.mu.Unlock()
	m.SetMissingMarker(&countingMarker{})

	if err := m.SetShuffle(true); err != nil {
		t.Fatalf("SetShuffle(true): %v", err)
	}
	for range 3 {
		if err := m.Next(); err != nil {
			t.Fatalf("Next: %v", err)
		}
	}
	if err := m.Next(); !errors.Is(err, ErrPlaybackFailed) {
		t.Fatalf("Next after a full cycle: want ErrPlaybackFailed, got %v", err)
	}
	return m, q[4:]
}

// missingTrack returns a queue entry whose file does not exist.
func missingTrack(t *testing.T, queueID int64) QueueTrack {
	t.Helper()
	return QueueTrack{Path: t.TempDir() + "/gone.flac", Hash: "hx", QueueID: queueID}
}

// TestResumeAfterQueueExtension_ShufflePlaysNewEntries proves that resuming an
// exhausted shuffle cycle plays the newly eligible row, skips additions missing
// on disk, returns the row it actually played, and leaves the completed cycle in
// place instead of reanchoring.
func TestResumeAfterQueueExtension_ShufflePlaysNewEntries(t *testing.T) {
	var mu sync.Mutex
	var received []ipcTestCommand
	m, extra := exhaustedShuffleCycle(t, "resume-shuf", &mu, &received)

	// Nothing to resume into yet.
	if _, err := m.ResumeAfterQueueExtension(); !errors.Is(err, ErrPlaybackFailed) {
		t.Fatalf("resume without an extension: want ErrPlaybackFailed, got %v", err)
	}
	completed := readTraversalState(m)
	if len(completed.history) != 4 {
		t.Fatalf("completed cycle history = %v, want 4 entries", completed.history)
	}

	// Extend the queue with a row missing on disk followed by a playable one.
	m.AppendToQueue(missingTrack(t, 99))
	m.AppendToQueue(extra[0])

	idx, err := m.ResumeAfterQueueExtension()
	if err != nil {
		t.Fatalf("ResumeAfterQueueExtension: %v", err)
	}
	if idx != 5 || currentIdx(m) != 5 {
		t.Fatalf("resume returned %d (currentIndex %d), want 5 (the playable addition)", idx, currentIdx(m))
	}

	// The completed cycle is preserved: the played history still leads, with the
	// resumed entry appended after it. The missing addition was dropped.
	resumed := readTraversalState(m)
	if len(resumed.history) != 5 || !slices.Equal(resumed.history[:4], completed.history) {
		t.Fatalf("resume rewrote the completed cycle: %v, want %v plus one entry", resumed.history, completed.history)
	}
	// Order: the completed cycle, the resumed entry, then the skipped addition
	// parked at the end with nothing remaining.
	assertTraversal(t, m.QueueTraversal(), []int{0, 1, 2, 3, 5, 4}, 4, 0, true)
}

// TestResumeAfterQueueExtension_DoesNotRescheduleSkippedAdditions proves that a
// later extension makes only the new row eligible: reconciling it must not
// resurrect an addition an earlier resume skipped as missing, and once the new
// row is played the cycle is exhausted again.
func TestResumeAfterQueueExtension_DoesNotRescheduleSkippedAdditions(t *testing.T) {
	var mu sync.Mutex
	var received []ipcTestCommand
	m, extra := exhaustedShuffleCycle(t, "resume-reschedule", &mu, &received)

	m.AppendToQueue(missingTrack(t, 99))
	m.AppendToQueue(extra[0])
	if _, err := m.ResumeAfterQueueExtension(); err != nil {
		t.Fatalf("ResumeAfterQueueExtension: %v", err)
	}

	m.AppendToQueue(extra[1])
	assertTraversal(t, m.QueueTraversal(), []int{0, 1, 2, 3, 5, 6, 4}, 4, 1, true)

	idx, err := m.ResumeAfterQueueExtension()
	if err != nil {
		t.Fatalf("second ResumeAfterQueueExtension: %v", err)
	}
	if idx != 6 {
		t.Fatalf("second resume returned index %d, want 6", idx)
	}
	if _, err := m.ResumeAfterQueueExtension(); !errors.Is(err, ErrPlaybackFailed) {
		t.Fatalf("resume with nothing new: want ErrPlaybackFailed, got %v", err)
	}

	// No row played twice across the whole run.
	final := readTraversalState(m)
	if len(final.history) != 6 {
		t.Fatalf("final history = %v, want 6 entries", final.history)
	}
	seen := map[string]bool{}
	for _, id := range final.history {
		if seen[id] {
			t.Fatalf("identity %q played twice: %v", id, final.history)
		}
		seen[id] = true
	}
}

// TestResumeAfterQueueExtension_SequentialPlaysAppendedRows verifies the
// shuffle-off path: resuming plays the entry after the current index, skipping
// additions missing on disk, and reports the index it landed on.
func TestResumeAfterQueueExtension_SequentialPlaysAppendedRows(t *testing.T) {
	var mu sync.Mutex
	var received []ipcTestCommand
	m := newConnectedManager(t, "resume-seq", &mu, &received)

	q := shuffleTestQueue(t, 6)
	m.mu.Lock()
	m.queue = append([]QueueTrack(nil), q[:4]...)
	m.currentIndex = 3 // played through to the end of the queue
	m.mu.Unlock()
	m.SetMissingMarker(&countingMarker{})

	if _, err := m.ResumeAfterQueueExtension(); !errors.Is(err, ErrPlaybackFailed) {
		t.Fatalf("resume without an extension: want ErrPlaybackFailed, got %v", err)
	}

	m.AppendToQueue(q[4])
	idx, err := m.ResumeAfterQueueExtension()
	if err != nil {
		t.Fatalf("ResumeAfterQueueExtension: %v", err)
	}
	if idx != 4 || currentIdx(m) != 4 {
		t.Fatalf("resume returned %d (currentIndex %d), want 4", idx, currentIdx(m))
	}

	// A missing addition is skipped in favour of the next playable one, and the
	// returned index is the row that actually played.
	m.AppendToQueue(QueueTrack{Path: t.TempDir() + "/gone.flac", Hash: "hx", QueueID: 99})
	m.AppendToQueue(q[5])
	idx, err = m.ResumeAfterQueueExtension()
	if err != nil {
		t.Fatalf("ResumeAfterQueueExtension over a missing addition: %v", err)
	}
	if idx != 6 || currentIdx(m) != 6 {
		t.Fatalf("resume returned %d (currentIndex %d), want 6", idx, currentIdx(m))
	}
	assertTraversal(t, m.QueueTraversal(), []int{0, 1, 2, 3, 4, 5, 6}, 6, 0, false)
}

// exhaustedSequentialQueue returns a connected manager with shuffle off whose
// three-row queue has been played through to its last row and is now exhausted,
// together with three further tracks a test can insert into it. The queue is
// installed through SetQueue, so the manager holds the same identities a later
// SetQueue insertion has to reconcile against.
func exhaustedSequentialQueue(t *testing.T, name string, mu *sync.Mutex, received *[]ipcTestCommand) (*Manager, []QueueTrack) {
	t.Helper()
	m := newConnectedManager(t, name, mu, received)
	m.SetMissingMarker(&countingMarker{})

	q := shuffleTestQueue(t, 6)
	m.SetQueue(append([]QueueTrack(nil), q[:3]...))
	if err := m.PlayFromIndex(0); err != nil {
		t.Fatalf("PlayFromIndex(0): %v", err)
	}
	for range 2 {
		if err := m.Next(); err != nil {
			t.Fatalf("Next: %v", err)
		}
	}
	if got := currentIdx(m); got != 2 {
		t.Fatalf("after playing through, currentIndex = %d, want 2", got)
	}
	if err := m.Next(); !errors.Is(err, ErrPlaybackFailed) {
		t.Fatalf("Next past the last row: want ErrPlaybackFailed, got %v", err)
	}
	return m, q[3:]
}

// queueEntryAt reads the manager's queue row at index under the lock.
func queueEntryAt(m *Manager, index int) QueueTrack {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.queue[index]
}

// insertBefore returns the queue that results from splicing extra into the
// manager's current queue at index pos.
func insertBefore(m *Manager, pos int, extra ...QueueTrack) []QueueTrack {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]QueueTrack, 0, len(m.queue)+len(extra))
	out = append(out, m.queue[:pos]...)
	out = append(out, extra...)
	out = append(out, m.queue[pos:]...)
	return out
}

// TestResumeAfterQueueExtension_SequentialInsertionBeforeCompletedRow covers the
// shuffle-off extension that is not an append: a paste or a "play next" inserts
// at or before the row an exhausted traversal stopped on, so numeric
// continuation (currentIndex+1) both misses the new row and lands on one that
// already played. The completed row must survive the insertion by identity and
// the resume must start the addition instead.
func TestResumeAfterQueueExtension_SequentialInsertionBeforeCompletedRow(t *testing.T) {
	t.Run("insertion at the head plays the new row", func(t *testing.T) {
		var mu sync.Mutex
		var received []ipcTestCommand
		m, extra := exhaustedSequentialQueue(t, "seq-ins-head", &mu, &received)
		completed := queueEntryAt(m, 2)

		m.SetQueue(insertBefore(m, 0, extra[0]))

		// The completed row moved with the insertion instead of being renamed to
		// whatever slid into its slot, and the addition reads as still ahead.
		if got := currentIdx(m); got != 3 {
			t.Fatalf("currentIndex = %d after inserting a row at the head, want 3 (the completed row followed)", got)
		}
		assertTraversal(t, m.QueueTraversal(), []int{0, 1, 2, 3}, 3, 1, false)

		mu.Lock()
		received = nil
		mu.Unlock()

		idx, err := m.ResumeAfterQueueExtension()
		if err != nil {
			t.Fatalf("ResumeAfterQueueExtension: %v", err)
		}
		if idx != 0 || currentIdx(m) != 0 {
			t.Fatalf("resume returned %d (currentIndex %d), want 0 (the inserted row)", idx, currentIdx(m))
		}
		if got := countCommands(&mu, &received, loadfileFor(extra[0].Path, "replace")); got != 1 {
			t.Fatalf("loadfile replace of the inserted row: got %d, want 1", got)
		}
		if got := countCommands(&mu, &received, loadfileFor(completed.Path, "replace")); got != 0 {
			t.Fatalf("the completed row was replayed %d times, want 0", got)
		}
	})

	t.Run("insertion into the middle plays the new row", func(t *testing.T) {
		var mu sync.Mutex
		var received []ipcTestCommand
		m, extra := exhaustedSequentialQueue(t, "seq-ins-mid", &mu, &received)
		completed := queueEntryAt(m, 2)

		// "Play next" on an exhausted queue inserts ahead of a row that already
		// played, which is still behind the cursor.
		m.SetQueue(insertBefore(m, 2, extra[0]))
		mu.Lock()
		received = nil
		mu.Unlock()

		idx, err := m.ResumeAfterQueueExtension()
		if err != nil {
			t.Fatalf("ResumeAfterQueueExtension: %v", err)
		}
		if idx != 2 || currentIdx(m) != 2 {
			t.Fatalf("resume returned %d (currentIndex %d), want 2 (the inserted row)", idx, currentIdx(m))
		}
		if got := countCommands(&mu, &received, loadfileFor(completed.Path, "replace")); got != 0 {
			t.Fatalf("the completed row was replayed %d times, want 0", got)
		}
	})

	t.Run("the earliest insertion wins and is played once", func(t *testing.T) {
		var mu sync.Mutex
		var received []ipcTestCommand
		m, extra := exhaustedSequentialQueue(t, "seq-ins-two", &mu, &received)

		m.SetQueue(insertBefore(m, 0, extra[0], extra[1]))
		if idx, err := m.ResumeAfterQueueExtension(); err != nil || idx != 0 {
			t.Fatalf("first resume = (%d, %v), want (0, nil)", idx, err)
		}
		// Playing the addition established a new cursor; the second insertion is
		// now the row right after it, which plain continuation reaches.
		idx, err := m.ResumeAfterQueueExtension()
		if err != nil {
			t.Fatalf("second ResumeAfterQueueExtension: %v", err)
		}
		if idx != 1 {
			t.Fatalf("second resume returned %d, want 1 (the other insertion)", idx)
		}
		if got := countCommands(&mu, &received, loadfileFor(extra[0].Path, "replace")); got != 1 {
			t.Fatalf("the first insertion was loaded %d times, want 1", got)
		}
	})

	t.Run("a missing insertion leaves the queue exhausted", func(t *testing.T) {
		var mu sync.Mutex
		var received []ipcTestCommand
		m, _ := exhaustedSequentialQueue(t, "seq-ins-gone", &mu, &received)
		completed := queueEntryAt(m, 2)

		m.SetQueue(insertBefore(m, 0, missingTrack(t, 99)))
		mu.Lock()
		received = nil
		mu.Unlock()

		if _, err := m.ResumeAfterQueueExtension(); !errors.Is(err, ErrPlaybackFailed) {
			t.Fatalf("resume into a missing insertion: want ErrPlaybackFailed, got %v", err)
		}
		if got := countCommands(&mu, &received, isLoadfileReplace); got != 0 {
			t.Fatalf("a missing insertion sent %d replacements, want 0", got)
		}
		if got := countCommands(&mu, &received, loadfileFor(completed.Path, "replace")); got != 0 {
			t.Fatalf("the completed row was replayed %d times, want 0", got)
		}
		if got := currentIdx(m); got != 3 {
			t.Fatalf("currentIndex = %d after a failed resume, want 3 (the completed row)", got)
		}
		// The row it gave up on is not retried when a later extension arrives.
		if _, err := m.ResumeAfterQueueExtension(); !errors.Is(err, ErrPlaybackFailed) {
			t.Fatalf("second resume with nothing new: want ErrPlaybackFailed, got %v", err)
		}
		if got := countCommands(&mu, &received, isLoadfileReplace); got != 0 {
			t.Fatalf("the missing insertion was retried: %d replacements, want 0", got)
		}
	})

	t.Run("a tail append still resumes positionally", func(t *testing.T) {
		var mu sync.Mutex
		var received []ipcTestCommand
		m, extra := exhaustedSequentialQueue(t, "seq-ins-tail", &mu, &received)

		m.SetQueue(insertBefore(m, 3, extra[0]))
		if got := currentIdx(m); got != 2 {
			t.Fatalf("currentIndex = %d after a tail append, want 2 (unmoved)", got)
		}
		idx, err := m.ResumeAfterQueueExtension()
		if err != nil {
			t.Fatalf("ResumeAfterQueueExtension: %v", err)
		}
		if idx != 3 || currentIdx(m) != 3 {
			t.Fatalf("resume returned %d (currentIndex %d), want 3 (the appended row)", idx, currentIdx(m))
		}
		assertTraversal(t, m.QueueTraversal(), []int{0, 1, 2, 3}, 3, 0, false)
	})

	t.Run("an insertion behind a live cursor is dropped once playback moves on", func(t *testing.T) {
		var mu sync.Mutex
		var received []ipcTestCommand
		m, extra := exhaustedSequentialQueue(t, "seq-ins-live", &mu, &received)

		// Start the queue over so the cursor is live with rows ahead of it, then
		// insert behind it — a paste above the playing row.
		if err := m.PlayFromIndex(0); err != nil {
			t.Fatalf("PlayFromIndex(0): %v", err)
		}
		m.SetQueue(insertBefore(m, 0, extra[0]))
		if got := currentIdx(m); got != 1 {
			t.Fatalf("currentIndex = %d after inserting above the playing row, want 1", got)
		}

		// Playing on past it puts the insertion behind the cursor for good:
		// positional traversal never goes back, so a later extension must resume
		// into the appended row rather than jumping backwards onto it.
		for range 2 {
			if err := m.Next(); err != nil {
				t.Fatalf("Next: %v", err)
			}
		}
		if err := m.Next(); !errors.Is(err, ErrPlaybackFailed) {
			t.Fatalf("Next past the last row: want ErrPlaybackFailed, got %v", err)
		}

		m.SetQueue(insertBefore(m, 4, extra[1]))
		idx, err := m.ResumeAfterQueueExtension()
		if err != nil {
			t.Fatalf("ResumeAfterQueueExtension: %v", err)
		}
		if idx != 4 {
			t.Fatalf("resume returned %d, want 4 (the appended row, not the insertion left behind)", idx)
		}
	})
}
