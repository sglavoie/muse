package player

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestCleanStaleSocket_NoFile(t *testing.T) {
	path := shortSockPath(t, "stale-nofile")
	if err := cleanStaleSocket(path); err != nil {
		t.Fatalf("expected nil for missing socket, got %v", err)
	}
}

func TestCleanStaleSocket_StaleSocket(t *testing.T) {
	path := shortSockPath(t, "stale-orphan")

	// Create a listener then close it — the socket file remains.
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	ln.Close()

	if err := cleanStaleSocket(path); err != nil {
		t.Fatalf("expected nil (stale socket removed), got %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("stale socket file should have been removed")
	}
}

func TestCleanStaleSocket_LiveSocket(t *testing.T) {
	path := shortSockPath(t, "stale-live")

	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		ln.Close()
		os.Remove(path)
	}()

	// Accept connections so the dial in cleanStaleSocket succeeds.
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	if err := cleanStaleSocket(path); err == nil {
		t.Fatal("expected error for live socket, got nil")
	}
}

func TestNewManager_SocketPath(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", tmp)

	m, err := NewManager()
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	defer m.Close()

	want := filepath.Join(tmp, "muse", "mpv.sock")
	if m.SocketPath() != want {
		t.Fatalf("socket path = %q, want %q", m.SocketPath(), want)
	}
}

func TestManager_ErrNotConnected(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", tmp)

	m, err := NewManager()
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	defer m.Close()

	// Without calling Start (or before the socket is ready), all Player
	// methods must return ErrNotConnected.
	checks := []struct {
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
	}
	for _, tc := range checks {
		if err := tc.fn(); !errors.Is(err, ErrNotConnected) {
			t.Errorf("%s: want ErrNotConnected, got %v", tc.name, err)
		}
	}
}

func TestManager_QueueDrainOnConnect(t *testing.T) {
	sock := shortSockPath(t, "mgr-drain")

	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{
		socketPath: sock,
		ctx:        ctx,
		cancel:     cancel,
	}
	defer m.Close()

	// Collect status messages emitted by the manager.
	var statusMsgs []string
	m.SetStatusFunc(func(msg string) {
		statusMsgs = append(statusMsgs, msg)
	})

	// Simulate Start() by initialising the queue without launching mpv.
	m.mu.Lock()
	m.cmdQueue = []func() error{}
	m.mu.Unlock()

	// Queue Play and Pause before the server is ready.
	if err := m.Play(); err != nil {
		t.Fatalf("Play before ready: want nil (queued), got %v", err)
	}
	if err := m.Pause(); err != nil {
		t.Fatalf("Pause before ready: want nil (queued), got %v", err)
	}
	if len(statusMsgs) != 2 {
		t.Fatalf("expected 2 status messages, got %d: %v", len(statusMsgs), statusMsgs)
	}
	for _, msg := range statusMsgs {
		if msg != "Connecting to mpv…" {
			t.Errorf("unexpected status message %q", msg)
		}
	}

	// Track commands received by the fake server.
	var mu sync.Mutex
	var received []string
	fakeServerWithHandler(t, sock, func(req map[string]any) map[string]any {
		if cmd, ok := req["command"].([]any); ok && len(cmd) > 0 {
			mu.Lock()
			received = append(received, fmt.Sprintf("%v", cmd[0]))
			mu.Unlock()
		}
		return successHandler(req)
	})

	// pollUntilReady connects and drains the queue.
	go m.pollUntilReady(ctx)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(received)
		mu.Unlock()
		if n >= 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(received) < 2 {
		t.Fatalf("expected 2 queued commands replayed, got %d: %v", len(received), received)
	}
}

