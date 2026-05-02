package player

import (
	"context"
	"os/exec"
	"testing"
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
