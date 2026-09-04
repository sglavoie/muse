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

// TestEOF_BufferedStartAndEOFAfterReplacementDoesNotAdvance is the deterministic
// regression for the ordering the test above deliberately excludes: it waits for
// the playing entry to be correlated *before* it holds the socket, so the
// start-file of the gapless successor is always dispatched while the generation
// it belongs to is still live.
//
// Here the hold goes on first, so the successor's start-file is buffered too and
// is only read once the replacement has been acknowledged. Stamping it with the
// generation live at that later instant hands the discarded entry the
// selection's generation, and the *second* buffered end-file — emitted by that
// same superseded entry — then reads as live. Nothing in the state it is decided
// against says otherwise: the promotion takes the selection's own prefetch as
// the successor that just started, moves the current row onto it, and runs the
// completed-slot cleanup against the selection's playlist, deleting the row mpv
// is actually playing.
//
// Both events belong to entries the replacement discarded, so both must decide
// nothing: the selected row stands and so does the successor prefetched for it.
func TestEOF_BufferedStartAndEOFAfterReplacementDoesNotAdvance(t *testing.T) {
	m, f := newFakePlaylistManager(t, "eof-buffered-start")
	queue := queueOf(t, 5)
	installQueue(m, queue, -1, true)
	runEventReader(t, m, f)

	if err := m.PlayFromIndex(0); err != nil {
		t.Fatalf("PlayFromIndex(0): %v", err)
	}
	f.waitForEntries(t, paths(queue, 0, 1))
	assertPrefetch(t, m, queue, 1)
	waitForCorrelatedLoad(t, m, f)

	// Freeze the event socket. Every event below is buffered behind it,
	// start-files included.
	release := f.holdEvents(t)

	// queue[0] plays out and mpv moves onto the prefetched queue[1] on its own:
	// end-file/eof plus start-file, both unread.
	f.finishCurrent()
	// queue[1] plays out too. Nothing was prefetched behind it — the reader has
	// not seen the transition yet, so no maintenance ran — and mpv goes idle
	// after a second unread end-file/eof.
	f.finishCurrent()

	// The user selects a different row while all three events are still in the
	// pipe. The selection runs end to end — playlist replaced, generation
	// bumped, window closed — and its own successor is prefetched behind it.
	if err := m.PlayFromIndex(3); err != nil {
		t.Fatalf("PlayFromIndex(3): %v", err)
	}
	f.waitForEntries(t, paths(queue, 3, 4))
	assertPrefetch(t, m, queue, 4)
	removalsBefore := f.count(isPlaylistRemove)
	replacesBefore := f.count(isLoadfileReplace)

	// Only now does the reader get to the discarded entries' events, in the
	// order mpv emitted them.
	drainHeldEvents(t, m, f, release)
	// The damage a promotion does lands in its background maintenance, after
	// the decision itself has returned.
	time.Sleep(150 * time.Millisecond)

	if got := currentIdx(m); got != 3 {
		t.Fatalf("currentIndex = %d, want 3: a buffered start-file re-armed the end-file that followed it", got)
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
	// The correlation must end up on the generation the selection loaded, or
	// the next genuine end-file would be discarded and the queue would stall.
	waitForCorrelatedLoad(t, m, f)
}

// TestEOF_BufferedLoadStartAcrossAReplacementKeepsItsOwnGeneration is the other
// face of the same finding. Above it is a *gapless* start-file that survives a
// replacement; here it is the start-file of a load, buffered while a second load
// supersedes the first.
//
// mpv answers loads in the order they were sent, so the generation of a
// start-file is fixed when the loadfile goes out, not when the reader gets to
// it. Reading the live generation instead hands the first load's entry the
// second load's stamp, and the end-file that entry emitted before it was
// discarded then reads as live against the row the user just selected.
func TestEOF_BufferedLoadStartAcrossAReplacementKeepsItsOwnGeneration(t *testing.T) {
	m, f := newFakePlaylistManager(t, "eof-buffered-load-start")
	queue := queueOf(t, 5)
	installQueue(m, queue, -1, true)
	runEventReader(t, m, f)

	if err := m.PlayFromIndex(0); err != nil {
		t.Fatalf("PlayFromIndex(0): %v", err)
	}
	f.waitForEntries(t, paths(queue, 0, 1))
	waitForCorrelatedLoad(t, m, f)

	release := f.holdEvents(t)

	// A first selection lands in mpv, but its start-file stays on the socket.
	if err := m.PlayFromIndex(1); err != nil {
		t.Fatalf("PlayFromIndex(1): %v", err)
	}
	f.waitForEntries(t, paths(queue, 1, 2))

	// It then plays out, so its end-file is buffered behind its own start-file.
	f.finishCurrent()

	// A second selection supersedes all of it and prefetches its own successor.
	if err := m.PlayFromIndex(3); err != nil {
		t.Fatalf("PlayFromIndex(3): %v", err)
	}
	f.waitForEntries(t, paths(queue, 3, 4))
	assertPrefetch(t, m, queue, 4)
	removalsBefore := f.count(isPlaylistRemove)
	replacesBefore := f.count(isLoadfileReplace)

	drainHeldEvents(t, m, f, release)
	time.Sleep(150 * time.Millisecond)

	if got := currentIdx(m); got != 3 {
		t.Fatalf("currentIndex = %d, want 3: a buffered load start-file took the newer load's generation", got)
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
	waitForCorrelatedLoad(t, m, f)
}

// formerPendingLoadStartLimit is the fixed length the pending-load queue used to
// be trimmed to. The scenario below has to reproduce the exact arithmetic that
// trimming produced, so it names the historical number: there is no live
// constant left to read, which is the point of the repair.
const formerPendingLoadStartLimit = 8

// waitForPlayingWithSuccessor blocks until mpv holds exactly path plus one
// appended successor behind it, without naming which successor. Under shuffle
// the traversal picks that row, and this scenario only needs the two-entry shape
// a gapless transition depends on.
func (f *fakePlaylist) waitForPlayingWithSuccessor(t *testing.T, path string) {
	t.Helper()
	f.waitFor(t, fmt.Sprintf("playlist %s plus its successor", path), func() bool {
		got := f.entriesSnapshot()
		return len(got) == 2 && got[0] == path
	})
}

// TestEOF_BufferedLoadStartsBeyondCorrelationLimitKeepTheirGenerations is the
// deterministic regression for a pending-load queue trimmed to a fixed length.
//
// The correlation between a start-file and the load that emitted it is
// positional: the k-th entry of the queue names the generation of the k-th
// start-file still to be read. Dropping the oldest entry to bound the queue
// therefore does not lose one correlation, it shifts every remaining one — and
// accepted replacements are bounded by nothing the event reader does, so a user
// clicking through a queue faster than the socket drains overflows it.
//
// Once it does, the shift walks the live generation backwards onto an entry a
// replacement already discarded: with the old cap of eight, the eighth buffered
// start-file takes the ninth load's stamp. The end-file that entry emitted
// before it was discarded then passes eofState.superseded and is acted on in
// full — it promotes the selection's own prefetch as if it were the successor
// that had just started, moves the current row onto it, and runs the
// completed-slot cleanup against the selection's playlist, deleting the row mpv
// is actually playing.
//
// Every one of those events belongs to a superseded entry, so all of them must
// decide nothing: the last selection stands, and so do its prefetch, mpv's
// playlist, and the traversal position.
func TestEOF_BufferedLoadStartsBeyondCorrelationLimitKeepTheirGenerations(t *testing.T) {
	t.Run("sequential", func(t *testing.T) {
		m, f := newFakePlaylistManager(t, "eof-load-overflow-seq")
		queue := queueOf(t, formerPendingLoadStartLimit+4)
		installQueue(m, queue, -1, true)
		runEventReader(t, m, f)
		bufferedLoadStartsOverflowScenario(t, m, f, queue)
	})

	// The shuffle half moves the traversal rather than the visible index: a
	// promotion here consumes an identity onto the cycle the last selection
	// reanchored, leaving a Previous entry that was never heard.
	t.Run("shuffled", func(t *testing.T) {
		m, f := newFakePlaylistManager(t, "eof-load-overflow-shuf")
		queue := queueOf(t, formerPendingLoadStartLimit+4)
		m.mu.Lock()
		m.gaplessEnabled = true
		m.mu.Unlock()
		buildShuffleState(t, m, queue, 0, reversePermute)
		runEventReader(t, m, f)
		bufferedLoadStartsOverflowScenario(t, m, f, queue)
	})
}

// bufferedLoadStartsOverflowScenario buffers more accepted loads than the old
// pending-load cap allowed, lets one of the superseded entries end while they
// are all still unread, and then drains the socket against the selection that
// displaced them.
func bufferedLoadStartsOverflowScenario(t *testing.T, m *Manager, f *fakePlaylist, queue []QueueTrack) {
	t.Helper()

	if err := m.PlayFromIndex(0); err != nil {
		t.Fatalf("PlayFromIndex(0): %v", err)
	}
	f.waitForPlayingWithSuccessor(t, queue[0].Path)
	waitForCorrelatedLoad(t, m, f)
	// The successor is only counted against the correlated entry once both the
	// append and the start-file have landed, in either order. Waiting for the
	// count settles that race, so the buffered events below are dispatched from
	// one known starting state.
	f.waitFor(t, "the prefetched successor to be counted against the correlated entry", func() bool {
		return gaplessSuccessorsOf(m) == 1
	})

	// Freeze the event socket: from here mpv keeps accepting loads and keeps
	// emitting, and the reader sees none of it.
	release := f.holdEvents(t)

	// Exactly as many selections as the old cap. Their start-files fill the
	// pending queue without overflowing it, and not one of them is read.
	for i := 1; i <= formerPendingLoadStartLimit; i++ {
		if err := m.PlayFromIndex(i); err != nil {
			t.Fatalf("PlayFromIndex(%d): %v", i, err)
		}
		f.waitForPlayingWithSuccessor(t, queue[i].Path)
	}

	// The last of them plays out, so the end-file of a superseded entry now sits
	// behind every one of those unread start-files.
	f.finishCurrent()

	// One more selection overflows the old cap and supersedes all of it. It runs
	// end to end — playlist replaced, generation bumped, window closed — and its
	// own successor is prefetched behind it.
	last := formerPendingLoadStartLimit + 1
	if err := m.PlayFromIndex(last); err != nil {
		t.Fatalf("PlayFromIndex(%d): %v", last, err)
	}
	f.waitForPlayingWithSuccessor(t, queue[last].Path)
	// The append reaches mpv before the tracking that describes it is recorded,
	// so the playlist holding two entries is not yet proof that the Manager has
	// a promotable successor. Wait for the tracking itself before reading the
	// state this scenario must find unchanged.
	f.waitFor(t, "the selection's own successor to be tracked", func() bool {
		got, gen := prefetchTracking(m)
		return got.index >= 0 && got.gen == gen
	})

	wantEntries := f.entriesSnapshot()
	wantPrefetch, _ := prefetchTracking(m)
	removalsBefore := f.count(isPlaylistRemove)
	replacesBefore := f.count(isLoadfileReplace)

	// Only now does the reader reach the discarded entries' events, in the order
	// mpv emitted them.
	drainHeldEvents(t, m, f, release)
	// The damage a promotion does lands in its background maintenance, after the
	// decision itself has returned.
	time.Sleep(150 * time.Millisecond)

	if got := currentIdx(m); got != last {
		t.Fatalf("currentIndex = %d, want %d: a buffered load start-file beyond the old cap "+
			"took a newer load's generation", got, last)
	}
	if got := f.playingPath(); got != queue[last].Path {
		t.Fatalf("mpv is playing %q, want the selected %q", got, queue[last].Path)
	}
	if got := f.entriesSnapshot(); !slices.Equal(got, wantEntries) {
		t.Fatalf("entries = %v, want %v: the selected row and its successor", got, wantEntries)
	}
	if got, _ := prefetchTracking(m); got != wantPrefetch {
		t.Fatalf("prefetch = %+v, want %+v: a superseded EOF consumed the selection's successor",
			got, wantPrefetch)
	}
	if got := f.count(isPlaylistRemove); got != removalsBefore {
		t.Fatalf("playlist-remove count = %d, want %d: the superseded EOF ran its completed-slot cleanup "+
			"against the selection's playlist", got, removalsBefore)
	}
	if got := f.count(isLoadfileReplace); got != replacesBefore {
		t.Fatalf("loadfile replace count = %d, want %d: the superseded EOF sent an advance of its own",
			got, replacesBefore)
	}
	if history, pos, _ := shuffleSnapshotForTest(m); len(history) > 1 || pos > 0 {
		t.Fatalf("shuffleHistory = %v pos = %d, want the reanchored selection alone: "+
			"a superseded EOF committed onto the new cycle", history, pos)
	}
	// The correlation must land on the generation the last selection loaded, or
	// the next genuine end-file would be discarded and the queue would stall.
	waitForCorrelatedLoad(t, m, f)
}

// TestEOF_BufferedOldEventAfterReplacementDoesNotMutateShuffle is the shuffle
// half of the same ordering. The traversal, not the visible index, is what a
// promotion moves here: committing the successor the discarded entry had
// resolved grafts an unplayed identity onto the cycle the selection reanchored,
// leaving a Previous entry that was never heard.
func TestEOF_BufferedOldEventAfterReplacementDoesNotMutateShuffle(t *testing.T) {
	m, f := newFakePlaylistManager(t, "eof-buffered-shuffle")
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
	waitForCorrelatedLoad(t, m, f)

	release := f.holdEvents(t)
	f.finishCurrent()

	// Reanchoring around index 1 resets the cycle; the identity the old entry
	// was on its way to must not be consumed by it.
	if err := m.PlayFromIndex(1); err != nil {
		t.Fatalf("PlayFromIndex(1): %v", err)
	}
	f.waitForEntries(t, paths(queue, 1, 3))

	drainHeldEvents(t, m, f, release)
	time.Sleep(150 * time.Millisecond)

	history, pos, _ := shuffleSnapshotForTest(m)
	if len(history) != 1 || pos != 0 {
		t.Fatalf("shuffleHistory = %v pos = %d, want the reanchored selection alone: "+
			"a buffered EOF committed onto the new cycle", history, pos)
	}
	if got := currentIdx(m); got != 1 {
		t.Fatalf("currentIndex = %d, want 1", got)
	}
	if got := f.playingPath(); got != queue[1].Path {
		t.Fatalf("mpv is playing %q, want the selected %q", got, queue[1].Path)
	}
	if got := f.entriesSnapshot(); !slices.Equal(got, paths(queue, 1, 3)) {
		t.Fatalf("entries = %v, want %v: the selected row and the successor its cycle resolves",
			got, paths(queue, 1, 3))
	}
}

// TestEOF_RejectedReplacementKeepsTheNextEOFLive is the other side of the
// correlation: a window that opens is not yet a replacement. mpv rejected the
// loadfile, so it never stopped playing the entry it had — that entry is still
// the live one, and its own end-file must still be acted on. A correlation that
// only ever moved forward would strand it one generation behind and turn a
// failed selection into a queue that stops at the end of the current track.
func TestEOF_RejectedReplacementKeepsTheNextEOFLive(t *testing.T) {
	m, f := newFakePlaylistManager(t, "eof-rejected-replace")
	queue := queueOf(t, 3)
	installQueue(m, queue, -1, true)
	runEventReader(t, m, f)

	if err := m.PlayFromIndex(0); err != nil {
		t.Fatalf("PlayFromIndex(0): %v", err)
	}
	f.waitForEntries(t, paths(queue, 0, 1))
	assertPrefetch(t, m, queue, 1)
	waitForCorrelatedLoad(t, m, f)

	// The selection is refused by mpv, which leaves its playlist — and so the
	// entry the next end-file will belong to — exactly as it was.
	f.failCommand("loadfile:replace", "device error")
	if err := m.PlayFromIndex(2); err == nil {
		t.Fatal("PlayFromIndex(2) succeeded; the fake was told to reject the load")
	}
	f.clearFailure("loadfile:replace")
	if got := currentIdx(m); got != 0 {
		t.Fatalf("currentIndex = %d, want 0: a rejected load must not move the queue position", got)
	}
	if got := f.playingPath(); got != queue[0].Path {
		t.Fatalf("mpv is playing %q, want the unchanged %q", got, queue[0].Path)
	}

	// The track it never stopped playing now ends, and the gapless successor
	// mpv already holds must be promoted as usual.
	f.finishCurrent()

	waitForCurrentIndex(t, m, 1)
	if got := f.playingPath(); got != queue[1].Path {
		t.Fatalf("mpv is playing %q, want the promoted successor %q", got, queue[1].Path)
	}
	f.waitForEntries(t, paths(queue, 1, 2))
	assertPrefetch(t, m, queue, 2)
}

// TestEOF_GaplessStartInsideARejectedReplacementStaysLive covers a window that
// loaded nothing while mpv moved into the prefetched successor by itself.
//
// That successor was appended behind the entry the window is superseding, so it
// belongs to the generation the window displaced rather than to the one the
// window opened — the load that would have justified the new generation never
// reached mpv. Handing the generation back on close is what keeps the two
// matched. Leaving it open would strand the correlation behind the live
// generation and discard the successor's own end-file, stalling the queue.
func TestEOF_GaplessStartInsideARejectedReplacementStaysLive(t *testing.T) {
	m, f := newFakePlaylistManager(t, "eof-gapless-in-window")
	queue := queueOf(t, 3)
	installQueue(m, queue, -1, true)
	runEventReader(t, m, f)

	if err := m.PlayFromIndex(0); err != nil {
		t.Fatalf("PlayFromIndex(0): %v", err)
	}
	f.waitForEntries(t, paths(queue, 0, 1))
	waitForCorrelatedLoad(t, m, f)

	// Park a selection so the window stays open, then let mpv reach the end of
	// the current entry inside it. Spending the successor expectation is what
	// proves the reader has dispatched that end-file from inside the window;
	// the start-file it read next cannot be waited on directly, because its
	// handler blocks on the command socket the parked loadfile is holding.
	gate, selected := parkReplacement(t, m, f, 2)
	f.finishCurrent()
	f.waitFor(t, "the superseded end-file to be dispatched inside the window", func() bool {
		return gaplessSuccessorsOf(m) == 0
	})

	// The selection is then refused, so the window closes having loaded nothing
	// — and the successor mpv is playing keeps matching the live generation.
	f.failCommand("loadfile:replace", "device error")
	gate.release()
	f.unblock("loadfile:replace")
	if err := <-selected; err == nil {
		t.Fatal("PlayFromIndex(2) succeeded; the fake was told to reject the load")
	}
	f.clearFailure("loadfile:replace")
	waitForCorrelatedLoad(t, m, f)

	// The successor now ends. Its end-file is the live one and must be acted on.
	f.finishCurrent()
	waitForCurrentIndex(t, m, 1)
	f.waitFor(t, "the queue to keep running past the successor", func() bool {
		return f.count(isAppendOf(queue[2].Path)) > 0
	})
}

// TestEOF_ReaderRestartAdoptsTheLiveLoadGeneration covers the correlation's blind
// spot: a load that lands while no event reader is scanning. Its start-file goes
// to a socket nobody is reading, so the stamp on the entry mpv is playing is left
// naming the generation the previous reader last saw a file start in — and every
// end-file from then on reads as superseded, stalling the queue for good. The
// reader that comes up next has to adopt what mpv is playing as its own.
//
// The adoption is driven directly rather than by starting a second reader: both
// readers would hold a live connection to the one fake socket and split the event
// stream between them. It is called at exactly this point in startEventReader,
// next to the TrackChangedMsg emitted for the same unobserved start-file.
func TestEOF_ReaderRestartAdoptsTheLiveLoadGeneration(t *testing.T) {
	m, f := newFakePlaylistManager(t, "eof-reader-restart")
	queue := queueOf(t, 3)
	installQueue(m, queue, -1, true)
	runEventReader(t, m, f)

	if err := m.PlayFromIndex(0); err != nil {
		t.Fatalf("PlayFromIndex(0): %v", err)
	}
	f.waitForEntries(t, paths(queue, 0, 1))
	assertPrefetch(t, m, queue, 1)
	waitForCorrelatedLoad(t, m, f)

	// A replacement opens and lands with the reader down — an mpv restart
	// reloading the current file, or a load deferred to the reconnect queue and
	// drained during activation. mpv is playing the new entry; nothing correlated
	// it, so the stamp is a generation behind.
	m.mu.Lock()
	m.loadGen++
	m.mu.Unlock()

	m.adoptCurrentLoad(m.loadStartBoundary())

	// The entry mpv is playing now ends. It is the live one, so the queue keeps
	// running: without the adoption its end-file looks superseded and is dropped.
	f.finishCurrent()
	waitForCurrentIndex(t, m, 1)
	if got := f.playingPath(); got != queue[1].Path {
		t.Fatalf("mpv is playing %q, want the promoted successor %q", got, queue[1].Path)
	}
	assertPrefetch(t, m, queue, 2)
}

// TestEOF_InitialActivationDropsUnobservedLoadStartExpectation covers the
// adoption path on a reader whose correlation has never been armed.
//
// activateConnection replays the commands captured during the outage and only
// then spawns the event reader, so a queued playlist replacement reaches mpv
// while nothing is scanning the event socket: the start-file it produces is
// broadcast to connections that do not exist yet, and the expectation recorded
// for it can never be answered. Retiring those expectations only when
// startedLoadGen is nonzero leaves this one in place — an initial reader's
// correlation is deliberately disarmed — and the next load's start-file then
// consumes the dead generation instead of its own. That load reads as
// superseded from the instant it starts, its end-file is discarded, and
// auto-advance stalls with mpv idle at the end of the track.
func TestEOF_InitialActivationDropsUnobservedLoadStartExpectation(t *testing.T) {
	m, f := newFakePlaylistManager(t, "eof-initial-reader-adopt")
	queue := queueOf(t, 5)
	installQueue(m, queue, -1, true)
	// Model mpv's broadcast: the replayed load's start-file must be genuinely
	// unobservable to the reader that comes up after it, not merely early.
	f.dropUnsubscribedEvents()

	// The manager is mid-activation — a client is installed but the gate is
	// still closed — so the selection is captured for replay rather than sent,
	// and no event reader exists to observe what replaying it will produce.
	m.mu.Lock()
	client := m.client
	m.activating = true
	m.cmdQueue = []func() error{}
	m.mu.Unlock()

	if err := m.PlayFromIndex(0); err != nil {
		t.Fatalf("PlayFromIndex(0): %v", err)
	}
	if got := f.count(isLoadfileReplace); got != 0 {
		t.Fatalf("loadfile replace count = %d, want 0: the load was meant to be queued for replay", got)
	}

	// Activation replays it against mpv, rebuilds the prefetch, and only then
	// starts the reader.
	m.activateConnection(m.ctx, client, nil)
	f.waitForEntries(t, paths(queue, 0, 1))
	f.waitForEventStream(t)

	// The expectation that replay left behind names a start-file this reader
	// cannot ever read, so the adoption has to retire it.
	f.waitFor(t, "the initial reader to retire the unobservable load expectation", func() bool {
		return len(pendingLoadStartsOf(m)) == 0
	})

	// A load performed now is the first one this reader can correlate, and it
	// must be stamped with its own generation rather than with the retired one.
	if err := m.PlayFromIndex(2); err != nil {
		t.Fatalf("PlayFromIndex(2): %v", err)
	}
	f.waitForEntries(t, paths(queue, 2, 3))
	waitForCorrelatedLoad(t, m, f)
	// Wait for the successor to be tracked and counted rather than merely
	// present in mpv's playlist: appendPrefetchLocked records the tracking only
	// after the command returns, and a promotion decided before that lands
	// would take the fallback advance instead.
	f.waitFor(t, "the selection's successor to be tracked as a gapless one", func() bool {
		got, gen := prefetchTracking(m)
		return got.index == 3 && got.gen == gen && gaplessSuccessorsOf(m) == 1
	})

	// Its end-file is therefore live: mpv moves into the prefetched successor
	// and the queue keeps running instead of stopping at the end of the track.
	f.finishCurrent()
	waitForCurrentIndex(t, m, 3)
	if got := f.playingPath(); got != queue[3].Path {
		t.Fatalf("mpv is playing %q, want the promoted successor %q", got, queue[3].Path)
	}
	f.waitForEntries(t, paths(queue, 3, 4))
	f.waitFor(t, "the promoted transition to refill the prefetch", func() bool {
		got, gen := prefetchTracking(m)
		return got.index == 4 && got.path == queue[4].Path && got.gen == gen
	})
}

// TestEOF_ReaderAdoptionPreservesObservableLoadExpectations is the deterministic
// regression for the other side of that adoption: the expectations it must not
// retire.
//
// activateConnection opens the command gate when its drain completes and only
// then spawns the event reader, so from the instant that reader's connection
// exists until the instant it adopts, the manager keeps accepting explicit
// selections — and every one of them emits a start-file to a socket this reader
// is already on the receiving end of. Retiring the whole pending queue discards
// those generations together with the prefix that really was unobservable.
//
// Two replacements in that window are what makes the loss visible. The first
// buffered start-file then finds the queue empty and falls back to the live
// generation, so the entry the second replacement discarded reads as the one mpv
// is playing: the end-file it emitted before it was discarded passes
// eofState.superseded, promotes the selection's own prefetch as if it were the
// successor that had just started, and runs the completed-slot cleanup against
// the selection's playlist — deleting the row mpv is actually playing.
//
// Both events belong to a superseded entry, so both must decide nothing: the
// last selection stands, and so do its prefetch, mpv's playlist, and the
// traversal position.
func TestEOF_ReaderAdoptionPreservesObservableLoadExpectations(t *testing.T) {
	t.Run("sequential", func(t *testing.T) {
		m, f := newUnwiredFakePlaylistManager(t, "eof-adopt-boundary-seq")
		queue := queueOf(t, 6)
		installQueue(m, queue, -1, true)
		readerAdoptionBoundaryScenario(t, m, f, queue)
	})

	// The shuffle half moves the traversal rather than the visible index: a
	// promotion here consumes an identity onto the cycle the last selection
	// reanchored, leaving a Previous entry that was never heard.
	t.Run("shuffled", func(t *testing.T) {
		m, f := newUnwiredFakePlaylistManager(t, "eof-adopt-boundary-shuf")
		queue := queueOf(t, 6)
		m.mu.Lock()
		m.gaplessEnabled = true
		m.mu.Unlock()
		buildShuffleState(t, m, queue, 0, reversePermute)
		readerAdoptionBoundaryScenario(t, m, f, queue)
	})
}

// readerAdoptionBoundaryScenario replays a load with no reader connected, admits
// two more once the reader's connection exists but before it has adopted, and
// then lets that reader read everything mpv emitted while it was parked.
//
// The manager it is given must still have its callbacks unwired: that is what
// holds startEventReader between its dial and its adoption, which is the window
// under test.
func readerAdoptionBoundaryScenario(t *testing.T, m *Manager, f *fakePlaylist, queue []QueueTrack) {
	t.Helper()

	// Model mpv's broadcast: the replayed load's start-file must be genuinely
	// unobservable to the reader that comes up after it, not merely early.
	f.dropUnsubscribedEvents()

	// The manager is mid-activation — a client is installed but the gate is
	// still closed — so this selection is captured for replay rather than sent.
	m.mu.Lock()
	client := m.client
	m.activating = true
	m.cmdQueue = []func() error{}
	m.mu.Unlock()

	if err := m.PlayFromIndex(0); err != nil {
		t.Fatalf("PlayFromIndex(0): %v", err)
	}
	if got := f.count(isLoadfileReplace); got != 0 {
		t.Fatalf("loadfile replace count = %d, want 0: the load was meant to be queued for replay", got)
	}

	// Activation replays it against mpv, rebuilds the prefetch, and spawns the
	// reader. With the callbacks unwired the reader connects and then parks, so
	// its event connection exists and it has adopted nothing.
	m.activateConnection(m.ctx, client, nil)
	f.waitForPlayingWithSuccessor(t, queue[0].Path)
	f.waitForEventStream(t)

	// Freeze the stream at the socket. Everything mpv emits from here is
	// observable to this reader — it is connected — and none of it is read yet,
	// which is exactly the state the adoption below has to reason about.
	release := f.holdEvents(t)

	// The first live replacement of the window. Its refill is refused so the
	// entry it starts ends alone: the end-file that follows is then the only
	// event between the two start-files, and each start-file answers exactly one
	// load.
	appendsBefore := f.count(isLoadfileAppend)
	f.failCommandOnce("loadfile:append-play", "mpv rejected the append")
	if err := m.PlayFromIndex(2); err != nil {
		t.Fatalf("PlayFromIndex(2): %v", err)
	}
	f.waitFor(t, "the first admitted replacement to attempt its refill", func() bool {
		return f.count(isLoadfileAppend) > appendsBefore
	})
	f.clearFailure("loadfile:append-play")
	if got := f.entriesSnapshot(); !slices.Equal(got, paths(queue, 2)) {
		t.Fatalf("entries = %v, want %v: the superseded entry must end without a successor",
			got, paths(queue, 2))
	}
	supersededGen := loadGenOf(m)

	// It plays out, so the end-file of an entry the next replacement is about to
	// discard now sits between the two unread start-files.
	f.finishCurrent()

	// The second live replacement supersedes all of it and prefetches its own
	// successor behind itself.
	if err := m.PlayFromIndex(4); err != nil {
		t.Fatalf("PlayFromIndex(4): %v", err)
	}
	f.waitForPlayingWithSuccessor(t, queue[4].Path)
	// The append reaches mpv before the tracking that describes it is recorded,
	// so the playlist holding two entries is not yet proof that the Manager has a
	// promotable successor. Wait for the tracking itself.
	f.waitFor(t, "the selection's own successor to be tracked", func() bool {
		got, gen := prefetchTracking(m)
		return got.index >= 0 && got.gen == gen
	})
	selectedGen := loadGenOf(m)

	wantEntries := f.entriesSnapshot()
	wantPrefetch, _ := prefetchTracking(m)
	removalsBefore := f.count(isPlaylistRemove)
	replacesBefore := f.count(isLoadfileReplace)
	if got := pendingLoadStartsOf(m); len(got) != 3 {
		t.Fatalf("pendingLoadStarts = %v, want the replayed load plus the two admitted since", got)
	}

	// Wiring the callbacks releases the parked reader into its adoption. The
	// stream is still frozen, so nothing consumes the queue behind it and the
	// length it settles on is the adoption's own decision.
	m.SetEventCallbacks(noopEventCallbacks())
	f.waitFor(t, "the reader to adopt the current load", func() bool {
		return len(pendingLoadStartsOf(m)) != 3
	})
	if got := pendingLoadStartsOf(m); !slices.Equal(got, []uint64{supersededGen, selectedGen}) {
		t.Fatalf("pendingLoadStarts = %v, want %v: adoption retired a generation whose start-file "+
			"this reader can still read", got, []uint64{supersededGen, selectedGen})
	}

	// Only now does the reader reach the buffered events, in the order mpv
	// emitted them.
	drainHeldEvents(t, m, f, release)
	// The damage a promotion does lands in its background maintenance, after the
	// decision itself has returned.
	time.Sleep(150 * time.Millisecond)

	if got := currentIdx(m); got != 4 {
		t.Fatalf("currentIndex = %d, want 4: a buffered load start-file took the newer load's generation", got)
	}
	if got := f.playingPath(); got != queue[4].Path {
		t.Fatalf("mpv is playing %q, want the selected %q", got, queue[4].Path)
	}
	if got := f.entriesSnapshot(); !slices.Equal(got, wantEntries) {
		t.Fatalf("entries = %v, want %v: the selected row and its successor", got, wantEntries)
	}
	if got, _ := prefetchTracking(m); got != wantPrefetch {
		t.Fatalf("prefetch = %+v, want %+v: a superseded EOF consumed the selection's successor",
			got, wantPrefetch)
	}
	if got := f.count(isPlaylistRemove); got != removalsBefore {
		t.Fatalf("playlist-remove count = %d, want %d: the superseded EOF ran its completed-slot cleanup "+
			"against the selection's playlist", got, removalsBefore)
	}
	if got := f.count(isLoadfileReplace); got != replacesBefore {
		t.Fatalf("loadfile replace count = %d, want %d: the superseded EOF sent an advance of its own",
			got, replacesBefore)
	}
	if history, pos, _ := shuffleSnapshotForTest(m); len(history) > 1 || pos > 0 {
		t.Fatalf("shuffleHistory = %v pos = %d, want the reanchored selection alone: "+
			"a superseded EOF committed onto the new cycle", history, pos)
	}
	// The correlation must land on the generation the last selection loaded, or
	// the next genuine end-file would be discarded and the queue would stall.
	waitForCorrelatedLoad(t, m, f)
}

// TestEOF_GaplessTransitionRemovesCompletedSlotBeforeAppending covers the happy
// path and the ordering inside it: mpv advances to the prefetched entry on its
// own, and the maintenance that follows removes the finished entry before
// appending the next one, so mpv is never asked to hold three entries and slot 1
// keeps naming the successor.
func TestEOF_GaplessTransitionRemovesCompletedSlotBeforeAppending(t *testing.T) {
	m, f := newFakePlaylistManager(t, "eof-order")
	queue := queueOf(t, 4)
	installQueue(m, queue, -1, true)
	runEventReader(t, m, f)

	if err := m.PlayFromIndex(0); err != nil {
		t.Fatalf("PlayFromIndex(0): %v", err)
	}
	f.waitForEntries(t, paths(queue, 0, 1))

	f.finishCurrent()

	waitForCurrentIndex(t, m, 1)
	f.waitForEntries(t, paths(queue, 1, 2))
	if got := f.playingPath(); got != queue[1].Path {
		t.Fatalf("mpv is playing %q, want the promoted successor %q", got, queue[1].Path)
	}
	assertPrefetch(t, m, queue, 2)

	removed := f.commandOrder(isPlaylistRemove)
	appended := f.commandOrder(isAppendOf(queue[2].Path))
	if removed < 0 {
		t.Fatalf("the completed slot was never removed; commands = %v", f.commands())
	}
	if appended < 0 {
		t.Fatalf("the successor was never appended; commands = %v", f.commands())
	}
	if removed > appended {
		t.Fatalf("playlist-remove at %d ran after the append at %d; the completed slot must go first",
			removed, appended)
	}
}

// TestEOF_CleanupFailureSkipsRefillAndRecoversOnNextEOF covers the failure
// branch. When mpv rejects the removal of the completed slot, Go's slot
// numbering no longer matches mpv's playlist, so nothing more may be appended
// into it. The transition stands (mpv really did move on), prefetch tracking
// stays clear, and the next EOF recovers through the serialized replacement
// instead of stacking another entry onto a playlist that is already wrong.
func TestEOF_CleanupFailureSkipsRefillAndRecoversOnNextEOF(t *testing.T) {
	m, f := newFakePlaylistManager(t, "eof-cleanup-fail")
	queue := queueOf(t, 4)
	installQueue(m, queue, -1, true)
	runEventReader(t, m, f)

	if err := m.PlayFromIndex(0); err != nil {
		t.Fatalf("PlayFromIndex(0): %v", err)
	}
	f.waitForEntries(t, paths(queue, 0, 1))
	appendsBefore := f.count(isLoadfileAppend)

	f.failCommand("playlist-remove", "mpv rejected the removal")
	f.finishCurrent()

	waitForCurrentIndex(t, m, 1)
	if got := f.playingPath(); got != queue[1].Path {
		t.Fatalf("mpv is playing %q, want %q", got, queue[1].Path)
	}

	// No refill: the playlist keeps the two entries it had, and nothing is
	// tracked as prefetched, so the next EOF cannot promote anything.
	time.Sleep(100 * time.Millisecond)
	if got := f.count(isLoadfileAppend); got != appendsBefore {
		t.Fatalf("append-play count = %d, want %d: a failed cleanup must not refill", got, appendsBefore)
	}
	if got := f.entriesSnapshot(); !slices.Equal(got, paths(queue, 0, 1)) {
		t.Fatalf("entries = %v, want %v", got, paths(queue, 0, 1))
	}
	assertPrefetch(t, m, queue, -1)

	// The next EOF takes the fallback, which replaces the playlist outright and
	// so repairs the entry mpv was left holding.
	f.clearFailure("playlist-remove")
	f.finishCurrent()

	waitForCurrentIndex(t, m, 2)
	f.waitForEntries(t, paths(queue, 2, 3))
	if got := f.playingPath(); got != queue[2].Path {
		t.Fatalf("mpv is playing %q, want %q", got, queue[2].Path)
	}
	assertPrefetch(t, m, queue, 3)
}

// TestEOF_ReplacementDuringMaintenanceKeepsLiveEntry covers a replacement that
// lands between the committed transition and its cleanup. The completed slot the
// maintenance was going to remove is gone — the replacement swept the whole
// playlist — so position 0 now holds the entry mpv is playing. Removing it
// would stop playback outright, which is the same silence the stall produces.
//
// The test holds the playlist lock to park the maintenance, which is what the
// real contention does when a replacement wins the lock first.
func TestEOF_ReplacementDuringMaintenanceKeepsLiveEntry(t *testing.T) {
	m, f := newFakePlaylistManager(t, "eof-replace-race")
	queue := queueOf(t, 4)
	installQueue(m, queue, -1, true)
	runEventReader(t, m, f)

	if err := m.PlayFromIndex(0); err != nil {
		t.Fatalf("PlayFromIndex(0): %v", err)
	}
	f.waitForEntries(t, paths(queue, 0, 1))
	removalsBefore := f.count(isPlaylistRemove)

	// Park every playlist transaction, then end the track: the transition is
	// committed on the event reader, but its maintenance cannot run yet.
	m.playlistMu.Lock()
	f.finishCurrent()
	waitForCurrentIndex(t, m, 1)

	// The replacement runs inside the window, through the same primitive an
	// explicit selection uses.
	if err := m.replaceLocked(queue[2], nil, func() func() { return m.commitIndexLocked(2) }); err != nil {
		m.playlistMu.Unlock()
		t.Fatalf("replaceLocked(queue[2]): %v", err)
	}
	m.playlistMu.Unlock()

	// The maintenance resumes against a playlist that has moved on: it must skip
	// the removal and leave the refill to put mpv back at two entries.
	f.waitForEntries(t, paths(queue, 2, 3))
	if got := f.playingPath(); got != queue[2].Path {
		t.Fatalf("mpv is playing %q, want %q", got, queue[2].Path)
	}
	waitForCurrentIndex(t, m, 2)
	if got := f.count(isPlaylistRemove); got != removalsBefore {
		t.Fatalf("playlist-remove count = %d, want %d: the superseded cleanup removed a live row",
			got, removalsBefore)
	}
	assertPrefetch(t, m, queue, 3)
}

// TestEOF_LatePrefetchAppendDoesNotCorruptTransition covers a prefetch that is
// still inside mpv when the track ends. The EOF finds nothing tracked, falls
// back to the traversal load, and the late append is then swept away by that
// load's replacement — leaving mpv and the Manager on the same entry rather than
// on the two different ones the append and the load each aimed at.
func TestEOF_LatePrefetchAppendDoesNotCorruptTransition(t *testing.T) {
	m, f := newFakePlaylistManager(t, "eof-late-append")
	queue := queueOf(t, 3)
	installQueue(m, queue, -1, true)
	runEventReader(t, m, f)

	gate := f.block(t, "loadfile:append-play")
	if err := m.PlayFromIndex(0); err != nil {
		t.Fatalf("PlayFromIndex(0): %v", err)
	}
	gate.waitArrived(t, "the prefetch append")

	// mpv reaches the end of its only entry while the append is still in flight.
	f.finishCurrent()
	if !f.idle() {
		t.Fatalf("expected an idle playlist; currentPos = %d", f.currentPos())
	}

	f.unblock("loadfile:append-play")

	waitForCurrentIndex(t, m, 1)
	f.waitForEntries(t, paths(queue, 1, 2))
	if got := f.playingPath(); got != queue[1].Path {
		t.Fatalf("mpv is playing %q, want %q", got, queue[1].Path)
	}
	assertPrefetch(t, m, queue, 2)
}

// TestEOF_EventReaderStaysResponsiveWhileMaintenanceBlocked pins the split
// between the decision and the work. The maintenance is parked inside mpv
// holding the playlist transaction, and the event reader must still dispatch the
// events that arrive behind it.
//
// The EOF is delivered on its own, without the start-file that normally follows
// a gapless transition: a start-file handler issues commands of its own, and the
// single command socket serializes those behind whatever the maintenance has in
// flight. What this test isolates is the event reader's own loop — it must not
// be the thing waiting on the playlist transaction.
func TestEOF_EventReaderStaysResponsiveWhileMaintenanceBlocked(t *testing.T) {
	m, f := newFakePlaylistManager(t, "eof-responsive")
	queue := queueOf(t, 3)
	installQueue(m, queue, -1, true)
	runEventReader(t, m, f)

	if err := m.PlayFromIndex(0); err != nil {
		t.Fatalf("PlayFromIndex(0): %v", err)
	}
	f.waitForEntries(t, paths(queue, 0, 1))

	gate := f.block(t, "playlist-remove")
	f.emit(eventLine("end-file", "eof"))

	// The decision itself happened on the event-reader goroutine, so the index
	// has already moved even though the maintenance is stuck.
	waitForCurrentIndex(t, m, 1)
	gate.waitArrived(t, "the completed-slot removal")

	f.emit(`{"event":"property-change","name":"pause","data":true}`)
	f.waitFor(t, "the event reader to dispatch a pause change", func() bool {
		m.mu.RLock()
		defer m.mu.RUnlock()
		return m.lastPaused
	})

	f.unblock("playlist-remove")
	f.waitForEntries(t, paths(queue, 1, 2))
}

// TestEOF_ConcurrentQueueShrinkRejectsStalePrefetch covers the queue mutation
// that lands between the prefetch and the EOF: the tracked successor no longer
// has a row, so promoting it would move currentIndex past the end of the live
// queue. The handler must reject it and report the exhaustion the shrunken queue
// really represents.
func TestEOF_ConcurrentQueueShrinkRejectsStalePrefetch(t *testing.T) {
	m, f := newFakePlaylistManager(t, "eof-shrink")
	queue := queueOf(t, 3)
	installQueue(m, queue, -1, true)
	ended := playbackEndedSignal(m)
	runEventReader(t, m, f)

	if err := m.PlayFromIndex(0); err != nil {
		t.Fatalf("PlayFromIndex(0): %v", err)
	}
	f.waitForEntries(t, paths(queue, 0, 1))
	assertPrefetch(t, m, queue, 1)

	// Shrink the queue without clearing the tracking, the way a mutation racing
	// the EOF handler leaves it.
	m.mu.Lock()
	m.queue = queue[:1]
	m.mu.Unlock()

	f.finishCurrent()

	select {
	case <-ended:
	case <-time.After(2 * time.Second):
		t.Fatal("OnPlaybackEnded did not fire after the prefetch was invalidated by a shrink")
	}
	if got := currentIdx(m); got != 0 {
		t.Fatalf("currentIndex = %d, want 0: a prefetch outside the live queue must not be promoted", got)
	}
}

// TestEOF_LoadFailureIsNotReportedAsQueueExhaustion is the false-exhaustion
// boundary. mpv rejects the fallback load, which says nothing about how many
// tracks are left; reporting it as the end of the queue would stop the TUI's
// playback toggle behind a queue that is still full.
func TestEOF_LoadFailureIsNotReportedAsQueueExhaustion(t *testing.T) {
	m, f := newFakePlaylistManager(t, "eof-load-failure")
	queue := queueOf(t, 3)
	installQueue(m, queue, 0, false)
	ended := playbackEndedSignal(m)
	runEventReader(t, m, f)

	f.failCommand("loadfile:replace", "device error")
	f.emit(`{"event":"end-file","reason":"eof"}`)

	select {
	case <-ended:
		t.Fatal("OnPlaybackEnded fired for a failed load; only a genuine traversal exhaustion may end playback")
	case <-time.After(200 * time.Millisecond):
	}
	if got := currentIdx(m); got != 0 {
		t.Fatalf("currentIndex = %d, want 0: a failed load must not move the queue position", got)
	}
}

// TestEOF_ExhaustedQueueEndsPlayback is the other half of that boundary: when
// the traversal really has nothing left, playback ends.
func TestEOF_ExhaustedQueueEndsPlayback(t *testing.T) {
	m, f := newFakePlaylistManager(t, "eof-exhausted")
	queue := queueOf(t, 2)
	installQueue(m, queue, 1, true)
	ended := playbackEndedSignal(m)
	runEventReader(t, m, f)

	f.emit(`{"event":"end-file","reason":"eof"}`)

	select {
	case <-ended:
	case <-time.After(2 * time.Second):
		t.Fatal("OnPlaybackEnded did not fire for a genuinely exhausted queue")
	}
}

// TestEOF_StopAfterCurrentLeavesPlaylistAlone verifies the stop-after-current
// path still short-circuits the whole transition: playback ends, and mpv is left
// idle rather than being loaded with a successor.
func TestEOF_StopAfterCurrentLeavesPlaylistAlone(t *testing.T) {
	m, f := newFakePlaylistManager(t, "eof-stop-after")
	queue := queueOf(t, 3)
	installQueue(m, queue, 0, true)
	ended := playbackEndedSignal(m)
	runEventReader(t, m, f)

	m.SetStopAfterCurrent(true)
	loadsBefore := f.count(isLoadfileReplace)
	f.emit(`{"event":"end-file","reason":"eof"}`)

	select {
	case <-ended:
	case <-time.After(2 * time.Second):
		t.Fatal("OnPlaybackEnded did not fire for stop-after-current")
	}
	time.Sleep(50 * time.Millisecond)
	if got := f.count(isLoadfileReplace); got != loadsBefore {
		t.Fatalf("loadfile replace count = %d, want %d: stop-after-current must not advance", got, loadsBefore)
	}
	if got := currentIdx(m); got != 0 {
		t.Fatalf("currentIndex = %d, want 0", got)
	}
	if m.StopAfterCurrent() {
		t.Error("stop-after-current is still set; it must fire exactly once")
	}
}

// TestEOF_ShuffleFallbackAdvancesTraversal covers the shuffle half of the
// fallback: with nothing promotable, the advance walks the existing traversal
// rather than reanchoring the cycle around the next visible index.
func TestEOF_ShuffleFallbackAdvancesTraversal(t *testing.T) {
	m, f := newFakePlaylistManager(t, "eof-shuffle-fallback")
	queue := queueOf(t, 4)
	buildShuffleState(t, m, queue, 0, reversePermute)
	runEventReader(t, m, f)

	// reversePermute over the three non-anchor entries leaves index 3 first in
	// the unseen pool, so that is where a traversal-aware advance must land —
	// not on index 1, which is where visible-order arithmetic would go.
	f.emit(`{"event":"end-file","reason":"eof"}`)

	waitForCurrentIndex(t, m, 3)
	f.waitFor(t, "mpv to play the shuffle successor", func() bool {
		return f.playingPath() == queue[3].Path
	})
}

// TestEOF_ShufflePrefetchPromotionCommitsHistory covers the shuffle half of the
// promotion: mpv transitions gaplessly into the identity the prefetch resolved,
// and shuffle history is advanced onto it only now that the transition happened.
func TestEOF_ShufflePrefetchPromotionCommitsHistory(t *testing.T) {
	m, f := newFakePlaylistManager(t, "eof-shuffle-promote")
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
	history, pos, _ := shuffleSnapshotForTest(m)
	if len(history) != 1 || pos != 0 {
		t.Fatalf("shuffleHistory = %v pos = %d, want the anchor alone: a prefetch must not consume history",
			history, pos)
	}

	f.finishCurrent()

	waitForCurrentIndex(t, m, 3)
	history, pos, _ = shuffleSnapshotForTest(m)
	if pos != 1 || len(history) != 2 {
		t.Fatalf("shuffleHistory = %v pos = %d, want the successor committed at position 1", history, pos)
	}
	if got := f.playingPath(); got != queue[3].Path {
		t.Fatalf("mpv is playing %q, want %q", got, queue[3].Path)
	}
}

// gaplessSuccessorsOf reads how many appended entries mpv is still expected to
// start on its own behind the file the event reader has correlated. The count is
// spent as the end-file of the entry they sit behind is dispatched, which makes
// it the one observable a test can wait on when the start-file that follows will
// block on the command socket.
func gaplessSuccessorsOf(m *Manager) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.gaplessSuccessors
}

// pendingLoadStartsOf reads the load generations still waiting for the
// start-file that answers them, in queue order. An entry the event reader can
// never be handed — one whose event was emitted before the reader connected — is
// a stale correlation that the next start-file would consume in its place; an
// entry retired even though its event is still coming is the same error
// mirrored, and shows up here as a generation that has gone missing.
func pendingLoadStartsOf(m *Manager) []uint64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	gens := make([]uint64, 0, len(m.pendingLoadStarts))
	for _, pending := range m.pendingLoadStarts {
		gens = append(gens, pending.gen)
	}
	return gens
}

// loadGenOf reads the live load generation, which after a completed replacement
// is the generation that replacement loaded.
func loadGenOf(m *Manager) uint64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.loadGen
}
