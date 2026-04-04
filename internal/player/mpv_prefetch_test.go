package player

import (
	"context"
	"net"
	"os"
	"slices"
	"sync"
	"testing"
	"time"
)

// tempFile creates a temporary file and returns its path.
// The file is removed via t.Cleanup.
func tempFile(t *testing.T, name string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), name)
	if err != nil {
		t.Fatalf("tempFile: %v", err)
	}
	f.Close()
	return f.Name()
}

// newConnectedManager creates a Manager with a fake IPC server and connected
// client. The received commands are appended to *received under mu.
func newConnectedManager(t *testing.T, name string, mu *sync.Mutex, received *[][]any) *Manager {
	t.Helper()
	sock := shortSockPath(t, name)
	fakeServerWithHandler(t, sock, func(req map[string]any) map[string]any {
		if cmd, ok := req["command"].([]any); ok {
			mu.Lock()
			*received = append(*received, cmd)
			mu.Unlock()
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
	m.mu.Unlock()
	return m
}

// waitForCommand waits until a command matching the predicate appears in
// received, or times out.
func waitForCommand(t *testing.T, mu *sync.Mutex, received *[][]any, match func([]any) bool, timeout time.Duration, desc string) {
	t.Helper()
	deadline := time.After(timeout)
	for {
		mu.Lock()
		if slices.ContainsFunc(*received, match) {
			mu.Unlock()
			return
		}
		mu.Unlock()
		select {
		case <-deadline:
			mu.Lock()
			t.Fatalf("timed out waiting for %s; got %v", desc, *received)
			mu.Unlock()
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func isLoadfileAppend(cmd []any) bool {
	if len(cmd) < 3 {
		return false
	}
	verb, _ := cmd[0].(string)
	mode, _ := cmd[2].(string)
	return verb == "loadfile" && mode == "append"
}

func isPlaylistRemove(cmd []any) bool {
	if len(cmd) < 2 {
		return false
	}
	verb, _ := cmd[0].(string)
	return verb == "playlist-remove"
}

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
	m.prefetchNext() // second call should be a no-op

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

func TestEndFileEOF_UsesPrefetchedPath(t *testing.T) {
	sock := shortSockPath(t, "eof-prefetch")

	connDone := make(chan struct{})

	os.Remove(sock)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close(); os.Remove(sock) })

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = conn.Write([]byte(`{"event":"end-file","reason":"eof"}` + "\n"))
		<-connDone
	}()
	defer close(connDone)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	trackChangedCh := make(chan int, 1)

	pathA := tempFile(t, "a*.flac")
	pathB := tempFile(t, "b*.flac")

	m := &Manager{
		socketPath:      sock,
		ctx:             ctx,
		cancel:          cancel,
		callbacksReady:  make(chan struct{}),
		gaplessEnabled:  true,
		prefetchedIndex: 1,
		prefetchedPath:  pathB,
		queue: []QueueTrack{
			{Path: pathA, Hash: "h0", QueueID: 1},
			{Path: pathB, Hash: "h1", QueueID: 2},
		},
		currentIndex: 0,
		callbacks: EventCallbacks{
			OnTrackChanged: func(_, _, _, _ string, idx int) {
				trackChangedCh <- idx
			},
			OnPlaybackEnded: func() {},
			OnStatus:        func(string) {},
			OnMissingFile:   func(string) {},
		},
	}
	close(m.callbacksReady)

	go m.startEventReader(ctx)

	// Wait for the event reader to process the end-file event.
	// The prefetch path should cause currentIndex to advance to 1
	// without sending a loadfile replace.
	time.Sleep(200 * time.Millisecond)

	m.mu.RLock()
	idx := m.currentIndex
	prefIdx := m.prefetchedIndex
	m.mu.RUnlock()

	if idx != 1 {
		t.Errorf("currentIndex = %d, want 1", idx)
	}
	if prefIdx != -1 {
		t.Errorf("prefetchedIndex = %d, want -1 (cleared after use)", prefIdx)
	}
}

// TestEndFileEOF_UsesPrefetchedPath_SkipsMissing verifies that when the
// prefetched track is at idx+2 (because idx+1 was missing on disk), the
// gapless transition still works: currentIndex advances to prefetchedIndex
// instead of falling through to the loadfile-replace fallback path.
func TestEndFileEOF_UsesPrefetchedPath_SkipsMissing(t *testing.T) {
	sock := shortSockPath(t, "eof-pf-skip")

	connDone := make(chan struct{})

	os.Remove(sock)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close(); os.Remove(sock) })

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = conn.Write([]byte(`{"event":"end-file","reason":"eof"}` + "\n"))
		<-connDone
	}()
	defer close(connDone)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pathA := tempFile(t, "a*.flac")
	pathC := tempFile(t, "c*.flac")

	m := &Manager{
		socketPath:     sock,
		ctx:            ctx,
		cancel:         cancel,
		callbacksReady: make(chan struct{}),
		gaplessEnabled: true,
		// idx+1 ("/missing.flac") was missing, so prefetchNext picked idx+2.
		prefetchedIndex: 2,
		prefetchedPath:  pathC,
		queue: []QueueTrack{
			{Path: pathA, Hash: "h0", QueueID: 1},
			{Path: "/missing.flac", Hash: "h1", QueueID: 2},
			{Path: pathC, Hash: "h2", QueueID: 3},
		},
		currentIndex: 0,
		callbacks: EventCallbacks{
			OnTrackChanged:  func(_, _, _, _ string, _ int) {},
			OnPlaybackEnded: func() {},
			OnStatus:        func(string) {},
			OnMissingFile:   func(string) {},
		},
	}
	close(m.callbacksReady)

	go m.startEventReader(ctx)

	// Wait for the event reader to process the end-file event.
	time.Sleep(200 * time.Millisecond)

	m.mu.RLock()
	idx := m.currentIndex
	prefIdx := m.prefetchedIndex
	m.mu.RUnlock()

	if idx != 2 {
		t.Errorf("currentIndex = %d, want 2 (should skip missing idx+1)", idx)
	}
	if prefIdx != -1 {
		t.Errorf("prefetchedIndex = %d, want -1 (cleared after use)", prefIdx)
	}
}

