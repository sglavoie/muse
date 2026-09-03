package player

import (
	"testing"
	"time"
)

// These tests pin the boundary the gapless prefetch was missing: an mpv that is
// idle because playback intentionally ended is not an mpv waiting for the next
// track. Prefetch appends with `append-play`, which starts an idle playlist, so
// without a logical-liveness check any later queue mutation can turn a
// best-effort append into audible playback the user did not ask for.

// livenessOf reads the manager's logical playback liveness under the lock.
func livenessOf(m *Manager) playbackLiveness {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.liveness
}

// TestPrefetchDoesNotRestartAfterStopAfterCurrent covers the stop-after-current
// half of the finding. Once the flag has fired it is cleared and currentIndex
// still names the row that finished, so nothing in the prefetch's own inputs
// distinguishes "the user stopped playback" from "the next track is due" —
// while AppendToQueue happily schedules a prefetch into exactly that state.
func TestPrefetchDoesNotRestartAfterStopAfterCurrent(t *testing.T) {
	m, f := newFakePlaylistManager(t, "liveness-stop-after")
	queue := queueOf(t, 3)
	installQueue(m, queue[:2], -1, true)
	ended := playbackEndedSignal(m)
	runEventReader(t, m, f)

	if err := m.PlayFromIndex(0); err != nil {
		t.Fatalf("PlayFromIndex(0): %v", err)
	}
	f.waitForEntries(t, paths(queue, 0, 1))

	// Arming drops the prefetch through a playlist transaction of its own, so
	// mpv is left holding the current entry alone.
	m.SetStopAfterCurrent(true)
	f.waitForEntries(t, paths(queue, 0))

	f.finishCurrent()
	select {
	case <-ended:
	case <-time.After(2 * time.Second):
		t.Fatal("OnPlaybackEnded did not fire for stop-after-current")
	}

	// The preconditions the finding describes: mpv idle on purpose, the flag
	// already spent, and the traversal still positioned on the completed row.
	f.waitFor(t, "mpv to go idle after stop-after-current", f.idle)
	if m.StopAfterCurrent() {
		t.Fatal("stop-after-current is still armed; the EOF did not consume it")
	}
	if got := currentIdx(m); got != 0 {
		t.Fatalf("currentIndex = %d, want 0: the completed row is what makes the successor resolvable", got)
	}
	if got := livenessOf(m); got != livenessEnded {
		t.Fatalf("liveness = %d, want livenessEnded: the idle mpv is intentional", got)
	}

	appendsBefore := f.count(isLoadfileAppend)

	// An unrelated queue mutation. It schedules a prefetch of its own; run one
	// synchronously too so the assertion cannot pass merely by outrunning the
	// scheduled goroutine — both serialize on playlistMu.
	m.AppendToQueue(queue[2])
	m.prefetchNext()

	if got := f.count(isLoadfileAppend); got != appendsBefore {
		t.Fatalf("append-play count = %d, want %d: prefetch appended into a playlist that had intentionally stopped",
			got, appendsBefore)
	}
	if !f.idle() {
		t.Fatalf("mpv is playing %q after a queue append; stop-after-current was overridden", f.playingPath())
	}
	if got := f.entriesSnapshot(); len(got) != 1 || got[0] != queue[0].Path {
		t.Fatalf("playlist = %v, want just the completed entry %q", got, queue[0].Path)
	}

	// The guard suppresses best-effort work, not the user's next explicit
	// request: an explicit selection still restarts playback.
	if err := m.PlayFromIndex(2); err != nil {
		t.Fatalf("PlayFromIndex(2): %v", err)
	}
	f.waitFor(t, "the explicit selection to start playing", func() bool {
		return f.playingPath() == queue[2].Path
	})
	if got := livenessOf(m); got != livenessLive {
		t.Fatalf("liveness = %d, want livenessLive after an explicit selection", got)
	}
}

// TestResumeAfterQueueExtensionIsSoleExhaustionRestart covers the exhaustion
// half. A naturally exhausted traversal leaves mpv idle for the same reason,
// and the queue then grows — the Auto-DJ refill, a user append. Only the
// explicit continuation may pick the traversal back up; a prefetch racing it
// would start audio at an arbitrary moment and from an index nobody chose.
func TestResumeAfterQueueExtensionIsSoleExhaustionRestart(t *testing.T) {
	m, f := newFakePlaylistManager(t, "liveness-exhausted")
	queue := queueOf(t, 3)
	installQueue(m, queue[:2], -1, true)
	ended := playbackEndedSignal(m)
	runEventReader(t, m, f)

	// Start on the last row: there is no successor, so nothing is prefetched
	// and the EOF runs the exhaustion path rather than a gapless transition.
	if err := m.PlayFromIndex(1); err != nil {
		t.Fatalf("PlayFromIndex(1): %v", err)
	}
	f.waitForEntries(t, paths(queue, 1))

	f.finishCurrent()
	select {
	case <-ended:
	case <-time.After(2 * time.Second):
		t.Fatal("OnPlaybackEnded did not fire for an exhausted queue")
	}
	f.waitFor(t, "mpv to go idle after exhaustion", f.idle)
	if got := livenessOf(m); got != livenessEnded {
		t.Fatalf("liveness = %d, want livenessEnded: the traversal ran out", got)
	}

	appendsBefore := f.count(isLoadfileAppend)

	// Both mutations that grow an exhausted queue schedule a prefetch. Neither
	// may resume playback; each is followed by a synchronous prefetchNext so
	// the check does not depend on the scheduled goroutine having run.
	m.AppendToQueue(queue[2])
	m.prefetchNext()
	m.SetQueue(queue)
	m.prefetchNext()

	if got := f.count(isLoadfileAppend); got != appendsBefore {
		t.Fatalf("append-play count = %d, want %d: prefetch resumed an exhausted traversal", got, appendsBefore)
	}
	if !f.idle() {
		t.Fatalf("mpv is playing %q after growing an exhausted queue; only the explicit resume may restart it",
			f.playingPath())
	}
	if got := f.entriesSnapshot(); len(got) != 1 || got[0] != queue[1].Path {
		t.Fatalf("playlist = %v, want just the completed entry %q", got, queue[1].Path)
	}

	idx, err := m.ResumeAfterQueueExtension()
	if err != nil {
		t.Fatalf("ResumeAfterQueueExtension: %v", err)
	}
	if idx != 2 {
		t.Fatalf("ResumeAfterQueueExtension = %d, want 2: the entry the extension added", idx)
	}
	f.waitFor(t, "the resumed entry to start playing", func() bool {
		return f.playingPath() == queue[2].Path
	})
	if got := currentIdx(m); got != 2 {
		t.Fatalf("currentIndex = %d, want 2", got)
	}
	if got := livenessOf(m); got != livenessLive {
		t.Fatalf("liveness = %d, want livenessLive after the explicit resume", got)
	}
}