func TestManager_HealthCheckDetectsDisconnect(t *testing.T) {
	sock := shortSockPath(t, "hc-disc")

	os.Remove(sock)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close(); os.Remove(sock) })

	// connClosed is closed after the server deliberately drops the connection.
	connClosed := make(chan struct{})
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		// Handle two ping requests, then close the connection.
		dec := json.NewDecoder(conn)
		enc := json.NewEncoder(conn)
		for i := 0; i < 2; i++ {
			var req map[string]any
			if err := dec.Decode(&req); err != nil {
				break
			}
			_ = enc.Encode(successHandler(req))
		}
		conn.Close()
		close(connClosed)
	}()

	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{
		socketPath:   sock,
		ctx:          ctx,
		cancel:       cancel,
		disconnectCh: make(chan error, 1),
		pingInterval: 50 * time.Millisecond,
	}
	defer m.Close()

	client, err := NewIPCClient(sock)
	if err != nil {
		t.Fatalf("NewIPCClient: %v", err)
	}
	m.mu.Lock()
	m.client = client
	m.mu.Unlock()

	go m.healthCheck(ctx, client)

	// Wait for the server to close the connection.
	select {
	case <-connClosed:
	case <-time.After(2 * time.Second):
		t.Fatal("server did not close connection in time")
	}

	// The health-check goroutine must surface the disconnect within a few ping intervals.
	select {
	case gotErr := <-m.disconnectCh:
		if gotErr == nil {
			t.Error("expected non-nil disconnect error")
		}
	case <-time.After(time.Second):
		t.Fatal("health check did not detect disconnect within 1s")
	}

	m.mu.RLock()
	c := m.client
	m.mu.RUnlock()
	if c != nil {
		t.Error("expected m.client to be nil after disconnect")
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
	// so both Next (→ 2) and Previous (→ 0) are in range.
	makeFile := func(name string) string {
		dir := t.TempDir()
		path := dir + "/" + name
		if err := os.WriteFile(path, []byte{}, 0o600); err != nil {
			t.Fatalf("create temp file %s: %v", path, err)
		}
		return path
	}
	m.queue = []QueueTrack{
		{Path: makeFile("a.flac"), Hash: "h0", QueueID: 1},
		{Path: makeFile("b.flac"), Hash: "h1", QueueID: 2},
		{Path: makeFile("c.flac"), Hash: "h2", QueueID: 3},
	}
	m.currentIndex = 1 // Next→2, Previous→0 — both valid

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

// mockMarker records calls to MarkTrackMissing and MarkQueueEntryMissing.
type mockMarker struct {
	trackHash string
	queueID   int64
}

func (m *mockMarker) MarkTrackMissing(hash string) error {
	m.trackHash = hash
	return nil
}

func (m *mockMarker) MarkQueueEntryMissing(id int64) error {
	m.queueID = id
	return nil
}

func TestManager_PlayQueueEntry_MissingFile(t *testing.T) {
	sock := shortSockPath(t, "pqe-missing")

	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{
		socketPath: sock,
		ctx:        ctx,
		cancel:     cancel,
		cmdQueue:   []func() error{}, // mark as started
	}
	defer m.Close()

	var statusMsg string
	m.SetStatusFunc(func(msg string) { statusMsg = msg })

	// Connect the manager to a fake server so ipc() succeeds.
	fakeServer(t, sock)
	client, err := NewIPCClient(sock)
	if err != nil {
		t.Fatalf("NewIPCClient: %v", err)
	}
	m.mu.Lock()
	m.client = client
	m.mu.Unlock()

	marker := &mockMarker{}
	entry := QueueTrack{
		Path:    "/nonexistent/path/track.flac",
		Hash:    "abc123",
		QueueID: 42,
	}

	gotErr := m.PlayQueueEntry(entry, marker)
	if !errors.Is(gotErr, ErrMissingFile) {
		t.Fatalf("PlayQueueEntry: want ErrMissingFile, got %v", gotErr)
	}
	if marker.trackHash != "abc123" {
		t.Errorf("MarkTrackMissing called with %q, want %q", marker.trackHash, "abc123")
	}
	if marker.queueID != 42 {
		t.Errorf("MarkQueueEntryMissing called with %d, want 42", marker.queueID)
	}
	if statusMsg == "" {
		t.Error("expected a status message for the missing file, got none")
	}
}

func TestManager_PlayQueueEntry_PresentFile(t *testing.T) {
	sock := shortSockPath(t, "pqe-present")

	// Create a real temporary file to stat.
	tmp, err := os.CreateTemp("", "muse-test-*.flac")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	tmp.Close()
	t.Cleanup(func() { os.Remove(tmp.Name()) })

	// Track loadfile commands received by the fake server.
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
	m.cmdQueue = []func() error{}
	m.mu.Unlock()

	marker := &mockMarker{}
	entry := QueueTrack{
		Path:    tmp.Name(),
		Hash:    "deadbeef",
		QueueID: 7,
	}

	if err := m.PlayQueueEntry(entry, marker); err != nil {
		t.Fatalf("PlayQueueEntry: unexpected error: %v", err)
	}

	// Marker must not have been called.
	if marker.trackHash != "" || marker.queueID != 0 {
		t.Errorf("marker was called unexpectedly: hash=%q queueID=%d", marker.trackHash, marker.queueID)
	}

	// loadfile must have been sent to mpv.
	mu.Lock()
	defer mu.Unlock()
	for _, args := range received {
		if len(args) > 0 {
			if name, _ := args[0].(string); name == "loadfile" {
				return
			}
		}
	}
	t.Errorf("no loadfile command received; got %v", received)
}

func TestManager_PlayQueueEntry_NotConnectedQueues(t *testing.T) {
	sock := shortSockPath(t, "pqe-queue")

	// Create a real temporary file so the stat check passes.
	tmp, err := os.CreateTemp("", "muse-test-*.flac")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	tmp.Close()
	t.Cleanup(func() { os.Remove(tmp.Name()) })

	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{
		socketPath: sock,
		ctx:        ctx,
		cancel:     cancel,
		cmdQueue:   []func() error{}, // started but not connected
	}
	defer m.Close()

	marker := &mockMarker{}
	entry := QueueTrack{Path: tmp.Name(), Hash: "ff", QueueID: 1}

	// Should queue rather than fail when not yet connected.
	if err := m.PlayQueueEntry(entry, marker); err != nil {
		t.Fatalf("PlayQueueEntry before connect: want nil (queued), got %v", err)
	}

	m.mu.RLock()
	qlen := len(m.cmdQueue)
	m.mu.RUnlock()
	if qlen != 1 {
		t.Errorf("cmdQueue len = %d, want 1", qlen)
	}
}

// TestManager_AutoRestart verifies that on a detected IPC disconnect the
// manager restarts mpv, reconnects, sends a loadfile command with the
// correct start= offset, and emits a status bar notification.
func TestManager_AutoRestart(t *testing.T) {
	sock := shortSockPath(t, "auto-restart")

	// Phase 1: a fake server that handles a couple of pings then closes the
	// connection, simulating an mpv crash.
	os.Remove(sock)
	ln1, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen phase 1: %v", err)
	}
	t.Cleanup(func() { ln1.Close(); os.Remove(sock) })

	connClosed := make(chan struct{})
	go func() {
		conn, err := ln1.Accept()
		if err != nil {
			return
		}
		dec := json.NewDecoder(conn)
		enc := json.NewEncoder(conn)
		for i := 0; i < 2; i++ {
			var req map[string]any
			if err := dec.Decode(&req); err != nil {
				break
			}
			_ = enc.Encode(successHandler(req))
		}
		conn.Close()
		ln1.Close()
		close(connClosed)
	}()

	// Collect commands received by the phase 2 server after reconnect.
	var mu sync.Mutex
	var received [][]any

	// Collect status notifications.
	var statusMu sync.Mutex
	var statusMsgs []string

	rootCtx, rootCancel := context.WithCancel(context.Background())
	ctx, cancel := context.WithCancel(rootCtx)
	defer rootCancel()

	const (
		wantPath    = "/music/track.flac"
		wantElapsed = 42.5
	)

	m := &Manager{
		socketPath:    sock,
		rootCtx:       rootCtx,
		rootCancel:    rootCancel,
		ctx:           ctx,
		cancel:        cancel,
		disconnectCh:  make(chan error, 1),
		pingInterval:  50 * time.Millisecond,
		lastTrackPath: wantPath,
		lastElapsed:   wantElapsed,
	}
	m.statusFn = func(msg string) {
		statusMu.Lock()
		statusMsgs = append(statusMsgs, msg)
		statusMu.Unlock()
	}
	// launchFn starts the phase 2 fake server instead of a real mpv process.
	m.launchFn = func(_ context.Context, socketPath string) (*exec.Cmd, error) {
		fakeServerWithHandler(t, socketPath, func(req map[string]any) map[string]any {
			if cmd, ok := req["command"].([]any); ok && len(cmd) > 0 {
				mu.Lock()
				received = append(received, cmd)
				mu.Unlock()
			}
			return successHandler(req)
		})
		return nil, nil
	}

	// Manually connect the initial client (bypasses real mpv startup).
	client, err := NewIPCClient(sock)
	if err != nil {
		t.Fatalf("NewIPCClient: %v", err)
	}
	m.mu.Lock()
	m.client = client
	m.cmdQueue = []func() error{} // mark as started
	m.mu.Unlock()

	go m.healthCheck(ctx, client)
	go m.watchAndRestart()

	// Wait for phase 1 to close the connection.
	select {
	case <-connClosed:
	case <-time.After(2 * time.Second):
		t.Fatal("phase 1 server did not close in time")
	}

	// Wait for the loadfile command to arrive at the phase 2 server.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(received)
		mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	// Verify loadfile was sent with the correct start= offset.
	mu.Lock()
	defer mu.Unlock()

	wantStart := fmt.Sprintf("start=%g", wantElapsed)
	foundLoadfile := false
	for _, args := range received {
		if len(args) == 0 {
			continue
		}
		name, _ := args[0].(string)
		if name != "loadfile" {
			continue
		}
		foundLoadfile = true
		hasStart := false
		for _, arg := range args {
			if s, ok := arg.(string); ok && strings.Contains(s, wantStart) {
				hasStart = true
				break
			}
		}
		if !hasStart {
			t.Errorf("loadfile missing %s option; full args: %v", wantStart, args)
		}
		// Verify the path argument.
		if len(args) < 2 {
			t.Errorf("loadfile has too few args: %v", args)
		} else if p, _ := args[1].(string); p != wantPath {
			t.Errorf("loadfile path = %q, want %q", p, wantPath)
		}
	}
	if !foundLoadfile {
		t.Fatalf("no loadfile command received after restart; got %v", received)
	}

	// Verify a restart status notification was emitted.
	statusMu.Lock()
	defer statusMu.Unlock()
	for _, msg := range statusMsgs {
		if strings.Contains(msg, "restart") || strings.Contains(msg, "disconnected") {
			return
		}
	}
	t.Errorf("no restart status notification emitted; got %v", statusMsgs)
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

	// Wait for both commands to arrive.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(received)
		mu.Unlock()
		if n >= 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(received) < 2 {
		t.Fatalf("expected at least 2 commands, got %d: %v", len(received), received)
	}

	// Find set_property pause true and seek 0 absolute, asserting order.
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
	makeFile := func(name string) string {
		dir := t.TempDir()
		path := dir + "/" + name
		if err := os.WriteFile(path, []byte{}, 0o600); err != nil {
			t.Fatalf("create temp file %s: %v", name, err)
		}
		return path
	}
	m.queue = []QueueTrack{
		{Path: makeFile("a.flac"), Hash: "h0", QueueID: 1},
		{Path: makeFile("b.flac"), Hash: "h1", QueueID: 2},
		{Path: makeFile("c.flac"), Hash: "h2", QueueID: 3},
	}
	m.currentIndex = 1 // Next→2, Previous→0 — both valid

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
		tc := tc
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

// TestManager_RemoveFromQueue verifies the three index-adjustment cases for
// RemoveFromQueue: removal after, before, and at currentIndex.
func TestManager_RemoveFromQueue(t *testing.T) {
	makeQueue := func() []QueueTrack {
		return []QueueTrack{
			{Path: "/a.flac", Hash: "h0", QueueID: 1},
			{Path: "/b.flac", Hash: "h1", QueueID: 2},
			{Path: "/c.flac", Hash: "h2", QueueID: 3},
			{Path: "/d.flac", Hash: "h3", QueueID: 4},
		}
	}

	t.Run("remove after currentIndex", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		m := &Manager{ctx: ctx, cancel: cancel, queue: makeQueue(), currentIndex: 1}
		defer m.Close()

		if err := m.RemoveFromQueue(2); err != nil {
			t.Fatalf("RemoveFromQueue(2): unexpected error: %v", err)
		}
		if got := len(m.queue); got != 3 {
			t.Errorf("queue len = %d, want 3", got)
		}
		if m.currentIndex != 1 {
			t.Errorf("currentIndex = %d, want 1 (unchanged)", m.currentIndex)
		}
	})

	t.Run("remove before currentIndex", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		m := &Manager{ctx: ctx, cancel: cancel, queue: makeQueue(), currentIndex: 2}
		defer m.Close()

		if err := m.RemoveFromQueue(1); err != nil {
			t.Fatalf("RemoveFromQueue(1): unexpected error: %v", err)
		}
		if got := len(m.queue); got != 3 {
			t.Errorf("queue len = %d, want 3", got)
		}
		if m.currentIndex != 1 {
			t.Errorf("currentIndex = %d, want 1 (decremented)", m.currentIndex)
		}
	})

	t.Run("remove at currentIndex", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		m := &Manager{ctx: ctx, cancel: cancel, queue: makeQueue(), currentIndex: 2}
		defer m.Close()

		if err := m.RemoveFromQueue(2); err != nil {
			t.Fatalf("RemoveFromQueue(2): unexpected error: %v", err)
		}
		if got := len(m.queue); got != 3 {
			t.Errorf("queue len = %d, want 3", got)
		}
		// currentIndex stays at 2 so auto-advance (PlayFromIndex(2)) resolves
		// to what was previously index 3 (now at position 2 after removal).
		if m.currentIndex != 2 {
			t.Errorf("currentIndex = %d, want 2 (unchanged)", m.currentIndex)
		}
	})

	t.Run("remove last entry at currentIndex clamps index", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		q := []QueueTrack{
			{Path: "/a.flac", Hash: "h0", QueueID: 1},
			{Path: "/b.flac", Hash: "h1", QueueID: 2},
			{Path: "/c.flac", Hash: "h2", QueueID: 3},
		}
		m := &Manager{ctx: ctx, cancel: cancel, queue: q, currentIndex: 2}
		defer m.Close()

		if err := m.RemoveFromQueue(2); err != nil {
			t.Fatalf("RemoveFromQueue(2): unexpected error: %v", err)
		}
		if got := len(m.queue); got != 2 {
			t.Errorf("queue len = %d, want 2", got)
		}
		if m.currentIndex != 1 {
			t.Errorf("currentIndex = %d, want 1 (clamped to last entry)", m.currentIndex)
		}
	})

	t.Run("remove only entry resets currentIndex to 0", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		q := []QueueTrack{
			{Path: "/a.flac", Hash: "h0", QueueID: 1},
		}
		m := &Manager{ctx: ctx, cancel: cancel, queue: q, currentIndex: 0}
		defer m.Close()

		if err := m.RemoveFromQueue(0); err != nil {
			t.Fatalf("RemoveFromQueue(0): unexpected error: %v", err)
		}
		if got := len(m.queue); got != 0 {
			t.Errorf("queue len = %d, want 0", got)
		}
		if m.currentIndex != 0 {
			t.Errorf("currentIndex = %d, want 0", m.currentIndex)
		}
	})

	t.Run("out of range returns error", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		m := &Manager{ctx: ctx, cancel: cancel, queue: makeQueue(), currentIndex: 1}
		defer m.Close()

		if err := m.RemoveFromQueue(-1); err == nil {
			t.Error("RemoveFromQueue(-1): want error, got nil")
		}
		if err := m.RemoveFromQueue(4); err == nil {
			t.Error("RemoveFromQueue(4): want error, got nil")
		}
		if got := len(m.queue); got != 4 {
			t.Errorf("queue len = %d, want 4 (unmodified)", got)
		}
	})
}

