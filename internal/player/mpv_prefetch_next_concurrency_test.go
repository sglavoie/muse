package player

import (
	"sync"
	"testing"
)

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
