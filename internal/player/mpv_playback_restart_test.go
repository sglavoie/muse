package player

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

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
