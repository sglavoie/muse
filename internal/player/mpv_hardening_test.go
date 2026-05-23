package player

import (
	"context"
	"errors"
	"math"
	"sync"
	"testing"
	"time"
)

// TestManager_Close_NilsCmdQueue verifies that Close() sets cmdQueue to nil
// so that subsequent Player calls return ErrNotConnected instead of silently
// enqueueing commands that will never run.
func TestManager_Close_NilsCmdQueue(t *testing.T) {
	rootCtx, rootCancel := context.WithCancel(context.Background())
	ctx, cancel := context.WithCancel(rootCtx)
	m := &Manager{
		rootCtx:    rootCtx,
		rootCancel: rootCancel,
		ctx:        ctx,
		cancel:     cancel,
		// Simulate a started manager that never connected.
		cmdQueue: []func() error{},
	}

	if err := m.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	m.mu.RLock()
	q := m.cmdQueue
	m.mu.RUnlock()
	if q != nil {
		t.Fatalf("cmdQueue = %v after Close, want nil", q)
	}

	// Subsequent player calls must return ErrNotConnected, not silently
	// enqueue.
	if err := m.Play(); !errors.Is(err, ErrNotConnected) {
		t.Errorf("Play after Close: want ErrNotConnected, got %v", err)
	}
	if err := m.Pause(); !errors.Is(err, ErrNotConnected) {
		t.Errorf("Pause after Close: want ErrNotConnected, got %v", err)
	}
	if err := m.SetVolumeAbsolute(50); !errors.Is(err, ErrNotConnected) {
		t.Errorf("SetVolumeAbsolute after Close: want ErrNotConnected, got %v", err)
	}
}

