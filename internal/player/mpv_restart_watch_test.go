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

// TestWatchAndRestart_GivesUpAfterMaxAttempts verifies that a single disconnect
// drives the supervisor's inner attempt loop, which makes exactly the
// configured number of launch attempts before giving up and emitting a
// "gave up" status notification — no external disconnect injection required.
func TestWatchAndRestart_GivesUpAfterMaxAttempts(t *testing.T) {
	rootCtx, rootCancel := context.WithCancel(context.Background())
	ctx, cancel := context.WithCancel(rootCtx)

	const maxAttempts = 3

	var launchMu sync.Mutex
	launches := 0

	m := &Manager{
		socketPath:                 shortSockPath(t, "restart-giveup"),
		rootCtx:                    rootCtx,
		rootCancel:                 rootCancel,
		ctx:                        ctx,
		cancel:                     cancel,
		disconnectCh:               make(chan error, 1),
		callbacksReady:             make(chan struct{}),
		restartBaseDelay:           1 * time.Millisecond,
		restartMaxAttemptsOverride: maxAttempts,
	}
	defer m.Close()

	m.launchFn = func(_ context.Context, _ string) (*exec.Cmd, error) {
		launchMu.Lock()
		launches++
		launchMu.Unlock()
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

	// A single disconnect is enough; the inner loop owns the retries.
	m.disconnectCh <- fmt.Errorf("test disconnect")

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("watchAndRestart did not give up within 5s")
	}

	launchMu.Lock()
	gotLaunches := launches
	launchMu.Unlock()
	if gotLaunches != maxAttempts {
		t.Errorf("launch attempts = %d, want exactly %d before give-up", gotLaunches, maxAttempts)
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
		socketPath:                 shortSockPath(t, "restart-giveup-queue"),
		rootCtx:                    rootCtx,
		rootCancel:                 rootCancel,
		ctx:                        ctx,
		cancel:                     cancel,
		disconnectCh:               make(chan error, 1),
		callbacksReady:             make(chan struct{}),
		restartBaseDelay:           1 * time.Millisecond,
		restartMaxAttemptsOverride: 3,
		// The socket never comes up, so each attempt hits the connect timeout.
		// Keep it short so the give-up path completes quickly.
		socketPollTimeoutOverride: 20 * time.Millisecond,
		cmdQueue:                  []func() error{},
	}
	defer m.Close()

	// launchFn "succeeds" but no server ever listens, so connectSocket times
	// out on every attempt.
	m.launchFn = func(context.Context, string) (*exec.Cmd, error) {
		return nil, nil
	}

	done := make(chan struct{})
	go func() {
		m.watchAndRestart()
		close(done)
	}()

	m.disconnectCh <- fmt.Errorf("test disconnect")

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

// TestConnectSocket_ConnectsToLiveServer verifies the connection helper returns
// a ready client when a server is already listening.
func TestConnectSocket_ConnectsToLiveServer(t *testing.T) {
	sock := shortSockPath(t, "connect-live")
	fakeServer(t, sock)

	m := &Manager{socketPath: sock}
	client, err := m.connectSocket(context.Background())
	if err != nil {
		t.Fatalf("connectSocket: %v", err)
	}
	if client == nil {
		t.Fatal("connectSocket returned nil client without error")
	}
	_ = client.Close()
}

// TestConnectSocket_TimesOut verifies the helper returns errConnectTimeout,
// bounded by the socket-poll-timeout override, when no server ever listens.
func TestConnectSocket_TimesOut(t *testing.T) {
	m := &Manager{
		socketPath:                shortSockPath(t, "connect-timeout"),
		socketPollTimeoutOverride: 30 * time.Millisecond,
	}
	start := time.Now()
	client, err := m.connectSocket(context.Background())
	if !errors.Is(err, errConnectTimeout) {
		t.Fatalf("connectSocket err = %v, want errConnectTimeout", err)
	}
	if client != nil {
		t.Error("connectSocket returned a non-nil client on timeout")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("connectSocket took %v to time out; expected close to the 30ms override", elapsed)
	}
}

// TestConnectSocket_CancelledContext verifies the helper aborts promptly and
// returns the context error when its context is cancelled.
func TestConnectSocket_CancelledContext(t *testing.T) {
	m := &Manager{
		socketPath:                shortSockPath(t, "connect-cancel"),
		socketPollTimeoutOverride: 5 * time.Second,
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client, err := m.connectSocket(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("connectSocket err = %v, want context.Canceled", err)
	}
	if client != nil {
		t.Error("connectSocket returned a non-nil client after cancellation")
	}
}