// TestManager_ConnectDisconnectInfoLogs verifies that slog.Info is called with
// "mpv connected" when the IPC socket connects and "mpv disconnected" when the
// health-check goroutine detects a lost connection. Both messages must appear
// at Info level in the JSON log output.
func TestManager_ConnectDisconnectInfoLogs(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	prev := slog.Default()
	slog.SetDefault(logger)
	t.Cleanup(func() { slog.SetDefault(prev) })

	sock := shortSockPath(t, "conn-disc-log")

	// Phase 1: a fake server that handles two pings then closes the connection.
	os.Remove(sock)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close(); os.Remove(sock) })

	connClosed := make(chan struct{})
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		dec := json.NewDecoder(conn)
		enc := json.NewEncoder(conn)
		for i := 0; i < 2; i++ {
			var req map[string]any
			if err := dec.Decode(&req); err != nil {
				break
			}
			_ = enc.Encode(successHandler(req))
		}
		conn.Close()
		close(connClosed)
	}()

	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{
		socketPath:   sock,
		ctx:          ctx,
		cancel:       cancel,
		disconnectCh: make(chan error, 1),
		pingInterval: 50 * time.Millisecond,
	}
	defer m.Close()

	// Simulate Start(): mark as started, then poll until the socket connects.
	m.mu.Lock()
	m.cmdQueue = []func() error{}
	m.mu.Unlock()

	go m.pollUntilReady(ctx)

	// Wait for the health-check to detect the disconnect.
	select {
	case <-m.disconnectCh:
	case <-time.After(3 * time.Second):
		t.Fatal("disconnect was not detected within 3s")
	}

	// Also wait for connClosed to ensure the server side is done.
	select {
	case <-connClosed:
	case <-time.After(time.Second):
	}

	out := buf.String()
	if !strings.Contains(out, "mpv connected") {
		t.Errorf("expected 'mpv connected' in Info log output, got:\n%s", out)
	}
	if !strings.Contains(out, "mpv disconnected") {
		t.Errorf("expected 'mpv disconnected' in Info log output, got:\n%s", out)
	}
}

