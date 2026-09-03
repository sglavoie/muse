package player

import (
	"os"
	"slices"
	"sync"
	"testing"
	"time"
)

// These tests drive the playlist transaction layer against the stateful fake
// playlist, so every assertion is made about mpv's modelled playlist as well as
// the Go bookkeeping that mirrors it.

// TestPlaylistTxn_ReplaceThenPrefetchAppendsSuccessor is the happy path: an
// explicit selection replaces the playlist, and the follow-up prefetch appends
// the successor with append-play, leaving mpv holding exactly two entries and Go
// tracking slot 1.
func TestPlaylistTxn_ReplaceThenPrefetchAppendsSuccessor(t *testing.T) {
	m, f := newFakePlaylistManager(t, "txn-happy")
	queue := queueOf(t, 3)
	installQueue(m, queue, -1, true)

	if err := m.PlayFromIndex(0); err != nil {
		t.Fatalf("PlayFromIndex(0): %v", err)
	}
	f.waitForEntries(t, paths(queue, 0, 1))

	if got := f.currentPos(); got != 0 {
		t.Fatalf("mpv currentPos = %d, want 0", got)
	}
	if got := currentIdx(m); got != 0 {
		t.Fatalf("currentIndex = %d, want 0", got)
	}
	assertPrefetch(t, m, queue, 1)
	if got := f.count(isLoadfileAppend); got != 1 {
		t.Fatalf("append-play count = %d, want 1", got)
	}
}

// TestPlaylistTxn_ReplaceInvalidatesPrefetchWithoutRemoval verifies that a
// successful replacement is treated as authoritative prefetch invalidation: the
// tracking is dropped without a playlist-remove, because mpv has already
// discarded the prefetched entry and slot 1 would name a live row.
func TestPlaylistTxn_ReplaceInvalidatesPrefetchWithoutRemoval(t *testing.T) {
	m, f := newFakePlaylistManager(t, "txn-invalidate")
	queue := queueOf(t, 4)
	installQueue(m, queue, -1, true)

	if err := m.PlayFromIndex(0); err != nil {
		t.Fatalf("PlayFromIndex(0): %v", err)
	}
	f.waitForEntries(t, paths(queue, 0, 1))

	if err := m.PlayFromIndex(2); err != nil {
		t.Fatalf("PlayFromIndex(2): %v", err)
	}
	f.waitForEntries(t, paths(queue, 2, 3))

	if got := f.count(isPlaylistRemove); got != 0 {
		t.Fatalf("playlist-remove count = %d, want 0: a replacement must not remove slot 1", got)
	}
	if got := currentIdx(m); got != 2 {
		t.Fatalf("currentIndex = %d, want 2", got)
	}
	assertPrefetch(t, m, queue, 3)
}

// TestPlaylistTxn_ReplaceAndPrefetchCannotInterleave parks a replacement inside
// mpv and proves a concurrent prefetch cannot slip past it: nothing is appended
// while the replacement is in flight, and once both have run mpv's playlist and
// the Go tracking agree — no successor is retained that the replacement removed.
func TestPlaylistTxn_ReplaceAndPrefetchCannotInterleave(t *testing.T) {
	m, f := newFakePlaylistManager(t, "txn-barrier")
	queue := queueOf(t, 4)
	installQueue(m, queue, -1, true)

	if err := m.PlayFromIndex(0); err != nil {
		t.Fatalf("PlayFromIndex(0): %v", err)
	}
	f.waitForEntries(t, paths(queue, 0, 1))
	appendsBefore := f.count(isLoadfileAppend)

	gate := f.block(t, "loadfile:replace")
	replaced := make(chan error, 1)
	go func() { replaced <- m.PlayFromIndex(2) }()
	gate.waitArrived(t, "the replacement")

	// The replacement now holds the playlist transaction lock. A concurrent
	// best-effort prefetch must block on it rather than append into a playlist
	// that is being replaced.
	prefetched := make(chan struct{})
	go func() {
		m.prefetchNext()
		close(prefetched)
	}()

	select {
	case <-prefetched:
		t.Fatal("prefetch completed while a replacement held the playlist transaction")
	case <-time.After(100 * time.Millisecond):
	}
	if got := f.count(isLoadfileAppend); got != appendsBefore {
		t.Fatalf("append-play count = %d, want %d: prefetch reached mpv mid-replacement", got, appendsBefore)
	}

	f.unblock("loadfile:replace")
	if err := <-replaced; err != nil {
		t.Fatalf("PlayFromIndex(2): %v", err)
	}
	<-prefetched

	f.waitForEntries(t, paths(queue, 2, 3))
	if got := currentIdx(m); got != 2 {
		t.Fatalf("currentIndex = %d, want 2", got)
	}
	assertPrefetch(t, m, queue, 3)
	if got := f.count(isPlaylistRemove); got != 0 {
		t.Fatalf("playlist-remove count = %d, want 0", got)
	}
}

