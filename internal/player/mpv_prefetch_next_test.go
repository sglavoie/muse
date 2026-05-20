package player

import (
	"sync"
	"testing"
)

func TestPrefetchNext_AppendsNextTrack(t *testing.T) {
	var mu sync.Mutex
	var received [][]any
	m := newConnectedManager(t, "prefetch-append", &mu, &received)

	pathA := tempFile(t, "a*.flac")
	pathB := tempFile(t, "b*.flac")

	m.mu.Lock()
	m.gaplessEnabled = true
	m.queue = []QueueTrack{
		{Path: pathA, Hash: "h0", QueueID: 1},
		{Path: pathB, Hash: "h1", QueueID: 2},
	}
	m.currentIndex = 0
	m.mu.Unlock()

	m.prefetchNext()

	mu.Lock()
	defer mu.Unlock()

	found := false
	for _, cmd := range received {
		if isLoadfileAppend(cmd) {
			path, _ := cmd[1].(string)
			if path == pathB {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("expected loadfile %s append; got %v", pathB, received)
	}

	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.prefetchedIndex != 1 {
		t.Errorf("prefetchedIndex = %d, want 1", m.prefetchedIndex)
	}
	if m.prefetchedPath != pathB {
		t.Errorf("prefetchedPath = %q, want %q", m.prefetchedPath, pathB)
	}
}

func TestPrefetchNext_Idempotent(t *testing.T) {
	var mu sync.Mutex
	var received [][]any
	m := newConnectedManager(t, "prefetch-idem", &mu, &received)

	pathA := tempFile(t, "a*.flac")
	pathB := tempFile(t, "b*.flac")

	m.mu.Lock()
	m.gaplessEnabled = true
	m.queue = []QueueTrack{
		{Path: pathA, Hash: "h0", QueueID: 1},
		{Path: pathB, Hash: "h1", QueueID: 2},
	}
	m.currentIndex = 0
	m.mu.Unlock()

	m.prefetchNext()
	m.prefetchNext()

	mu.Lock()
	defer mu.Unlock()

	count := 0
	for _, cmd := range received {
		if isLoadfileAppend(cmd) {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 loadfile append, got %d; commands: %v", count, received)
	}
}

func TestPrefetchNext_SkipsWhenGaplessDisabled(t *testing.T) {
	var mu sync.Mutex
	var received [][]any
	m := newConnectedManager(t, "prefetch-nogap", &mu, &received)

	pathA := tempFile(t, "a*.flac")
	pathB := tempFile(t, "b*.flac")

	m.mu.Lock()
	m.gaplessEnabled = false
	m.queue = []QueueTrack{
		{Path: pathA, Hash: "h0", QueueID: 1},
		{Path: pathB, Hash: "h1", QueueID: 2},
	}
	m.currentIndex = 0
	m.mu.Unlock()

	m.prefetchNext()

	mu.Lock()
	defer mu.Unlock()
	for _, cmd := range received {
		if isLoadfileAppend(cmd) {
			t.Errorf("should not prefetch when gapless disabled; got %v", cmd)
		}
	}
}

func TestPrefetchNext_SkipsWhenStopAfterCurrent(t *testing.T) {
	var mu sync.Mutex
	var received [][]any
	m := newConnectedManager(t, "prefetch-stop", &mu, &received)

	pathA := tempFile(t, "a*.flac")
	pathB := tempFile(t, "b*.flac")

	m.mu.Lock()
	m.gaplessEnabled = true
	m.stopAfterCurrent = true
	m.queue = []QueueTrack{
		{Path: pathA, Hash: "h0", QueueID: 1},
		{Path: pathB, Hash: "h1", QueueID: 2},
	}
	m.currentIndex = 0
	m.mu.Unlock()

	m.prefetchNext()

	mu.Lock()
	defer mu.Unlock()
	for _, cmd := range received {
		if isLoadfileAppend(cmd) {
			t.Errorf("should not prefetch when stopAfterCurrent; got %v", cmd)
		}
	}
}

func TestPrefetchNext_SkipsWhenLastTrack(t *testing.T) {
	var mu sync.Mutex
	var received [][]any
	m := newConnectedManager(t, "prefetch-last", &mu, &received)

	pathA := tempFile(t, "a*.flac")

	m.mu.Lock()
	m.gaplessEnabled = true
	m.queue = []QueueTrack{
		{Path: pathA, Hash: "h0", QueueID: 1},
	}
	m.currentIndex = 0
	m.mu.Unlock()

	m.prefetchNext()

	mu.Lock()
	defer mu.Unlock()
	for _, cmd := range received {
		if isLoadfileAppend(cmd) {
			t.Errorf("should not prefetch when at last track; got %v", cmd)
		}
	}
}

func TestClearPrefetch_SendsPlaylistRemove(t *testing.T) {
	var mu sync.Mutex
	var received [][]any
	m := newConnectedManager(t, "clear-pf", &mu, &received)

	m.mu.Lock()
	m.prefetchedIndex = 1
	m.prefetchedPath = "/b.flac"
	m.mu.Unlock()

	m.clearPrefetch()

	mu.Lock()
	defer mu.Unlock()

	found := false
	for _, cmd := range received {
		if isPlaylistRemove(cmd) {
			found = true
		}
	}
	if !found {
		t.Errorf("expected playlist-remove; got %v", received)
	}

	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.prefetchedIndex != -1 {
		t.Errorf("prefetchedIndex = %d, want -1", m.prefetchedIndex)
	}
}

func TestClearPrefetch_NoopWhenNothingPrefetched(t *testing.T) {
	var mu sync.Mutex
	var received [][]any
	m := newConnectedManager(t, "clear-noop", &mu, &received)

	m.clearPrefetch()

	mu.Lock()
	defer mu.Unlock()
	for _, cmd := range received {
		if isPlaylistRemove(cmd) {
			t.Errorf("should not send playlist-remove when nothing prefetched; got %v", cmd)
		}
	}
}

func TestPrefetchNext_SkipsMissingFiles(t *testing.T) {
	var mu sync.Mutex
	var received [][]any
	m := newConnectedManager(t, "prefetch-skipmiss", &mu, &received)

	pathA := tempFile(t, "a*.flac")
	pathC := tempFile(t, "c*.flac")

	m.mu.Lock()
	m.gaplessEnabled = true
	m.queue = []QueueTrack{
		{Path: pathA, Hash: "h0", QueueID: 1},
		{Path: "/nonexistent1.flac", Hash: "h1", QueueID: 2},
		{Path: "/nonexistent2.flac", Hash: "h2", QueueID: 3},
		{Path: pathC, Hash: "h3", QueueID: 4},
	}
	m.currentIndex = 0
	m.mu.Unlock()

	m.prefetchNext()

	mu.Lock()
	defer mu.Unlock()

	found := false
	for _, cmd := range received {
		if isLoadfileAppend(cmd) {
			path, _ := cmd[1].(string)
			if path == pathC {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("expected loadfile %s append (skipping missing); got %v", pathC, received)
	}

	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.prefetchedIndex != 3 {
		t.Errorf("prefetchedIndex = %d, want 3", m.prefetchedIndex)
	}
	if m.prefetchedPath != pathC {
		t.Errorf("prefetchedPath = %q, want %q", m.prefetchedPath, pathC)
	}
}

func TestPrefetchNext_ReplacesStale(t *testing.T) {
	var mu sync.Mutex
	var received [][]any
	m := newConnectedManager(t, "prefetch-stale", &mu, &received)

	pathA := tempFile(t, "a*.flac")
	pathB := tempFile(t, "b*.flac")
	pathC := tempFile(t, "c*.flac")

	m.mu.Lock()
	m.gaplessEnabled = true
	m.queue = []QueueTrack{
		{Path: pathA, Hash: "h0", QueueID: 1},
		{Path: pathB, Hash: "h1", QueueID: 2},
		{Path: pathC, Hash: "h2", QueueID: 3},
	}
	m.currentIndex = 0
	m.prefetchedIndex = 1
	m.prefetchedPath = pathB
	m.mu.Unlock()

	m.mu.Lock()
	m.queue[1].Path = "/now-missing.flac"
	m.prefetchedIndex = -1
	m.prefetchedPath = ""
	m.mu.Unlock()

	m.prefetchNext()

	mu.Lock()
	defer mu.Unlock()

	found := false
	for _, cmd := range received {
		if isLoadfileAppend(cmd) {
			path, _ := cmd[1].(string)
			if path == pathC {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("expected loadfile %s append; got %v", pathC, received)
	}

	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.prefetchedIndex != 2 {
		t.Errorf("prefetchedIndex = %d, want 2", m.prefetchedIndex)
	}
}

// TestPrefetchNext_NoopWhenCurrentIndexIsNegative verifies that prefetchNext
// does not append any track to mpv's playlist when currentIndex is -1 (no
// track is currently active), even with gapless enabled and a non-empty queue.
// Without this guard the loop would start at i=0 and spuriously pre-load the
// first queue entry.
func TestPrefetchNext_NoopWhenCurrentIndexIsNegative(t *testing.T) {
	var mu sync.Mutex
	var received [][]any
	m := newConnectedManager(t, "prefetch-noidx", &mu, &received)

	pathA := tempFile(t, "a*.flac")
	pathB := tempFile(t, "b*.flac")

	m.mu.Lock()
	m.gaplessEnabled = true
	m.queue = []QueueTrack{
		{Path: pathA, Hash: "h0", QueueID: 1},
		{Path: pathB, Hash: "h1", QueueID: 2},
	}
	m.currentIndex = -1
	m.mu.Unlock()

	m.prefetchNext()

	mu.Lock()
	defer mu.Unlock()
	for _, cmd := range received {
		if isLoadfileAppend(cmd) {
			t.Errorf("prefetchNext must not append when currentIndex is -1; got %v", cmd)
		}
	}

	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.prefetchedIndex != -1 {
		t.Errorf("prefetchedIndex = %d after no-op, want -1", m.prefetchedIndex)
	}
}

func TestPrefetchNext_ConcurrentNoDuplicate(t *testing.T) {
	var mu sync.Mutex
	var received [][]any
	m := newConnectedManager(t, "prefetch-race", &mu, &received)

	pathA := tempFile(t, "a*.flac")
	pathB := tempFile(t, "b*.flac")

	m.mu.Lock()
	m.gaplessEnabled = true
	m.queue = []QueueTrack{
		{Path: pathA, Hash: "h0", QueueID: 1},
		{Path: pathB, Hash: "h1", QueueID: 2},
	}
	m.currentIndex = 0
	m.mu.Unlock()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); m.prefetchNext() }()
	go func() { defer wg.Done(); m.prefetchNext() }()
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()

	var appendCount int
	for _, cmd := range received {
		if isLoadfileAppend(cmd) {
			appendCount++
		}
	}
	if appendCount != 1 {
		t.Errorf("expected exactly 1 loadfile append, got %d; commands: %v", appendCount, received)
	}
}
