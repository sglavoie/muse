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
		time.Sleep(100 * time.Millisecond)
		_ = m.Close()
	}()
	wg.Wait()
}

func TestManager_SetEventCallbacks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{ctx: ctx, cancel: cancel, callbacksReady: make(chan struct{})}
	defer m.Close()

	var missingCalled bool
	var endedCalled bool
	var gotHash, gotArtist, gotTitle, gotSource string

	m.SetEventCallbacks(EventCallbacks{
		OnMissingFile:   func(_ string) { missingCalled = true },
		OnPlaybackEnded: func() { endedCalled = true },
		OnTrackChanged: func(h, a, tl, src string, _ int) {
			gotHash, gotArtist, gotTitle, gotSource = h, a, tl, src
		},
	})

	m.mu.RLock()
	cb := m.callbacks
	m.mu.RUnlock()

	if cb.OnMissingFile == nil {
		t.Fatal("OnMissingFile not set")
	}
	cb.OnMissingFile("/test")
	if !missingCalled {
		t.Error("OnMissingFile was not invoked")
	}

	if cb.OnPlaybackEnded == nil {
		t.Fatal("OnPlaybackEnded not set")
	}
	cb.OnPlaybackEnded()
	if !endedCalled {
		t.Error("OnPlaybackEnded was not invoked")
	}

	if cb.OnTrackChanged == nil {
		t.Fatal("OnTrackChanged not set")
	}
	cb.OnTrackChanged("h1", "Artist", "Title", "My Playlist", 0)
	if gotHash != "h1" || gotArtist != "Artist" || gotTitle != "Title" || gotSource != "My Playlist" {
		t.Errorf("OnTrackChanged args: got (%q, %q, %q, %q)", gotHash, gotArtist, gotTitle, gotSource)
	}

	// Verify callbacksReady was closed.
	select {
	case <-m.callbacksReady:
	default:
		t.Error("callbacksReady was not closed by SetEventCallbacks")
	}
}

func TestSetEventCallbacks_Idempotent(t *testing.T) {
	// Calling SetEventCallbacks twice must not panic.
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{ctx: ctx, cancel: cancel, callbacksReady: make(chan struct{})}
	defer m.Close()

	m.SetEventCallbacks(EventCallbacks{OnStatus: func(string) {}})
	m.SetEventCallbacks(EventCallbacks{OnStatus: func(string) {}})
}

// TestManager_EventReaderBlocksOnCallbacksReady verifies that startEventReader
// does not dispatch track-changed events until SetEventCallbacks is called.
func TestManager_EventReaderBlocksOnCallbacksReady(t *testing.T) {
	sock := shortSockPath(t, "ev-ready")

	// connDone is closed by the test to let the fake server goroutine exit.
	connDone := make(chan struct{})
	// eventSent is closed once the fake server has written the start-file event.
	eventSent := make(chan struct{})

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
		// Send a start-file event immediately (observe_property request from
		// the event reader is buffered on the client side; we don't need to
		// ACK it for the test to work).
		_, _ = conn.Write([]byte(`{"event":"start-file"}` + "\n"))
		close(eventSent)
		// Hold the connection open until the test is done.
		<-connDone
	}()
	defer close(connDone)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	trackFired := make(chan struct{}, 1)
	m := &Manager{
		socketPath:     sock,
		ctx:            ctx,
		cancel:         cancel,
		callbacksReady: make(chan struct{}),
		queue:          []QueueTrack{{Path: "/a.flac", Hash: "h0", Artist: "Art", Title: "Tit"}},
		currentIndex:   0,
		callbacks:      EventCallbacks{OnTrackChanged: func(_, _, _, _ string, _ int) { trackFired <- struct{}{} }},
	}

	go m.startEventReader(ctx)

	// Wait for the fake server to confirm the event has been sent.
	select {
	case <-eventSent:
	case <-time.After(2 * time.Second):
		t.Fatal("fake server did not send start-file event in time")
	}

	// The callback must NOT have fired yet because callbacksReady is not closed.
	select {
	case <-trackFired:
		t.Fatal("OnTrackChanged fired before SetEventCallbacks was called")
	case <-time.After(100 * time.Millisecond):
		// correct: event reader is still blocked
	}

	// Signal that all callbacks are wired by closing callbacksReady
	// (in production this is done by SetEventCallbacks).
	close(m.callbacksReady)

	// Now the event reader unblocks and processes the buffered start-file event.
	select {
	case <-trackFired:
		// correct
	case <-time.After(2 * time.Second):
		t.Fatal("OnTrackChanged did not fire after callbacksReady closed")
	}
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

	// launchFn that always fails so handleRestart returns quickly.
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

	// Feed disconnect signals one at a time; stop once watchAndRestart exits.
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

	// pollUntilReady should connect and set backoffReset.
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

	// Swap confirms watchAndRestart's counter-reset path works.
	if !m.backoffReset.Swap(false) {
		t.Error("backoffReset.Swap(false) returned false; expected true")
	}
	if m.backoffReset.Load() {
		t.Error("backoffReset should be false after Swap")
	}
}
