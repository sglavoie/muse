package player

import (
	"context"
	"errors"
	"os/exec"
	"sync"
	"testing"
	"time"
)

// TestManager_HandleRestart_LaunchFnError_ClearsQueue verifies that when the
// restart's launchFn fails the manager clears its cmdQueue, so subsequent
// player calls return ErrNotConnected rather than silently queueing commands
// that will never be replayed (because no pollUntilReady goroutine starts).
func TestManager_HandleRestart_LaunchFnError_ClearsQueue(t *testing.T) {
	sock := shortSockPath(t, "restart-launchfail")

	var statusMu sync.Mutex
	var statusMsgs []string

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
		// Initialise as if Start had run: cmdQueue non-nil simulates the
		// post-Start state where commands queue while waiting for IPC.
		cmdQueue: []func() error{},
	}
	m.callbacks.OnStatus = func(msg string) {
		statusMu.Lock()
		statusMsgs = append(statusMsgs, msg)
		statusMu.Unlock()
	}
	// launchFn always fails — simulate "mpv binary not found".
	m.launchFn = func(_ context.Context, _ string) (*exec.Cmd, error) {
		return nil, errors.New("mpv: binary not found")
	}

	// Drive a single restart cycle synchronously.
	m.handleRestart()

	// After a failed restart cmdQueue must be cleared so callers fail fast.
	m.mu.RLock()
	q := m.cmdQueue
	m.mu.RUnlock()
	if q != nil {
		t.Fatalf("cmdQueue = %v after failed restart; want nil so callers get ErrNotConnected", q)
	}

	// Verify Play surfaces ErrNotConnected (the call path that was previously
	// silently queueing into a dead queue).
	if err := m.Play(); !errors.Is(err, ErrNotConnected) {
		t.Errorf("Play after failed restart: want ErrNotConnected, got %v", err)
	}

	statusMu.Lock()
	defer statusMu.Unlock()
	foundFailure := false
	for _, msg := range statusMsgs {
		if msg == "mpv: restart failed: mpv: binary not found" {
			foundFailure = true
		}
	}
	if !foundFailure {
		t.Errorf("expected restart failure status; got %v", statusMsgs)
	}
}

// TestManager_HandleRestart_LaunchFnNil_ClearsQueue covers the same code path
// when launchFn itself is nil (a misconfigured Manager built via struct
// literal instead of NewManager).
func TestManager_HandleRestart_LaunchFnNil_ClearsQueue(t *testing.T) {
	sock := shortSockPath(t, "restart-launchnil")

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
		cmdQueue:     []func() error{},
	}
	m.callbacks.OnStatus = func(string) {}
	// launchFn is intentionally left nil.

	m.handleRestart()

	m.mu.RLock()
	q := m.cmdQueue
	m.mu.RUnlock()
	if q != nil {
		t.Fatalf("cmdQueue = %v after failed restart with nil launchFn; want nil", q)
	}

	if err := m.Pause(); !errors.Is(err, ErrNotConnected) {
		t.Errorf("Pause after failed restart: want ErrNotConnected, got %v", err)
	}

	// Drain time so any background goroutines (none should be running) settle.
	time.Sleep(10 * time.Millisecond)
}
