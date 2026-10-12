package player

import (
	"context"
	"sync"
	"testing"
	"time"
)

// This file covers what happens *after* a shuffle-off resume steps back onto a
// row that was inserted behind an exhausted cursor. The resume itself is pinned
// by mpv_queue_continuation_test.go; here the traversal keeps running, and the
// question is which row each natural end-file transition picks. Positional
// arithmetic from the insertion's new index answers with rows that finished
// before the insertion existed, through the gapless prefetch mpv promotes on its
// own and through the fallback advance behind it.

// seqEOFHarness drives a shuffle-off queue through natural end-file transitions
// against a fake mpv that records every command it is sent.
type seqEOFHarness struct {
	t        *testing.T
	m        *Manager
	events   chan<- string
	mu       sync.Mutex
	received []ipcTestCommand
	ended    chan struct{}
	// extra holds playable tracks that are not in the queue yet, for the test to
	// insert.
	extra []QueueTrack
}

// newSeqEOFHarness returns a harness whose three-row queue has been played
// straight through by natural end-file transitions and is now exhausted, plus
// three further playable tracks. Gapless is on, so each transition is the
// promotion mpv makes by itself rather than a replacement.
func newSeqEOFHarness(t *testing.T, name string) *seqEOFHarness {
	t.Helper()
	h := &seqEOFHarness{t: t, ended: make(chan struct{}, 8)}

	sock := shortSockPath(t, name)
	h.events = fakeEventCommandServer(t, sock, &h.mu, &h.received)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	h.m = &Manager{
		socketPath:      sock,
		ctx:             ctx,
		cancel:          cancel,
		callbacksReady:  make(chan struct{}),
		prefetchedIndex: -1,
		gaplessEnabled:  true,
		callbacks: EventCallbacks{
			OnTrackChanged:  func(_, _, _, _ string, _ int, _ int64) {},
			OnPlaybackEnded: func() { h.signalEnded() },
			OnStatus:        func(string) {},
			OnMissingFile:   func(string) {},
		},
	}
	t.Cleanup(func() { h.m.Close() })
	h.m.SetMissingMarker(&countingMarker{})

	client, err := NewIPCClient(sock)
	if err != nil {
		t.Fatalf("NewIPCClient: %v", err)
	}
	h.m.mu.Lock()
	h.m.client = client
	h.m.mu.Unlock()
	close(h.m.callbacksReady)

	go h.m.startEventReader(ctx, nil)
	waitForCommand(t, &h.mu, &h.received, func(cmd ipcTestCommand) bool {
		return cmd.stringIs(0, cmdObserveProperty) && cmd.stringIs(2, propPause)
	}, 2*time.Second, "pause property observation")

	q := shuffleTestQueue(t, 6)
	h.extra = q[3:]
	h.m.SetQueue(append([]QueueTrack(nil), q[:3]...))
	if err := h.m.PlayFromIndex(0); err != nil {
		t.Fatalf("PlayFromIndex(0): %v", err)
	}
	h.waitPrefetch(q[1].Path)
	h.endFile()
	h.waitCurrentIndex(1)
	h.waitPrefetch(q[2].Path)
	h.endFile()
	h.waitCurrentIndex(2)
	h.endFile()
	h.waitEnded("the initial queue to run out")
	h.reset()
	return h
}

func (h *seqEOFHarness) signalEnded() {
	select {
	case h.ended <- struct{}{}:
	default:
	}
}

// reset discards the recorded commands and any pending exhaustion signal, so
// every assertion after it describes only what the test did next.
func (h *seqEOFHarness) reset() {
	h.mu.Lock()
	h.received = nil
	h.mu.Unlock()
	for {
		select {
		case <-h.ended:
		default:
			return
		}
	}
}

// endFile reports that mpv finished the file it was playing.
func (h *seqEOFHarness) endFile() {
	h.t.Helper()
	select {
	case h.events <- `{"event":"end-file","reason":"eof"}`:
	case <-time.After(2 * time.Second):
		h.t.Fatal("timed out delivering an end-file event")
	}
}

func (h *seqEOFHarness) waitCurrentIndex(want int) {
	h.t.Helper()
	waitForCurrentIndex(h.t, h.m, want)
}

func (h *seqEOFHarness) waitPrefetch(path string) {
	h.t.Helper()
	waitForCommand(h.t, &h.mu, &h.received, loadfileFor(path, "append-play"),
		2*time.Second, "the gapless prefetch of "+path)
}

func (h *seqEOFHarness) waitEnded(desc string) {
	h.t.Helper()
	select {
	case <-h.ended:
	case <-time.After(2 * time.Second):
		h.t.Fatalf("timed out waiting for %s to report the queue exhausted", desc)
	}
}

