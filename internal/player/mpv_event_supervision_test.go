package player

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"
)

// dropAfterObserveServer accepts a single connection, replies to the event
// reader's observe_property subscription, then closes the socket — simulating
// the dedicated event connection dropping while the command socket stays
// healthy.
func dropAfterObserveServer(t *testing.T, socketPath string) {
	t.Helper()
	os.Remove(socketPath)
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("dropAfterObserveServer: listen: %v", err)
	}
	t.Cleanup(func() { ln.Close(); os.Remove(socketPath) })

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		dec := json.NewDecoder(conn)
		enc := json.NewEncoder(conn)
		var req ipcTestRequest
		if err := dec.Decode(&req); err == nil {
			_ = enc.Encode(successHandler(req))
		}
		// Drop the event socket. The command client is a separate, still-open
		// connection, so only the event stream fails here.
		conn.Close()
	}()
}

// TestEventReader_EventSocketDropSignalsSupervisorOnce verifies that when only
// the dedicated event socket drops (the command client remains healthy), the
// event reader routes the failure through the shared disconnect transition:
// exactly one supervisor signal, the command client detached, and command
// queueing re-enabled. This is the core "closing only the event socket triggers
// one supervised recovery" guarantee.
func TestEventReader_EventSocketDropSignalsSupervisorOnce(t *testing.T) {
	eventSock := shortSockPath(t, "ev-drop")
	dropAfterObserveServer(t, eventSock)

	// A healthy command client on a separate socket stands in for the command
	// connection that would otherwise remain connected.
	client := newTestClient(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m := &Manager{
		socketPath:     eventSock,
		ctx:            ctx,
		cancel:         cancel,
		disconnectCh:   make(chan error, 1),
		callbacksReady: make(chan struct{}),
		currentIndex:   -1,
		client:         client,
		cmdQueue:       nil,
	}
	close(m.callbacksReady)

	go m.startEventReader(ctx, client)

	select {
	case <-m.disconnectCh:
	case <-time.After(2 * time.Second):
		t.Fatal("event-socket drop did not signal the restart supervisor")
	}

	// The transition must detach the command client and re-enable queueing so
	// the supervisor owns recovery and callers keep queueing meanwhile.
	m.mu.RLock()
	gotClient := m.client
	gotQueue := m.cmdQueue
	m.mu.RUnlock()
	if gotClient != nil {
		t.Error("command client was not detached after event-socket drop")
	}
	if gotQueue == nil {
		t.Error("command queueing was not re-enabled after event-socket drop")
	}

	// Exactly once: no second signal is emitted for the same generation.
	select {
	case <-m.disconnectCh:
		t.Fatal("event-socket drop signalled the supervisor more than once")
	case <-time.After(200 * time.Millisecond):
	}
}

// TestHandleConnectionFailure_ConcurrentCallersRestartOnce verifies that when
// the health check and the event reader fail simultaneously for the same
// generation (modelled here as many concurrent callers), the shared transition
// detaches the client and signals the supervisor exactly once — no duplicate
// restarts.
func TestHandleConnectionFailure_ConcurrentCallersRestartOnce(t *testing.T) {
	client := newTestClient(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m := &Manager{
		ctx:          ctx,
		cancel:       cancel,
		disconnectCh: make(chan error, 1),
		client:       client,
		cmdQueue:     nil,
	}

	const callers = 12
	var wg sync.WaitGroup
	wg.Add(callers)
	for i := range callers {
		go func(i int) {
			defer wg.Done()
			m.handleConnectionFailure(ctx, client, fmt.Errorf("failure %d", i))
		}(i)
	}
	wg.Wait()

	// Exactly one signal reached the supervisor.
	select {
	case <-m.disconnectCh:
	case <-time.After(time.Second):
		t.Fatal("no disconnect signal from concurrent connection failures")
	}
	select {
	case <-m.disconnectCh:
		t.Fatal("concurrent connection failures signalled the supervisor more than once")
	case <-time.After(200 * time.Millisecond):
	}

	m.mu.RLock()
	gotClient := m.client
	gotQueue := m.cmdQueue
	m.mu.RUnlock()
	if gotClient != nil {
		t.Error("command client was not detached exactly once")
	}
	if gotQueue == nil {
		t.Error("command queueing was not re-enabled")
	}
}

// TestHandleConnectionFailure_IgnoresStaleGeneration verifies that a failure
// reported from a canceled (superseded) generation is ignored: it neither
// detaches the current client nor signals the supervisor, so a dead reader
// cannot restart a healthy replacement.
func TestHandleConnectionFailure_IgnoresStaleGeneration(t *testing.T) {
	client := newTestClient(t)
	replacement := newTestClient(t)

	// The generation whose ctx has already been canceled by a restart.
	staleCtx, staleCancel := context.WithCancel(context.Background())
	staleCancel()

	m := &Manager{
		disconnectCh: make(chan error, 1),
		client:       replacement, // a healthy replacement is already installed
		cmdQueue:     nil,
	}

	m.handleConnectionFailure(staleCtx, client, fmt.Errorf("stale failure"))

	select {
	case <-m.disconnectCh:
		t.Fatal("stale-generation failure signalled the supervisor")
	case <-time.After(200 * time.Millisecond):
	}

	m.mu.RLock()
	gotClient := m.client
	gotQueue := m.cmdQueue
	m.mu.RUnlock()
	if gotClient != replacement {
		t.Error("stale-generation failure detached the healthy replacement client")
	}
	if gotQueue != nil {
		t.Error("stale-generation failure resurrected the command queue")
	}
}

// eofOnEventStreamServer starts a fake mpv IPC server that behaves normally for
// the command client but, on the dedicated event connection (identified by its
// observe_property subscription), delivers a single end-file/eof event so the
// re-established event reader can be observed auto-advancing after recovery.
func eofOnEventStreamServer(t *testing.T, socketPath string, mu *sync.Mutex, received *[]ipcTestCommand) {
	t.Helper()
	os.Remove(socketPath)
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("eofOnEventStreamServer: listen: %v", err)
	}

	var wg sync.WaitGroup
	var connsMu sync.Mutex
	conns := make(map[net.Conn]struct{})
	t.Cleanup(func() {
		_ = ln.Close()
		connsMu.Lock()
		for conn := range conns {
			_ = conn.Close()
		}
		connsMu.Unlock()
		wg.Wait()
		os.Remove(socketPath)
	})

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			connsMu.Lock()
			conns[conn] = struct{}{}
			connsMu.Unlock()

			wg.Add(1)
			go func(conn net.Conn) {
				defer wg.Done()
				defer func() {
					connsMu.Lock()
					delete(conns, conn)
					connsMu.Unlock()
				}()
				dec := json.NewDecoder(conn)
				enc := json.NewEncoder(conn)
				sentEOF := false
				for {
					var req ipcTestRequest
					if err := dec.Decode(&req); err != nil {
						return
					}
					recordCommand(mu, received, req.Command)
					_ = enc.Encode(successHandler(req))
					if req.Command.stringIs(0, cmdObserveProperty) && !sentEOF {
						sentEOF = true
						// Deliver an EOF on the dedicated event stream; hold the
						// connection open afterward so this does not itself look
						// like an event-socket drop.
						_, _ = fmt.Fprintln(conn, `{"event":"end-file","reason":"eof"}`)
					}
				}
			}(conn)
		}
	}()
}

