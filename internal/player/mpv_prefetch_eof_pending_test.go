package player

import (
	"sync"
	"testing"
	"time"
)

// This file pins the direct prefetch invalidators — the controls and queue
// mutations that reach mpv's playlist through clearPrefetch rather than through
// a playlist transaction — against the one interval in which position 1 is not a
// parked successor.
//
// mpv owns its playlist. When the file it is playing ends and an entry has been
// appended behind it, mpv moves straight into that entry and only afterwards
// does the Manager read the end-file and decide what the transition means.
// Throughout that interval the prefetch tracking still names the row mpv has
// already started, so a `playlist-remove 1` sent by an unrelated toggle or queue
// mutation stops the track the user is listening to.

// prefetchInvalidation is one direct invalidation: a control or a queue mutation
// that calls clearPrefetch outside the playlist transaction layer.
//
// rebuilds says whether the invalidation is expected to re-append a successor
// afterwards. A queue mutation invalidates the resolved successor and
// immediately resolves a fresh one, while stop-after-current and a gapless
// disable suppress the prefetch altogether.
type prefetchInvalidation struct {
	name     string
	rebuilds bool
	run      func(t *testing.T, m *Manager, queue []QueueTrack)
}

// prefetchInvalidations enumerates every direct invalidator FR-007 names.
func prefetchInvalidations() []prefetchInvalidation {
	return []prefetchInvalidation{
		{
			name: "stop after current",
			run: func(_ *testing.T, m *Manager, _ []QueueTrack) {
				m.SetStopAfterCurrent(true)
			},
		},
		{
			name: "gapless disabled",
			run: func(t *testing.T, m *Manager, _ []QueueTrack) {
				t.Helper()
				if err := m.SetGapless(false); err != nil {
					t.Fatalf("SetGapless(false): %v", err)
				}
			},
		},
		{
			name:     "queue invalidation",
			rebuilds: true,
			run: func(_ *testing.T, m *Manager, queue []QueueTrack) {
				m.SetQueue(queue)
			},
		},
	}
}

