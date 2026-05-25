package player

import (
	"context"
	"sync"
	"testing"
)

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

func TestClearPrefetch_PreservesStateWhenPlaylistRemoveFails(t *testing.T) {
	sock := shortSockPath(t, "clear-pf-fail")
	fakeServerWithHandler(t, sock, func(req ipcTestRequest) map[string]any {
		cmd := req.Command
		if len(cmd) > 0 && cmd[0] == "playlist-remove" {
			return map[string]any{
				"error":      "failure",
				"request_id": req.RequestID,
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
	t.Cleanup(func() { m.Close() })

	client, err := NewIPCClient(sock)
	if err != nil {
		t.Fatalf("NewIPCClient: %v", err)
	}
	m.mu.Lock()
	m.client = client
	m.prefetchedIndex = 1
	m.prefetchedPath = "/b.flac"
	m.mu.Unlock()

	m.clearPrefetch()

	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.prefetchedIndex != 1 {
		t.Errorf("prefetchedIndex = %d, want 1 (unchanged on remove failure)", m.prefetchedIndex)
	}
	if m.prefetchedPath != "/b.flac" {
		t.Errorf("prefetchedPath = %q, want /b.flac", m.prefetchedPath)
	}
}