func TestEndFileEOF_FallbackWhenNoPrefetch(t *testing.T) {
	sock := shortSockPath(t, "eof-fallback")

	connDone := make(chan struct{})

	os.Remove(sock)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close(); os.Remove(sock) })

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = conn.Write([]byte(`{"event":"end-file","reason":"eof"}` + "\n"))
		<-connDone
	}()
	defer close(connDone)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	endedCh := make(chan struct{}, 1)

	m := &Manager{
		socketPath:      sock,
		ctx:             ctx,
		cancel:          cancel,
		callbacksReady:  make(chan struct{}),
		prefetchedIndex: -1,
		queue: []QueueTrack{
			{Path: "/nonexistent.flac", Hash: "h0", QueueID: 1},
		},
		currentIndex: 0,
		callbacks: EventCallbacks{
			OnPlaybackEnded: func() { endedCh <- struct{}{} },
			OnStatus:        func(string) {},
			OnMissingFile:   func(string) {},
		},
	}
	close(m.callbacksReady)

	go m.startEventReader(ctx)

	// Without prefetch, the fallback path (PlayFromIndex) should fire.
	// Since there's no valid next track, OnPlaybackEnded fires.
	select {
	case <-endedCh:
		// correct
	case <-time.After(2 * time.Second):
		t.Fatal("OnPlaybackEnded did not fire for fallback path")
	}
}

func TestSetStopAfterCurrent_ClearsPrefetch(t *testing.T) {
	var mu sync.Mutex
	var received [][]any
	m := newConnectedManager(t, "stop-clears-pf", &mu, &received)

	m.mu.Lock()
	m.prefetchedIndex = 1
	m.prefetchedPath = "/b.flac"
	m.mu.Unlock()

	m.SetStopAfterCurrent(true)

	// clearPrefetch is synchronous, so playlist-remove should be sent.
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

	// Verify loop-playlist=no was sent.
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

	// Allow goroutine to settle.
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

	// Pre-set prefetch state.
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

	// prefetchNext runs in a goroutine; wait for it.
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

	// RemoveFromQueue should invalidate prefetch.
	if err := m.RemoveFromQueue(2); err != nil {
		t.Fatalf("RemoveFromQueue(2): %v", err)
	}

	// Wait for playlist-remove to be sent.
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

// TestPrefetchNext_SkipsMissingFiles verifies that prefetchNext skips
// intermediate files that do not exist on disk and prefetches the first
// playable entry after the current index.
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

// TestPrefetchNext_ReplacesStale verifies that when the prefetched entry
// becomes stale (e.g. after a queue mutation changes the next track),
// prefetchNext removes the old entry and appends the new one.
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

	// Simulate that the next track changed to pathC (index 2).
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

// TestPrefetchNext_ConcurrentNoDuplicate verifies that two concurrent
// prefetchNext calls produce exactly one loadfile-append command, not two.
// Before the prefetchMu fix, both goroutines would read prefetchedIndex == -1
// and both append the same track, creating a duplicate playlist entry.
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

	// Launch two concurrent prefetchNext calls — the race scenario.
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