// loads counts every loadfile of path, whatever its mode: a completed row is
// replayed just as surely by a gapless append as by a replacement.
func (h *seqEOFHarness) loads(path string) int {
	return countCommands(&h.mu, &h.received, func(cmd ipcTestCommand) bool {
		return len(cmd) >= 2 && cmd.stringIs(0, cmdLoadfile) && cmd.stringIs(1, path)
	})
}

// assertNoReplays fails when any of the given rows was handed to mpv again.
func (h *seqEOFHarness) assertNoReplays(completed ...QueueTrack) {
	h.t.Helper()
	for i, entry := range completed {
		if got := h.loads(entry.Path); got != 0 {
			h.t.Fatalf("completed row %d was loaded %d times after the resume, want 0", i, got)
		}
	}
}

// queueSnapshot returns a copy of the manager's live queue.
func (h *seqEOFHarness) queueSnapshot() []QueueTrack {
	h.m.mu.RLock()
	defer h.m.mu.RUnlock()
	return append([]QueueTrack(nil), h.m.queue...)
}

// insertAt splices extra into the live queue at pos and installs the result the
// way a paste or a "play next" does, through SetQueue.
func (h *seqEOFHarness) insertAt(pos int, extra ...QueueTrack) {
	h.t.Helper()
	h.m.SetQueue(insertBefore(h.m, pos, extra...))
}

// removeAt deletes the queue row at pos the way the Queue view's delete does.
func (h *seqEOFHarness) removeAt(pos int) {
	h.t.Helper()
	if err := h.m.RemoveFromQueue(pos); err != nil {
		h.t.Fatalf("RemoveFromQueue(%d): %v", pos, err)
	}
}

// replaceRowAt swaps the queue row at pos for entry and installs the result
// through SetQueue, which is what a paste over an existing row does: the old row
// leaves the queue without any row being deleted.
func (h *seqEOFHarness) replaceRowAt(pos int, entry QueueTrack) {
	h.t.Helper()
	q := h.queueSnapshot()
	q[pos] = entry
	h.m.SetQueue(q)
}

// moveRow lifts the queue row at from and reinstalls the queue with it at to,
// which is what a reorder in the Queue view does: no row enters or leaves the
// queue, so the moved row keeps the identity it already had.
func (h *seqEOFHarness) moveRow(from, to int) {
	h.t.Helper()
	q := h.queueSnapshot()
	row := q[from]
	rest := make([]QueueTrack, 0, len(q)-1)
	rest = append(rest, q[:from]...)
	rest = append(rest, q[from+1:]...)
	out := make([]QueueTrack, 0, len(q))
	out = append(out, rest[:to]...)
	out = append(out, row)
	out = append(out, rest[to:]...)
	h.m.SetQueue(out)
}

// waitLoads blocks until path has been handed to mpv at least n times, whatever
// the mode. A queue mutation clears the tracked successor and rebuilds it, so
// this is how a test waits for the prefetch that describes the queue *after* the
// mutation rather than reading the one from before it.
func (h *seqEOFHarness) waitLoads(path string, n int) {
	h.t.Helper()
	waitForCommandCount(h.t, &h.mu, &h.received, func(cmd ipcTestCommand) bool {
		return len(cmd) >= 2 && cmd.stringIs(0, cmdLoadfile) && cmd.stringIs(1, path)
	}, n, 2*time.Second, "loadfile "+path)
}

// resume continues the traversal and asserts it landed on wantIdx.
func (h *seqEOFHarness) resume(wantIdx int) {
	h.t.Helper()
	idx, err := h.m.ResumeAfterQueueExtension()
	if err != nil {
		h.t.Fatalf("ResumeAfterQueueExtension: %v", err)
	}
	if idx != wantIdx {
		h.t.Fatalf("resume returned %d, want %d", idx, wantIdx)
	}
}

