package player

import (
	"context"
	"net"
	"os"
	"testing"
	"time"
)

func TestStopAfterCurrent_DefaultFalse(t *testing.T) {
	m := &Manager{}
	if m.StopAfterCurrent() {
		t.Error("expected StopAfterCurrent to be false by default")
	}
}

func TestStopAfterCurrent_SetAndGet(t *testing.T) {
	m := &Manager{}
	m.SetStopAfterCurrent(true)
	if !m.StopAfterCurrent() {
		t.Error("expected StopAfterCurrent to be true after SetStopAfterCurrent(true)")
	}
	m.SetStopAfterCurrent(false)
	if m.StopAfterCurrent() {
		t.Error("expected StopAfterCurrent to be false after SetStopAfterCurrent(false)")
	}
}

// TestStopAfterCurrent_EventReaderStops verifies that when stopAfterCurrent is
// set, an EOF end-file event fires OnPlaybackEnded instead of auto-advancing.
func TestStopAfterCurrent_EventReaderStops(t *testing.T) {
	sock := shortSockPath(t, "stopafter")

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
		// Send an end-file EOF event.
		_, _ = conn.Write([]byte(`{"event":"end-file","reason":"eof"}` + "\n"))
		close(eventSent)
		<-connDone
	}()
	defer close(connDone)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	endedCh := make(chan struct{}, 1)
	trackChangedCh := make(chan struct{}, 1)

	m := &Manager{
		socketPath:       sock,
		ctx:              ctx,
		cancel:           cancel,
		callbacksReady:   make(chan struct{}),
		stopAfterCurrent: true,
		queue: []QueueTrack{
			{Path: "/a.flac", Hash: "h0", QueueID: 1},
			{Path: "/b.flac", Hash: "h1", QueueID: 2},
		},
		currentIndex: 0,
		callbacks: EventCallbacks{
			OnPlaybackEnded: func() { endedCh <- struct{}{} },
			OnTrackChanged:  func(_, _, _, _ string, _ int) { trackChangedCh <- struct{}{} },
		},
	}
	close(m.callbacksReady)

	go m.startEventReader(ctx)

	select {
	case <-eventSent:
	case <-time.After(2 * time.Second):
		t.Fatal("fake server did not send event in time")
	}

	// OnPlaybackEnded should fire.
	select {
	case <-endedCh:
		// correct
	case <-time.After(2 * time.Second):
		t.Fatal("OnPlaybackEnded did not fire")
	}

	// OnTrackChanged should NOT fire (no auto-advance).
	select {
	case <-trackChangedCh:
		t.Error("OnTrackChanged should not fire when stopAfterCurrent is set")
	case <-time.After(100 * time.Millisecond):
		// correct
	}

	// The flag should have been auto-reset.
	if m.StopAfterCurrent() {
		t.Error("expected stopAfterCurrent to be auto-reset to false")
	}
}

// TestStopAfterCurrent_NormalAdvanceWhenDisabled verifies that when
// stopAfterCurrent is false, EOF still triggers normal auto-advance.
func TestStopAfterCurrent_NormalAdvanceWhenDisabled(t *testing.T) {
	sock := shortSockPath(t, "stopafter-off")

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

	// With no valid files in the queue, PlayFromIndex will fail and
	// OnPlaybackEnded fires — but through the normal queue-exhausted path.
	endedCh := make(chan struct{}, 1)

	m := &Manager{
		socketPath:       sock,
		ctx:              ctx,
		cancel:           cancel,
		callbacksReady:   make(chan struct{}),
		stopAfterCurrent: false,
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

	// OnPlaybackEnded fires via the normal queue-exhausted path (not stop-after).
	select {
	case <-endedCh:
		// correct — PlayFromIndex(1) fails, queue exhausted
	case <-time.After(2 * time.Second):
		t.Fatal("OnPlaybackEnded did not fire for queue-exhausted path")
	}
}