// TestManager_SetQueue_ClampsCurrentIndex verifies that SetQueue clamps
// currentIndex to the last valid index when the new queue is shorter than
// the old one, preventing out-of-bounds access from Next/Previous.
func TestManager_SetQueue_ClampsCurrentIndex(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{
		ctx:    ctx,
		cancel: cancel,
		queue: []QueueTrack{
			{Path: "/a.flac", Hash: "h0", QueueID: 1},
			{Path: "/b.flac", Hash: "h1", QueueID: 2},
			{Path: "/c.flac", Hash: "h2", QueueID: 3},
			{Path: "/d.flac", Hash: "h3", QueueID: 4},
		},
		currentIndex: 3, // last entry
	}
	defer m.Close()

	// Replace with a shorter queue — currentIndex must be clamped.
	m.SetQueue([]QueueTrack{
		{Path: "/x.flac", Hash: "hx", QueueID: 10},
		{Path: "/y.flac", Hash: "hy", QueueID: 11},
	})

	m.mu.RLock()
	idx := m.currentIndex
	qLen := len(m.queue)
	m.mu.RUnlock()

	if idx >= qLen {
		t.Fatalf("currentIndex = %d after SetQueue with %d entries; expected clamped to %d", idx, qLen, qLen-1)
	}
	if idx != 1 {
		t.Errorf("currentIndex = %d, want 1 (last valid index)", idx)
	}
}