// TestClearPrefetchDuringEOFPendingDoesNotRemoveStartedSuccessor is FR-007's
// regression.
//
// The window is driven by hand rather than by timing: handleEndOfFile is exactly
// snapshotEOF followed by dispatchEOF, so the test stands in for the event reader
// and stops between the two halves — the instant mpv has entered the appended
// successor and the promotion that commits it has not run. Each invalidation is
// applied there, and none of them may take the row mpv is playing away.
//
// The second half of each case is the other side of the contract: the same
// invalidation, applied while the successor really is parked behind a playing
// entry, must still remove it.
func TestClearPrefetchDuringEOFPendingDoesNotRemoveStartedSuccessor(t *testing.T) {
	for _, tc := range prefetchInvalidations() {
		t.Run(tc.name+" leaves the started successor playing", func(t *testing.T) {
			m, f := newFakePlaylistManager(t, "eofpend-"+shortName(tc.name))
			queue := queueOf(t, 3)
			installQueue(m, queue, -1, true)

			if err := m.PlayFromIndex(0); err != nil {
				t.Fatalf("PlayFromIndex(0): %v", err)
			}
			f.waitForEntries(t, paths(queue, 0, 1))

			// mpv reaches the end of the first entry and moves into the successor
			// already behind it: the completed row is still at position 0 and the
			// row now playing is at position 1 — the slot the tracking names.
			f.finishCurrent()
			f.waitFor(t, "mpv to start the appended successor", func() bool {
				return f.playingPath() == queue[1].Path
			})

			// The reader has read the end-file; the transition it belongs to has
			// not been decided yet.
			state := m.snapshotEOF()
			if got := livenessOf(m); got != livenessEnding {
				t.Fatalf("liveness = %v, want ending: the EOF capture did not publish the pending end", got)
			}

			tc.run(t, m, queue)

			if got := f.count(isPlaylistRemove); got != 0 {
				t.Fatalf("%s sent %d playlist-remove commands during the pending end; "+
					"position 1 is the row mpv started, not a parked successor: %v",
					tc.name, got, f.commands())
			}
			if got := f.playingPath(); got != queue[1].Path {
				t.Fatalf("mpv is playing %q, want the successor %q it had started gaplessly", got, queue[1].Path)
			}
			f.waitForEntries(t, paths(queue, 0, 1))

			// The tracking has to survive too: it is what the promotion moves the
			// bookkeeping onto, and dropping it here would send the transition
			// down the fallback advance instead.
			assertPrefetch(t, m, queue, 1)

			// The transition still commits onto the row mpv actually started.
			m.dispatchEOF(state)
			f.waitFor(t, "the promotion to remove the completed slot", func() bool {
				return f.currentPos() == 0 && f.playingPath() == queue[1].Path
			})
			if got := currentIdx(m); got != 1 {
				t.Fatalf("currentIndex = %d, want 1: the gapless transition was not promoted", got)
			}
		})
	}

	for _, tc := range prefetchInvalidations() {
		t.Run(tc.name+" still removes a parked successor", func(t *testing.T) {
			m, f := newFakePlaylistManager(t, "eofstdy-"+shortName(tc.name))
			queue := queueOf(t, 3)
			installQueue(m, queue, -1, true)

			if err := m.PlayFromIndex(0); err != nil {
				t.Fatalf("PlayFromIndex(0): %v", err)
			}
			f.waitForEntries(t, paths(queue, 0, 1))

			// No end-file: mpv is playing position 0 and position 1 is a genuinely
			// future successor, so the invalidation must reach it.
			tc.run(t, m, queue)

			f.waitFor(t, "the invalidation to remove the parked successor", func() bool {
				return f.count(isPlaylistRemove) > 0
			})
			if got := f.playingPath(); got != queue[0].Path {
				t.Fatalf("mpv is playing %q, want the current row %q", got, queue[0].Path)
			}

			if tc.rebuilds {
				f.waitForEntries(t, paths(queue, 0, 1))
				assertPrefetch(t, m, queue, 1)
				return
			}
			f.waitForEntries(t, paths(queue, 0))
			assertPrefetch(t, m, queue, -1)
		})
	}
}

// endReport records what OnPlaybackEnded observed at the moment it fired.
//
// The playlist is sampled inside the callback because that instant is what
// FR-008 turns on: the end may be reported only once mpv has actually stopped,
// never while it is still playing a row the traversal has declined.
type endReport struct {
	mu      sync.Mutex
	fired   int
	playing string
	signal  chan struct{}
}

// watchPlaybackEnded installs callbacks that record every OnPlaybackEnded.
func watchPlaybackEnded(m *Manager, f *fakePlaylist) *endReport {
	r := &endReport{signal: make(chan struct{}, 4)}
	m.SetEventCallbacks(EventCallbacks{
		OnStatus:      func(string) {},
		OnMissingFile: func(string) {},
		OnPlaybackEnded: func() {
			playing := f.playingPath()
			r.mu.Lock()
			r.fired++
			r.playing = playing
			r.mu.Unlock()
			select {
			case r.signal <- struct{}{}:
			default:
			}
		},
		OnTrackChanged: func(_, _, _, _ string, _ int, _ int64) {},
		OnPauseChanged: func(bool) {},
	})
	return r
}

// await blocks until the end is reported and fails if it never is.
func (r *endReport) await(t *testing.T) {
	t.Helper()
	select {
	case <-r.signal:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for OnPlaybackEnded: the exhausted traversal never reported the end")
	}
}

func (r *endReport) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.fired
}

// playingAtEnd is the path mpv was playing when the end was reported, empty when
// it was already idle.
func (r *endReport) playingAtEnd() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.playing
}

// eofPendingWindow is a manager stopped inside the pending-EOF window: mpv has
// entered the appended successor at position 1, the end-file has been captured,
// and the transition that decides what it means has not been dispatched.
type eofPendingWindow struct {
	m     *Manager
	f     *fakePlaylist
	queue []QueueTrack
	state eofState
	ended *endReport
}

