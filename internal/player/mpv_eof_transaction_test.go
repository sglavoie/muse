package player

import (
	"fmt"
	"slices"
	"testing"
	"time"
)

// These tests drive the end-file/eof transition against the stateful fake
// playlist with the real event reader attached, so every assertion compares what
// mpv is actually playing with what the Manager believes it is playing. That
// pairing is the point: the reported stall was mpv sitting idle while Manager
// state had advanced on prefetch metadata alone.

// TestEOF_StalePrefetchAfterReplacementPlaysSuccessor is the deterministic
// regression for the last-second stall. The flow is the reported one: a track
// plays with its successor prefetched, an explicit replacement discards that
// successor inside mpv, and then the track ends.
//
// Manager state must not advance on the tracking the replacement invalidated.
// It must fall back to the traversal-aware load, so mpv plays the successor
// instead of going quiet with a queue that still has tracks in it.
func TestEOF_StalePrefetchAfterReplacementPlaysSuccessor(t *testing.T) {
	m, f := newFakePlaylistManager(t, "eof-stale")
	queue := queueOf(t, 3)
	installQueue(m, queue, -1, true)
	runEventReader(t, m, f)

	if err := m.PlayFromIndex(0); err != nil {
		t.Fatalf("PlayFromIndex(0): %v", err)
	}
	f.waitForEntries(t, paths(queue, 0, 1))
	assertPrefetch(t, m, queue, 1)
	stale, staleGen := prefetchTracking(m)

	// The explicit replacement: mpv's playlist is swapped for the selected file
	// alone, so the prefetched successor no longer exists there. Failing the
	// refill keeps mpv at one entry, which is the state the stall starts from.
	f.failCommand("loadfile:append-play", "mpv rejected the append")
	appendsBefore := f.count(isLoadfileAppend)
	if err := m.PlayFromIndex(0); err != nil {
		t.Fatalf("PlayFromIndex(0) replay: %v", err)
	}
	f.waitFor(t, "the refill append attempt", func() bool {
		return f.count(isLoadfileAppend) > appendsBefore
	})
	f.clearFailure("loadfile:append-play")
	if got := f.entriesSnapshot(); !slices.Equal(got, paths(queue, 0)) {
		t.Fatalf("entries = %v, want %v after the replacement", got, paths(queue, 0))
	}
	assertPrefetch(t, m, queue, -1)

	// Reinstate the tracking exactly as the pre-repair code retained it: Go
	// believes mpv holds queue[1] as the gapless successor, but that belief
	// belongs to the playlist generation the replacement superseded.
	m.restorePrefetchTracking(stale)
	if _, gen := prefetchTracking(m); gen == staleGen {
		t.Fatalf("playlistGen = %d, want a newer generation than the stale prefetch's %d", gen, staleGen)
	}

	// The last second of the track.
	f.finishCurrent()

	f.waitFor(t, "mpv to play the successor", func() bool {
		return f.playingPath() == queue[1].Path
	})
	waitForCurrentIndex(t, m, 1)
	if f.idle() {
		t.Fatal("mpv is idle after EOF while a playable successor remained in the queue")
	}
	// The successor's own successor is prefetched again, so the queue keeps
	// running gaplessly from here.
	f.waitForEntries(t, paths(queue, 1, 2))
	assertPrefetch(t, m, queue, 2)
}

// parkReplacement starts an explicit selection of index and parks it inside mpv
// with the loadfile still unanswered — the window where the traversal state for
// the selected row is already published but mpv is still playing the row it is
// about to discard. It returns the gate holding the load and the channel the
// selection's error arrives on.
func parkReplacement(t *testing.T, m *Manager, f *fakePlaylist, index int) (*fakeGate, <-chan error) {
	t.Helper()
	gate := f.block(t, "loadfile:replace")
	done := make(chan error, 1)
	go func() { done <- m.PlayFromIndex(index) }()
	gate.waitArrived(t, fmt.Sprintf("the explicit replacement of index %d", index))
	return gate, done
}

