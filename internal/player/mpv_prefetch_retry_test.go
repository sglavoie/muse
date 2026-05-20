package player

import (
	"context"
	"sync"
	"testing"
)

// TestPrefetchNext_BailsWhenPlaylistRemoveFails verifies that prefetchNext does
// not append a new track when the playlist-remove command for the stale entry
// fails. Without this guard mpv's internal playlist would grow to three entries
// ([current, old-stale, new]) while Go tracking only accounts for two, causing
// the stale track to play instead of the intended one on the next gapless EOF.
func TestPrefetchNext_BailsWhenPlaylistRemoveFails(t *testing.T) {
	var (
		mu       sync.Mutex
		received [][]any
	)

	pathA := tempFile(t, "a*.flac")
	pathB := tempFile(t, "b*.flac")
	pathC := tempFile(t, "c*.flac")

	sock := shortSockPath(t, "prefetch-removefail")
	fakeServerWithHandler(t, sock, func(req map[string]any) map[string]any {
		if cmd, ok := req["command"].([]any); ok {
			mu.Lock()
			received = append(received, append([]any(nil), cmd...))
			mu.Unlock()

			if isPlaylistRemove(cmd) {
				return errorHandler("playlist-remove failed")(req)
			}
		}
		return successHandler(req)
	})

	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{
		socketPath:      sock,
		ctx:             ctx,
		cancel:          cancel,
		prefetchedIndex: -1,
	}
	t.Cleanup(func() { _ = m.Close() })

	client, err := NewIPCClient(sock)
	if err != nil {
		t.Fatalf("NewIPCClient: %v", err)
	}
	m.mu.Lock()
	m.client = client
	m.gaplessEnabled = true
	m.queue = []QueueTrack{
		{Path: pathA, Hash: "h0", QueueID: 1},
		{Path: pathB, Hash: "h1", QueueID: 2},
		{Path: pathC, Hash: "h2", QueueID: 3},
	}
	m.currentIndex = 1
	m.prefetchedIndex = 1
	m.prefetchedPath = pathB
	m.mu.Unlock()

	m.prefetchNext()

	// playlist-remove failed: tracking state must be unchanged and no append
	// must have been sent (appending without removing would desync mpv).
	m.mu.RLock()
	if m.prefetchedIndex != 1 {
		t.Errorf("prefetchedIndex = %d, want 1 (unchanged after remove failure)", m.prefetchedIndex)
	}
	if m.prefetchedPath != pathB {
		t.Errorf("prefetchedPath = %q, want %q (unchanged)", m.prefetchedPath, pathB)
	}
	m.mu.RUnlock()

	mu.Lock()
	defer mu.Unlock()
	for _, cmd := range received {
		if isLoadfileAppend(cmd) {
			t.Errorf("no loadfile append should be sent when playlist-remove fails; got %v", cmd)
		}
	}
}

func TestPrefetchNext_RetriesAfterReplacementAppendFailure(t *testing.T) {
	var (
		mu       sync.Mutex
		received [][]any
	)

	pathA := tempFile(t, "a*.flac")
	pathB := tempFile(t, "b*.flac")
	pathC := tempFile(t, "c*.flac")

	sock := shortSockPath(t, "prefetch-retry")
	failPathC := true
	fakeServerWithHandler(t, sock, func(req map[string]any) map[string]any {
		if cmd, ok := req["command"].([]any); ok {
			mu.Lock()
			received = append(received, append([]any(nil), cmd...))
			mu.Unlock()

			if isLoadfileAppend(cmd) {
				path, _ := cmd[1].(string)
				if path == pathC && failPathC {
					failPathC = false
					return errorHandler("load failed")(req)
				}
			}
		}
		return successHandler(req)
	})

	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{
		socketPath:      sock,
		ctx:             ctx,
		cancel:          cancel,
		prefetchedIndex: -1,
	}
	t.Cleanup(func() { _ = m.Close() })

	client, err := NewIPCClient(sock)
	if err != nil {
		t.Fatalf("NewIPCClient: %v", err)
	}
	m.mu.Lock()
	m.client = client
	m.gaplessEnabled = true
	m.queue = []QueueTrack{
		{Path: pathA, Hash: "h0", QueueID: 1},
		{Path: pathB, Hash: "h1", QueueID: 2},
		{Path: pathC, Hash: "h2", QueueID: 3},
	}
	m.currentIndex = 1
	m.prefetchedIndex = 1
	m.prefetchedPath = pathB
	m.mu.Unlock()

	m.prefetchNext()

	m.mu.RLock()
	if m.prefetchedIndex != -1 {
		t.Fatalf("prefetchedIndex after failed replacement = %d, want -1", m.prefetchedIndex)
	}
	if m.prefetchedPath != "" {
		t.Fatalf("prefetchedPath after failed replacement = %q, want empty", m.prefetchedPath)
	}
	m.mu.RUnlock()

	m.mu.Lock()
	m.currentIndex = 0
	m.mu.Unlock()

	m.prefetchNext()

	mu.Lock()
	defer mu.Unlock()

	var pathBAppends int
	for _, cmd := range received {
		if isLoadfileAppend(cmd) {
			path, _ := cmd[1].(string)
			if path == pathB {
				pathBAppends++
			}
		}
	}
	if pathBAppends != 1 {
		t.Fatalf("expected exactly 1 retry append for %s, got %d; commands: %v", pathB, pathBAppends, received)
	}
}
