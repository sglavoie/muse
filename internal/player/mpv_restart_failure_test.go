package player

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestManager_RestartGiveUp_LaunchFnError_ClearsQueue verifies that when every
// launch attempt fails, the supervisor exhausts its attempt budget, clears the
// cmdQueue, and notifies the user — so subsequent player calls return
// ErrNotConnected rather than silently queueing commands that will never be
// replayed. A single transient launch failure no longer tears down the queue;
// only the final give-up does.
func TestManager_RestartGiveUp_LaunchFnError_ClearsQueue(t *testing.T) {
	sock := shortSockPath(t, "restart-launchfail")

	var statusMu sync.Mutex
	var statusMsgs []string

	rootCtx, rootCancel := context.WithCancel(context.Background())
	ctx, cancel := context.WithCancel(rootCtx)
	defer rootCancel()

	m := &Manager{
		socketPath:                 sock,
		rootCtx:                    rootCtx,
		rootCancel:                 rootCancel,
		ctx:                        ctx,
		cancel:                     cancel,
		disconnectCh:               make(chan error, 1),
		restartBaseDelay:           1 * time.Millisecond,
		restartMaxAttemptsOverride: 3,
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

	// Drive a full recovery cycle synchronously; every attempt fails.
	m.handleRestart()

	// After giving up, cmdQueue must be cleared so callers fail fast.
	m.mu.RLock()
	q := m.cmdQueue
	m.mu.RUnlock()
	if q != nil {
		t.Fatalf("cmdQueue = %v after give-up; want nil so callers get ErrNotConnected", q)
	}

	// Verify Play surfaces ErrNotConnected (the call path that was previously
	// silently queueing into a dead queue).
	if err := m.Play(); !errors.Is(err, ErrNotConnected) {
		t.Errorf("Play after give-up: want ErrNotConnected, got %v", err)
	}

	statusMu.Lock()
	defer statusMu.Unlock()
	foundGiveUp := false
	for _, msg := range statusMsgs {
		if strings.Contains(msg, "gave up") {
			foundGiveUp = true
		}
	}
	if !foundGiveUp {
		t.Errorf("expected 'gave up' status after exhausting attempts; got %v", statusMsgs)
	}
}

// TestManager_RestartGiveUp_LaunchFnNil_ClearsQueue covers the same give-up
// path when launchFn itself is nil (a misconfigured Manager built via struct
// literal instead of NewManager).
func TestManager_RestartGiveUp_LaunchFnNil_ClearsQueue(t *testing.T) {
	sock := shortSockPath(t, "restart-launchnil")

	rootCtx, rootCancel := context.WithCancel(context.Background())
	ctx, cancel := context.WithCancel(rootCtx)
	defer rootCancel()

	m := &Manager{
		socketPath:                 sock,
		rootCtx:                    rootCtx,
		rootCancel:                 rootCancel,
		ctx:                        ctx,
		cancel:                     cancel,
		disconnectCh:               make(chan error, 1),
		restartBaseDelay:           1 * time.Millisecond,
		restartMaxAttemptsOverride: 3,
		cmdQueue:                   []func() error{},
	}
	m.callbacks.OnStatus = func(string) {}
	// launchFn is intentionally left nil.

	m.handleRestart()

	m.mu.RLock()
	q := m.cmdQueue
	m.mu.RUnlock()
	if q != nil {
		t.Fatalf("cmdQueue = %v after give-up with nil launchFn; want nil", q)
	}

	if err := m.Pause(); !errors.Is(err, ErrNotConnected) {
		t.Errorf("Pause after give-up: want ErrNotConnected, got %v", err)
	}

	// Drain time so any background goroutines (none should be running) settle.
	time.Sleep(10 * time.Millisecond)
}