// TestPlaylistTxn_ReplaceFailureRestoresIndexAndPrefetch covers rollback: when
// mpv rejects the load, the index the transaction published ahead of it is
// restored and the existing prefetch — which mpv never discarded — is left
// intact so it stays promotable.
func TestPlaylistTxn_ReplaceFailureRestoresIndexAndPrefetch(t *testing.T) {
	m, f := newFakePlaylistManager(t, "txn-rollback")
	queue := queueOf(t, 3)
	installQueue(m, queue, -1, true)

	if err := m.PlayFromIndex(0); err != nil {
		t.Fatalf("PlayFromIndex(0): %v", err)
	}
	f.waitForEntries(t, paths(queue, 0, 1))

	f.failCommand("loadfile:replace", "device error")
	if err := m.PlayFromIndex(2); err == nil {
		t.Fatal("PlayFromIndex(2) should have failed")
	}

	if got := currentIdx(m); got != 0 {
		t.Fatalf("currentIndex = %d, want 0: a failed load must restore the prior index", got)
	}
	if got := f.entriesSnapshot(); !slices.Equal(got, paths(queue, 0, 1)) {
		t.Fatalf("entries = %v, want %v", got, paths(queue, 0, 1))
	}
	assertPrefetch(t, m, queue, 1)
}

// shuffleSnapshotForTest reads the shuffle traversal state under the lock.
func shuffleSnapshotForTest(m *Manager) (history []string, pos int, unseen []string) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return slices.Clone(m.shuffleHistory), m.shufflePos, slices.Clone(m.shuffleUnseen)
}

// TestPlaylistTxn_ShuffleAdvanceFailureRestoresTraversal covers the traversal
// half of rollback: a shuffle step is provisional until mpv accepts the load, so
// a rejected load must leave history, cursor, and unseen pool exactly as they
// were and let a retry re-attempt the same identity.
func TestPlaylistTxn_ShuffleAdvanceFailureRestoresTraversal(t *testing.T) {
	m, f := newFakePlaylistManager(t, "txn-shuffle-roll")
	queue := queueOf(t, 4)
	buildShuffleState(t, m, queue, 0, reversePermute)

	wantHistory, wantPos, wantUnseen := shuffleSnapshotForTest(m)

	f.failCommand("loadfile:replace", "device error")
	if err := m.Next(); err == nil {
		t.Fatal("Next() should have failed")
	}

	gotHistory, gotPos, gotUnseen := shuffleSnapshotForTest(m)
	if !slices.Equal(gotHistory, wantHistory) {
		t.Fatalf("shuffleHistory = %v, want %v", gotHistory, wantHistory)
	}
	if gotPos != wantPos {
		t.Fatalf("shufflePos = %d, want %d", gotPos, wantPos)
	}
	if !slices.Equal(gotUnseen, wantUnseen) {
		t.Fatalf("shuffleUnseen = %v, want %v", gotUnseen, wantUnseen)
	}
	if got := currentIdx(m); got != 0 {
		t.Fatalf("currentIndex = %d, want 0", got)
	}

	// The identity was not consumed, so the retry plays it.
	f.clearFailure("loadfile:replace")
	if err := m.Next(); err != nil {
		t.Fatalf("Next() after clearing the failure: %v", err)
	}
	if got := f.playingPath(); got != queue[currentIdx(m)].Path {
		t.Fatalf("mpv is playing %q, want the newly current entry %q", got, queue[currentIdx(m)].Path)
	}
}

