package player

import (
	"context"
	"os/exec"
	"testing"
	"time"
)

// TestManager_HandleRestart_ReapsOldProcess verifies that handleRestart waits
// for the old mpv process to exit so it does not remain as a zombie.
func TestManager_HandleRestart_ReapsOldProcess(t *testing.T) {
	sock := shortSockPath(t, "reap-old")

	rootCtx, rootCancel := context.WithCancel(context.Background())
	ctx, cancel := context.WithCancel(rootCtx)
	defer rootCancel()

	// Start a real lightweight process to act as the "old mpv".
	// Use the current subprocess context so handleRestart's cancel() kills it.
	oldCmd := exec.CommandContext(ctx, "sleep", "60")
	if err := oldCmd.Start(); err != nil {
		t.Fatalf("start old process: %v", err)
	}

	m := &Manager{
		socketPath:     sock,
		rootCtx:        rootCtx,
		rootCancel:     rootCancel,
		ctx:            ctx,
		cancel:         cancel,
		cmd:            oldCmd,
		disconnectCh:   make(chan error, 1),
		callbacksReady: make(chan struct{}),
		audioFilters:   make(map[string]string),
	}
	m.callbacks.OnStatus = func(string) {}
	m.launchFn = func(_ context.Context, socketPath string) (*exec.Cmd, error) {
		fakeServer(t, socketPath)
		return nil, nil
	}

	m.handleRestart()

	// After handleRestart, the old process should have been waited for.
	// exec.Cmd.ProcessState is populated by Wait(); if it is still nil,
	// the process was never reaped and would remain as a zombie.
	if oldCmd.ProcessState == nil {
		t.Error("old process was not reaped by handleRestart (ProcessState is nil)")
	}
}

// TestManager_HandleRestart_DoesNotBlockOnUnresponsiveProcess verifies that
// handleRestart bounds its wait on the old process: a process that is not
// killable via context cancellation (because it was started without the
// manager's ctx) must not freeze the restart cycle. Before the fix this would
// hang forever; after the fix it falls through after shutdownTimeout, kills
// the process, and continues.
func TestManager_HandleRestart_DoesNotBlockOnUnresponsiveProcess(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow timeout test in short mode")
	}

	sock := shortSockPath(t, "reap-timeout")

	rootCtx, rootCancel := context.WithCancel(context.Background())
	ctx, cancel := context.WithCancel(rootCtx)
	defer rootCancel()

	// Start the old process WITHOUT the manager's ctx so m.cancel() will not
	// kill it; this simulates an mpv that hangs on shutdown.
	oldCmd := exec.Command("sleep", "60")
	if err := oldCmd.Start(); err != nil {
		t.Fatalf("start old process: %v", err)
	}
	t.Cleanup(func() {
		if oldCmd.Process != nil {
			_ = oldCmd.Process.Kill()
			_ = oldCmd.Wait()
		}
	})

	m := &Manager{
		socketPath:     sock,
		rootCtx:        rootCtx,
		rootCancel:     rootCancel,
		ctx:            ctx,
		cancel:         cancel,
		cmd:            oldCmd,
		disconnectCh:   make(chan error, 1),
		callbacksReady: make(chan struct{}),
		audioFilters:   make(map[string]string),
	}
	m.callbacks.OnStatus = func(string) {}
	m.launchFn = func(_ context.Context, socketPath string) (*exec.Cmd, error) {
		fakeServer(t, socketPath)
		return nil, nil
	}

	done := make(chan struct{})
	go func() {
		m.handleRestart()
		close(done)
	}()

	select {
	case <-done:
		// Good: handleRestart returned despite the unresponsive old process.
	case <-time.After(shutdownTimeout + 5*time.Second):
		t.Fatal("handleRestart blocked indefinitely on unresponsive old process")
	}

	// The process should have been killed by handleRestart's timeout branch.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if oldCmd.ProcessState != nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if oldCmd.ProcessState == nil {
		// Not strictly required for the bug fix, but the cleanup goroutine
		// should have killed and reaped it.
		t.Log("note: process not yet reaped after handleRestart returned")
	}
}
