package player

import (
	"testing"
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
