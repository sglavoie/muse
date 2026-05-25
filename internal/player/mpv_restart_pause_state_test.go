package player

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestManager_AutoRestart_PreservesPause verifies that when lastPaused is true
// the restarted loadfile command includes pause=yes instead of pause=no.
func TestManager_AutoRestart_PreservesPause(t *testing.T) {
	sock := shortSockPath(t, "restart-pause")

	// Phase 1: fake server that accepts a couple pings then closes.
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
			var req ipcTestRequest
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

	rootCtx, rootCancel := context.WithCancel(context.Background())
	ctx, cancel := context.WithCancel(rootCtx)
	defer rootCancel()

	m := &Manager{
		socketPath:    sock,
		rootCtx:       rootCtx,
		rootCancel:    rootCancel,
		ctx:           ctx,
		cancel:        cancel,
		disconnectCh:  make(chan error, 1),
		pingInterval:  50 * time.Millisecond,
		lastTrackPath: "/music/paused.flac",
		lastElapsed:   10.0,
		lastPaused:    true,
	}
	m.callbacks.OnStatus = func(string) {}
	m.launchFn = func(_ context.Context, socketPath string) (*exec.Cmd, error) {
		fakeServerWithHandler(t, socketPath, func(req ipcTestRequest) map[string]any {
			if cmd := req.Command; len(cmd) > 0 {
				mu.Lock()
				received = append(received, cmd)
				mu.Unlock()
			}
			return successHandler(req)
		})
		return nil, nil
	}

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

	select {
	case <-connClosed:
	case <-time.After(2 * time.Second):
		t.Fatal("phase 1 server did not close in time")
	}

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

	mu.Lock()
	defer mu.Unlock()

	for _, args := range received {
		if len(args) == 0 {
			continue
		}
		name, _ := args[0].(string)
		if name != "loadfile" {
			continue
		}
		for _, arg := range args {
			s, ok := arg.(string)
			if !ok {
				continue
			}
			if strings.Contains(s, "pause=yes") {
				return // success
			}
			if strings.Contains(s, "pause=no") {
				t.Fatalf("loadfile used pause=no but lastPaused was true; args: %v", args)
			}
		}
	}
	t.Fatal("no loadfile command with pause option found")
}
