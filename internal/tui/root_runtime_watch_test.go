package tui

import (
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/library"
)

// TestWaitForWatchBatchDeliversSingleEventImmediately verifies that a single
// event on a still-open watcher channel is returned as one batch without
// waiting for scanBatchSize-1 further events.
func TestWaitForWatchBatchDeliversSingleEventImmediately(t *testing.T) {
	ch := make(chan library.WatchEvent, scanBatchSize)
	ch <- library.WatchEvent{Track: library.Track{Hash: "a", Path: "/a.mp3"}}
	// Channel is intentionally left OPEN: no further events, no close.

	cmd := waitForWatchBatch(ch)

	msgCh := make(chan any, 1)
	go func() { msgCh <- cmd() }()

	select {
	case msg := <-msgCh:
		batch, ok := msg.(watchBatchMsg)
		if !ok {
			t.Fatalf("expected watchBatchMsg, got %T", msg)
		}
		if len(batch.tracks) != 1 || batch.tracks[0].Hash != "a" {
			t.Fatalf("expected one track [a], got %+v", batch.tracks)
		}
		if len(batch.deleted) != 0 {
			t.Fatalf("expected no deletions, got %v", batch.deleted)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("waitForWatchBatch blocked on an open channel with one buffered event")
	}
}

// TestWaitForWatchBatchCoalescesBufferedEvents verifies that companion events
// already buffered on the channel are drained into the same batch up to the
// cap.
func TestWaitForWatchBatchCoalescesBufferedEvents(t *testing.T) {
	ch := make(chan library.WatchEvent, scanBatchSize)
	ch <- library.WatchEvent{Track: library.Track{Hash: "a", Path: "/a.mp3"}}
	ch <- library.WatchEvent{Deleted: true, Path: "/b.mp3"}
	ch <- library.WatchEvent{Track: library.Track{Hash: "c", Path: "/c.mp3"}}

	msg := waitForWatchBatch(ch)()
	batch, ok := msg.(watchBatchMsg)
	if !ok {
		t.Fatalf("expected watchBatchMsg, got %T", msg)
	}
	if len(batch.tracks) != 2 {
		t.Fatalf("expected 2 tracks, got %d", len(batch.tracks))
	}
	if len(batch.deleted) != 1 || batch.deleted[0] != "/b.mp3" {
		t.Fatalf("expected one deletion [/b.mp3], got %v", batch.deleted)
	}
}

// TestWaitForWatchBatchClosedChannelReturnsNil verifies that an empty, closed
// channel stops further scheduling by returning a nil message.
func TestWaitForWatchBatchClosedChannelReturnsNil(t *testing.T) {
	ch := make(chan library.WatchEvent)
	close(ch)

	if msg := waitForWatchBatch(ch)(); msg != nil {
		t.Fatalf("expected nil msg for closed empty channel, got %T", msg)
	}
}
