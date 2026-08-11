package player

import (
	"context"
	"net"
	"os"
	"sync"
	"testing"
	"time"
)

func TestEndFileEOF_UsesPrefetchedPath(t *testing.T) {
	sock := shortSockPath(t, "eof-prefetch")

	connDone := make(chan struct{})

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
		_, _ = conn.Write([]byte(`{"event":"end-file","reason":"eof"}` + "\n"))
		<-connDone
	}()
	defer close(connDone)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	trackChangedCh := make(chan int, 1)

	pathA := tempFile(t, "a*.flac")
	pathB := tempFile(t, "b*.flac")

	m := &Manager{
		socketPath:      sock,
		ctx:             ctx,
		cancel:          cancel,
		callbacksReady:  make(chan struct{}),
		gaplessEnabled:  true,
		prefetchedIndex: 1,
		prefetchedPath:  pathB,
		queue: []QueueTrack{
			{Path: pathA, Hash: "h0", QueueID: 1},
			{Path: pathB, Hash: "h1", QueueID: 2},
		},
		currentIndex: 0,
		callbacks: EventCallbacks{
			OnTrackChanged: func(_, _, _, _ string, idx int) {
				trackChangedCh <- idx
			},
			OnPlaybackEnded: func() {},
			OnStatus:        func(string) {},
			OnMissingFile:   func(string) {},
		},
	}
	close(m.callbacksReady)

	go m.startEventReader(ctx, nil)

	time.Sleep(200 * time.Millisecond)

	m.mu.RLock()
	idx := m.currentIndex
	prefIdx := m.prefetchedIndex
	m.mu.RUnlock()

	if idx != 1 {
		t.Errorf("currentIndex = %d, want 1", idx)
	}
	if prefIdx != -1 {
		t.Errorf("prefetchedIndex = %d, want -1 (cleared after use)", prefIdx)
	}
}

func TestEndFileEOF_UsesPrefetchedPath_UnpausesAfterAdvance(t *testing.T) {
	sock := shortSockPath(t, "eof-prefetch-unpause")

	var mu sync.Mutex
	var received []ipcTestCommand
	events := fakeEventCommandServer(t, sock, &mu, &received)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pathA := tempFile(t, "a*.flac")
	pathB := tempFile(t, "b*.flac")

	m := &Manager{
		socketPath:      sock,
		ctx:             ctx,
		cancel:          cancel,
		callbacksReady:  make(chan struct{}),
		gaplessEnabled:  true,
		lastPaused:      true,
		prefetchedIndex: 1,
		prefetchedPath:  pathB,
		queue: []QueueTrack{
			{Path: pathA, Hash: "h0", QueueID: 1},
			{Path: pathB, Hash: "h1", QueueID: 2},
		},
		currentIndex: 0,
		callbacks: EventCallbacks{
			OnTrackChanged:  func(_, _, _, _ string, _ int) {},
			OnPlaybackEnded: func() {},
		},
	}
	defer m.Close()

	client, err := NewIPCClient(sock)
	if err != nil {
		t.Fatalf("NewIPCClient: %v", err)
	}
	m.mu.Lock()
	m.client = client
	m.mu.Unlock()
	close(m.callbacksReady)

	go m.startEventReader(ctx, nil)

	waitForCommand(t, &mu, &received, func(cmd ipcTestCommand) bool {
		return cmd.stringIs(0, cmdObserveProperty) && cmd.stringIs(2, propPause)
	}, time.Second, "pause property observation")

	events <- `{"event":"end-file","reason":"eof"}`

	waitForCommand(t, &mu, &received, func(cmd ipcTestCommand) bool {
		return isSetPause(cmd, false)
	}, time.Second, "set_property pause false after EOF advance")

	m.mu.RLock()
	idx := m.currentIndex
	prefIdx := m.prefetchedIndex
	lastPaused := m.lastPaused
	m.mu.RUnlock()

	if idx != 1 {
		t.Errorf("currentIndex = %d, want 1", idx)
	}
	if prefIdx != -1 {
		t.Errorf("prefetchedIndex = %d, want -1 (cleared after use)", prefIdx)
	}
	if lastPaused {
		t.Error("lastPaused = true after prefetched EOF advance; want false")
	}
}

