package player

import (
	"context"
	"os/exec"
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

// awaitPlaybackEnded blocks until the manager reports the traversal ended.
func awaitPlaybackEnded(t *testing.T, ended <-chan struct{}, desc string) {
	t.Helper()
	select {
	case <-ended:
	case <-time.After(2 * time.Second):
		t.Fatalf("OnPlaybackEnded did not fire for %s", desc)
	}
}

// restartOnto drives one full outage-recovery attempt and returns the fake
// playlist standing in for the replacement mpv process.
//
// attemptRestart reaps the old process, unlinks the socket, calls launchFn, and
// reconnects, so pointing launchFn at a second fakePlaylist on the same path
// runs the production recovery sequence end to end — including the resume hook
// activateConnection invokes before it drains the commands queued during the
// outage. launchFn is called inline on this goroutine, which is what makes the
// replacement fake safe to capture by closure.
func restartOnto(t *testing.T, m *Manager, restore restoreState) *fakePlaylist {
	t.Helper()
	sock := m.socketPath
	var next *fakePlaylist
	m.mu.Lock()
	m.launchFn = func(context.Context, string) (*exec.Cmd, error) {
		next = newFakePlaylistAt(t, sock)
		return nil, nil
	}
	m.mu.Unlock()

	if !m.attemptRestart(restore) {
		t.Fatal("attemptRestart did not reconnect to the replacement process")
	}
	if next == nil {
		t.Fatal("attemptRestart reconnected without launching a replacement process")
	}
	return next
}

// assertRestartLeavesTraversalEnded is the shared body of the two intentional
// ends: it restarts mpv from the state the end left behind and pins that the
// fresh process is handed nothing to play, that the end survives the restart,
// and that an explicit operation can still lift it afterwards.
//
// endedIdx is the queue row playback finished on — the row lastTrackPath still
// names, which is the finding's whole premise.
func assertRestartLeavesTraversalEnded(t *testing.T, m *Manager, f *fakePlaylist, queue []QueueTrack, endedIdx int) {
	t.Helper()
	f.waitFor(t, "mpv to go idle on the intentional end", f.idle)
	if got := livenessOf(m); got != livenessEnded {
		t.Fatalf("liveness = %d, want livenessEnded before the restart", got)
	}
	// The TUI persists the position on a timer, so the row that finished is
	// exactly what the supervisor finds when the process dies afterwards.
	m.SetCurrentPosition(queue[endedIdx].Path, 12.5)

	restore := m.snapshotRestore()
	if restore.path != queue[endedIdx].Path {
		t.Fatalf("restore.path = %q, want the completed row %q: the finding needs a resumable-looking path",
			restore.path, queue[endedIdx].Path)
	}
	if restore.liveness != livenessEnded {
		t.Fatalf("restore.liveness = %d, want livenessEnded: recovery must carry logical liveness, not just mpv state",
			restore.liveness)
	}

	next := restartOnto(t, m, restore)

	// Growing the queue on the fresh process schedules a best-effort prefetch,
	// which append-plays into an idle playlist. Run one synchronously too so the
	// assertions cannot pass merely by outrunning the scheduled goroutine — both
	// serialize on playlistMu.
	m.AppendToQueue(queue[2])
	m.prefetchNext()

	assertNothingWasHandedToPlay(t, next)
	if got := livenessOf(m); got != livenessEnded {
		t.Fatalf("liveness = %d, want livenessEnded after the restart", got)
	}
	if got := currentIdx(m); got != endedIdx {
		t.Fatalf("currentIndex = %d, want %d: the restart moved the traversal", got, endedIdx)
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
		t.Fatalf("liveness = %d, want livenessLive after an explicit selection", got)
	}
}

// assertNothingWasHandedToPlay pins that a replacement mpv process was given
// nothing to play: no loadfile of any mode reached it, its playlist is empty,
// and it holds no current entry. A fresh process in that state is the idle the
// one it replaced was deliberately left in.
func assertNothingWasHandedToPlay(t *testing.T, f *fakePlaylist) {
	t.Helper()
	if got := f.count(isAnyLoadfile); got != 0 {
		t.Fatalf("the restart sent %d loadfile command(s) to the replacement process; commands=%v",
			got, f.commands())
	}
	if got := f.entriesSnapshot(); len(got) != 0 {
		t.Fatalf("replacement playlist = %v, want empty: an ended traversal has nothing to reinstate", got)
	}
	if !f.idle() {
		t.Fatalf("the replacement process is playing %q; the restart resumed a traversal that had ended",
			f.playingPath())
	}
}

// assertRestartResumesLiveTraversal pins the behaviour the repair must not
// change: an outage that interrupts a running traversal — playing or paused —
// still reinstates the entry it interrupted, in the pause state it had.
func assertRestartResumesLiveTraversal(t *testing.T, name string, paused bool, wantPauseOpt string) {
	t.Helper()
	m, f := newFakePlaylistManager(t, name)
	queue := queueOf(t, 2)
	installQueue(m, queue, -1, true)
	runEventReader(t, m, f)

	if err := m.PlayFromIndex(0); err != nil {
		t.Fatalf("PlayFromIndex(0): %v", err)
	}
	f.waitForEntries(t, paths(queue, 0, 1))
	if paused {
		if err := m.Pause(); err != nil {
			t.Fatalf("Pause: %v", err)
		}
	}
	m.SetCurrentPosition(queue[0].Path, 42.5)

	restore := m.snapshotRestore()
	if restore.liveness != livenessLive {
		t.Fatalf("restore.liveness = %d, want livenessLive: the outage interrupted a running traversal",
			restore.liveness)
	}

	next := restartOnto(t, m, restore)

	next.waitFor(t, "the interrupted entry to be reinstated", func() bool {
		return next.playingPath() == queue[0].Path
	})
	assertResumeLoad(t, next, queue[0].Path, wantPauseOpt)
	if got := livenessOf(m); got != livenessLive {
		t.Fatalf("liveness = %d, want livenessLive after resuming an interrupted traversal", got)
	}
}

// assertResumeLoad finds the post-restart resume loadfile for path and checks it
// carries the expected seek and pause options.
func assertResumeLoad(t *testing.T, f *fakePlaylist, path, wantPauseOpt string) {
	t.Helper()
	for _, cmd := range f.commands() {
		if !isLoadfileReplace(cmd) {
			continue
		}
		if got, _ := cmd.stringAt(1); got != path {
			continue
		}
		if !cmd.hasStringContaining("start=42.5") {
			t.Fatalf("resume loadfile did not seek to the persisted position: %v", cmd)
		}
		if !cmd.hasStringContaining(wantPauseOpt) {
			t.Fatalf("resume loadfile = %v, want %s", cmd, wantPauseOpt)
		}
		return
	}
	t.Fatalf("no resume loadfile for %q reached the replacement process; commands=%v", path, f.commands())
}
