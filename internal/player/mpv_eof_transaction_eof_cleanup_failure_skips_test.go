package player

import (
	"slices"
	"testing"
	"time"
)

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

// startedLoadGenOf reads the generation the event reader stamped on the entry
// mpv is playing. It is what an end-file is judged against, so a value naming an
// older generation than the live one is exactly the state in which a genuine EOF
// is discarded as superseded.
func startedLoadGenOf(m *Manager) uint64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.startedLoadGen
}

// loadGenOf reads the live load generation, which after a completed replacement
// is the generation that replacement loaded.
func loadGenOf(m *Manager) uint64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.loadGen
}
