package player

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

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
		socketPath:     sock,
		ctx:            ctx,
		cancel:         cancel,
		cmdQueue:       []func() error{},
		callbacksReady: make(chan struct{}),
	}
	defer m.Close()

	var statusMsg string
	m.SetEventCallbacks(EventCallbacks{OnStatus: func(msg string) { statusMsg = msg }})

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

// TestManager_PlayQueueEntry_InaccessibleFile verifies that PlayQueueEntry
// returns ErrMissingFile when os.Stat fails with a non-"not exist" error
// (e.g. permission denied), rather than falling through to load the file.
func TestManager_PlayQueueEntry_InaccessibleFile(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("test requires non-root to trigger permission errors")
	}

	sock := shortSockPath(t, "pqe-noperm")

	// Create a file inside a directory, then revoke the directory's execute
	// permission so os.Stat on the file returns a permission error.
	dir := t.TempDir()
	filePath := dir + "/track.flac"
	if err := os.WriteFile(filePath, []byte("data"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.Chmod(dir, 0o000); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) }) //nolint:gosec // restoring dir permissions requires 0o700

	// Verify that the file is genuinely inaccessible (permission error, not
	// "not exist") before testing the player code path.
	if _, err := os.Stat(filePath); err == nil {
		t.Fatal("expected os.Stat to fail on inaccessible file")
	} else if os.IsNotExist(err) {
		t.Fatalf("expected permission error, got not-exist: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{
		socketPath:     sock,
		ctx:            ctx,
		cancel:         cancel,
		cmdQueue:       []func() error{},
		callbacksReady: make(chan struct{}),
	}
	defer m.Close()

	var statusMsg string
	m.SetEventCallbacks(EventCallbacks{OnStatus: func(msg string) { statusMsg = msg }})

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
		Path:    filePath,
		Hash:    "perm-denied",
		QueueID: 99,
	}

	gotErr := m.PlayQueueEntry(entry, marker)
	if !errors.Is(gotErr, ErrMissingFile) {
		t.Fatalf("PlayQueueEntry: want ErrMissingFile for inaccessible file, got %v", gotErr)
	}
	if marker.trackHash != "perm-denied" {
		t.Errorf("MarkTrackMissing called with %q, want %q", marker.trackHash, "perm-denied")
	}
	if marker.queueID != 99 {
		t.Errorf("MarkQueueEntryMissing called with %d, want 99", marker.queueID)
	}
	if statusMsg == "" {
		t.Error("expected a status message for the inaccessible file, got none")
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
		cmdQueue:   []func() error{},
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
		for range 2 {
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

	var mu sync.Mutex
	var received [][]any

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
	m.callbacks.OnStatus = func(msg string) {
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
	m.cmdQueue = []func() error{}
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
		time.Sleep(200 * time.Millisecond)
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
		if len(args) < 2 {
			t.Errorf("loadfile has too few args: %v", args)
		} else if p, _ := args[1].(string); p != wantPath {
			t.Errorf("loadfile path = %q, want %q", p, wantPath)
		}
	}
	if !foundLoadfile {
		t.Fatalf("no loadfile command received after restart; got %v", received)
	}

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

func TestManager_SetCurrentPosition(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{ctx: ctx, cancel: cancel}
	defer m.Close()

	m.SetCurrentPosition("/music/track.flac", 99.5)

	m.mu.RLock()
	gotPath := m.lastTrackPath
	gotElapsed := m.lastElapsed
	m.mu.RUnlock()

	if gotPath != "/music/track.flac" {
		t.Errorf("lastTrackPath = %q, want /music/track.flac", gotPath)
	}
	if gotElapsed != 99.5 {
		t.Errorf("lastElapsed = %f, want 99.5", gotElapsed)
	}
}

func TestManager_ElapsedPosition_Connected(t *testing.T) {
	sock := shortSockPath(t, "elapsed")

	fakeServerWithHandler(t, sock, func(req map[string]any) map[string]any {
		cmd, _ := req["command"].([]any)
		if len(cmd) >= 2 {
			prop, _ := cmd[1].(string)
			switch prop {
			case "time-pos":
				return map[string]any{
					"error":      "success",
					"data":       42.5,
					"request_id": req["request_id"],
				}
			case "duration":
				return map[string]any{
					"error":      "success",
					"data":       180.0,
					"request_id": req["request_id"],
				}
			}
		}
		return successHandler(req)
	})

	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{socketPath: sock, ctx: ctx, cancel: cancel}
	defer m.Close()

	client, err := NewIPCClient(sock)
	if err != nil {
		t.Fatalf("NewIPCClient: %v", err)
	}
	m.mu.Lock()
	m.client = client
	m.mu.Unlock()

	elapsed, total, err := m.ElapsedPosition()
	if err != nil {
		t.Fatalf("ElapsedPosition: %v", err)
	}
	if elapsed != 42.5 {
		t.Errorf("elapsed = %f, want 42.5", elapsed)
	}
	if total != 180.0 {
		t.Errorf("total = %f, want 180.0", total)
	}
}

func TestManager_ElapsedPosition_NotConnected(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{ctx: ctx, cancel: cancel}
	defer m.Close()

	_, _, err := m.ElapsedPosition()
	if !errors.Is(err, ErrNotConnected) {
		t.Errorf("ElapsedPosition: want ErrNotConnected, got %v", err)
	}
}

func TestManager_SyncPosition_Connected(t *testing.T) {
	sock := shortSockPath(t, "syncpos")

	fakeServerWithHandler(t, sock, func(req map[string]any) map[string]any {
		cmd, _ := req["command"].([]any)
		if len(cmd) >= 2 {
			prop, _ := cmd[1].(string)
			switch prop {
			case "path":
				return map[string]any{
					"error":      "success",
					"data":       "/music/track.flac",
					"request_id": req["request_id"],
				}
			case "time-pos":
				return map[string]any{
					"error":      "success",
					"data":       55.3,
					"request_id": req["request_id"],
				}
			}
		}
		return successHandler(req)
	})

	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{socketPath: sock, ctx: ctx, cancel: cancel}
	defer m.Close()

	client, err := NewIPCClient(sock)
	if err != nil {
		t.Fatalf("NewIPCClient: %v", err)
	}
	m.mu.Lock()
	m.client = client
	m.mu.Unlock()

	if err := m.SyncPosition(); err != nil {
		t.Fatalf("SyncPosition: %v", err)
	}

	m.mu.RLock()
	path := m.lastTrackPath
	elapsed := m.lastElapsed
	m.mu.RUnlock()

	if path != "/music/track.flac" {
		t.Errorf("lastTrackPath = %q, want /music/track.flac", path)
	}
	if elapsed != 55.3 {
		t.Errorf("lastElapsed = %f, want 55.3", elapsed)
	}
}

func TestManager_SyncPosition_NotConnected(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{ctx: ctx, cancel: cancel}
	defer m.Close()

	err := m.SyncPosition()
	if !errors.Is(err, ErrNotConnected) {
		t.Errorf("SyncPosition: want ErrNotConnected, got %v", err)
	}
}
