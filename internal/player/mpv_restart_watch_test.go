package player

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

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
		for range 2 {
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

	m.mu.Lock()
	m.cmdQueue = []func() error{}
	m.mu.Unlock()

	go m.pollUntilReady(ctx)

	select {
	case <-m.disconnectCh:
	case <-time.After(3 * time.Second):
		t.Fatal("disconnect was not detected within 3s")
	}

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
	m.launchFn = func(_ context.Context, _ string) (*exec.Cmd, error) {
		return nil, nil
	}

	m.mu.Lock()
	m.cmd = nil
	m.cmdQueue = []func() error{}
	m.mu.Unlock()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		m.handleRestart()
	}()
	go func() {
		defer wg.Done()
		time.Sleep(100 * time.Millisecond)
		_ = m.Close()
	}()
	wg.Wait()
}

// TestWatchAndRestart_GivesUpAfterMaxAttempts verifies that watchAndRestart
// stops retrying after restartMaxAttempts consecutive failures and emits a
// "gave up" status notification.
func TestWatchAndRestart_GivesUpAfterMaxAttempts(t *testing.T) {
	rootCtx, rootCancel := context.WithCancel(context.Background())
	ctx, cancel := context.WithCancel(rootCtx)

	m := &Manager{
		socketPath:       shortSockPath(t, "restart-giveup"),
		rootCtx:          rootCtx,
		rootCancel:       rootCancel,
		ctx:              ctx,
		cancel:           cancel,
		disconnectCh:     make(chan error, 1),
		callbacksReady:   make(chan struct{}),
		restartBaseDelay: 1 * time.Millisecond,
	}
	defer m.Close()

	m.launchFn = func(_ context.Context, _ string) (*exec.Cmd, error) {
		return nil, fmt.Errorf("simulated launch failure")
	}

	var statusMu sync.Mutex
	var statusMsgs []string
	m.SetEventCallbacks(EventCallbacks{OnStatus: func(msg string) {
		statusMu.Lock()
		statusMsgs = append(statusMsgs, msg)
		statusMu.Unlock()
	}})

	done := make(chan struct{})
	go func() {
		m.watchAndRestart()
		close(done)
	}()

	go func() {
		for {
			select {
			case m.disconnectCh <- fmt.Errorf("test disconnect"):
			case <-done:
				return
			}
		}
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("watchAndRestart did not give up within 5s")
	}

	statusMu.Lock()
	defer statusMu.Unlock()
	for _, msg := range statusMsgs {
		if strings.Contains(msg, "gave up") {
			return
		}
	}
	t.Errorf("expected 'gave up' status message; got: %v", statusMsgs)
}

func TestWatchAndRestart_GiveUpClearsQueueAfterLaunchWithoutReconnect(t *testing.T) {
	rootCtx, rootCancel := context.WithCancel(context.Background())
	ctx, cancel := context.WithCancel(rootCtx)

	m := &Manager{
		socketPath:       shortSockPath(t, "restart-giveup-queue"),
		rootCtx:          rootCtx,
		rootCancel:       rootCancel,
		ctx:              ctx,
		cancel:           cancel,
		disconnectCh:     make(chan error, restartMaxAttempts+1),
		callbacksReady:   make(chan struct{}),
		restartBaseDelay: 1 * time.Millisecond,
		cmdQueue:         []func() error{},
	}
	defer m.Close()

	m.launchFn = func(context.Context, string) (*exec.Cmd, error) {
		return nil, nil
	}

	done := make(chan struct{})
	go func() {
		m.watchAndRestart()
		close(done)
	}()

	for range restartMaxAttempts + 1 {
		m.disconnectCh <- fmt.Errorf("test disconnect")
	}

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("watchAndRestart did not give up within 5s")
	}

	m.mu.RLock()
	q := m.cmdQueue
	m.mu.RUnlock()
	if q != nil {
		t.Fatalf("cmdQueue = %v after restart give-up; want nil so callers fail fast", q)
	}
}

// TestWatchAndRestart_ResetsBackoffOnSuccessfulConnect verifies that
// backoffReset is set to true by pollUntilReady when the IPC socket connects,
// and that watchAndRestart resets its consecutive-failure counter after a
// successful reconnect.
func TestWatchAndRestart_ResetsBackoffOnSuccessfulConnect(t *testing.T) {
	sock := shortSockPath(t, "restart-reset")
	fakeServer(t, sock)

	rootCtx, rootCancel := context.WithCancel(context.Background())
	ctx, cancel := context.WithCancel(rootCtx)

	m := &Manager{
		socketPath:     sock,
		rootCtx:        rootCtx,
		rootCancel:     rootCancel,
		ctx:            ctx,
		cancel:         cancel,
		disconnectCh:   make(chan error, 1),
		callbacksReady: make(chan struct{}),
		cmdQueue:       []func() error{},
	}
	defer m.Close()
	m.SetEventCallbacks(EventCallbacks{})

	go m.pollUntilReady(ctx)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if m.backoffReset.Load() {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if !m.backoffReset.Load() {
		t.Fatal("backoffReset was not set to true after successful pollUntilReady")
	}

	if !m.backoffReset.Swap(false) {
		t.Error("backoffReset.Swap(false) returned false; expected true")
	}
	if m.backoffReset.Load() {
		t.Error("backoffReset should be false after Swap")
	}
}
