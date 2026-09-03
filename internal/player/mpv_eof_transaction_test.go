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
