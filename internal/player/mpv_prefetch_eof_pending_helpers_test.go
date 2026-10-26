package player

import (
	"sync"
	"testing"
	"time"
)

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
	// The fake lists the append before the manager tracks it; an end captured
	// in between has no successor to promote and reloads it instead.
	assertPrefetch(t, m, queue, 1)

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
