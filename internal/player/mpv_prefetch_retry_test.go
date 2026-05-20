package player

import (
	"context"
	"sync"
	"testing"
)

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