// TestPlaylistTxn_ReconnectQueuedReplaceTakesPlaylistLock verifies reconnect
// ordering: a playlist mutation deferred to the reconnect queue re-acquires the
// playlist transaction lock when the drain replays it, so it cannot reach mpv
// while another transaction is open.
func TestPlaylistTxn_ReconnectQueuedReplaceTakesPlaylistLock(t *testing.T) {
	m, f := newFakePlaylistManager(t, "txn-reconnect")
	queue := queueOf(t, 2)
	installQueue(m, queue, -1, true)

	// Put the manager in the activation window: the client is installed, but
	// commands still queue behind the restore sequence.
	m.mu.Lock()
	m.activating = true
	m.cmdQueue = []func() error{}
	m.mu.Unlock()

	if err := m.PlayQueueEntry(queue[0], nil); err != nil {
		t.Fatalf("PlayQueueEntry: %v", err)
	}
	if got := f.count(isLoadfileReplace); got != 0 {
		t.Fatalf("loadfile replace count = %d, want 0: the load must be deferred", got)
	}

	// Hold the playlist transaction open; the replay must wait for it.
	m.playlistMu.Lock()
	drained := make(chan struct{})
	go func() {
		m.drainQueue()
		close(drained)
	}()

	select {
	case <-drained:
		t.Fatal("the reconnect drain replayed a playlist mutation without the playlist lock")
	case <-time.After(100 * time.Millisecond):
	}
	if got := f.count(isLoadfileReplace); got != 0 {
		t.Fatalf("loadfile replace count = %d, want 0 while the playlist lock is held", got)
	}

	m.playlistMu.Unlock()
	<-drained
	f.waitForEntries(t, paths(queue, 0))
}

// TestPlaylistTxn_PrefetchSuppressedDuringActivation verifies that best-effort
// prefetch does not bypass the reconnect gate. An append landing mid-restore
// would be wiped by the resume load, leaving Go tracking a successor mpv no
// longer holds.
func TestPlaylistTxn_PrefetchSuppressedDuringActivation(t *testing.T) {
	m, f := newFakePlaylistManager(t, "txn-gate")
	queue := queueOf(t, 3)
	installQueue(m, queue, -1, true)

	if err := m.PlayFromIndex(0); err != nil {
		t.Fatalf("PlayFromIndex(0): %v", err)
	}
	f.waitForEntries(t, paths(queue, 0, 1))
	m.clearPrefetch()
	f.waitForEntries(t, paths(queue, 0))
	appends := f.count(isLoadfileAppend)

	m.mu.Lock()
	m.activating = true
	m.mu.Unlock()

	m.prefetchNext()
	if got := f.count(isLoadfileAppend); got != appends {
		t.Fatalf("append-play count = %d, want %d: prefetch bypassed the activation gate", got, appends)
	}
	assertPrefetch(t, m, queue, -1)

	// Once activation completes the prefetch is rebuilt.
	m.mu.Lock()
	m.activating = false
	m.mu.Unlock()
	m.prefetchNext()
	f.waitForEntries(t, paths(queue, 0, 1))
	assertPrefetch(t, m, queue, 1)
}