// TestManager_SetQueue_EmptyQueue verifies that SetQueue resets currentIndex
// to 0 when the new queue is empty.
func TestManager_SetQueue_EmptyQueue(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{
		ctx:    ctx,
		cancel: cancel,
		queue: []QueueTrack{
			{Path: "/a.flac", Hash: "h0", QueueID: 1},
		},
		currentIndex: 0,
	}
	defer m.Close()

	m.SetQueue(nil)

	m.mu.RLock()
	idx := m.currentIndex
	m.mu.RUnlock()

	if idx != 0 {
		t.Errorf("currentIndex = %d after SetQueue(nil), want 0", idx)
	}
}

// TestManager_SetCurrentIndex verifies that SetCurrentIndex sets the index
// correctly and clamps out-of-range values.
func TestManager_SetCurrentIndex(t *testing.T) {
	makeManager := func(queueLen, startIdx int) *Manager {
		ctx, cancel := context.WithCancel(context.Background())
		q := make([]QueueTrack, queueLen)
		for i := range q {
			q[i] = QueueTrack{Path: fmt.Sprintf("/%d.flac", i), Hash: fmt.Sprintf("h%d", i), QueueID: int64(i + 1)}
		}
		return &Manager{ctx: ctx, cancel: cancel, queue: q, currentIndex: startIdx}
	}

	t.Run("valid index", func(t *testing.T) {
		m := makeManager(5, 0)
		defer m.Close()
		m.SetCurrentIndex(3)
		m.mu.RLock()
		got := m.currentIndex
		m.mu.RUnlock()
		if got != 3 {
			t.Errorf("currentIndex = %d, want 3", got)
		}
	})

	t.Run("negative clamped to 0", func(t *testing.T) {
		m := makeManager(5, 2)
		defer m.Close()
		m.SetCurrentIndex(-1)
		m.mu.RLock()
		got := m.currentIndex
		m.mu.RUnlock()
		if got != 0 {
			t.Errorf("currentIndex = %d, want 0", got)
		}
	})

	t.Run("too large clamped to last", func(t *testing.T) {
		m := makeManager(3, 0)
		defer m.Close()
		m.SetCurrentIndex(10)
		m.mu.RLock()
		got := m.currentIndex
		m.mu.RUnlock()
		if got != 2 {
			t.Errorf("currentIndex = %d, want 2", got)
		}
	})

	t.Run("empty queue sets 0", func(t *testing.T) {
		m := makeManager(0, 0)
		defer m.Close()
		m.SetCurrentIndex(5)
		m.mu.RLock()
		got := m.currentIndex
		m.mu.RUnlock()
		if got != 0 {
			t.Errorf("currentIndex = %d, want 0", got)
		}
	})
}

