package player

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
			if s, ok := arg.(string); ok && s == wantStart {
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