// TestPlaylistTxn_DuplicatePrefetchSuppressed verifies that repeated prefetch
// calls resolving the same successor are no-ops: mpv is not asked to remove and
// re-append the entry it already holds.
func TestPlaylistTxn_DuplicatePrefetchSuppressed(t *testing.T) {
	m, f := newFakePlaylistManager(t, "txn-dup")
	queue := queueOf(t, 3)
	installQueue(m, queue, -1, true)

	if err := m.PlayFromIndex(0); err != nil {
		t.Fatalf("PlayFromIndex(0): %v", err)
	}
	f.waitForEntries(t, paths(queue, 0, 1))

	for range 3 {
		m.prefetchNext()
	}

	if got := f.count(isLoadfileAppend); got != 1 {
		t.Fatalf("append-play count = %d, want 1", got)
	}
	if got := f.count(isPlaylistRemove); got != 0 {
		t.Fatalf("playlist-remove count = %d, want 0", got)
	}
	if got := f.entriesSnapshot(); !slices.Equal(got, paths(queue, 0, 1)) {
		t.Fatalf("entries = %v, want %v", got, paths(queue, 0, 1))
	}
	assertPrefetch(t, m, queue, 1)
}

// TestPlaylistTxn_SkipsMissingFilesOnSelection verifies that a selection walks
// past entries that no longer exist on disk without touching mpv, and commits
// the index of the entry it actually loaded.
func TestPlaylistTxn_SkipsMissingFilesOnSelection(t *testing.T) {
	m, f := newFakePlaylistManager(t, "txn-missing-sel")
	queue := queueOf(t, 4)
	installQueue(m, queue, -1, false)
	for _, i := range []int{1, 2} {
		if err := os.Remove(queue[i].Path); err != nil {
			t.Fatalf("remove %q: %v", queue[i].Path, err)
		}
	}

	if err := m.PlayFromIndex(1); err != nil {
		t.Fatalf("PlayFromIndex(1): %v", err)
	}

	if got := currentIdx(m); got != 3 {
		t.Fatalf("currentIndex = %d, want 3", got)
	}
	if got := f.entriesSnapshot(); !slices.Equal(got, paths(queue, 3)) {
		t.Fatalf("entries = %v, want %v", got, paths(queue, 3))
	}
	if got := f.count(isLoadfileReplace); got != 1 {
		t.Fatalf("loadfile replace count = %d, want 1: missing entries must not reach mpv", got)
	}
}

// TestPlaylistTxn_PrefetchSkipsMissingFile verifies the same skipping in the
// prefetch path: the tracked successor is the first entry that still exists.
func TestPlaylistTxn_PrefetchSkipsMissingFile(t *testing.T) {
	m, f := newFakePlaylistManager(t, "txn-missing-pre")
	queue := queueOf(t, 3)
	installQueue(m, queue, -1, true)
	if err := os.Remove(queue[1].Path); err != nil {
		t.Fatalf("remove %q: %v", queue[1].Path, err)
	}

	if err := m.PlayFromIndex(0); err != nil {
		t.Fatalf("PlayFromIndex(0): %v", err)
	}
	f.waitForEntries(t, paths(queue, 0, 2))
	assertPrefetch(t, m, queue, 2)
}

// TestPlaylistTxn_AppendPlayResumesIdlePlaylist covers the stall the transaction
// layer exists to prevent. mpv reaches the end of its playlist before Go learns
// about it; the next prefetch must restart playback rather than park a track
// behind an idle playlist, which is what plain `append` would do.
func TestPlaylistTxn_AppendPlayResumesIdlePlaylist(t *testing.T) {
	m, f := newFakePlaylistManager(t, "txn-idle")
	queue := queueOf(t, 3)
	installQueue(m, queue, -1, true)

	if err := m.PlayFromIndex(0); err != nil {
		t.Fatalf("PlayFromIndex(0): %v", err)
	}
	f.waitForEntries(t, paths(queue, 0, 1))

	// mpv plays through both entries without Go observing either EOF.
	f.finishCurrent()
	f.finishCurrent()
	if !f.idle() {
		t.Fatalf("expected an idle playlist; currentPos = %d", f.currentPos())
	}

	// A queue mutation invalidates the successor, then prefetch resolves it
	// again — into a playlist that has gone idle.
	m.clearPrefetch()
	m.prefetchNext()

	if f.idle() {
		t.Fatal("append-play should have resumed playback on the idle playlist")
	}
	if got := f.playingPath(); got != queue[1].Path {
		t.Fatalf("playingPath = %q, want %q", got, queue[1].Path)
	}
	assertPrefetch(t, m, queue, 1)
}

