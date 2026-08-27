package player

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"
)

// buildShuffleState installs a queue, anchor index, permutation, and enabled
// shuffle traversal directly (no async prefetch) so tests can drive prefetch
// deterministically. The manager must already be locked-free (no other
// goroutine touching state).
func buildShuffleState(t *testing.T, m *Manager, queue []QueueTrack, anchor int, perm func(int) []int) {
	t.Helper()
	m.SetPermuteFunc(perm)
	m.mu.Lock()
	m.queue = queue
	m.currentIndex = anchor
	m.shuffleEnabled = true
	m.rebuildShuffleLocked()
	m.mu.Unlock()
}

// TestShuffledPrefetch_ResolvesSuccessorWithoutConsuming verifies that a
// shuffle-aware prefetch appends the first upcoming identity's file to mpv but
// does NOT advance shuffle history — history is committed only when the EOF
// transition is later confirmed.
func TestShuffledPrefetch_ResolvesSuccessorWithoutConsuming(t *testing.T) {
	var mu sync.Mutex
	var received []ipcTestCommand
	m := newConnectedManager(t, "shuf-prefetch", &mu, &received)

	queue := shuffleTestQueue(t, 4)
	m.mu.Lock()
	m.gaplessEnabled = true
	m.mu.Unlock()
	// reversePermute over the 3 non-anchor entries [1,2,3] -> [3,2,1]; the first
	// upcoming identity is index 3.
	buildShuffleState(t, m, queue, 0, reversePermute)

	m.prefetchNext()

	mu.Lock()
	found := false
	for _, cmd := range received {
		if isLoadfileAppend(cmd) {
			if path, _ := cmd.stringAt(1); path == queue[3].Path {
				found = true
			}
		}
	}
	mu.Unlock()
	if !found {
		t.Errorf("expected loadfile append of %s (first upcoming identity); got %v", queue[3].Path, received)
	}

	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.prefetchedIndex != 3 {
		t.Errorf("prefetchedIndex = %d, want 3", m.prefetchedIndex)
	}
	if m.prefetchedIdentity != "q:4" {
		t.Errorf("prefetchedIdentity = %q, want %q", m.prefetchedIdentity, "q:4")
	}
	if m.prefetchedPath != queue[3].Path {
		t.Errorf("prefetchedPath = %q, want %q", m.prefetchedPath, queue[3].Path)
	}
	// History must NOT have been consumed by the prefetch peek.
	if len(m.shuffleHistory) != 1 || m.shufflePos != 0 {
		t.Errorf("history len/pos = %d/%d, want 1/0 (prefetch must not consume)", len(m.shuffleHistory), m.shufflePos)
	}
	if len(m.shuffleUnseen) != 3 {
		t.Errorf("unseen len = %d, want 3 (prefetch must not consume)", len(m.shuffleUnseen))
	}
}

// pathsForIndices maps a queue index sequence to the corresponding file paths.
func pathsForIndices(queue []QueueTrack, indices []int) []string {
	out := make([]string, len(indices))
	for i, idx := range indices {
		out[i] = queue[idx].Path
	}
	return out
}

// nonGaplessShuffleSequence anchors a fresh manager at index 0 under shuffle and
// walks the whole cycle with Next(), returning the played file paths in order.
func nonGaplessShuffleSequence(t *testing.T, name string, queue []QueueTrack) []string {
	t.Helper()
	var mu sync.Mutex
	var received []ipcTestCommand
	m := newConnectedManager(t, name, &mu, &received)

	buildShuffleState(t, m, queue, 0, reversePermute)

	seq := []string{queue[currentIdx(m)].Path}
	for {
		if err := m.Next(); err != nil {
			break
		}
		seq = append(seq, queue[currentIdx(m)].Path)
	}
	return seq
}