// TestEventReader_AdvancesOnEOFAfterRecovery verifies the end-to-end contract:
// a full supervised whole-generation recovery re-establishes the dedicated
// event reader, and that reader still auto-advances the queue on an EOF event
// after reconnection.
func TestEventReader_AdvancesOnEOFAfterRecovery(t *testing.T) {
	sock := shortSockPath(t, "ev-recover-eof")

	rootCtx, rootCancel := context.WithCancel(context.Background())
	ctx, cancel := context.WithCancel(rootCtx)
	defer rootCancel()

	var mu sync.Mutex
	var received []ipcTestCommand

	pathA := tempFile(t, "a*.flac")
	pathB := tempFile(t, "b*.flac")

	m := &Manager{
		socketPath:       sock,
		rootCtx:          rootCtx,
		rootCancel:       rootCancel,
		ctx:              ctx,
		cancel:           cancel,
		disconnectCh:     make(chan error, 1),
		callbacksReady:   make(chan struct{}),
		restartBaseDelay: 1 * time.Millisecond,
		currentIndex:     0,
		prefetchedIndex:  -1,
		queue: []QueueTrack{
			{Path: pathA, Hash: "h0", QueueID: 1},
			{Path: pathB, Hash: "h1", QueueID: 2},
		},
	}
	defer m.Close()
	m.SetEventCallbacks(EventCallbacks{
		OnStatus:        func(string) {},
		OnDisconnected:  func() {},
		OnTrackChanged:  func(_, _, _, _ string, _ int) {},
		OnPlaybackEnded: func() {},
		OnMissingFile:   func(string) {},
	})

	m.launchFn = func(_ context.Context, socketPath string) (*exec.Cmd, error) {
		eofOnEventStreamServer(t, socketPath, &mu, &received)
		return nil, nil
	}

	// Drive a full outage recovery: relaunch, reconnect the command client, and
	// re-establish the event reader.
	m.handleRestart()

	m.mu.RLock()
	client := m.client
	m.mu.RUnlock()
	if client == nil {
		t.Fatal("no command client installed after recovery")
	}

	// The re-established event reader must dispatch the EOF and advance to the
	// next queue entry.
	waitForCurrentIndex(t, m, 1)
}