// TestPlaylistTxn_ConcurrentMutationsHoldInvariants hammers every queue-owned
// playlist entry point from several goroutines at once. Beyond exercising the
// race detector it pins the invariants the transaction layer exists to
// guarantee: mpv never holds more than the current entry plus one prefetched
// successor, mpv's current entry is the one Go considers current, and a tracked
// prefetch always names mpv's slot 1 in the live playlist generation.
func TestPlaylistTxn_ConcurrentMutationsHoldInvariants(t *testing.T) {
	m, f := newFakePlaylistManager(t, "txn-race")
	queue := queueOf(t, 6)
	installQueue(m, queue, 0, true)

	const workers, iterations = 6, 25
	var wg sync.WaitGroup
	for w := range workers {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := range iterations {
				switch (w + i) % 6 {
				case 0:
					_ = m.PlayFromIndex((w + i) % len(queue))
				case 1:
					m.prefetchNext()
				case 2:
					m.clearPrefetch()
				case 3:
					_ = m.Next()
				case 4:
					_ = m.Previous()
				case 5:
					_ = m.RestartQueueCycle()
				}
			}
		}(w)
	}
	wg.Wait()

	// Holding the transaction lock quiesces every in-flight playlist mutation,
	// including the asynchronous prefetches each transaction schedules, so the
	// fake and the bookkeeping can be compared as one consistent state.
	m.playlistMu.Lock()
	defer m.playlistMu.Unlock()
	assertPlaylistInvariants(t, m, f, queue)
}

// assertPlaylistInvariants checks the properties the transaction layer
// guarantees whenever no transaction is open. The caller must hold m.playlistMu
// so mpv's playlist and the Go bookkeeping are read as one consistent state.
func assertPlaylistInvariants(t *testing.T, m *Manager, f *fakePlaylist, queue []QueueTrack) {
	t.Helper()
	entries := f.entriesSnapshot()
	assertCurrentEntry(t, m, entries, queue)
	assertPrefetchSlot(t, m, entries, queue)
}

// assertCurrentEntry checks that mpv holds the current entry plus at most one
// successor, and that the entry it is playing is the one currentIndex names.
func assertCurrentEntry(t *testing.T, m *Manager, entries []string, queue []QueueTrack) {
	t.Helper()
	if len(entries) < 1 || len(entries) > 2 {
		t.Fatalf("mpv playlist holds %d entries (%v), want 1 or 2", len(entries), entries)
	}
	current := currentIdx(m)
	if current < 0 || current >= len(queue) {
		t.Fatalf("currentIndex = %d, out of range [0, %d)", current, len(queue))
	}
	if entries[0] != queue[current].Path {
		t.Fatalf("mpv is playing %q but currentIndex %d names %q", entries[0], current, queue[current].Path)
	}
}

// assertPrefetchSlot checks that the tracked prefetch resolves in the queue, is
// stamped with the live playlist generation, and is exactly what mpv holds at
// slot 1 — or that nothing is tracked and mpv holds no successor.
func assertPrefetchSlot(t *testing.T, m *Manager, entries []string, queue []QueueTrack) {
	t.Helper()
	tracked, gen := prefetchTracking(m)
	if tracked.index < 0 {
		if len(entries) != 1 {
			t.Fatalf("nothing is tracked as prefetched but mpv holds %v", entries)
		}
		return
	}
	if tracked.gen != gen {
		t.Fatalf("prefetchedGen = %d, want playlistGen %d", tracked.gen, gen)
	}
	if tracked.index >= len(queue) || tracked.path != queue[tracked.index].Path {
		t.Fatalf("prefetch tracking %+v does not resolve in the queue", tracked)
	}
	if len(entries) != 2 || entries[1] != tracked.path {
		t.Fatalf("mpv playlist %v does not hold the tracked prefetch %q at slot 1", entries, tracked.path)
	}
}
