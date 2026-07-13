package player

import (
	"context"
	"fmt"
	"os/exec"
	"sync"
	"testing"
	"time"
)

// TestRecoverOutage_ReconnectsAfterTransientLaunchFailures verifies that the
// supervisor reconnects on a later attempt when the first launches fail — with
// no external disconnect injection driving the retries. It also confirms the
// resume loadfile is replayed after reconnection.
func TestRecoverOutage_ReconnectsAfterTransientLaunchFailures(t *testing.T) {
	sock := shortSockPath(t, "restart-transient-launch")

	rootCtx, rootCancel := context.WithCancel(context.Background())
	ctx, cancel := context.WithCancel(rootCtx)
	defer rootCancel()

	var mu sync.Mutex
	var received []ipcTestCommand
	attempts := 0

	m := &Manager{
		socketPath:       sock,
		rootCtx:          rootCtx,
		rootCancel:       rootCancel,
		ctx:              ctx,
		cancel:           cancel,
		disconnectCh:     make(chan error, 1),
		callbacksReady:   make(chan struct{}),
		restartBaseDelay: 1 * time.Millisecond,
		cmdQueue:         []func() error{},
		lastTrackPath:    "/music/resume.flac",
		lastElapsed:      7.5,
	}
	defer m.Close()
	m.SetEventCallbacks(EventCallbacks{OnStatus: func(string) {}})

	m.launchFn = func(_ context.Context, socketPath string) (*exec.Cmd, error) {
		mu.Lock()
		attempts++
		n := attempts
		mu.Unlock()
		if n < 3 {
			return nil, fmt.Errorf("transient launch failure %d", n)
		}
		fakeServerWithHandler(t, socketPath, func(req ipcTestRequest) ipcTestResponse {
			recordCommand(&mu, &received, req.Command)
			return successHandler(req)
		})
		return nil, nil
	}

	m.handleRestart()

	mu.Lock()
	gotAttempts := attempts
	mu.Unlock()
	if gotAttempts != 3 {
		t.Fatalf("launch attempts = %d, want 3 (two transient failures then success)", gotAttempts)
	}

	m.mu.RLock()
	client := m.client
	m.mu.RUnlock()
	if client == nil {
		t.Fatal("no client installed after a successful later attempt")
	}

	waitForCommand(t, &mu, &received, func(c ipcTestCommand) bool {
		return c.stringIs(0, "loadfile") && c.hasStringContaining("start=7.5")
	}, 3*time.Second, "resume loadfile after reconnect")
}

// TestRecoverOutage_ReconnectsAfterTransientSocketTimeouts verifies the same
// recovery when the first attempts launch but never open their socket (connect
// timeout), exercising the socket-timeout branch of the retry path.
func TestRecoverOutage_ReconnectsAfterTransientSocketTimeouts(t *testing.T) {
	sock := shortSockPath(t, "restart-transient-socket")

	rootCtx, rootCancel := context.WithCancel(context.Background())
	ctx, cancel := context.WithCancel(rootCtx)
	defer rootCancel()

	var mu sync.Mutex
	var received []ipcTestCommand
	attempts := 0

	m := &Manager{
		socketPath:                sock,
		rootCtx:                   rootCtx,
		rootCancel:                rootCancel,
		ctx:                       ctx,
		cancel:                    cancel,
		disconnectCh:              make(chan error, 1),
		callbacksReady:            make(chan struct{}),
		restartBaseDelay:          1 * time.Millisecond,
		socketPollTimeoutOverride: 30 * time.Millisecond,
		cmdQueue:                  []func() error{},
		lastTrackPath:             "/music/resume2.flac",
		lastElapsed:               3.0,
	}
	defer m.Close()
	m.SetEventCallbacks(EventCallbacks{OnStatus: func(string) {}})

	m.launchFn = func(_ context.Context, socketPath string) (*exec.Cmd, error) {
		mu.Lock()
		attempts++
		n := attempts
		mu.Unlock()
		if n < 3 {
			// Launch "succeeds" but no socket ever comes up → connect timeout.
			return nil, nil
		}
		fakeServerWithHandler(t, socketPath, func(req ipcTestRequest) ipcTestResponse {
			recordCommand(&mu, &received, req.Command)
			return successHandler(req)
		})
		return nil, nil
	}

	m.handleRestart()

	mu.Lock()
	gotAttempts := attempts
	mu.Unlock()
	if gotAttempts != 3 {
		t.Fatalf("launch attempts = %d, want 3 (two socket timeouts then success)", gotAttempts)
	}

	waitForCommand(t, &mu, &received, func(c ipcTestCommand) bool {
		return c.stringIs(0, "loadfile")
	}, 3*time.Second, "resume loadfile after socket-timeout recovery")
}

