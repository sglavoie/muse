package player

import (
	"sync"
	"testing"
)

func TestPrefetchNext_SkipsMissingFiles(t *testing.T) {
	var mu sync.Mutex
	var received []ipcTestCommand
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
			path, _ := cmd.stringAt(1)
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
	var received []ipcTestCommand
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
			path, _ := cmd.stringAt(1)
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
