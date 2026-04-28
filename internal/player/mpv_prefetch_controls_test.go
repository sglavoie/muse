package player

import (
	"sync"
	"testing"
	"time"
)

func TestSetStopAfterCurrent_ClearsPrefetch(t *testing.T) {
	var mu sync.Mutex
	var received [][]any
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
	var received [][]any
	m := newConnectedManager(t, "repeat-all", &mu, &received)

	if err := m.SetRepeat("all"); err != nil {
		t.Fatalf("SetRepeat(all): %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	for _, args := range received {
		if len(args) >= 3 {
			verb, _ := args[0].(string)
			prop, _ := args[1].(string)
			val, _ := args[2].(string)
			if verb == "set_property" && prop == "loop-playlist" && val == "inf" {
				t.Errorf("SetRepeat(all) should NOT set loop-playlist=inf; got %v", received)
			}
		}
	}

	found := false
	for _, args := range received {
		if len(args) >= 3 {
			verb, _ := args[0].(string)
			prop, _ := args[1].(string)
			val, _ := args[2].(string)
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
	var received [][]any
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
			verb, _ := args[0].(string)
			prop, _ := args[1].(string)
			val, _ := args[2].(string)
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
	var received [][]any
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
	var received [][]any
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

	waitForCommand(t, &mu, &received, func(cmd []any) bool {
		if !isLoadfileAppend(cmd) {
			return false
		}
		path, _ := cmd[1].(string)
		return path == pathB
	}, 2*time.Second, "loadfile append for "+pathB)
}

// TestPlayFromIndex_ClearsExistingPrefetch verifies that PlayFromIndex sends
// playlist-remove to mpv when a prefetched entry exists, not just clearing the
// tracking state. Without this, mpv's internal playlist would accumulate stale
// entries across PlayFromIndex calls.
func TestPlayFromIndex_ClearsExistingPrefetch(t *testing.T) {
	var mu sync.Mutex
	var received [][]any
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
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	// A playlist-remove must have been sent to clear the stale prefetch
	// (pathC at index 2) before the loadfile replace for pathB.
	foundRemove := false
	for _, cmd := range received {
		if isPlaylistRemove(cmd) {
			foundRemove = true
			break
		}
	}
	if !foundRemove {
		t.Errorf("expected playlist-remove to clear stale prefetch; got %v", received)
	}

	m.mu.RLock()
	defer m.mu.RUnlock()
	// After PlayFromIndex(1), prefetchedIndex should be 2 (prefetched pathC
	// again for gapless), not stale.
	if m.prefetchedIndex != 2 {
		t.Errorf("prefetchedIndex = %d, want 2 (re-prefetched after clearing stale)", m.prefetchedIndex)
	}
}

// TestRemoveFromQueue_ClearsPrefetchUnderMutex verifies that RemoveFromQueue
// clears prefetch state using clearPrefetch (which holds prefetchMu) rather
// than sending playlist-remove outside the mutex. This prevents a race where
// a concurrent prefetchNext could append a new entry that the unprotected
// playlist-remove then incorrectly removes.
func TestRemoveFromQueue_ClearsPrefetchUnderMutex(t *testing.T) {
	var mu sync.Mutex
	var received [][]any
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

func TestQueueMutation_InvalidatesPrefetch(t *testing.T) {
	var mu sync.Mutex
	var received [][]any
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