// TestRecoverOutage_RestoresStateAndReplaysQueuedControls verifies that after a
// recovery which survives transient failures: (1) all pre-crash user-visible
// settings (volume, repeat, gapless, audio filters) are re-applied, (2) the
// resume loadfile is sent, and (3) a control the user queued during the outage
// is replayed — after the restore, never before it.
func TestRecoverOutage_RestoresStateAndReplaysQueuedControls(t *testing.T) {
	sock := shortSockPath(t, "restart-restore-order")

	rootCtx, rootCancel := context.WithCancel(context.Background())
	ctx, cancel := context.WithCancel(rootCtx)
	defer rootCancel()

	var mu sync.Mutex
	var received []ipcTestCommand
	attempts := 0

	m := &Manager{
		socketPath:       sock,
		rootCtx:          rootCtx,
		rootCancel:       rootCancel,
		ctx:              ctx,
		cancel:           cancel,
		disconnectCh:     make(chan error, 1),
		callbacksReady:   make(chan struct{}),
		restartBaseDelay: 1 * time.Millisecond,
		// Pre-crash user-visible state that must be restored.
		lastTrackPath:  "/music/state.flac",
		lastElapsed:    12.0,
		lastVolume:     42,
		volumeKnown:    true,
		repeatMode:     "all",
		gaplessEnabled: true,
		audioFilters:   map[string]string{"loudnorm": "loudnorm"},
	}
	defer m.Close()
	m.SetEventCallbacks(EventCallbacks{OnStatus: func(string) {}})

	// A control queued by the user during the outage. It must be replayed only
	// after the restore commands.
	m.mu.Lock()
	m.cmdQueue = []func() error{
		func() error {
			c, err := m.ipc()
			if err != nil {
				return err
			}
			_, err = c.SendCommand(cmdSetProperty, "user-queued-marker", "1")
			return err
		},
	}
	m.mu.Unlock()

	m.launchFn = func(_ context.Context, socketPath string) (*exec.Cmd, error) {
		mu.Lock()
		attempts++
		n := attempts
		mu.Unlock()
		if n < 2 {
			return nil, fmt.Errorf("transient launch failure %d", n)
		}
		fakeServerWithHandler(t, socketPath, func(req ipcTestRequest) ipcTestResponse {
			recordCommand(&mu, &received, req.Command)
			return successHandler(req)
		})
		return nil, nil
	}

	m.handleRestart()

	// The queued marker is the last thing replayed, so waiting for it guarantees
	// the whole restore sequence has been sent.
	markerPred := func(c ipcTestCommand) bool {
		return c.stringIs(0, cmdSetProperty) && c.stringIs(1, "user-queued-marker")
	}
	waitForCommand(t, &mu, &received, markerPred, 3*time.Second, "queued control replayed after recovery")

	mu.Lock()
	defer mu.Unlock()

	indexOf := func(pred func(ipcTestCommand) bool) int {
		for i, c := range received {
			if pred(c) {
				return i
			}
		}
		return -1
	}

	volumeIdx := indexOf(func(c ipcTestCommand) bool {
		if !c.stringIs(0, cmdSetProperty) || !c.stringIs(1, propVolume) {
			return false
		}
		v, ok := c.float64At(2)
		return ok && v == 42
	})
	gaplessIdx := indexOf(func(c ipcTestCommand) bool {
		return c.stringIs(0, cmdSetProperty) && c.stringIs(1, propGaplessAudio)
	})
	afIdx := indexOf(func(c ipcTestCommand) bool {
		return c.stringIs(0, cmdSetProperty) && c.stringIs(1, propAF) && c.hasStringContaining("loudnorm")
	})
	repeatIdx := indexOf(func(c ipcTestCommand) bool {
		return c.stringIs(0, cmdSetProperty) && c.stringIs(1, propLoopPlaylist)
	})
	loadfileIdx := indexOf(func(c ipcTestCommand) bool {
		return c.stringIs(0, cmdLoadfile)
	})
	markerIdx := indexOf(markerPred)

	if volumeIdx < 0 {
		t.Errorf("volume not restored to 42 after reconnect; got %v", received)
	}
	if repeatIdx < 0 {
		t.Errorf("repeat mode not restored after reconnect; got %v", received)
	}
	if gaplessIdx < 0 {
		t.Errorf("gapless not restored after reconnect; got %v", received)
	}
	if afIdx < 0 {
		t.Errorf("audio filters not restored after reconnect; got %v", received)
	}
	if loadfileIdx < 0 {
		t.Fatalf("resume loadfile not sent after reconnect; got %v", received)
	}

	// Stable properties first, resume second, queued controls last.
	if !(volumeIdx < loadfileIdx && gaplessIdx < loadfileIdx && afIdx < loadfileIdx && repeatIdx < loadfileIdx) {
		t.Errorf("stable properties must precede the resume loadfile; volume=%d repeat=%d gapless=%d af=%d loadfile=%d",
			volumeIdx, repeatIdx, gaplessIdx, afIdx, loadfileIdx)
	}
	if markerIdx < loadfileIdx {
		t.Errorf("queued control (idx %d) replayed before the resume loadfile (idx %d); want after", markerIdx, loadfileIdx)
	}
}

