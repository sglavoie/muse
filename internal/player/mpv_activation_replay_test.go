package player

import (
	"context"
	"sync"
	"testing"
)

// freezeDrainManager builds a Manager wired to a recording fake server, seeds
// the command queue with the given already-captured backlog closures, and dials
// a client the caller hands to activateConnection. It is the shared scaffold for
// the activation-replay atomicity tests: a backlog command can block the drain
// at a controlled point so a concurrent post-gate control lands strictly inside
// the activation window.
func freezeDrainManager(t *testing.T, name string, backlog []func() error) (*Manager, *IPCClient, *sync.Mutex, *[]ipcTestCommand, context.Context) {
	t.Helper()
	sock := shortSockPath(t, name)

	rootCtx, rootCancel := context.WithCancel(context.Background())
	ctx, cancel := context.WithCancel(rootCtx)
	t.Cleanup(rootCancel)
	t.Cleanup(cancel)

	var mu sync.Mutex
	received := &[]ipcTestCommand{}
	fakeServerWithHandler(t, sock, func(req ipcTestRequest) ipcTestResponse {
		recordCommand(&mu, received, req.Command)
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
		cmdQueue:       backlog,
	}
	t.Cleanup(func() { m.Close() })
	m.SetEventCallbacks(EventCallbacks{OnStatus: func(string) {}})

	client, err := NewIPCClient(sock)
	if err != nil {
		t.Fatalf("dial fake server: %v", err)
	}
	return m, client, &mu, received, ctx
}

// TestActivateConnection_ConcurrentControlAdmittedNotRejectedDuringDrain covers
// the enqueue-versus-gate race in FR-009: a control issued while the activation
// gate is still draining the replay backlog must be admitted (queued) rather
// than race the queue-to-nil transition and receive ErrNotConnected — and it
// must be replayed exactly once, after the earlier backlog, never run directly
// against the fresh client ahead of it.
func TestActivateConnection_ConcurrentControlAdmittedNotRejectedDuringDrain(t *testing.T) {
	m, client, mu, received, ctx := freezeDrainManager(t, "activation-admit", nil)

	backlogRunning := make(chan struct{})
	postGateIssued := make(chan struct{})

	// An outage-captured backlog command that blocks the drain until the
	// concurrent post-gate control has been issued, so the admission race is
	// deterministic. It resolves the client through the ungated ipc() helper,
	// exactly as admit's replay wrapper does.
	m.mu.Lock()
	m.cmdQueue = []func() error{
		func() error {
			close(backlogRunning)
			<-postGateIssued
			c, err := m.ipc()
			if err != nil {
				return err
			}
			_, err = c.SendCommand(cmdSetProperty, "queued-backlog", 1)
			return err
		},
	}
	m.mu.Unlock()

	var postGateErr error
	go func() {
		<-backlogRunning
		// Issued strictly while the backlog is still replaying (activating is
		// true), so admit must queue it rather than reject it or run it directly.
		postGateErr = m.SetVolumeAbsolute(99)
		close(postGateIssued)
	}()

	// activateConnection drains synchronously before returning.
	m.activateConnection(ctx, client, nil)

	if postGateErr != nil {
		t.Fatalf("post-gate control was rejected during activation drain: %v", postGateErr)
	}

	mu.Lock()
	defer mu.Unlock()
	backlogIdx := indexOfCmd(*received, func(c ipcTestCommand) bool {
		return c.stringIs(1, "queued-backlog")
	})
	volumeIdxs := allIndexesOfCmd(*received, func(c ipcTestCommand) bool {
		v, ok := c.float64At(2)
		return c.stringIs(0, cmdSetProperty) && c.stringIs(1, propVolume) && ok && v == 99
	})
	if backlogIdx < 0 {
		t.Fatalf("backlog command not replayed; got %v", *received)
	}
	if len(volumeIdxs) != 1 {
		t.Fatalf("post-gate volume replayed %d times, want exactly 1; got %v", len(volumeIdxs), *received)
	}
	if volumeIdxs[0] < backlogIdx {
		t.Errorf("post-gate control (idx %d) ran before the earlier backlog command (idx %d); got %v",
			volumeIdxs[0], backlogIdx, *received)
	}
}

// TestActivateConnection_PostGateControlDoesNotOvertakeQueuedReplay covers the
// overtake half of FR-009: a control that arrives after the gate opened would,
// without an in-order drain, reach the new client before the still-draining
// backlog. The drain keeps the gate closed until the whole backlog replays, so
// the post-gate control is strictly ordered behind every earlier command.
func TestActivateConnection_PostGateControlDoesNotOvertakeQueuedReplay(t *testing.T) {
	m, client, mu, received, ctx := freezeDrainManager(t, "activation-overtake", nil)

	backlogRunning := make(chan struct{})
	postGateIssued := make(chan struct{})

	// Two outage-captured backlog commands. The first blocks the drain until the
	// post-gate control has been issued; the second is a plain queued command
	// that must still precede the post-gate control (strict FIFO replay).
	m.mu.Lock()
	m.cmdQueue = []func() error{
		func() error {
			close(backlogRunning)
			<-postGateIssued
			c, err := m.ipc()
			if err != nil {
				return err
			}
			_, err = c.SendCommand(cmdSetProperty, "queued-backlog-1", 1)
			return err
		},
		func() error {
			c, err := m.ipc()
			if err != nil {
				return err
			}
			_, err = c.SendCommand(cmdSetProperty, "queued-backlog-2", 1)
			return err
		},
	}
	m.mu.Unlock()

	go func() {
		<-backlogRunning
		if err := m.SetVolumeAbsolute(99); err != nil {
			t.Errorf("post-gate SetVolumeAbsolute: %v", err)
		}
		close(postGateIssued)
	}()

	m.activateConnection(ctx, client, nil)

	mu.Lock()
	defer mu.Unlock()
	idx1 := indexOfCmd(*received, func(c ipcTestCommand) bool { return c.stringIs(1, "queued-backlog-1") })
	idx2 := indexOfCmd(*received, func(c ipcTestCommand) bool { return c.stringIs(1, "queued-backlog-2") })
	nIdx := indexOfCmd(*received, func(c ipcTestCommand) bool {
		v, ok := c.float64At(2)
		return c.stringIs(0, cmdSetProperty) && c.stringIs(1, propVolume) && ok && v == 99
	})
	if idx1 < 0 || idx2 < 0 || nIdx < 0 {
		t.Fatalf("missing commands: backlog1=%d backlog2=%d postGate=%d; got %v", idx1, idx2, nIdx, *received)
	}
	if !(idx1 < idx2 && idx2 < nIdx) {
		t.Errorf("post-gate control overtook queued replay; want backlog1 < backlog2 < postGate, got %d, %d, %d (%v)",
			idx1, idx2, nIdx, *received)
	}
}

func indexOfCmd(received []ipcTestCommand, pred func(ipcTestCommand) bool) int {
	for i, c := range received {
		if pred(c) {
			return i
		}
	}
	return -1
}

func allIndexesOfCmd(received []ipcTestCommand, pred func(ipcTestCommand) bool) []int {
	var out []int
	for i, c := range received {
		if pred(c) {
			out = append(out, i)
		}
	}
	return out
}
