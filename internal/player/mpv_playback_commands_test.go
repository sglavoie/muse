package player

import (
	"context"
	"sync"
	"testing"
	"time"

	"os"
)

func testPlaybackQueue(t *testing.T) []QueueTrack {
	t.Helper()

	makeFile := func(name string) string {
		dir := t.TempDir()
		path := dir + "/" + name
		if err := os.WriteFile(path, []byte{}, 0o600); err != nil {
			t.Fatalf("create temp file %s: %v", path, err)
		}
		return path
	}

	return []QueueTrack{
		{Path: makeFile("a.flac"), Hash: "h0", QueueID: 1},
		{Path: makeFile("b.flac"), Hash: "h1", QueueID: 2},
		{Path: makeFile("c.flac"), Hash: "h2", QueueID: 3},
	}
}

func TestManager_PlayerInterface(t *testing.T) {
	sock := shortSockPath(t, "mgr-player")

	// Start a fake mpv IPC server on a short path.
	fakeServer(t, sock)

	// Build a Manager directly to avoid the long path from t.TempDir().
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{
		socketPath: sock,
		ctx:        ctx,
		cancel:     cancel,
	}
	defer m.Close()

	// Manually connect the client (as pollUntilReady would do after Start).
	client, err := NewIPCClient(sock)
	if err != nil {
		t.Fatalf("NewIPCClient: %v", err)
	}
	m.mu.Lock()
	m.client = client
	m.mu.Unlock()

	// Next/Previous now use Go-side queue arithmetic, so populate the queue
	// with real temp files (os.Stat must succeed) and set currentIndex to 1
	// so both Next (-> 2) and Previous (-> 0) are in range.
	m.queue = testPlaybackQueue(t)
	m.currentIndex = 1

	for _, tc := range []struct {
		name string
		fn   func() error
	}{
		{"Play", m.Play},
		{"Pause", m.Pause},
		{"Stop", m.Stop},
		{"SeekRelative", func() error { return m.SeekRelative(5) }},
		{"SetVolumeRelative", func() error { return m.SetVolumeRelative(5) }},
		{"Next", m.Next},
		{"Previous", m.Previous},
	} {
		if err := tc.fn(); err != nil {
			t.Errorf("%s: unexpected error: %v", tc.name, err)
		}
	}
}

// TestManager_Stop_PauseThenSeek verifies that Stop() sends set_property pause
// true before seek 0 absolute, satisfying AC-bf424ea07c26cdccb9fd76f5e37c1d62.
func TestManager_Stop_PauseThenSeek(t *testing.T) {
	sock := shortSockPath(t, "stop-order")

	var mu sync.Mutex
	var received [][]any
	fakeServerWithHandler(t, sock, func(req map[string]any) map[string]any {
		if cmd, ok := req["command"].([]any); ok && len(cmd) > 0 {
			mu.Lock()
			received = append(received, cmd)
			mu.Unlock()
		}
		return successHandler(req)
	})

	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{
		socketPath: sock,
		ctx:        ctx,
		cancel:     cancel,
	}
	defer m.Close()

	client, err := NewIPCClient(sock)
	if err != nil {
		t.Fatalf("NewIPCClient: %v", err)
	}
	m.mu.Lock()
	m.client = client
	m.mu.Unlock()

	if err := m.Stop(); err != nil {
		t.Fatalf("Stop: unexpected error: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(received)
		mu.Unlock()
		if n >= 2 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(received) < 2 {
		t.Fatalf("expected at least 2 commands, got %d: %v", len(received), received)
	}

	pauseIdx := -1
	seekIdx := -1
	for i, args := range received {
		if len(args) >= 3 {
			if verb, _ := args[0].(string); verb == "set_property" {
				if prop, _ := args[1].(string); prop == "pause" {
					if val, _ := args[2].(bool); val {
						pauseIdx = i
					}
				}
			}
		}
		if len(args) >= 3 {
			if verb, _ := args[0].(string); verb == "seek" {
				if pos, _ := args[1].(float64); pos == 0 {
					if mode, _ := args[2].(string); mode == "absolute" {
						seekIdx = i
					}
				}
			}
		}
	}

	if pauseIdx == -1 {
		t.Fatalf("'set_property pause true' not found in received commands: %v", received)
	}
	if seekIdx == -1 {
		t.Fatalf("'seek 0 absolute' not found in received commands: %v", received)
	}
	if pauseIdx >= seekIdx {
		t.Errorf("expected pause (index %d) before seek (index %d); commands: %v", pauseIdx, seekIdx, received)
	}
}

// TestManager_PlaybackLatency verifies that Play, Pause, PlayFromIndex, Next,
// and Previous each complete within 1 second when the IPC socket is already
// connected to a fake server.
func TestManager_PlaybackLatency(t *testing.T) {
	sock := shortSockPath(t, "latency")
	fakeServer(t, sock)

	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{
		socketPath: sock,
		ctx:        ctx,
		cancel:     cancel,
	}
	defer m.Close()

	client, err := NewIPCClient(sock)
	if err != nil {
		t.Fatalf("NewIPCClient: %v", err)
	}
	m.mu.Lock()
	m.client = client
	m.mu.Unlock()

	// Populate the queue with real temp files so PlayFromIndex, Next, and
	// Previous can resolve entries without hitting ErrMissingFile.
	m.queue = testPlaybackQueue(t)
	m.currentIndex = 1

	limit := 1 * time.Second
	cases := []struct {
		name string
		fn   func() error
	}{
		{"Play", m.Play},
		{"Pause", m.Pause},
		{"PlayFromIndex", func() error { return m.PlayFromIndex(0) }},
		{"Next", m.Next},
		{"Previous", m.Previous},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			start := time.Now()
			if err := tc.fn(); err != nil {
				t.Fatalf("%s returned unexpected error: %v", tc.name, err)
			}
			if dur := time.Since(start); dur >= limit {
				t.Errorf("%s took %v, want < %v", tc.name, dur, limit)
			}
		})
	}
}