// endCurrentFileDuring emits the end-file/eof of the track that is still playing
// and blocks until the event reader has finished deciding what to do with it.
//
// Only the end-file is emitted: a gapless start-file would make the reader issue
// commands of its own, and the single command socket would park those behind the
// gated loadfile. The pause change that follows is the barrier — the reader
// dispatches in arrival order on one goroutine, so observing it proves the EOF
// decision has already run to completion.
func endCurrentFileDuring(t *testing.T, m *Manager, f *fakePlaylist) {
	t.Helper()
	f.emit(eventLine("end-file", "eof"))
	f.emit(`{"event":"property-change","name":"pause","data":true}`)
	f.waitFor(t, "the event reader to dispatch the end-file and the pause change", func() bool {
		m.mu.RLock()
		defer m.mu.RUnlock()
		return m.lastPaused
	})
}

// TestEOF_ReplacementInFlightDoesNotAdvancePastSelection is the deterministic
// regression for an EOF decided from a half-committed replacement.
//
// An explicit selection publishes the row it is about to play before the
// loadfile reaches mpv, and bumps the playlist generation only once mpv answers.
// Inside that window every generation-stamped fact still describes the entry
// being discarded while currentIndex already names the selection, so an EOF from
// the old entry reads the new target with the old prefetch: it rejects the
// promotion and queues a fallback measured from the selected row. That fallback
// waits on the transaction lock and then replaces the user's selection with the
// row after it.
//
// The EOF belongs to an entry the replacement is superseding, so it must decide
// nothing at all: the selection stands, and the successor mpv holds next is the
// selection's own.
func TestEOF_ReplacementInFlightDoesNotAdvancePastSelection(t *testing.T) {
	m, f := newFakePlaylistManager(t, "eof-inflight")
	queue := queueOf(t, 5)
	installQueue(m, queue, -1, true)
	runEventReader(t, m, f)

	if err := m.PlayFromIndex(0); err != nil {
		t.Fatalf("PlayFromIndex(0): %v", err)
	}
	f.waitForEntries(t, paths(queue, 0, 1))
	assertPrefetch(t, m, queue, 1)

	gate, selected := parkReplacement(t, m, f, 3)
	replacesDuring := f.count(isLoadfileReplace)

	// The last second of the track that is still playing.
	endCurrentFileDuring(t, m, f)

	gate.release()
	f.unblock("loadfile:replace")
	if err := <-selected; err != nil {
		t.Fatalf("PlayFromIndex(3): %v", err)
	}

	waitForCurrentIndex(t, m, 3)
	f.waitForEntries(t, paths(queue, 3, 4))
	f.waitFor(t, "the selection's own successor to be tracked", func() bool {
		got, gen := prefetchTracking(m)
		return got.index == 4 && got.gen == gen
	})
	assertPrefetch(t, m, queue, 4)

	// And it keeps standing: a fallback queued by the superseded EOF only
	// reaches mpv once the replacement has released the transaction lock, so the
	// damage lands after the state above already looked correct.
	time.Sleep(150 * time.Millisecond)
	if got := currentIdx(m); got != 3 {
		t.Fatalf("currentIndex = %d, want 3: the superseded EOF advanced past the selected row", got)
	}
	if got := f.playingPath(); got != queue[3].Path {
		t.Fatalf("mpv is playing %q, want the selected %q", got, queue[3].Path)
	}
	if got := f.entriesSnapshot(); !slices.Equal(got, paths(queue, 3, 4)) {
		t.Fatalf("entries = %v, want %v: the selected row and its successor", got, paths(queue, 3, 4))
	}
	if got := f.count(isLoadfileReplace); got != replacesDuring {
		t.Fatalf("loadfile replace count = %d, want %d: the superseded EOF sent a replacement of its own",
			got, replacesDuring)
	}
}

