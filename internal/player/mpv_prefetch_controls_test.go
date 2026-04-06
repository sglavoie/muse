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