// TestActivateConnection_GatesUserCommandsUntilRestoreCompletes proves the
// activation-window gate: a concurrent user command issued after the fresh
// client is installed but before restore completes must be queued and replayed
// after the restore sequence — never run against the fresh client and then
// clobbered by restore. Without the gate the user's volume would be executed
// mid-restore and overwritten by the restored value.
func TestActivateConnection_GatesUserCommandsUntilRestoreCompletes(t *testing.T) {
	sock := shortSockPath(t, "activation-gate")

	rootCtx, rootCancel := context.WithCancel(context.Background())
	ctx, cancel := context.WithCancel(rootCtx)
	defer rootCancel()
	defer cancel()

	var mu sync.Mutex
	var received []ipcTestCommand
	fakeServerWithHandler(t, sock, func(req ipcTestRequest) ipcTestResponse {
		recordCommand(&mu, &received, req.Command)
		return successHandler(req)
	})

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
	m.SetEventCallbacks(EventCallbacks{OnStatus: func(string) {}})

	client, err := NewIPCClient(sock)
	if err != nil {
		t.Fatalf("dial fake server: %v", err)
	}

	restoreStarted := make(chan struct{})
	userIssued := make(chan struct{})

	// restore runs with the client installed and activating=true. It signals it
	// has begun, waits until the concurrent user commands are guaranteed issued
	// (so they land strictly inside the activation window), then sends its
	// restore sequence: volume=42 followed by the resume loadfile.
	restore := func() {
		close(restoreStarted)
		select {
		case <-userIssued:
		case <-time.After(3 * time.Second):
			t.Error("timed out waiting for concurrent user commands to be issued")
			return
		}
		c, err := m.ipc()
		if err != nil {
			t.Errorf("restore: ipc unavailable during activation: %v", err)
			return
		}
		if _, err := c.SendCommand(cmdSetProperty, propVolume, 42.0); err != nil {
			t.Errorf("restore volume: %v", err)
		}
		if _, err := c.SendCommand(cmdLoadfile, "/music/resume.flac", "replace", 0, "start=1"); err != nil {
			t.Errorf("restore loadfile: %v", err)
		}
	}

	// A concurrent user pauses and sets an absolute volume strictly inside the
	// activation window. Both must be queued rather than run against the fresh
	// client, so the restore sequence cannot overwrite them.
	go func() {
		<-restoreStarted
		if err := m.Pause(); err != nil {
			t.Errorf("user Pause: %v", err)
		}
		if err := m.SetVolumeAbsolute(99); err != nil {
			t.Errorf("user SetVolumeAbsolute: %v", err)
		}
		close(userIssued)
	}()

	// activateConnection drains the queue synchronously before returning, so the
	// full command stream is recorded once it returns.
	m.activateConnection(ctx, client, restore)

	mu.Lock()
	defer mu.Unlock()

	indexOf := func(pred func(ipcTestCommand) bool) int {
		for i, c := range received {
			if pred(c) {
				return i
			}
		}
		return -1
	}
	lastIndexOf := func(pred func(ipcTestCommand) bool) int {
		last := -1
		for i, c := range received {
			if pred(c) {
				last = i
			}
		}
		return last
	}
	isVolume := func(want float64) func(ipcTestCommand) bool {
		return func(c ipcTestCommand) bool {
			if !c.stringIs(0, cmdSetProperty) || !c.stringIs(1, propVolume) {
				return false
			}
			v, ok := c.float64At(2)
			return ok && v == want
		}
	}

	restoreVolumeIdx := indexOf(isVolume(42))
	loadfileIdx := indexOf(func(c ipcTestCommand) bool { return c.stringIs(0, cmdLoadfile) })
	userPauseIdx := indexOf(func(c ipcTestCommand) bool { return isSetPause(c, true) })
	userVolumeIdx := indexOf(isVolume(99))
	lastVolumeIdx := lastIndexOf(func(c ipcTestCommand) bool {
		return c.stringIs(0, cmdSetProperty) && c.stringIs(1, propVolume)
	})

	if restoreVolumeIdx < 0 || loadfileIdx < 0 {
		t.Fatalf("restore sequence incomplete: volume42=%d loadfile=%d; got %v", restoreVolumeIdx, loadfileIdx, received)
	}
	if userPauseIdx < 0 || userVolumeIdx < 0 {
		t.Fatalf("user commands not replayed: pause=%d volume99=%d; got %v", userPauseIdx, userVolumeIdx, received)
	}

	// Every user command must land after the entire restore sequence.
	if userPauseIdx < loadfileIdx || userVolumeIdx < loadfileIdx {
		t.Errorf("user commands ran before the resume loadfile; pause=%d volume99=%d loadfile=%d",
			userPauseIdx, userVolumeIdx, loadfileIdx)
	}
	if userVolumeIdx < restoreVolumeIdx {
		t.Errorf("user volume (idx %d) ran before restore volume (idx %d)", userVolumeIdx, restoreVolumeIdx)
	}

	// The decisive anti-overwrite assertion: the last volume mpv is told to use
	// is the user's 99, not the restored 42.
	if lastVolumeIdx != userVolumeIdx {
		t.Errorf("restore overwrote the user volume: last volume set at idx %d, user volume at idx %d; got %v",
			lastVolumeIdx, userVolumeIdx, received)
	}
}