func TestEndFileEOF_GaplessSequenceContinuesAfterClearWithoutManualPlay(t *testing.T) {
	sock := shortSockPath(t, "eof-gapless-clear")

	var mu sync.Mutex
	var received []ipcTestCommand
	events := fakeEventCommandServer(t, sock, &mu, &received)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m := &Manager{
		socketPath:      sock,
		ctx:             ctx,
		cancel:          cancel,
		callbacksReady:  make(chan struct{}),
		prefetchedIndex: -1,
		callbacks: EventCallbacks{
			OnTrackChanged:  func(_, _, _, _ string, _ int) {},
			OnPlaybackEnded: func() {},
			OnStatus:        func(string) {},
			OnMissingFile:   func(string) {},
		},
	}
	defer m.Close()

	client, err := NewIPCClient(sock)
	if err != nil {
		t.Fatalf("NewIPCClient: %v", err)
	}
	m.mu.Lock()
	m.client = client
	m.mu.Unlock()
	close(m.callbacksReady)

	go m.startEventReader(ctx, nil)

	waitForCommand(t, &mu, &received, func(cmd ipcTestCommand) bool {
		return cmd.stringIs(0, cmdObserveProperty) && cmd.stringIs(2, propPause)
	}, time.Second, "pause property observation")

	if err := m.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	pathA := tempFile(t, "a*.flac")
	pathB := tempFile(t, "b*.flac")
	pathC := tempFile(t, "c*.flac")
	queue := []QueueTrack{
		{Path: pathA, Hash: "h0", QueueID: 1},
		{Path: pathB, Hash: "h1", QueueID: 2},
		{Path: pathC, Hash: "h2", QueueID: 3},
	}

	m.mu.Lock()
	m.gaplessEnabled = true
	m.mu.Unlock()
	m.SetQueue(queue)

	if err := m.PlayFromIndex(0); err != nil {
		t.Fatalf("PlayFromIndex(0): %v", err)
	}

	waitForCommand(t, &mu, &received, func(cmd ipcTestCommand) bool {
		return len(cmd) >= 3 &&
			cmd.stringIs(0, cmdLoadfile) &&
			cmd.stringIs(1, pathA) &&
			cmd.stringIs(2, "replace")
	}, time.Second, "initial loadfile replace")
	waitForCommandCount(t, &mu, &received, func(cmd ipcTestCommand) bool {
		return isSetPause(cmd, false)
	}, 1, time.Second, "set_property pause false")
	waitForCommand(t, &mu, &received, func(cmd ipcTestCommand) bool {
		path, _ := cmd.stringAt(1)
		return isLoadfileAppend(cmd) && path == pathB
	}, 2*time.Second, "prefetch of second track")

	events <- `{"event":"end-file","reason":"eof"}`

	waitForCurrentIndex(t, m, 1)
	waitForCommandCount(t, &mu, &received, func(cmd ipcTestCommand) bool {
		return isSetPause(cmd, false)
	}, 2, time.Second, "set_property pause false")
	waitForCommand(t, &mu, &received, func(cmd ipcTestCommand) bool {
		path, _ := cmd.stringAt(1)
		return isLoadfileAppend(cmd) && path == pathC
	}, 2*time.Second, "prefetch of third track")

	events <- `{"event":"end-file","reason":"eof"}`

	waitForCurrentIndex(t, m, 2)
	waitForCommandCount(t, &mu, &received, func(cmd ipcTestCommand) bool {
		return isSetPause(cmd, false)
	}, 3, time.Second, "set_property pause false")

	m.mu.RLock()
	lastPaused := m.lastPaused
	m.mu.RUnlock()
	if lastPaused {
		t.Error("lastPaused = true after gapless EOF sequence; want false")
	}
}