// TestManager_CloseProtectsCmdField verifies that Close reads m.cmd under the
// mutex, preventing a data race with concurrent handleRestart writes. Before
// the fix, both Close and handleRestart accessed m.cmd without synchronization.
func TestManager_CloseProtectsCmdField(t *testing.T) {
	rootCtx, rootCancel := context.WithCancel(context.Background())
	ctx, cancel := context.WithCancel(rootCtx)

	m := &Manager{
		socketPath:   "/tmp/muse-test-cmd-race.sock",
		rootCtx:      rootCtx,
		rootCancel:   rootCancel,
		ctx:          ctx,
		cancel:       cancel,
		disconnectCh: make(chan error, 1),
	}
	// Use a no-op launchFn that returns nil cmd (simulating test mode).
	m.launchFn = func(_ context.Context, _ string) (*exec.Cmd, error) {
		return nil, nil
	}

	// Start with a nil cmd (as if mpv was never launched or was in test mode).
	m.mu.Lock()
	m.cmd = nil
	m.cmdQueue = []func() error{}
	m.mu.Unlock()

	// Run Close and handleRestart concurrently. Before the fix this would
	// be detected by the race detector (go test -race) as a data race on m.cmd.
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		m.handleRestart()
	}()
	go func() {
		defer wg.Done()
		// Give handleRestart a moment to enter and write m.cmd.
		time.Sleep(5 * time.Millisecond)
		_ = m.Close()
	}()
	wg.Wait()
}

// TestManager_SetQueue_KeepsCurrentIndexWhenValid verifies that SetQueue does
// NOT change currentIndex when the new queue is large enough.
func TestManager_SetQueue_KeepsCurrentIndexWhenValid(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{
		ctx:    ctx,
		cancel: cancel,
		queue: []QueueTrack{
			{Path: "/a.flac", Hash: "h0", QueueID: 1},
			{Path: "/b.flac", Hash: "h1", QueueID: 2},
		},
		currentIndex: 1,
	}
	defer m.Close()

	// Replace with a queue that still has index 1 valid.
	m.SetQueue([]QueueTrack{
		{Path: "/x.flac", Hash: "hx", QueueID: 10},
		{Path: "/y.flac", Hash: "hy", QueueID: 11},
		{Path: "/z.flac", Hash: "hz", QueueID: 12},
	})

	m.mu.RLock()
	idx := m.currentIndex
	m.mu.RUnlock()

	if idx != 1 {
		t.Errorf("currentIndex = %d, want 1 (unchanged)", idx)
	}
}