// TestSequentialInsertionResumeDoesNotReplayCompletedRowsAtNextEOF is the
// regression for the frontier a resume has to preserve.
//
// Resuming an exhausted shuffle-off queue into a row inserted behind the cursor
// moves that cursor backwards, and every row the traversal already played is
// then a numeric successor of it. The insertion is the only new thing eligible,
// so the transitions after it must reach the remaining insertions and then stop
// — through the gapless successor mpv promotes on its own and through the
// fallback advance when there is none.
func TestSequentialInsertionResumeDoesNotReplayCompletedRowsAtNextEOF(t *testing.T) {
	t.Run("one head insertion ends the queue instead of replaying it", func(t *testing.T) {
		h := newSeqEOFHarness(t, "frontier-one")
		completed := h.queueSnapshot()

		h.insertAt(0, h.extra[0])
		h.reset()
		h.resume(0)

		// Nothing is left to prefetch: every other row played before the
		// insertion existed, so index 1 is not a successor.
		assertTraversal(t, h.m.QueueTraversal(), []int{0, 1, 2, 3}, 0, 0, false)
		h.endFile()
		h.waitEnded("the single insertion")
		h.assertNoReplays(completed...)
		if got := h.loads(h.extra[0].Path); got != 1 {
			t.Fatalf("the insertion was loaded %d times, want 1", got)
		}
	})

	t.Run("two head insertions play in order and then end", func(t *testing.T) {
		h := newSeqEOFHarness(t, "frontier-two")
		completed := h.queueSnapshot()

		h.insertAt(0, h.extra[0], h.extra[1])
		h.reset()
		h.resume(0)

		// The second insertion is the successor; the completed rows behind it
		// are not, even though they sit at lower indices than it does.
		assertTraversal(t, h.m.QueueTraversal(), []int{0, 1, 2, 3, 4}, 0, 1, false)
		h.waitPrefetch(h.extra[1].Path)
		h.endFile()
		h.waitCurrentIndex(1)

		assertTraversal(t, h.m.QueueTraversal(), []int{0, 1, 2, 3, 4}, 1, 0, false)
		h.endFile()
		h.waitEnded("the second insertion")
		h.assertNoReplays(completed...)
	})

	t.Run("two middle insertions are visited without the rows between them", func(t *testing.T) {
		h := newSeqEOFHarness(t, "frontier-mid")
		completed := h.queueSnapshot()

		// A "play next" on an exhausted queue inserts ahead of rows that already
		// played; two of them straddle a completed row.
		h.m.SetQueue([]QueueTrack{completed[0], h.extra[0], completed[1], h.extra[1], completed[2]})
		h.reset()
		h.resume(1)

		h.waitPrefetch(h.extra[1].Path)
		h.endFile()
		h.waitCurrentIndex(3)

		// Index 4 is the row the traversal finished on before the insertion, so
		// the queue is done rather than one row short of done.
		assertTraversal(t, h.m.QueueTraversal(), []int{0, 1, 2, 3, 4}, 3, 0, false)
		h.endFile()
		h.waitEnded("the second middle insertion")
		h.assertNoReplays(completed...)
	})

	t.Run("a tail append after the resume is still reached", func(t *testing.T) {
		h := newSeqEOFHarness(t, "frontier-tail")
		completed := h.queueSnapshot()

		h.insertAt(0, h.extra[0])
		h.reset()
		h.resume(0)

		// The frontier is what the append lands behind: the successor is the new
		// tail row, not the rows sitting between the cursor and the frontier.
		h.insertAt(4, h.extra[1])
		h.waitPrefetch(h.extra[1].Path)
		h.endFile()
		h.waitCurrentIndex(4)

		h.endFile()
		h.waitEnded("the tail append")
		h.assertNoReplays(completed...)
	})

	t.Run("an insertion missing from disk is skipped, not replaced by a completed row", func(t *testing.T) {
		h := newSeqEOFHarness(t, "frontier-gone")
		completed := h.queueSnapshot()

		h.insertAt(0, h.extra[0], missingTrack(t, 99), h.extra[1])
		h.reset()
		h.resume(0)

		// The prefetch steps over the missing row to the third insertion rather
		// than falling back onto index 1.
		h.waitPrefetch(h.extra[1].Path)
		h.endFile()
		h.waitCurrentIndex(2)

		h.endFile()
		h.waitEnded("the last playable insertion")
		h.assertNoReplays(completed...)
	})

	t.Run("the fallback advance honours the frontier when gapless is off", func(t *testing.T) {
		h := newSeqEOFHarness(t, "frontier-nogap")
		completed := h.queueSnapshot()
		if err := h.m.SetGapless(false); err != nil {
			t.Fatalf("SetGapless(false): %v", err)
		}

		h.insertAt(0, h.extra[0], h.extra[1])
		h.reset()
		h.resume(0)

		// With no prefetch to promote, the end-file goes through the traversal
		// advance instead; it must pick the same successor.
		h.endFile()
		h.waitCurrentIndex(1)
		if got := h.loads(h.extra[1].Path); got != 1 {
			t.Fatalf("the second insertion was loaded %d times, want 1", got)
		}

		h.endFile()
		h.waitEnded("the second insertion without gapless")
		h.assertNoReplays(completed...)
	})

	t.Run("an explicit selection retires the frontier", func(t *testing.T) {
		h := newSeqEOFHarness(t, "frontier-select")

		h.insertAt(0, h.extra[0])
		h.reset()
		h.resume(0)

		// Picking a row is not a continuation: "play the queue forward from
		// here" means the rows after it, whatever the traversal had reached.
		if err := h.m.PlayFromIndex(1); err != nil {
			t.Fatalf("PlayFromIndex(1): %v", err)
		}
		assertTraversal(t, h.m.QueueTraversal(), []int{0, 1, 2, 3}, 1, 2, false)
		h.endFile()
		h.waitCurrentIndex(2)
	})
}