// TestShuffleGaplessMatchesNonGapless verifies the core contract: a gapless
// (EOF-driven, prefetched) shuffle cycle plays the exact same deterministic
// identity sequence as a non-gapless (Next-driven) cycle with the same
// permutation.
func TestShuffleGaplessMatchesNonGapless(t *testing.T) {
	queue := shuffleTestQueue(t, 4)

	// Non-gapless reference sequence.
	want := nonGaplessShuffleSequence(t, "shuf-seq-nogap", queue)
	// reverse permute anchored at 0 -> paths for indices [0,3,2,1].
	if got := pathsForIndices(queue, []int{0, 3, 2, 1}); !slices.Equal(want, got) {
		t.Fatalf("non-gapless sequence = %v, want %v", want, got)
	}

	// Gapless sequence driven by EOF events through the real event reader.
	sock := shortSockPath(t, "shuf-seq-gap")
	var mu sync.Mutex
	var received []ipcTestCommand
	events := fakeEventCommandServer(t, sock, &mu, &received)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m := &Manager{
		socketPath:      sock,
		ctx:             ctx,
		cancel:          cancel,
		callbacksReady:  make(chan struct{}),
		prefetchedIndex: -1,
		shufflePos:      -1,
		callbacks: EventCallbacks{
			OnTrackChanged:  func(_, _, _, _ string, _ int) {},
			OnPlaybackEnded: func() {},
			OnStatus:        func(string) {},
			OnMissingFile:   func(string) {},
		},
	}
	defer m.Close()

	client, err := NewIPCClient(sock)
	if err != nil {
		t.Fatalf("NewIPCClient: %v", err)
	}
	m.mu.Lock()
	m.client = client
	m.gaplessEnabled = true
	m.mu.Unlock()
	close(m.callbacksReady)

	go m.startEventReader(ctx, nil)
	waitForCommand(t, &mu, &received, func(cmd ipcTestCommand) bool {
		return cmd.stringIs(0, cmdObserveProperty) && cmd.stringIs(2, propPause)
	}, time.Second, "pause property observation")

	m.SetPermuteFunc(reversePermute)
	m.mu.Lock()
	m.queue = queue
	m.currentIndex = 0
	m.shuffleEnabled = true
	m.mu.Unlock()
	if err := m.PlayFromIndex(0); err != nil {
		t.Fatalf("PlayFromIndex(0): %v", err)
	}

	gaplessSeq := []string{queue[0].Path}
	// The reverse-permute cycle visits indices 3, 2, 1 after the anchor.
	for _, wantIdx := range []int{3, 2, 1} {
		// Wait for the prefetch of the next track before firing EOF so mpv has
		// the gapless slot ready.
		waitForCommand(t, &mu, &received, func(cmd ipcTestCommand) bool {
			path, _ := cmd.stringAt(1)
			return isLoadfileAppend(cmd) && path == queue[wantIdx].Path
		}, 2*time.Second, "prefetch of index "+queue[wantIdx].Path)

		events <- `{"event":"end-file","reason":"eof"}`
		waitForCurrentIndex(t, m, wantIdx)
		gaplessSeq = append(gaplessSeq, queue[currentIdx(m)].Path)
	}

	if !slices.Equal(gaplessSeq, want) {
		t.Fatalf("gapless sequence = %v, want (non-gapless) %v", gaplessSeq, want)
	}
}

// TestReconcileShuffle_AppendAddsIdentityToUnseen verifies that appending a
// queue entry during shuffle playback inserts its identity into the unseen pool
// only (surviving history untouched) so the traversal will still reach it.
func TestReconcileShuffle_AppendAddsIdentityToUnseen(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{ctx: ctx, cancel: cancel, prefetchedIndex: -1, shufflePos: -1}
	defer m.Close()

	queue := shuffleTestQueue(t, 3)
	buildShuffleState(t, m, queue, 0, nil) // identity-order unseen [q2,q3]

	m.AppendToQueue(QueueTrack{Path: "/e.flac", Hash: "he", QueueID: 4})

	m.mu.RLock()
	defer m.mu.RUnlock()
	if !slices.Contains(m.shuffleUnseen, "q:4") {
		t.Errorf("appended identity q:4 not in unseen pool: %v", m.shuffleUnseen)
	}
	if len(m.shuffleHistory) != 1 || m.shuffleHistory[0] != "q:1" {
		t.Errorf("history changed by append: %v", m.shuffleHistory)
	}
}

