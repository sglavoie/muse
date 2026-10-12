package player

import (
	"sync"
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

// TestRestartPreservesEndedPlaybackLiveness pins the restart half of the same
// boundary. An mpv crash is process health; it says nothing about whether the
// user's traversal was still running. But every field the restart supervisor
// resumes from survives an intentional end untouched — EOF leaves lastTrackPath
// naming the row that finished, for stop-after-current and for a queue that ran
// out alike — so a supervisor that resumes whatever path it finds starts audio
// the user had stopped, and marks the traversal live again on the way.
func TestRestartPreservesEndedPlaybackLiveness(t *testing.T) {
	t.Run("natural exhaustion stays ended", func(t *testing.T) {
		m, f := newFakePlaylistManager(t, "rst-end-exh")
		queue := queueOf(t, 3)
		installQueue(m, queue[:2], -1, true)
		ended := playbackEndedSignal(m)
		runEventReader(t, m, f)

		// The last row has no successor, so nothing is prefetched and the EOF
		// runs the exhaustion path rather than a gapless transition.
		if err := m.PlayFromIndex(1); err != nil {
			t.Fatalf("PlayFromIndex(1): %v", err)
		}
		f.waitForEntries(t, paths(queue, 1))
		f.finishCurrent()
		awaitPlaybackEnded(t, ended, "an exhausted queue")

		assertRestartLeavesTraversalEnded(t, m, f, queue, 1)
	})

	t.Run("stop-after-current stays ended", func(t *testing.T) {
		m, f := newFakePlaylistManager(t, "rst-end-sac")
		queue := queueOf(t, 3)
		installQueue(m, queue[:2], -1, true)
		ended := playbackEndedSignal(m)
		runEventReader(t, m, f)

		if err := m.PlayFromIndex(0); err != nil {
			t.Fatalf("PlayFromIndex(0): %v", err)
		}
		f.waitForEntries(t, paths(queue, 0, 1))
		// Arming drops the prefetch through a transaction of its own, so mpv is
		// left holding the current entry alone.
		m.SetStopAfterCurrent(true)
		f.waitForEntries(t, paths(queue, 0))
		f.finishCurrent()
		awaitPlaybackEnded(t, ended, "stop-after-current")

		assertRestartLeavesTraversalEnded(t, m, f, queue, 0)
	})

	t.Run("a playing traversal still resumes unpaused", func(t *testing.T) {
		assertRestartResumesLiveTraversal(t, "rst-live-play", false, "pause=no")
	})

	t.Run("a paused traversal still resumes paused", func(t *testing.T) {
		assertRestartResumesLiveTraversal(t, "rst-live-pause", true, "pause=yes")
	})
}

// TestRestartDoesNotResumePendingNaturalExhaustion pins the overlap the tests
// above leave open: they wait for OnPlaybackEnded before restarting, so the
// traversal is already recorded as ended when recovery snapshots it.
//
// A natural end is not decided where the end-file is read. The advance that
// discovers the queue has run out is queued to a background goroutine and waits
// for the playlist transaction lock, and an event-socket failure is exactly what
// can start recovery inside that gap. The restart then supersedes the advance —
// it opens a new playlist generation, so the transaction is dropped without
// deciding anything — and a supervisor that read the traversal as still live
// would reload the track that had just played out, and mark the traversal live
// for it, with nothing left to contradict either.
//
// Holding playlistMu stands in for that delay: it parks the advance exactly
// where a slow transaction would be, without any dependence on timing.
func TestRestartDoesNotResumePendingNaturalExhaustion(t *testing.T) {
	m, f := newFakePlaylistManager(t, "rst-pend-exh")
	queue := queueOf(t, 3)
	installQueue(m, queue[:2], -1, true)
	runEventReader(t, m, f)

	// The last row has no successor, so the EOF takes the exhaustion path rather
	// than promoting a prefetch.
	if err := m.PlayFromIndex(1); err != nil {
		t.Fatalf("PlayFromIndex(1): %v", err)
	}
	f.waitForEntries(t, paths(queue, 1))

	// Park every playlist transaction from here on, the advance included.
	m.playlistMu.Lock()
	release := sync.OnceFunc(m.playlistMu.Unlock)
	defer release()

	f.finishCurrent()
	f.waitFor(t, "mpv to go idle on the natural end", f.idle)
	waitForLiveness(t, m, livenessEnding, "the dispatched end-file")

	// The TUI persists the position on a timer, so the row that just finished is
	// what the supervisor finds — the same resumable-looking path a confirmed end
	// leaves behind.
	m.SetCurrentPosition(queue[1].Path, 12.5)

	restore := m.snapshotRestore()
	if restore.path != queue[1].Path {
		t.Fatalf("restore.path = %q, want the completed row %q", restore.path, queue[1].Path)
	}
	if restore.liveness.live() {
		t.Fatalf("restore.liveness = %v, want a non-live state: the entry has played out whichever way the queued advance resolves",
			restore.liveness)
	}

	next := restartOnto(t, m, restore)
	assertNothingWasHandedToPlay(t, next)

	// Let the superseded advance run. Its transaction belongs to the playlist
	// generation the restart replaced, so it decides nothing — and the pending end
	// must not decay back into a live traversal because of that.
	release()
	waitForLiveness(t, m, livenessEnded, "the superseded exhaustion transaction")

	// Growing the queue on the fresh process schedules a best-effort prefetch,
	// which append-plays into an idle playlist. Run one synchronously too so the
	// assertions cannot pass merely by outrunning the scheduled goroutine.
	m.AppendToQueue(queue[2])
	m.prefetchNext()

	assertNothingWasHandedToPlay(t, next)
	if got := currentIdx(m); got != 1 {
		t.Fatalf("currentIndex = %d, want 1: the restart moved the traversal", got)
	}

	// The end is still liftable: the user's next explicit selection restarts
	// playback on the new process exactly as it would have without the outage.
	if err := m.PlayFromIndex(2); err != nil {
		t.Fatalf("PlayFromIndex(2) after the restart: %v", err)
	}
	next.waitFor(t, "the explicit selection to start playing", func() bool {
		return next.playingPath() == queue[2].Path
	})
	if got := livenessOf(m); got != livenessLive {
		t.Fatalf("liveness = %v, want live after an explicit selection", got)
	}
}
