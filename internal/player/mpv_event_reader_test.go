package player

import (
	"context"
	"net"
	"os"
	"testing"
	"time"
)

// TestManager_EventReaderBlocksOnCallbacksReady verifies that startEventReader
// does not dispatch track-changed events until SetEventCallbacks is called.
func TestManager_EventReaderBlocksOnCallbacksReady(t *testing.T) {
	sock := shortSockPath(t, "ev-ready")

	connDone := make(chan struct{})
	eventSent := make(chan struct{})

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
		_, _ = conn.Write([]byte(`{"event":"start-file"}` + "\n"))
		close(eventSent)
		<-connDone
	}()
	defer close(connDone)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	trackFired := make(chan struct{}, 1)
	m := &Manager{
		socketPath:     sock,
		ctx:            ctx,
		cancel:         cancel,
		callbacksReady: make(chan struct{}),
		queue:          []QueueTrack{{Path: "/a.flac", Hash: "h0", Artist: "Art", Title: "Tit"}},
		currentIndex:   0,
		callbacks:      EventCallbacks{OnTrackChanged: func(_, _, _, _ string, _ int) { trackFired <- struct{}{} }},
	}

	go m.startEventReader(ctx, nil)

	select {
	case <-eventSent:
	case <-time.After(2 * time.Second):
		t.Fatal("fake server did not send start-file event in time")
	}

	select {
	case <-trackFired:
		t.Fatal("OnTrackChanged fired before SetEventCallbacks was called")
	case <-time.After(100 * time.Millisecond):
	}

	close(m.callbacksReady)

	select {
	case <-trackFired:
	case <-time.After(2 * time.Second):
		t.Fatal("OnTrackChanged did not fire after callbacksReady closed")
	}
}

// TestEventReader_EmitsTrackInfoOnConnect verifies that startEventReader
// emits OnTrackChanged immediately after callbacksReady so the TUI always
// gets track info even when the start-file event was missed (race during
// reconnect or session restore).
func TestEventReader_EmitsTrackInfoOnConnect(t *testing.T) {
	sock := shortSockPath(t, "ev-init-emit")

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
		<-connDone
	}()
	defer close(connDone)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	type trackInfo struct {
		hash, artist, title, source string
		index                       int
	}
	got := make(chan trackInfo, 2)
	m := &Manager{
		socketPath:     sock,
		ctx:            ctx,
		cancel:         cancel,
		callbacksReady: make(chan struct{}),
		queue: []QueueTrack{
			{Path: "/a.flac", Hash: "h1", Artist: "Art1", Title: "Tit1", Source: "PL"},
		},
		currentIndex: 0,
		callbacks: EventCallbacks{
			OnTrackChanged: func(h, a, tl, src string, idx int) {
				got <- trackInfo{h, a, tl, src, idx}
			},
		},
	}

	go m.startEventReader(ctx, nil)
	close(m.callbacksReady)

	select {
	case info := <-got:
		if info.hash != "h1" || info.artist != "Art1" || info.title != "Tit1" || info.source != "PL" || info.index != 0 {
			t.Errorf("unexpected track info: %+v", info)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("OnTrackChanged not emitted on event reader connect")
	}
}

// TestEventReader_SkipsEmitWhenNoTrack verifies that startEventReader does
// NOT emit OnTrackChanged when currentIndex is out of range.
func TestEventReader_SkipsEmitWhenNoTrack(t *testing.T) {
	sock := shortSockPath(t, "ev-no-emit")

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
		<-connDone
	}()
	defer close(connDone)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	fired := make(chan struct{}, 1)
	m := &Manager{
		socketPath:     sock,
		ctx:            ctx,
		cancel:         cancel,
		callbacksReady: make(chan struct{}),
		queue:          nil,
		currentIndex:   -1,
		callbacks: EventCallbacks{
			OnTrackChanged: func(_, _, _, _ string, _ int) {
				fired <- struct{}{}
			},
		},
	}

	go m.startEventReader(ctx, nil)
	close(m.callbacksReady)

	select {
	case <-fired:
		t.Fatal("OnTrackChanged should not fire when queue is empty / index < 0")
	case <-time.After(200 * time.Millisecond):
	}
}

// TestEventReader_TracksPauseState verifies that the event reader updates
// lastPaused when it receives a property-change event for the pause property.
func TestEventReader_TracksPauseState(t *testing.T) {
	sock := shortSockPath(t, "ev-pause-track")

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
		_, _ = conn.Write([]byte(`{"event":"property-change","name":"pause","data":true}` + "\n"))
		<-connDone
	}()
	defer close(connDone)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pauseReceived := make(chan bool, 1)
	m := &Manager{
		socketPath:     sock,
		ctx:            ctx,
		cancel:         cancel,
		callbacksReady: make(chan struct{}),
		callbacks: EventCallbacks{
			OnPauseChanged: func(paused bool) {
				pauseReceived <- paused
			},
		},
		currentIndex: -1,
	}

	go m.startEventReader(ctx, nil)
	close(m.callbacksReady)

	select {
	case paused := <-pauseReceived:
		if !paused {
			t.Errorf("expected paused=true, got false")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("OnPauseChanged not called")
	}

	m.mu.RLock()
	got := m.lastPaused
	m.mu.RUnlock()
	if !got {
		t.Error("lastPaused should be true after receiving pause=true property-change")
	}
}

func TestEventReader_IgnoresInvalidPausePayload(t *testing.T) {
	sock := shortSockPath(t, "ev-pause-invalid")

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
		_, _ = conn.Write([]byte(`{"event":"property-change","name":"pause","data":"true"}` + "\n"))
		<-connDone
	}()
	defer close(connDone)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pauseReceived := make(chan bool, 1)
	m := &Manager{
		socketPath:     sock,
		ctx:            ctx,
		cancel:         cancel,
		callbacksReady: make(chan struct{}),
		callbacks: EventCallbacks{
			OnPauseChanged: func(paused bool) {
				pauseReceived <- paused
			},
		},
		currentIndex: -1,
	}

	go m.startEventReader(ctx, nil)
	close(m.callbacksReady)

	select {
	case paused := <-pauseReceived:
		t.Fatalf("OnPauseChanged fired for invalid pause payload: %v", paused)
	case <-time.After(200 * time.Millisecond):
	}

	m.mu.RLock()
	got := m.lastPaused
	m.mu.RUnlock()
	if got {
		t.Error("lastPaused should remain false after invalid pause payload")
	}
}