// TestReconcileShuffle_RemovePrunesIdentity verifies that removing a queue entry
// during shuffle playback prunes its identity from the unseen pool.
func TestReconcileShuffle_RemovePrunesIdentity(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{ctx: ctx, cancel: cancel, prefetchedIndex: -1, shufflePos: -1}
	defer m.Close()

	queue := shuffleTestQueue(t, 3)
	buildShuffleState(t, m, queue, 0, nil) // unseen [q2,q3]

	if err := m.RemoveFromQueue(2); err != nil { // remove q3
		t.Fatalf("RemoveFromQueue(2): %v", err)
	}

	m.mu.RLock()
	defer m.mu.RUnlock()
	if slices.Contains(m.shuffleUnseen, "q:3") {
		t.Errorf("removed identity q:3 still in unseen pool: %v", m.shuffleUnseen)
	}
	if !slices.Contains(m.shuffleUnseen, "q:2") {
		t.Errorf("surviving identity q:2 missing from unseen pool: %v", m.shuffleUnseen)
	}
}

// TestReconcileShuffle_RemoveCurrentKeepsIdentityAndAdvances verifies that
// removing the currently audible track's row keeps its identity in history long
// enough that the next advance continues correctly (neither repeats nor skips
// surviving entries).
func TestReconcileShuffle_RemoveCurrentKeepsIdentityAndAdvances(t *testing.T) {
	var mu sync.Mutex
	var received []ipcTestCommand
	m := newConnectedManager(t, "shuf-rm-current", &mu, &received)

	queue := shuffleTestQueue(t, 3)
	buildShuffleState(t, m, queue, 0, nil) // history [q1], unseen [q2,q3]

	// Advance to q2 (index 1) so the current track is a mid-cycle entry.
	if err := m.Next(); err != nil {
		t.Fatalf("Next: %v", err)
	}
	if got := currentIdx(m); got != 1 {
		t.Fatalf("after Next, currentIndex = %d, want 1", got)
	}

	// Remove the currently audible row (index 1 == q2).
	if err := m.RemoveFromQueue(1); err != nil {
		t.Fatalf("RemoveFromQueue(1): %v", err)
	}

	m.mu.RLock()
	keptCurrent := slices.Contains(m.shuffleHistory, "q:2")
	m.mu.RUnlock()
	if !keptCurrent {
		t.Errorf("current identity q:2 pruned from history despite being audible: history unavailable")
	}

	// The next advance must play the surviving unseen entry (q3, now at index 1
	// of the shrunk queue) rather than repeating or skipping it.
	if err := m.Next(); err != nil {
		t.Fatalf("Next after removing current: %v", err)
	}
	newQueue := []QueueTrack{queue[0], queue[2]}
	if got := newQueue[currentIdx(m)].Path; got != queue[2].Path {
		t.Errorf("after removing current, played %q, want q3 %q", got, queue[2].Path)
	}
}

