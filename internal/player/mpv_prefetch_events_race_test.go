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

			go m.startEventReader(ctx, nil)

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