// openEOFPendingWindow drives a three-entry gapless queue up to that instant.
// handleEndOfFile is exactly snapshotEOF followed by dispatchEOF, so standing in
// for the event reader and stopping between the halves is the window itself.
func openEOFPendingWindow(t *testing.T, name string) *eofPendingWindow {
	t.Helper()
	m, f := newFakePlaylistManager(t, name)
	ended := watchPlaybackEnded(m, f)
	queue := queueOf(t, 3)
	installQueue(m, queue, -1, true)

	if err := m.PlayFromIndex(0); err != nil {
		t.Fatalf("PlayFromIndex(0): %v", err)
	}
	f.waitForEntries(t, paths(queue, 0, 1))

	f.finishCurrent()
	f.waitFor(t, "mpv to start the appended successor", func() bool {
		return f.playingPath() == queue[1].Path
	})

	state := m.snapshotEOF()
	if got := livenessOf(m); got != livenessEnding {
		t.Fatalf("liveness = %v, want ending: the EOF capture did not publish the pending end", got)
	}
	return &eofPendingWindow{m: m, f: f, queue: queue, state: state, ended: ended}
}

// assertSuccessorIntact is FR-007's half of the contract: whatever invalidated
// the tracked row, the invalidation itself may not take the slot mpv is playing.
func (w *eofPendingWindow) assertSuccessorIntact(t *testing.T) {
	t.Helper()
	if got := w.f.count(isPlaylistRemove); got != 0 {
		t.Fatalf("the invalidation sent %d playlist-remove commands during the pending end; "+
			"position 1 is the row mpv started: %v", got, w.f.commands())
	}
	if got := w.f.playingPath(); got != w.queue[1].Path {
		t.Fatalf("mpv is playing %q, want the successor %q it had started gaplessly", got, w.queue[1].Path)
	}
	w.f.waitForEntries(t, paths(w.queue, 0, 1))
}

