package player

import (
	"sync"
	"testing"
	"time"
)

func TestSetStopAfterCurrent_ClearsPrefetch(t *testing.T) {
	var mu sync.Mutex
	var received []ipcTestCommand
	m := newConnectedManager(t, "stop-clears-pf", &mu, &received)

	m.mu.Lock()
	m.prefetchedIndex = 1
	m.prefetchedPath = "/b.flac"
	m.mu.Unlock()

	m.SetStopAfterCurrent(true)

	mu.Lock()
	defer mu.Unlock()
	found := false
	for _, cmd := range received {
		if isPlaylistRemove(cmd) {
			found = true
		}
	}
	if !found {
		t.Errorf("expected playlist-remove when stopAfterCurrent set; got %v", received)
	}

	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.prefetchedIndex != -1 {
		t.Errorf("prefetchedIndex = %d, want -1", m.prefetchedIndex)
	}
}

func TestSetRepeatAll_NoLoopPlaylist(t *testing.T) {
	var mu sync.Mutex
	var received []ipcTestCommand
	m := newConnectedManager(t, "repeat-all", &mu, &received)

	if err := m.SetRepeat("all"); err != nil {
		t.Fatalf("SetRepeat(all): %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	for _, args := range received {
		if len(args) >= 3 {
			verb, _ := args.stringAt(0)
			prop, _ := args.stringAt(1)
			val, _ := args.stringAt(2)
			if verb == "set_property" && prop == "loop-playlist" && val == "inf" {
				t.Errorf("SetRepeat(all) should NOT set loop-playlist=inf; got %v", received)
			}
		}
	}

	found := false
	for _, args := range received {
		if len(args) >= 3 {
			verb, _ := args.stringAt(0)
			prop, _ := args.stringAt(1)
			val, _ := args.stringAt(2)
			if verb == "set_property" && prop == "loop-playlist" && val == "no" {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("expected set_property loop-playlist no; got %v", received)
	}
}

func TestSetGapless_SetsPrefetchPlaylist(t *testing.T) {
	var mu sync.Mutex
	var received []ipcTestCommand
	m := newConnectedManager(t, "gap-prefetch", &mu, &received)

	if err := m.SetGapless(true); err != nil {
		t.Fatalf("SetGapless(true): %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	found := false
	for _, args := range received {
		if len(args) >= 3 {
			verb, _ := args.stringAt(0)
			prop, _ := args.stringAt(1)
			val, _ := args.stringAt(2)
			if verb == "set_property" && prop == "prefetch-playlist" && val == "yes" {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("expected set_property prefetch-playlist yes; got %v", received)
	}

	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.gaplessEnabled {
		t.Error("gaplessEnabled should be true")
	}
}

func TestSetGapless_DisableClearsPrefetch(t *testing.T) {
	var mu sync.Mutex
	var received []ipcTestCommand
	m := newConnectedManager(t, "gap-disable", &mu, &received)

	m.mu.Lock()
	m.prefetchedIndex = 1
	m.prefetchedPath = "/b.flac"
	m.mu.Unlock()

	if err := m.SetGapless(false); err != nil {
		t.Fatalf("SetGapless(false): %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	found := false
	for _, cmd := range received {
		if isPlaylistRemove(cmd) {
			found = true
		}
	}
	if !found {
		t.Errorf("expected playlist-remove when disabling gapless; got %v", received)
	}

	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.gaplessEnabled {
		t.Error("gaplessEnabled should be false")
	}
	if m.prefetchedIndex != -1 {
		t.Errorf("prefetchedIndex = %d, want -1", m.prefetchedIndex)
	}
}

func TestPlayFromIndex_TriggersPrefetch(t *testing.T) {
	var mu sync.Mutex
	var received []ipcTestCommand
	m := newConnectedManager(t, "pfi-prefetch", &mu, &received)

	pathA := tempFile(t, "a*.flac")
	pathB := tempFile(t, "b*.flac")

	m.mu.Lock()
	m.gaplessEnabled = true
	m.queue = []QueueTrack{
		{Path: pathA, Hash: "h0", QueueID: 1},
		{Path: pathB, Hash: "h1", QueueID: 2},
	}
	m.mu.Unlock()

	if err := m.PlayFromIndex(0); err != nil {
		t.Fatalf("PlayFromIndex(0): %v", err)
	}

	waitForCommand(t, &mu, &received, func(cmd ipcTestCommand) bool {
		if !isLoadfileAppend(cmd) {
			return false
		}
		path, _ := cmd.stringAt(1)
		return path == pathB
	}, 2*time.Second, "loadfile append for "+pathB)
}

// TestPlayFromIndex_ReplaceInvalidatesPrefetch verifies that PlayFromIndex
// relies on the `loadfile ... replace` itself to invalidate a stale prefetch:
// mpv's replace swaps the whole playlist, so the prefetched slot is already gone
// and a playlist-remove would delete the freshly loaded row instead. The tracking
// must therefore be dropped without an IPC removal, and the follow-up prefetch
// must re-append the successor.
func TestPlayFromIndex_ReplaceInvalidatesPrefetch(t *testing.T) {
	var mu sync.Mutex
	var received []ipcTestCommand
	m := newConnectedManager(t, "pfi-clrpf", &mu, &received)

	pathA := tempFile(t, "a*.flac")
	pathB := tempFile(t, "b*.flac")
	pathC := tempFile(t, "c*.flac")

	m.mu.Lock()
	m.gaplessEnabled = true
	m.prefetchedIndex = 2
	m.prefetchedPath = pathC
	m.queue = []QueueTrack{
		{Path: pathA, Hash: "h0", QueueID: 1},
		{Path: pathB, Hash: "h1", QueueID: 2},
		{Path: pathC, Hash: "h2", QueueID: 3},
	}
	m.currentIndex = 0
	m.mu.Unlock()

	if err := m.PlayFromIndex(1); err != nil {
		t.Fatalf("PlayFromIndex(1): %v", err)
	}

	// Wait for the async prefetchNext goroutine to complete.
	waitForCommand(t, &mu, &received, func(cmd ipcTestCommand) bool {
		path, _ := cmd.stringAt(1)
		return isLoadfileAppend(cmd) && path == pathC
	}, 2*time.Second, "re-prefetch append for "+pathC)

	mu.Lock()
	defer mu.Unlock()

	// No playlist-remove: the replace already emptied mpv's playlist, so
	// removing position 1 would target a row the replace just installed.
	for i, cmd := range received {
		if isPlaylistRemove(cmd) {
			t.Errorf("PlayFromIndex sent playlist-remove at %d; the replace already invalidated the prefetch: %v", i, received)
		}
	}

	m.mu.RLock()
	defer m.mu.RUnlock()
	// After PlayFromIndex(1), prefetchedIndex should be 2 (prefetched pathC
	// again for gapless), not stale.
	if m.prefetchedIndex != 2 {
		t.Errorf("prefetchedIndex = %d, want 2 (re-prefetched after the replace)", m.prefetchedIndex)
	}
	if m.prefetchedGen != m.playlistGen {
		t.Errorf("prefetchedGen = %d, want the live playlist generation %d", m.prefetchedGen, m.playlistGen)
	}
}

// TestRemoveFromQueue_ClearsPrefetchUnderMutex verifies that RemoveFromQueue
// clears prefetch state using clearPrefetch (which runs as a playlist
// transaction) rather than sending playlist-remove outside it. This prevents a race where
// a concurrent prefetchNext could append a new entry that the unprotected
// playlist-remove then incorrectly removes.
func TestRemoveFromQueue_ClearsPrefetchUnderMutex(t *testing.T) {
	var mu sync.Mutex
	var received []ipcTestCommand
	m := newConnectedManager(t, "rmq-pfmu", &mu, &received)

	pathA := tempFile(t, "a*.flac")
	pathB := tempFile(t, "b*.flac")
	pathC := tempFile(t, "c*.flac")

	m.mu.Lock()
	m.gaplessEnabled = true
	m.prefetchedIndex = 2
	m.prefetchedPath = pathC
	m.queue = []QueueTrack{
		{Path: pathA, Hash: "h0", QueueID: 1},
		{Path: pathB, Hash: "h1", QueueID: 2},
		{Path: pathC, Hash: "h2", QueueID: 3},
	}
	m.currentIndex = 0
	m.mu.Unlock()

	// Run RemoveFromQueue concurrently with prefetchNext to expose the
	// race if clearPrefetch is not used.
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_ = m.RemoveFromQueue(2)
	}()
	go func() {
		defer wg.Done()
		m.prefetchNext()
	}()
	wg.Wait()

	// After both complete, the prefetch state must be consistent:
	// prefetchedIndex is either -1 (nothing to prefetch after removal) or
	// points to a valid index with matching path.
	m.mu.RLock()
	idx := m.prefetchedIndex
	path := m.prefetchedPath
	qLen := len(m.queue)
	m.mu.RUnlock()

	if idx >= 0 {
		if idx >= qLen {
			t.Errorf("prefetchedIndex %d >= queue length %d", idx, qLen)
		} else {
			m.mu.RLock()
			actualPath := m.queue[idx].Path
			m.mu.RUnlock()
			if path != actualPath {
				t.Errorf("prefetchedPath = %q but queue[%d].Path = %q", path, idx, actualPath)
			}
		}
	}
}

// TestRemovePrefetch_NoSpuriousRemoveWhenAlreadyCleared verifies that
// removePrefetchLocked does not send cmdPlaylistRemove when prefetchedIndex has
// already been cleared to -1 by the end-file event handler between the moment
// it reads it and the moment it sends the IPC command. Before the
// fix, the RLock snapshot was taken, the lock was dropped, and then the IPC
// command was sent unconditionally — allowing a spurious remove that would
// corrupt mpv's internal playlist.
func TestRemovePrefetch_NoSpuriousRemoveWhenAlreadyCleared(t *testing.T) {
	var mu sync.Mutex
	var received []ipcTestCommand
	m := newConnectedManager(t, "clear-pf-race", &mu, &received)

	// Start with prefetchedIndex = -1 (simulates the end-file handler having
	// already cleared it before the removal runs).
	m.mu.Lock()
	m.prefetchedIndex = -1
	m.prefetchedPath = ""
	m.mu.Unlock()

	m.playlistMu.Lock()
	ok := m.removePrefetchLocked()
	m.playlistMu.Unlock()
	if !ok {
		t.Error("removePrefetchLocked() = false with nothing prefetched, want true")
	}

	mu.Lock()
	defer mu.Unlock()
	for _, cmd := range received {
		if isPlaylistRemove(cmd) {
			t.Errorf("removePrefetchLocked sent playlist-remove with prefetchedIndex=-1; got %v", received)
		}
	}
}

func TestQueueMutation_InvalidatesPrefetch(t *testing.T) {
	var mu sync.Mutex
	var received []ipcTestCommand
	m := newConnectedManager(t, "queue-mut", &mu, &received)

	m.mu.Lock()
	m.prefetchedIndex = 2
	m.prefetchedPath = "/c.flac"
	m.queue = []QueueTrack{
		{Path: "/a.flac", Hash: "h0", QueueID: 1},
		{Path: "/b.flac", Hash: "h1", QueueID: 2},
		{Path: "/c.flac", Hash: "h2", QueueID: 3},
	}
	m.currentIndex = 1
	m.mu.Unlock()

	if err := m.RemoveFromQueue(2); err != nil {
		t.Fatalf("RemoveFromQueue(2): %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	found := false
	for _, cmd := range received {
		if isPlaylistRemove(cmd) {
			found = true
		}
	}
	if !found {
		t.Errorf("expected playlist-remove after RemoveFromQueue; got %v", received)
	}

	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.prefetchedIndex != -1 {
		t.Errorf("prefetchedIndex = %d, want -1 after RemoveFromQueue", m.prefetchedIndex)
	}
}