// TestEOF_ReplacementInFlightDoesNotMutateReanchoredShuffle is the shuffle half
// of the same window. Here the promotion is not rejected but accepted: the
// prefetched identity still resolves to a live row and still carries the live
// generation, so the EOF commits it — onto the cycle the selection had just
// reanchored. The visible result is a Previous entry that was never played and a
// traversal one step ahead of the row mpv is on.
func TestEOF_ReplacementInFlightDoesNotMutateReanchoredShuffle(t *testing.T) {
	m, f := newFakePlaylistManager(t, "eof-inflight-shuffle")
	queue := queueOf(t, 4)
	m.mu.Lock()
	m.gaplessEnabled = true
	m.mu.Unlock()
	buildShuffleState(t, m, queue, 0, reversePermute)
	runEventReader(t, m, f)

	// reversePermute leaves index 3 first in the unseen pool, so the anchor's
	// prefetched successor is queue[3].
	if err := m.PlayFromIndex(0); err != nil {
		t.Fatalf("PlayFromIndex(0): %v", err)
	}
	f.waitForEntries(t, paths(queue, 0, 3))

	// Reanchoring around index 1 re-permutes everything else, so queue[3] is no
	// longer the identity the traversal is on its way to.
	gate, selected := parkReplacement(t, m, f, 1)

	endCurrentFileDuring(t, m, f)

	gate.release()
	f.unblock("loadfile:replace")
	if err := <-selected; err != nil {
		t.Fatalf("PlayFromIndex(1): %v", err)
	}

	waitForCurrentIndex(t, m, 1)
	time.Sleep(150 * time.Millisecond)

	history, pos, _ := shuffleSnapshotForTest(m)
	if len(history) != 1 || pos != 0 {
		t.Fatalf("shuffleHistory = %v pos = %d, want the reanchored selection alone: "+
			"a superseded EOF committed onto the new cycle", history, pos)
	}
	if got := currentIdx(m); got != 1 {
		t.Fatalf("currentIndex = %d, want 1", got)
	}
	if got := f.playingPath(); got != queue[1].Path {
		t.Fatalf("mpv is playing %q, want the selected %q", got, queue[1].Path)
	}
	// The successor the fresh cycle resolves is queue[3] again — the identity the
	// superseded EOF would have consumed.
	f.waitForEntries(t, paths(queue, 1, 3))
}

// TestEOF_ReplacementInFlightKeepsStopAfterCurrentArmed pins the flag half of
// the same decision. Stop-after-current is armed against the track the user is
// listening to; the entry a replacement is discarding is no longer that track,
// so spending the flag on its EOF would fire the end of playback against a row
// that never finished and disarm the mode the user is still expecting.
func TestEOF_ReplacementInFlightKeepsStopAfterCurrentArmed(t *testing.T) {
	m, f := newFakePlaylistManager(t, "eof-inflight-stop-after")
	queue := queueOf(t, 4)
	installQueue(m, queue, -1, true)
	ended := playbackEndedSignal(m)
	runEventReader(t, m, f)

	if err := m.PlayFromIndex(0); err != nil {
		t.Fatalf("PlayFromIndex(0): %v", err)
	}
	f.waitForEntries(t, paths(queue, 0, 1))

	// Arm it before parking anything: SetStopAfterCurrent drops the prefetch
	// through a playlist transaction of its own.
	m.SetStopAfterCurrent(true)
	f.waitForEntries(t, paths(queue, 0))

	gate, selected := parkReplacement(t, m, f, 2)

	endCurrentFileDuring(t, m, f)

	select {
	case <-ended:
		t.Fatal("OnPlaybackEnded fired for an entry a replacement was already discarding")
	default:
	}
	if !m.StopAfterCurrent() {
		t.Fatal("stop-after-current was consumed by a superseded EOF")
	}

	gate.release()
	f.unblock("loadfile:replace")
	if err := <-selected; err != nil {
		t.Fatalf("PlayFromIndex(2): %v", err)
	}
	waitForCurrentIndex(t, m, 2)

	time.Sleep(150 * time.Millisecond)
	if !m.StopAfterCurrent() {
		t.Fatal("stop-after-current is no longer armed; it must still fire at the end of the selected track")
	}
	select {
	case <-ended:
		t.Fatal("OnPlaybackEnded fired while the selected track was still playing")
	default:
	}
}

// waitForCorrelatedLoad blocks until the entry mpv is playing is correlated with
// the live load generation — the state in which an end-file from it is acted on
// rather than discarded. A test that wants to prove a *buffered* end-file is
// rejected must first establish that starting point; a test that drops a
// buffered one must land back on it, or the queue is left stalled.
func waitForCorrelatedLoad(t *testing.T, m *Manager, f *fakePlaylist) {
	t.Helper()
	f.waitFor(t, "the event reader to correlate the playing entry", func() bool {
		m.mu.RLock()
		defer m.mu.RUnlock()
		return m.startedLoadGen != 0 && m.startedLoadGen == m.loadGen
	})
}