// TestEOF_InvalidatedStartedSuccessorDoesNotEndWhilePlaying is FR-008's
// regression.
//
// FR-007 established that nothing may remove position 1 while mpv is playing it,
// which leaves the transition itself holding a row the queue may no longer agree
// with. A queue mutation applied inside the pending window can remove or replace
// the tracked occurrence, and the promotion then rejects it: the identity it
// resolved against is gone. What the fallback advance does next is the contract
// here — it either loads the row that took the successor's place, or, when the
// mutated queue has nothing left, settles mpv before reporting the end. Reporting
// exhaustion over a still-playing entry is the failure this pins.
func TestEOF_InvalidatedStartedSuccessorDoesNotEndWhilePlaying(t *testing.T) {
	t.Run("a removed successor is settled before the end is reported", func(t *testing.T) {
		w := openEOFPendingWindow(t, "eofinv-rm")

		// The started successor and the row behind it both leave the queue while
		// the transition is pending, so nothing the advance can reach survives.
		w.m.SetQueue(w.queue[:1])
		w.assertSuccessorIntact(t)
		assertPrefetch(t, w.m, w.queue, 1)

		w.m.dispatchEOF(w.state)

		w.ended.await(t)
		if got := w.ended.playingAtEnd(); got != "" {
			t.Fatalf("OnPlaybackEnded fired while mpv was playing %q; the queue was reported "+
				"exhausted over an entry that was still producing audio", got)
		}
		if got := w.ended.count(); got != 1 {
			t.Fatalf("OnPlaybackEnded fired %d times, want 1", got)
		}

		// mpv is left holding only the entry that finished, which is the shape an
		// ordinary exhaustion leaves behind.
		w.f.waitForEntries(t, paths(w.queue, 0))
		if !w.f.idle() {
			t.Fatalf("mpv is playing %q, want an idle playlist after the settlement", w.f.playingPath())
		}
		if got := livenessOf(w.m); got != livenessEnded {
			t.Fatalf("liveness = %v, want ended", got)
		}
		if got := currentIdx(w.m); got != 0 {
			t.Fatalf("currentIndex = %d, want 0: the exhausted advance moved the queue position", got)
		}
		assertPrefetch(t, w.m, w.queue, -1)
		if got := w.f.count(isPlaylistRemove); got != 1 {
			t.Fatalf("playlist-remove count = %d, want exactly 1 (the settlement): %v", got, w.f.commands())
		}

		// The not-live cleanup that follows an ended traversal has nothing left to
		// aim at: the settlement took the slot and the tracking with it.
		w.m.prefetchNext()
		if got := w.f.count(isPlaylistRemove); got != 1 {
			t.Fatalf("the cleanup after the end sent another playlist-remove (count = %d); "+
				"it targeted a slot the settlement had already released: %v", got, w.f.commands())
		}

		// The next transition still works: extending the queue and resuming plays
		// the newly appended row rather than replaying the settled one.
		extended := []QueueTrack{w.queue[0], w.queue[2]}
		w.m.SetQueue(extended)
		idx, err := w.m.ResumeAfterQueueExtension()
		if err != nil {
			t.Fatalf("ResumeAfterQueueExtension: %v", err)
		}
		if idx != 1 {
			t.Fatalf("ResumeAfterQueueExtension = %d, want 1", idx)
		}
		w.f.waitForEntries(t, paths(w.queue, 2))
		if got := livenessOf(w.m); got != livenessLive {
			t.Fatalf("liveness = %v, want live after the resume", got)
		}
	})

	t.Run("a replaced successor hands the transition to the row that took its slot", func(t *testing.T) {
		w := openEOFPendingWindow(t, "eofinv-rep")

		// Same position, different occurrence: the tracked row is gone from the
		// queue and another track now sits where the advance will look.
		replaced := []QueueTrack{w.queue[0], w.queue[2]}
		w.m.SetQueue(replaced)
		w.assertSuccessorIntact(t)

		w.m.dispatchEOF(w.state)

		// The advance loads through the replacement primitive, which resets mpv's
		// playlist to the single row it selected — the orphaned successor and the
		// completed entry go together.
		w.f.waitFor(t, "the advance to load the row that replaced the successor", func() bool {
			return w.f.playingPath() == w.queue[2].Path
		})
		w.f.waitForEntries(t, paths(w.queue, 2))
		if got := livenessOf(w.m); got != livenessLive {
			t.Fatalf("liveness = %v, want live: the pending end resolved into a successor", got)
		}
		if got := currentIdx(w.m); got != 1 {
			t.Fatalf("currentIndex = %d, want 1: the advance did not commit onto the replacing row", got)
		}
		if got := w.ended.count(); got != 0 {
			t.Fatalf("OnPlaybackEnded fired %d times; the queue still had a row to play", got)
		}
		if got := w.f.count(isPlaylistRemove); got != 0 {
			t.Fatalf("playlist-remove count = %d, want 0: the replacement already swept both slots: %v",
				got, w.f.commands())
		}
		assertPrefetch(t, w.m, w.queue, -1)

		// The next transition off that row is an ordinary exhaustion, and it ends
		// the same way: idle first, reported after.
		w.f.finishCurrent()
		w.m.dispatchEOF(w.m.snapshotEOF())
		w.ended.await(t)
		if got := w.ended.playingAtEnd(); got != "" {
			t.Fatalf("OnPlaybackEnded fired while mpv was playing %q, want an idle playlist", got)
		}
		if got := livenessOf(w.m); got != livenessEnded {
			t.Fatalf("liveness = %v, want ended", got)
		}
	})
}

// shortName maps a case name onto a socket-safe fragment: the fake playlist's
// socket path is length-bounded, so the names above cannot be used verbatim.
func shortName(name string) string {
	switch name {
	case "stop after current":
		return "stop"
	case "gapless disabled":
		return "gap"
	default:
		return "queue"
	}
}
