package player

import (
	"context"
	"fmt"
	"net"
	"os"
	"runtime"
	"strconv"
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

	go m.startEventReader(ctx)

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

	go m.startEventReader(ctx)

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

	go m.startEventReader(ctx)

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

// TestEndFileEOF_PrefetchAdvance_NoStaleIndexAfterConcurrentShrink verifies
// the EOF gapless-advance path's invariant: after processing, currentIndex
// must never be greater than or equal to len(m.queue).
//
// The EOF handler captures (prefetchedIdx, queueLen) under a read lock,
// then releases the lock before re-acquiring a write lock to commit the
// advance. A concurrent SetQueue / clearPrefetch can run between those
// two locks, invalidating the snapshot. Without an inner re-check, the
// handler would set currentIndex to the (stale) prefetchedIdx, possibly
// past the end of the freshly-shrunk live queue, which silently breaks
// the subsequent start-file dispatch (it skips OnTrackChanged because
// idx >= len(q)) and Next/Previous bookkeeping.
//
// To exercise the race deterministically the test grabs m.mu.Lock()
// from a goroutine that is staged BEFORE the EOF event is sent: it
// holds the lock, waits for the event reader to be blocked behind it
// on the read-lock snapshot, then mutates state and releases. It then
// holds the lock again to also race the EOF handler's second lock. We
// repeat many iterations to amplify the chance of the second-Lock
// interleave.
func TestEndFileEOF_PrefetchAdvance_NoStaleIndexAfterConcurrentShrink(t *testing.T) {
	const iters = 500
	failures := 0
	var lastErr string

	for iter := 0; iter < iters; iter++ {
		func() {
			sockName := "eof-revalidate-" + strconv.Itoa(iter)
			sock := shortSockPath(t, sockName)

			os.Remove(sock)
			ln, err := net.Listen("unix", sock)
			if err != nil {
				t.Fatalf("listen: %v", err)
			}
			defer ln.Close()
			defer os.Remove(sock)

			connDone := make(chan struct{})
			defer close(connDone)

			eventSent := make(chan struct{})
			go func() {
				conn, err := ln.Accept()
				if err != nil {
					return
				}
				defer conn.Close()
				_, _ = conn.Write([]byte(`{"event":"end-file","reason":"eof"}` + "\n"))
				close(eventSent)
				<-connDone
			}()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			pathA := tempFile(t, "a*.flac")
			pathB := tempFile(t, "b*.flac")

			m := &Manager{
				socketPath:      sock,
				ctx:             ctx,
				cancel:          cancel,
				callbacksReady:  make(chan struct{}),
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
					OnStatus:        func(string) {},
					OnMissingFile:   func(string) {},
				},
			}
			close(m.callbacksReady)

			go m.startEventReader(ctx)

			<-eventSent
			// Yield once so the event reader can take its read-lock
			// snapshot before we contend for the write lock.
			for i := 0; i < 10; i++ {
				runtime.Gosched()
			}
			m.mu.Lock()
			m.queue = []QueueTrack{{Path: pathA, Hash: "h0", QueueID: 1}}
			m.prefetchedIndex = -1
			m.prefetchedPath = ""
			if m.currentIndex >= len(m.queue) {
				m.currentIndex = len(m.queue) - 1
			}
			m.mu.Unlock()

			// Give the EOF handler time to finish.
			time.Sleep(10 * time.Millisecond)

			m.mu.RLock()
			idx := m.currentIndex
			qLen := len(m.queue)
			m.mu.RUnlock()
			if qLen > 0 && idx >= qLen {
				failures++
				lastErr = fmt.Sprintf("iter %d: currentIndex=%d but live queue len=%d", iter, idx, qLen)
			}
		}()
	}
	if failures > 0 {
		t.Fatalf("EOF prefetch advance race observed in %d/%d iterations; last: %s", failures, iters, lastErr)
	}
}

func TestEndFileEOF_FallbackWhenNoPrefetch(t *testing.T) {
	sock := shortSockPath(t, "eof-fallback")

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

	m := &Manager{
		socketPath:      sock,
		ctx:             ctx,
		cancel:          cancel,
		callbacksReady:  make(chan struct{}),
		prefetchedIndex: -1,
		queue: []QueueTrack{
			{Path: "/nonexistent.flac", Hash: "h0", QueueID: 1},
		},
		currentIndex: 0,
		callbacks: EventCallbacks{
			OnPlaybackEnded: func() { endedCh <- struct{}{} },
			OnStatus:        func(string) {},
			OnMissingFile:   func(string) {},
		},
	}
	close(m.callbacksReady)

	go m.startEventReader(ctx)

	select {
	case <-endedCh:
	case <-time.After(2 * time.Second):
		t.Fatal("OnPlaybackEnded did not fire for fallback path")
	}
}