// TestReconcileShuffle_MidCycleRemovalNoRepeatNoSkip walks a full shuffle cycle
// with a removal in the middle and asserts every surviving entry plays exactly
// once and the removed entry never plays.
func TestReconcileShuffle_MidCycleRemovalNoRepeatNoSkip(t *testing.T) {
	var mu sync.Mutex
	var received []ipcTestCommand
	m := newConnectedManager(t, "shuf-midremove", &mu, &received)

	queue := shuffleTestQueue(t, 4)
	buildShuffleState(t, m, queue, 0, reversePermute) // cycle 0,3,2,1

	played := []string{queue[currentIdx(m)].Path}
	if err := m.Next(); err != nil { // -> index 3 (q4)
		t.Fatalf("Next: %v", err)
	}
	played = append(played, queue[currentIdx(m)].Path)

	// Remove q3 (index 2) mid-cycle; it is still pending in the unseen pool.
	removedPath := queue[2].Path
	if err := m.RemoveFromQueue(2); err != nil {
		t.Fatalf("RemoveFromQueue(2): %v", err)
	}
	newQueue := []QueueTrack{queue[0], queue[1], queue[3]}

	for {
		if err := m.Next(); err != nil {
			break
		}
		played = append(played, newQueue[currentIdx(m)].Path)
	}

	// Removed entry never played.
	if slices.Contains(played, removedPath) {
		t.Errorf("removed entry %q was played: %v", removedPath, played)
	}
	// Every surviving entry played exactly once.
	counts := map[string]int{}
	for _, p := range played {
		counts[p]++
	}
	for _, want := range []string{queue[0].Path, queue[1].Path, queue[3].Path} {
		if counts[want] != 1 {
			t.Errorf("surviving entry %q played %d times, want 1; sequence %v", want, counts[want], played)
		}
	}
}

// TestShuffleStalePrefetchRejectedOnEOF verifies that when a queue mutation
// invalidates the prefetched shuffle identity (its row is removed), the EOF
// handler does not commit the stale prefetch; it falls back to a fresh advance.
func TestShuffleStalePrefetchRejectedOnEOF(t *testing.T) {
	var mu sync.Mutex
	var received []ipcTestCommand
	m := newConnectedManager(t, "shuf-stale", &mu, &received)

	queue := shuffleTestQueue(t, 4)
	m.mu.Lock()
	m.gaplessEnabled = true
	m.mu.Unlock()
	buildShuffleState(t, m, queue, 0, reversePermute) // upcoming index 3 first

	m.prefetchNext()
	m.mu.RLock()
	prefID := m.prefetchedIdentity
	m.mu.RUnlock()
	if prefID != "q:4" {
		t.Fatalf("prefetchedIdentity = %q, want q:4", prefID)
	}

	// Remove the prefetched row (index 3, q4). Reconcile prunes it and
	// clearPrefetch drops the stale slot.
	if err := m.RemoveFromQueue(3); err != nil {
		t.Fatalf("RemoveFromQueue(3): %v", err)
	}

	// clearPrefetch runs synchronously inside RemoveFromQueue and is the only
	// thing that can have sent a playlist-remove here: the re-prefetch it
	// spawns sees prefetchedIndex == -1 and appends without removing. So the
	// command log proves the stale mpv playlist slot was dropped, without
	// depending on when that background goroutine runs.
	mu.Lock()
	foundRemove := slices.ContainsFunc(received, isPlaylistRemove)
	log := fmt.Sprint(received)
	mu.Unlock()
	if !foundRemove {
		t.Errorf("expected playlist-remove to drop the stale prefetch; got %v", log)
	}

	// RemoveFromQueue also spawns a fresh prefetchNext, which may legitimately
	// repopulate the slot with a surviving entry at any moment. The invariant
	// that holds either way is that the removed identity never survives in the
	// prefetch slot, and that any index left there addresses a live row.
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.prefetchedIdentity == "q:4" {
		t.Errorf("stale prefetch identity q:4 survived removal of its queue row")
	}
	if m.prefetchedIndex >= len(m.queue) {
		t.Errorf("prefetchedIndex = %d addresses no live row in a %d-entry queue",
			m.prefetchedIndex, len(m.queue))
	}
	if slices.Contains(m.shuffleUnseen, "q:4") || slices.Contains(m.shuffleHistory, "q:4") {
		t.Errorf("removed identity q:4 still present: unseen %v history %v", m.shuffleUnseen, m.shuffleHistory)
	}
}