// drainHeldEvents releases a hold installed by holdEvents and blocks until every
// event buffered behind it has been dispatched. The pause change is the barrier:
// it is emitted last and the reader dispatches in arrival order on one
// goroutine, so observing it proves the buffered end-file was already decided.
func drainHeldEvents(t *testing.T, m *Manager, f *fakePlaylist, release func()) {
	t.Helper()
	release()
	f.emit(`{"event":"property-change","name":"pause","data":true}`)
	f.waitFor(t, "the event reader to dispatch the buffered events", func() bool {
		m.mu.RLock()
		defer m.mu.RUnlock()
		return m.lastPaused
	})
}

// TestEOF_BufferedOldEventAfterReplacementDoesNotAdvance is the deterministic
// regression for an EOF that outlives the replacement acknowledgement.
//
// The replacement window closes as soon as mpv answers the loadfile, and the
// playlist generation advances with it — so an end-file the old entry emitted
// *before* that answer, but which was still sitting unread on the event socket,
// is decided against state that describes the selection entirely: its
// currentIndex, its freshly stamped prefetch, its generation. Nothing in that
// mixture says the row that ended is not the row that is playing, so the
// promotion is accepted and the selection's successor is committed as current
// while mpv is still on the selection itself.
//
// The end-file belongs to an entry the replacement discarded, so it must decide
// nothing: the selected row stands and so does the successor prefetched for it.
func TestEOF_BufferedOldEventAfterReplacementDoesNotAdvance(t *testing.T) {
	m, f := newFakePlaylistManager(t, "eof-buffered")
	queue := queueOf(t, 5)
	installQueue(m, queue, -1, true)
	runEventReader(t, m, f)

	if err := m.PlayFromIndex(0); err != nil {
		t.Fatalf("PlayFromIndex(0): %v", err)
	}
	f.waitForEntries(t, paths(queue, 0, 1))
	assertPrefetch(t, m, queue, 1)
	waitForCorrelatedLoad(t, m, f)

	// Freeze the event socket. Everything mpv emits from here is buffered,
	// which is what lets the old entry's end-file survive into a world where
	// the replacement has already been acknowledged.
	release := f.holdEvents(t)

	// The last second of queue[0]: mpv moves onto the prefetched successor by
	// itself and emits end-file/eof plus start-file, neither of them visible to
	// the reader yet.
	f.finishCurrent()

	// The user selects a different row while those events are still in the
	// pipe. The selection runs end to end — playlist replaced, generation
	// bumped, window closed — and its own successor is prefetched behind it.
	if err := m.PlayFromIndex(3); err != nil {
		t.Fatalf("PlayFromIndex(3): %v", err)
	}
	f.waitForEntries(t, paths(queue, 3, 4))
	assertPrefetch(t, m, queue, 4)
	removalsBefore := f.count(isPlaylistRemove)
	replacesBefore := f.count(isLoadfileReplace)

	// Only now does the reader get to the end-file of the discarded entry.
	drainHeldEvents(t, m, f, release)
	// The damage a promotion does lands in its background maintenance, after
	// the decision itself has returned.
	time.Sleep(150 * time.Millisecond)

	if got := currentIdx(m); got != 3 {
		t.Fatalf("currentIndex = %d, want 3: a buffered EOF advanced past the selected row", got)
	}
	if got := f.playingPath(); got != queue[3].Path {
		t.Fatalf("mpv is playing %q, want the selected %q", got, queue[3].Path)
	}
	if got := f.entriesSnapshot(); !slices.Equal(got, paths(queue, 3, 4)) {
		t.Fatalf("entries = %v, want %v: the selected row and its successor", got, paths(queue, 3, 4))
	}
	assertPrefetch(t, m, queue, 4)
	if got := f.count(isPlaylistRemove); got != removalsBefore {
		t.Fatalf("playlist-remove count = %d, want %d: the superseded EOF ran its completed-slot cleanup "+
			"against the selection's playlist", got, removalsBefore)
	}
	if got := f.count(isLoadfileReplace); got != replacesBefore {
		t.Fatalf("loadfile replace count = %d, want %d: the superseded EOF sent an advance of its own",
			got, replacesBefore)
	}
}
