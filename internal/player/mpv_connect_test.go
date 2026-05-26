package player

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
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

func TestCleanStaleSocket_NonSocketPathIsPreserved(t *testing.T) {
	t.Run("regular file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "not-a-socket")
		if err := os.WriteFile(path, []byte("keep"), 0o644); err != nil {
			t.Fatalf("write regular file: %v", err)
		}

		if err := cleanStaleSocket(path); err == nil {
			t.Fatal("expected error for non-socket path, got nil")
		}
		if got, err := os.ReadFile(path); err != nil {
			t.Fatalf("regular file was not preserved: %v", err)
		} else if string(got) != "keep" {
			t.Fatalf("regular file contents = %q, want keep", got)
		}
	})

	t.Run("directory", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "not-a-socket-dir")
		if err := os.Mkdir(path, 0o755); err != nil {
			t.Fatalf("create directory: %v", err)
		}

		if err := cleanStaleSocket(path); err == nil {
			t.Fatal("expected error for non-socket path, got nil")
		}
		if info, err := os.Stat(path); err != nil {
			t.Fatalf("directory was not preserved: %v", err)
		} else if !info.IsDir() {
			t.Fatalf("path is not a directory after cleanup: %s", path)
		}
	})
}

func TestNewManager_SocketPath(t *testing.T) {
	want := shortSockPath(t, "mgr-sockpath")
	m := NewManager(want)
	defer m.Close()

	if m.SocketPath() != want {
		t.Fatalf("socket path = %q, want %q", m.SocketPath(), want)
	}
}

func TestManager_Close_WithoutCancelFuncsDoesNotPanic(t *testing.T) {
	m := &Manager{}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Close panicked without cancel funcs: %v", r)
		}
	}()

	if err := m.Close(); err != nil {
		t.Fatalf("Close error = %v, want nil", err)
	}
}

func TestManager_ErrNotConnected(t *testing.T) {
	m := NewManager(shortSockPath(t, "mgr-noconn"))
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
		socketPath:     sock,
		ctx:            ctx,
		cancel:         cancel,
		callbacksReady: make(chan struct{}),
	}
	defer m.Close()

	// Collect status messages emitted by the manager.
	var statusMsgs []string
	m.SetEventCallbacks(EventCallbacks{OnStatus: func(msg string) {
		statusMsgs = append(statusMsgs, msg)
	}})

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
	fakeServerWithHandler(t, sock, func(req ipcTestRequest) ipcTestResponse {
		if cmd := req.Command; len(cmd) > 0 {
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
		time.Sleep(100 * time.Millisecond)
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
		for range 2 {
			var req ipcTestRequest
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

	client.mu.Lock()
	closed := client.closed
	client.mu.Unlock()
	if !closed {
		t.Error("expected health check to close the disconnected client")
	}
}