func TestEndFileEOF_UsesPrefetchedPath_SkipsMissing(t *testing.T) {
	sock := shortSockPath(t, "eof-pf-skip")

	connDone := make(chan struct{})

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
		_, _ = conn.Write([]byte(`{"event":"end-file","reason":"eof"}` + "\n"))
		<-connDone
	}()
	defer close(connDone)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pathA := tempFile(t, "a*.flac")
	pathC := tempFile(t, "c*.flac")

	m := &Manager{
		socketPath:      sock,
		ctx:             ctx,
		cancel:          cancel,
		callbacksReady:  make(chan struct{}),
		gaplessEnabled:  true,
		prefetchedIndex: 2,
		prefetchedPath:  pathC,
		queue: []QueueTrack{
			{Path: pathA, Hash: "h0", QueueID: 1},
			{Path: "/missing.flac", Hash: "h1", QueueID: 2},
			{Path: pathC, Hash: "h2", QueueID: 3},
		},
		currentIndex: 0,
		callbacks: EventCallbacks{
			OnTrackChanged:  func(_, _, _, _ string, _ int) {},
			OnPlaybackEnded: func() {},
			OnStatus:        func(string) {},
			OnMissingFile:   func(string) {},
		},
	}
	close(m.callbacksReady)

	go m.startEventReader(ctx, nil)

	time.Sleep(200 * time.Millisecond)

	m.mu.RLock()
	idx := m.currentIndex
	prefIdx := m.prefetchedIndex
	m.mu.RUnlock()

	if idx != 2 {
		t.Errorf("currentIndex = %d, want 2 (should skip missing idx+1)", idx)
	}
	if prefIdx != -1 {
		t.Errorf("prefetchedIndex = %d, want -1 (cleared after use)", prefIdx)
	}
}

// TestEndFileEOF_PrefetchedIdxOutOfRange covers the race where SetQueue
// shrinks the queue while a prefetched index is still set: the EOF handler
// must not assign currentIndex past the queue end. Falling through to the
// PlayFromIndex path correctly reports queue exhaustion via OnPlaybackEnded.
func TestEndFileEOF_PrefetchedIdxOutOfRange(t *testing.T) {
	sock := shortSockPath(t, "eof-pf-oob")

	connDone := make(chan struct{})

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
		_, _ = conn.Write([]byte(`{"event":"end-file","reason":"eof"}` + "\n"))
		<-connDone
	}()
	defer close(connDone)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	endedCh := make(chan struct{}, 1)
	pathA := tempFile(t, "a*.flac")

	m := &Manager{
		socketPath:     sock,
		ctx:            ctx,
		cancel:         cancel,
		callbacksReady: make(chan struct{}),
		// Simulate the race: the queue was shrunk after the prefetch
		// was registered, leaving prefetchedIndex pointing past the
		// new queue end.
		prefetchedIndex: 5,
		prefetchedPath:  "/stale.flac",
		queue: []QueueTrack{
			{Path: pathA, Hash: "h0", QueueID: 1},
		},
		currentIndex: 0,
		callbacks: EventCallbacks{
			OnTrackChanged:  func(_, _, _, _ string, _ int) {},
			OnPlaybackEnded: func() { endedCh <- struct{}{} },
			OnStatus:        func(string) {},
			OnMissingFile:   func(string) {},
		},
	}
	close(m.callbacksReady)

	go m.startEventReader(ctx, nil)

	select {
	case <-endedCh:
	case <-time.After(2 * time.Second):
		t.Fatal("OnPlaybackEnded did not fire when prefetched index is OOB")
	}

	m.mu.RLock()
	idx := m.currentIndex
	m.mu.RUnlock()
	if idx >= 1 {
		t.Errorf("currentIndex = %d; must not be >= len(queue) (1) after OOB prefetched advance", idx)
	}
}
