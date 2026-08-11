package player

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sync"
	"testing"
	"time"
)

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

	go m.startEventReader(ctx, nil)

	select {
	case <-endedCh:
	case <-time.After(2 * time.Second):
		t.Fatal("OnPlaybackEnded did not fire for fallback path")
	}
}

func TestEndFileEOF_FallbackDoesNotBlockEventReader(t *testing.T) {
	sock := shortSockPath(t, "eof-fallback-nonblocking")

	events := make(chan string, 8)
	done := make(chan struct{})
	observedPause := make(chan struct{})
	loadfileStarted := make(chan struct{})
	releaseLoadfile := make(chan struct{})
	var observedOnce sync.Once
	var loadfileOnce sync.Once

	os.Remove(sock)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() {
		close(done)
		ln.Close()
		os.Remove(sock)
	})
	defer close(releaseLoadfile)

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()

				dec := json.NewDecoder(conn)
				enc := json.NewEncoder(conn)
				for {
					var req ipcTestRequest
					if err := dec.Decode(&req); err != nil {
						return
					}
					if req.Command.stringIs(0, cmdObserveProperty) {
						_ = enc.Encode(successHandler(req))
						observedOnce.Do(func() { close(observedPause) })
						for {
							select {
							case event := <-events:
								if _, err := fmt.Fprintln(conn, event); err != nil {
									return
								}
							case <-done:
								return
							}
						}
					}
					if req.Command.stringIs(0, cmdLoadfile) {
						loadfileOnce.Do(func() { close(loadfileStarted) })
						select {
						case <-releaseLoadfile:
						case <-done:
							return
						}
					}
					_ = enc.Encode(successHandler(req))
				}
			}(conn)
		}
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
		prefetchedIndex: -1,
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

	select {
	case <-observedPause:
	case <-time.After(time.Second):
		t.Fatal("event reader did not observe pause property")
	}

	events <- `{"event":"end-file","reason":"eof"}`
	select {
	case <-loadfileStarted:
	case <-time.After(time.Second):
		t.Fatal("fallback PlayFromIndex did not start loadfile")
	}

	events <- `{"event":"property-change","name":"pause","data":true}`
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		m.mu.RLock()
		paused := m.lastPaused
		m.mu.RUnlock()
		if paused {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("event reader did not process pause property while fallback loadfile was blocked")
}
