package player

import (
	"context"
	"os/exec"
	"testing"
	"time"
)

// TestRestartCannotSnapshotLiveDuringEOFFallbackDispatch pins the residual
// window the test above starts after. It waits for the pending end to be
// published before it snapshots recovery, so it can only prove that a snapshot
// taken *after* the mark is safe — while the mark itself used to be a separate
// lock acquisition from the EOF state capture it belongs to.
//
// In that gap the Manager still read as live, and recovery is free to run: it
// takes its own snapshot of playback state the instant an outage begins, and
// that snapshot is what the resume acts on for the whole outage. A restore
// captured there names the row that just finished — lastTrackPath is only
// rewritten when a new track starts — and carries livenessLive, so the fresh mpv
// is handed a completed track to reload. The pending mark that lands afterwards
// changes nothing: the restoreState was copied by value and is already on its
// way to attemptRestart.
//
// Driving the event reader's two halves by hand is what makes the interleaving
// deterministic. handleEndOfFile is exactly snapshotEOF followed by dispatchEOF,
// so calling them in order with recovery in between is the reader's own sequence
// interrupted at the one instant the finding is about — no timing, no hook, and
// no window a sleep could miss.
func TestRestartCannotSnapshotLiveDuringEOFFallbackDispatch(t *testing.T) {
	t.Run("recovery between the EOF capture and its dispatch finds no live traversal", func(t *testing.T) {
		m, f := newFakePlaylistManager(t, "rst-eof-atomic")
		queue := queueOf(t, 3)
		installQueue(m, queue[:2], -1, true)

		// The last row has no successor, so the EOF takes the exhaustion fallback
		// rather than promoting a prefetch. No event reader runs: this test is the
		// reader, so that it can stop between the two halves of its work.
		if err := m.PlayFromIndex(1); err != nil {
			t.Fatalf("PlayFromIndex(1): %v", err)
		}
		f.waitForEntries(t, paths(queue, 1))

		// The TUI persists the position on a timer, so the row that just finished
		// is what the supervisor finds — the same resumable-looking path a
		// confirmed end leaves behind.
		m.SetCurrentPosition(queue[1].Path, 12.5)

		// mpv plays the entry out: it emits end-file/eof and, with nothing behind
		// it in the playlist, goes idle.
		f.finishCurrent()
		f.waitFor(t, "mpv to go idle on the natural end", f.idle)

		// The first half of the reader's work: the EOF state is captured, and the
		// decision it feeds has not been made yet.
		state := m.snapshotEOF()

		// The outage begins here, before the dispatch runs. This is the snapshot
		// the whole recovery — every retry attempt — resumes from.
		restore := m.snapshotRestore()
		if restore.path != queue[1].Path {
			t.Fatalf("restore.path = %q, want the completed row %q: the finding needs a resumable-looking path",
				restore.path, queue[1].Path)
		}
		if restore.liveness.live() {
			t.Fatalf("restore.liveness = %v, want a non-live state: the entry has played out and the EOF that "+
				"observed it has already been captured", restore.liveness)
		}

		next := restartOnto(t, m, restore)
		assertNothingWasHandedToPlay(t, next)

		// The second half of the reader's work, now against a playlist generation
		// the restart replaced: the advance is dropped as superseded and decides
		// nothing, and the pending end must settle into a confirmed one rather
		// than decaying back into a live traversal.
		m.dispatchEOF(state)
		waitForLiveness(t, m, livenessEnded, "the superseded exhaustion transaction")

		// Growing the queue on the fresh process schedules a best-effort prefetch,
		// which append-plays into an idle playlist. Run one synchronously too so
		// the assertions cannot pass merely by outrunning the scheduled goroutine.
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
	})

	t.Run("a best-effort prefetch racing the undecided end touches no slot", func(t *testing.T) {
		m, f := newFakePlaylistManager(t, "eof-pend-slot")
		queue := queueOf(t, 3)
		installQueue(m, queue, -1, true)

		// Again the test stands in for the event reader, so a queue mutation can
		// be placed inside the window where the end has been captured and the
		// transition it belongs to has not been committed.
		if err := m.PlayFromIndex(0); err != nil {
			t.Fatalf("PlayFromIndex(0): %v", err)
		}
		f.waitForEntries(t, paths(queue, 0, 1))

		// mpv reaches the end of the first entry and moves straight into the
		// successor already behind it: the completed row is still at position 0
		// and the row now playing is at position 1 — which is the slot the
		// prefetch tracking names.
		f.finishCurrent()
		f.waitFor(t, "mpv to start the appended successor", func() bool {
			return f.playingPath() == queue[1].Path
		})

		state := m.snapshotEOF()

		// The mutation a user append or an Auto-DJ refill schedules. It must not
		// act on a playlist whose transition has not been committed: the pending
		// end suppresses the append, and the slot cleanup that a settled end would
		// run here would remove the entry mpv just moved onto.
		m.prefetchNext()

		if got := f.entriesSnapshot(); len(got) != 2 || got[0] != queue[0].Path || got[1] != queue[1].Path {
			t.Fatalf("playlist = %v, want %v: a best-effort prefetch removed a slot mid-transition",
				got, paths(queue, 0, 1))
		}
		if got := f.playingPath(); got != queue[1].Path {
			t.Fatalf("mpv is playing %q, want the successor %q it had started gaplessly", got, queue[1].Path)
		}

		// The transition still commits, and its maintenance still refills.
		m.dispatchEOF(state)
		f.waitForEntries(t, paths(queue, 1, 2))
		if got := livenessOf(m); got != livenessLive {
			t.Fatalf("liveness = %v, want live once the promotion committed", got)
		}
	})

	t.Run("a gapless promotion still leaves the traversal live", func(t *testing.T) {
		m, f := newFakePlaylistManager(t, "eof-promo-live")
		queue := queueOf(t, 3)
		installQueue(m, queue, -1, true)
		runEventReader(t, m, f)

		if err := m.PlayFromIndex(0); err != nil {
			t.Fatalf("PlayFromIndex(0): %v", err)
		}
		f.waitForEntries(t, paths(queue, 0, 1))

		// mpv transitions gaplessly into the entry already behind the one that
		// ended, so the EOF promotes the tracked prefetch instead of advancing.
		// The pending end the capture recorded has to be lifted by that promotion:
		// the maintenance that follows it refills through the best-effort append,
		// which is suppressed for a traversal that is not live.
		f.finishCurrent()
		f.waitForEntries(t, paths(queue, 1, 2))
		if got := currentIdx(m); got != 1 {
			t.Fatalf("currentIndex = %d, want 1: the gapless transition was not promoted", got)
		}
		if got := livenessOf(m); got != livenessLive {
			t.Fatalf("liveness = %v, want live: mpv is playing the successor it started on its own", got)
		}
	})
}

// waitForLiveness polls until the manager's logical liveness reaches want.
func waitForLiveness(t *testing.T, m *Manager, want playbackLiveness, desc string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if got := livenessOf(m); got == want {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s to leave liveness = %v; want %v", desc, livenessOf(m), want)
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