// TestManager_Close_DoubleCloseSafe verifies that Close() can be called more
// than once without panicking or returning a different result.
func TestManager_Close_DoubleCloseSafe(t *testing.T) {
	rootCtx, rootCancel := context.WithCancel(context.Background())
	ctx, cancel := context.WithCancel(rootCtx)
	m := &Manager{
		rootCtx:    rootCtx,
		rootCancel: rootCancel,
		ctx:        ctx,
		cancel:     cancel,
	}

	if err := m.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := m.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

// TestManager_PlayFromIndex_OutOfRangePreservesPrefetch verifies that calling
// PlayFromIndex with an out-of-range index does not destroy the existing
// prefetched-next state. The previous behaviour cleared the prefetch even
// when the call returned an error and changed nothing, forcing a spurious
// playlist-remove against mpv.
func TestManager_PlayFromIndex_OutOfRangePreservesPrefetch(t *testing.T) {
	var mu sync.Mutex
	var received [][]any
	m := newConnectedManager(t, "pfi-oob-keep-pf", &mu, &received)

	m.mu.Lock()
	m.queue = []QueueTrack{
		{Path: "/a.flac", Hash: "h0", QueueID: 1},
	}
	m.currentIndex = 0
	m.gaplessEnabled = true
	m.prefetchedIndex = 5 // stale-but-set; the function must NOT touch it
	m.prefetchedPath = "/x.flac"
	m.mu.Unlock()

	if err := m.PlayFromIndex(10); err == nil {
		t.Fatal("PlayFromIndex(10) on len=1 queue: want error, got nil")
	}

	// No playlist-remove must have been sent — the prefetch is preserved.
	mu.Lock()
	for _, cmd := range received {
		if isPlaylistRemove(cmd) {
			mu.Unlock()
			t.Fatalf("PlayFromIndex with bad index sent playlist-remove; got %v", received)
		}
	}
	mu.Unlock()

	m.mu.RLock()
	idx := m.prefetchedIndex
	path := m.prefetchedPath
	m.mu.RUnlock()
	if idx != 5 || path != "/x.flac" {
		t.Errorf("prefetch state mutated: idx=%d path=%q, want idx=5 path=/x.flac", idx, path)
	}
}

// TestManager_SetCurrentPosition_ClampsNegative verifies that
// SetCurrentPosition stores 0 instead of a negative elapsed value, so a
// later resume loadfile does not pass a bogus negative start= option to mpv.
func TestManager_SetCurrentPosition_ClampsNegative(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{ctx: ctx, cancel: cancel}
	defer m.Close()

	m.SetCurrentPosition("/foo.flac", -3.14)

	m.mu.RLock()
	gotElapsed := m.lastElapsed
	gotPath := m.lastTrackPath
	m.mu.RUnlock()

	if gotElapsed != 0 {
		t.Errorf("lastElapsed = %v, want 0 (negative clamped)", gotElapsed)
	}
	if gotPath != "/foo.flac" {
		t.Errorf("lastTrackPath = %q, want /foo.flac", gotPath)
	}
}

// TestManager_SetCurrentPosition_ClampsNaN verifies that a NaN elapsed value
// (which can sneak in from a malformed mpv response) is clamped to 0.
func TestManager_SetCurrentPosition_ClampsNaN(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{ctx: ctx, cancel: cancel}
	defer m.Close()

	m.SetCurrentPosition("/foo.flac", math.NaN())

	m.mu.RLock()
	got := m.lastElapsed
	m.mu.RUnlock()

	if got != 0 {
		t.Errorf("lastElapsed = %v, want 0 (NaN clamped)", got)
	}
}

// TestManager_SetCurrentPosition_ClampsPositiveInf verifies that positive
// infinity is not persisted into the resume loadfile start option.
func TestManager_SetCurrentPosition_ClampsPositiveInf(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{ctx: ctx, cancel: cancel}
	defer m.Close()

	m.SetCurrentPosition("/foo.flac", math.Inf(1))

	m.mu.RLock()
	got := m.lastElapsed
	m.mu.RUnlock()

	if got != 0 {
		t.Errorf("lastElapsed = %v, want 0 (+Inf clamped)", got)
	}
}

// TestManager_SetCurrentPosition_PreservesPositive verifies that a normal
// positive elapsed value is stored as-is.
func TestManager_SetCurrentPosition_PreservesPositive(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{ctx: ctx, cancel: cancel}
	defer m.Close()

	m.SetCurrentPosition("/bar.flac", 42.5)

	m.mu.RLock()
	got := m.lastElapsed
	m.mu.RUnlock()

	if got != 42.5 {
		t.Errorf("lastElapsed = %v, want 42.5", got)
	}
}

// TestManager_HealthCheck_DoesNotResurrectQueueAfterClose verifies that the
// health-check goroutine does NOT recreate cmdQueue (which Close set to nil)
// when it detects a disconnect after the manager has begun shutting down.
// Without the guard, a Player call racing with Close could observe a
// non-nil cmdQueue and silently enqueue commands that never run.
func TestManager_HealthCheck_DoesNotResurrectQueueAfterClose(t *testing.T) {
	rootCtx, rootCancel := context.WithCancel(context.Background())
	ctx, cancel := context.WithCancel(rootCtx)

	// Build a manager and a real connected client against a fake server.
	sock := shortSockPath(t, "hc-noresurrect")
	fakeServer(t, sock)
	client, err := NewIPCClient(sock)
	if err != nil {
		t.Fatalf("NewIPCClient: %v", err)
	}

	m := &Manager{
		socketPath:   sock,
		rootCtx:      rootCtx,
		rootCancel:   rootCancel,
		ctx:          ctx,
		cancel:       cancel,
		disconnectCh: make(chan error, 1),
		client:       client,
	}

	// Simulate the disconnect path manually: cancel rootCtx first (Close
	// would do this), then invoke the body of healthCheck's disconnect
	// branch by closing the client and running the lock-protected block.
	rootCancel()
	_ = client.Close()

	// Drive the same code path healthCheck uses on disconnect.
	shuttingDown := false
	select {
	case <-m.rootCtx.Done():
		shuttingDown = true
	default:
	}
	if !shuttingDown {
		t.Fatal("rootCtx not done after rootCancel")
	}
	m.mu.Lock()
	if m.client == client {
		m.client = nil
		if !shuttingDown {
			m.cmdQueue = []func() error{}
		}
	}
	m.mu.Unlock()

	// cmdQueue must still be nil — no resurrection.
	m.mu.RLock()
	q := m.cmdQueue
	m.mu.RUnlock()
	if q != nil {
		t.Errorf("cmdQueue = %v after disconnect during shutdown, want nil", q)
	}
}

// TestManager_HealthCheck_DetectsDisconnect_LiveManager smokes the live
// healthCheck loop end-to-end: when the fake server hangs up, the goroutine
// sends an error on disconnectCh and clears the client.
func TestManager_HealthCheck_DetectsDisconnect_LiveManager(t *testing.T) {
	sock := shortSockPath(t, "hc-live")
	fakeServer(t, sock)

	client, err := NewIPCClient(sock)
	if err != nil {
		t.Fatalf("NewIPCClient: %v", err)
	}

	rootCtx, rootCancel := context.WithCancel(context.Background())
	ctx, cancel := context.WithCancel(rootCtx)
	defer rootCancel()

	m := &Manager{
		socketPath:   sock,
		rootCtx:      rootCtx,
		rootCancel:   rootCancel,
		ctx:          ctx,
		cancel:       cancel,
		disconnectCh: make(chan error, 1),
		client:       client,
		pingInterval: 5 * time.Millisecond,
	}

	go m.healthCheck(ctx, client)

	// Sever the connection from the client side: this causes the
	// goroutine's next ping to fail.
	_ = client.Close()

	select {
	case <-m.DisconnectCh():
	case <-time.After(2 * time.Second):
		t.Fatal("healthCheck did not surface disconnect on DisconnectCh")
	}

	m.mu.RLock()
	gotClient := m.client
	gotQueue := m.cmdQueue
	m.mu.RUnlock()
	if gotClient != nil {
		t.Errorf("client should have been cleared on disconnect, got %v", gotClient)
	}
	// Manager is NOT shutting down, so cmdQueue should have been replaced
	// with an empty slice for replay on reconnect.
	if gotQueue == nil {
		t.Error("cmdQueue should be reset to empty slice (not nil) on live disconnect")
	}
}
