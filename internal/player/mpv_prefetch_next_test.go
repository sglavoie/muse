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
